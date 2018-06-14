package main

import (
	"flag"
	"html/template"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"fmt"
)

type AppConfigProperties map[string]string
var pathMap map[string]Pipe
var addr string
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}
var sourceConn *websocket.Conn
var sinkConn []*websocket.Conn
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
	sourceAdd chan struct{}
	sinkAdd chan struct{}
}

type Message struct {
	message []byte
	mt      int
}

var pipe Pipe

func main() {
	r := mux.NewRouter()
	flag.Parse()
	/*pipe = Pipe{
		sourceConn: nil,
		sinkConn:   nil,
	}*/
	pathMap = make(map[string]Pipe)
	//sourceToSink := make(chan Message, 1)
	//sourceAdd = make(chan struct{})
	//sinkAdd = make(chan struct{})
	//pipe.pipeChan = sourceToSink
	//go readFromSource()
	//go writeToSink()
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
	if params["userPath"] == path {
		homeTemplate.Execute(w, "ws://"+r.Host+"/sink/"+urlparam)
	} else {
		http.Error(w, "Page not found", 404)
	}
}

func source(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	sourcePath := params["sourcePath"]
	log.Printf("Establishing connection with path:: " + sourcePath)
	pipeConn,exist := pathMap[sourcePath]
	log.Printf("Checking for established connection on path:: "+sourcePath)
	if exist{
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
		pipeObj.sourceAdd =make(chan struct{})
		pipeObj.sinkAdd =make(chan struct{})
		pipeObj.pipeChan = make(chan Message, 1)
		pipeConn = pipeObj
		pathMap[sourcePath] = pipeConn
		log.Printf("starting go routines")
		go readFromSource(pipeConn)
		go writeToSink(pipeConn)
		path = sourcePath
		pipeConn.sourceAdd <- antiPoller


	//	defer s.Close()
	//	select {}

}

func sink(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	keys:=params["browserPath"]
	log.Printf("Viewer recieved at path:: "+keys)
	pipeConn,exist := pathMap[keys]
	s, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	if exist{
		log.Printf("Streaming availaible at path " + keys)
		pipeConn.sinkConn = append(pipeConn.sinkConn,s)
		pipeConn.sinkAdd <- antiPoller
	} else {

			log.Fatalf("upgrade:", err)
			return
		}





	/*if !ok || len(keys) < 1 {
		log.Println("Url Param 'servingPath' is missing")
	}


	s, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	pipe.sinkConn = append(pipe.sinkConn, s)
	sinkAdd <- antiPoller
	/*
		defer func() {
			for _, c := range pipe.sinkConn {
				c.Close()
			}
		}()
	*/
	//	select {}
}

func readFromSource(pipeObj Pipe) {
	<-pipeObj.sourceAdd
	for {
		log.Println("Read from source ===> Start source read message.")
		mtype, mesg, err := pipeObj.sourceConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Read from source ===> Unexpected error: %v", err)
				pipeObj.sourceConn = nil
				return
			}
			log.Println("Read from source ===> Error in conn. Waiting for new source.")
			pipeObj.sourceConn = nil
			return
		}
		log.Println("Read from source ===> Done read message.")
		log.Println("Read from source ===> " + string(mtype))
		m := Message{
			message: mesg,
			mt:      mtype,
		}
		pipeObj.pipeChan <- m
		log.Println("Read from source ===> Done  source read message.")
	}

	/*<-sourceAdd //george's code
	for {
		log.Println("Read from source ===> Start source read message.")
		mtype, mesg, err := pipe.sourceConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Read from source ===> Unexpected error: %v", err)
			}
			log.Println("Read from source ===> Error in conn. Waiting for new source.")



		}
		log.Println("Read from source ===> Done read message.")
		log.Println("Read from source ===> " + string(mtype))
		m := Message{
			message: mesg,
			mt:      mtype,
		}
		pipe.pipeChan <- m
		log.Println("Read from source ===> Done  source read message.")
	}
	log.Println("Read from source ===> For loop exited")*/
}

func writeToSink(pipeObj Pipe) {


	<-pipeObj.sinkAdd
	for {
		m := <-pipeObj.pipeChan

		for i, c := range pipe.sinkConn {
			log.Printf("Write to sink ===> Start sink read message. %d\n", m.mt)
			log.Printf("Write to sink ===> socked %s\n", c.RemoteAddr())
			if path == urlparam {
				err := c.WriteMessage(m.mt, m.message)
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						pipeObj.sinkConn = append(pipe.sinkConn[:i], pipe.sinkConn[(i+1):]...)
						log.Printf("Write to sink ===> Here error: %v\n", err)
					}
				}
				fmt.Printf("Write to sink ===> Here continue error :%v\n", err)
				continue
			}
			log.Println("Write to sink ===> Done sink sending message.")
		}

	}
	log.Println("Write to sink ===> For loop exited")

	/*<-sinkAdd
	for {
		m := <-pipe.pipeChan

		for i, c := range pipe.sinkConn {
			log.Printf("Write to sink ===> Start sink read message. %d\n", m.mt)
			log.Printf("Write to sink ===> socked %s\n", c.RemoteAddr())
			if path == urlparam {
				err := c.WriteMessage(m.mt, m.message)
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						pipe.sinkConn = append(pipe.sinkConn[:i], pipe.sinkConn[(i+1):]...)
						log.Printf("Write to sink ===> Here error: %v\n", err)
					}
				}
				fmt.Printf("Write to sink ===> Here continue error :%v\n", err)
				continue
			}
			log.Println("Write to sink ===> Done sink sending message.")
		}

	}
	log.Println("Write to sink ===> For loop exited")
	*/
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
img, err := png.Decode(bytes.NewReader(message))
			fileName := fmt.Sprintf("%s.png", time.Now().String())
			log.Printf("Writing %d bytes to %s", len(message), fileName)
			file, err := os.Create(fileName)
			if err != nil {
				log.Fatalf("%v", err)
			}
			png.Encode(file, img)
			file.Close()
*/
