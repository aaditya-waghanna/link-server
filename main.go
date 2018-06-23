package main

import (
	"flag"
	"html/template"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"strconv"
)

type AppConfigProperties map[string]string

var pathMap map[string]*Pipe
var addr string
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}
var sourceAdd chan struct{}
var sinkAdd chan struct{}
var terminateRoutineForPath chan string
var antiPoller struct{}
var path = ""
var urlparam = ""

func init() {
	flag.StringVar(&addr, "addr", "0.0.0.0:8080", "link address")
}

type Pipe struct {
	sourceConn *websocket.Conn
	sinkConn   []*websocket.Conn
	pipeChan   chan Message
}

type Message struct {
	message []byte
	mt      int
}

func main() {
	r := mux.NewRouter()
	flag.Parse()
	pathMap = make(map[string]*Pipe)
	sourceAdd = make(chan struct{})
	sinkAdd = make(chan struct{})
	terminateRoutineForPath = make(chan string)
	r.HandleFunc("/source/{sourcePath}", source)
	r.HandleFunc("/sink/{browserPath}", sink)
	r.HandleFunc("/{userPath}", home)
	log.Fatal(http.ListenAndServe(addr, r))

	select {}
}

func home(w http.ResponseWriter, r *http.Request) {
	log.Printf("serving static page.")
	params := mux.Vars(r)
	log.Println("Url Param 'key' is: " + params["userPath"] + "   " + path)
	urlparam = params["userPath"]
	_, exist := pathMap[ params["userPath"]]
	if exist {
		homeTemplate.Execute(w, "ws://"+r.Host+"/sink/"+urlparam)
	} else {
		http.Error(w, "Page not found", 404)
	}
}

func source(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	sourcePath := params["sourcePath"]
	log.Printf("Establishing connection with path:: " + sourcePath)
	pipeConn, exist := pathMap[sourcePath]
	log.Printf("Checking for established connection on path:: " + sourcePath)
	if exist {
		log.Printf("The path already exists closing existing connection " + sourcePath)
		pipeConn.sourceConn.Close()
		delete(pathMap, sourcePath)
	}
	s, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatalf("upgrade:", err)
		return
	}
	pipeObj := Pipe{
		sourceConn: s,
		sinkConn:   nil,
	}
	pipeObj.pipeChan = make(chan Message, 1)
	pathMap[sourcePath] = &pipeObj
	log.Printf("starting go routines")
	go readFromSource(sourcePath)
	go writeToSink(sourcePath)
	path = sourcePath
	sourceAdd <- antiPoller

}

func sink(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	keys := params["browserPath"]
	log.Printf("Viewer recieved at path:: " + keys)
	pipeConn, exist := pathMap[keys]
	s, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	if exist {
		log.Printf("Streaming availaible at path " + keys)
		pipeConn.sinkConn = append(pipeConn.sinkConn, s)
		pathMap[keys] = pipeConn
		sinkAdd <- antiPoller
	} else {

		log.Fatalf("upgrade:", err)
		return
	}
	temp := pathMap[keys]
	log.Printf("length of temp at sink " + strconv.Itoa(len(temp.sinkConn)))
}

func readFromSource(tempPath string) {
	pipeObj, _ := pathMap[tempPath]
	<-sourceAdd
	for {
		//log.Println("Read from source ===> Start source read message.")
		mtype, mesg, err := pipeObj.sourceConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Read from source ===> Unexpected error: %v", err)
				pipeObj.sourceConn = nil
				delete(pathMap, tempPath)
				terminateRoutineForPath <- tempPath
				log.Printf("terminating go routine writetosink for path " + tempPath)
				return
			}
			log.Println("Read from source ===> Error in conn. Waiting for new source.")
			pipeObj.sourceConn = nil
			delete(pathMap, tempPath)
			terminateRoutineForPath <- tempPath
			log.Printf("terminating go routine readfromsource for path " + tempPath)
			return
		}
		//log.Println("Read from source ===> Done read message.")
		//log.Println("Read from source ===> " + string(mtype))
		m := Message{
			message: mesg,
			mt:      mtype,
		}
		pipeObj.pipeChan <- m
		//log.Println("Read from source ===> Done  source read message.")
	}
}

func writeToSink(incPath string) {

	pipeObj, _ := pathMap[incPath]
	<-sinkAdd
	log.Printf("write to sink started")
	log.Printf("length is" + strconv.Itoa(len(pipeObj.sinkConn)))
	for {
		select {


		case m := <-pipeObj.pipeChan:

			for i, c := range pipeObj.sinkConn {
				//log.Printf("Write to sink ===> Start sink read message. %d\n", m.mt)
				//log.Printf("Write to sink ===> socked %s\n", c.RemoteAddr())
				//if path == urlparam {
				err := c.WriteMessage(m.mt, m.message)
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						pipeObj.sinkConn = append(pipeObj.sinkConn[:i], pipeObj.sinkConn[(i + 1):]...)
						//log.Printf("Write to sink ===> Here error: %v\n", err)
					}
				}
				//fmt.Printf("Write to sink ===> Here continue error :%v\n", err)
				continue
				//}
				//log.Println("Write to sink ===> Done sink sending message.")
			}
		case terminationPath := <-terminateRoutineForPath:
			if terminationPath == incPath {
				log.Printf("terminating go routine writetosink for path " + terminationPath)
				return
			}

		}
	}
	log.Println("Write to sink ===> For loop exited")

}

var homeTemplate = template.Must(template.New("").Parse(`
	<!DOCTYPE html>
  <head>
    <script type="text/javascript">
      var ws
      function fetchImage() {
       
        ws = new WebSocket("{{.}}");  
        ws.onopen = function(evt) {
            console.log("OPEN");
        }
        ws.onclose = function(evt) {
          console.log("CLOSE");
          ws = null;
        }
        ws.onmessage = function(evt) {
          drawImage(evt.data);
        }
        ws.onerror = function(evt) {
          console.log("ERROR: " + evt.data);
        }

      }

      function drawImage(data){
        document.images["screen"].src = URL.createObjectURL(data)
      }

      document.addEventListener("DOMContentLoaded", fetchImage)
    </script>
  </head>
  <body>
    <img width="100%" id="screen">
  </body>
</html>

	`))

/*
img, err  := png.Decode(bytes.NewReader(message))
			fileName := fmt.Sprintf("%s.png", time.Now().String())
			log.Printf("Writing %d bytes to %s", len(message), fileName)
			file, err := os.Create(fileName)
			if err != nil {
				log.Fatalf("%v", err)
			}
			png.Encode(file, img)
			file.Close()
*/
