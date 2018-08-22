package main

import (
	"flag"
	"html/template"
	"log"
	"net/http"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"strconv"
	"github.com/dustin/go-broadcast"
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
var hold chan struct{}
var terminateRoutineForPath chan string
var antiPoller struct{}
var path = ""
var urlparam = ""

func init() {
	flag.StringVar(&addr, "addr", "0.0.0.0:8081", "link address")
}

type Pipe struct {
	sourceConn *websocket.Conn
	sinkConn   []*websocket.Conn
	pipeChan   chan Message
	b broadcast.Broadcaster
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
	hold = make(chan struct{})
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
		//sourceAdd <- antiPoller
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
	pipeObj.b = broadcast.NewBroadcaster(1)
	pathMap[sourcePath] = &pipeObj
	log.Printf("starting go routines")
	go readFromSource(sourcePath)
	//go writeToSink(sourcePath)
	sourceAdd <- antiPoller
	sinkAdd <- antiPoller
	//path = sourcePath
	//sourceAdd <- antiPoller

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
		go writeToBrowser(s,keys,pipeConn.b)
		//pathMap[keys] = pipeConn
		//sinkAdd <- antiPoller

	} else {

		log.Fatalf("upgrade:", err)
		return
	}
	temp := pathMap[keys]
	log.Printf("length of temp at sink " + strconv.Itoa(len(temp.sinkConn)))
	sinkAdd <- antiPoller
}

func readFromSource(tempPath string) {
	log.Printf("read from source started ")
	pipeObj, _ := pathMap[tempPath]
	//var first = true
	<-sourceAdd
	log.Println("Source Added Succefully")
	for {

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
			pipeObj.sinkConn = nil
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
		pipeObj.b.Submit(m)

	}
	log.Println("out of loop ")
}

func writeToBrowser(browserConn   *websocket.Conn,incPath string,b broadcast.Broadcaster){
	pipeObj := make(chan interface{})
	b.Register(pipeObj)
	//pipeObj, _ := pathMap[incPath]
	for {
		select {
		// here is the problem. i am unable to fetch Message from the broadcaster.
		case m := <-pipeObj:
			err := browserConn.WriteMessage(m.mt, m.message)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					//pipeObj.sinkConn = append(pipeObj.sinkConn[:i], pipeObj.sinkConn[(i + 1):]...)
					log.Printf("Write to sink ===> Here error: %v\n", err)
				}
				log.Println("Write to sink ===> some error")
				return
			}

		case terminationPath := <-terminateRoutineForPath:
			if terminationPath == incPath {
				log.Printf("terminating go routine writetosink for path " + terminationPath)
				return
			}

		}
	}
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
