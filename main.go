package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"github.com/gorilla/websocket"
	"github.com/gorilla/mux"
)

type AppConfigProperties map[string]string
var addr string
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}
var sourceConn *websocket.Conn
var sinkConn []*websocket.Conn
var sourceAdd chan struct{}
var sinkAdd chan struct{}
var antiPoller struct{}
var path = ""

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

var pipe Pipe

func main() {
	r := mux.NewRouter()
	flag.Parse()
	pipe = Pipe{
		sourceConn: nil,
		sinkConn:   nil,
	}
	sourceToSink := make(chan Message, 1)
	sourceAdd = make(chan struct{})
	sinkAdd = make(chan struct{})
	pipe.pipeChan = sourceToSink
	go readFromSource()
	go writeToSink()
	r.HandleFunc("/source", source)
	r.HandleFunc("/path/",handleGet)
	r.HandleFunc("/sink", sink)
	r.HandleFunc("/{userPath}", home)
	log.Fatal(http.ListenAndServe(addr, r))

	select {}
}

func home(w http.ResponseWriter, r *http.Request) {
	log.Printf("serving static page.")
	params := mux.Vars(r)
	log.Println("Url Param 'key' is: " + params["userPath"]+ "   "+path)
	if params["userPath"] == path {
	homeTemplate.Execute(w, "ws://"+r.Host+"/sink")
	} else {
		http.Error(w, "Page not found", 404 )
	}
}

func source(w http.ResponseWriter, r *http.Request) {
	s, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatalf("upgrade:", err)
		return
	}
	if pipe.sourceConn != nil {
		pipe.sourceConn.Close()
	}

	pipe.sourceConn = s
	sourceAdd <- antiPoller
	//	defer s.Close()
	//	select {}

}

func handleGet(w http.ResponseWriter, r *http.Request){
	keys, ok := r.URL.Query()["servingPath"]
    
    if !ok || len(keys) < 1 {
        log.Println("Url Param 'servingPath' is missing")
    }

	path = keys[0]
}

func sink(w http.ResponseWriter, r *http.Request) {
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

func readFromSource() {
	<-sourceAdd
	for {
		log.Println("Start source read message.")
		mtype, mesg, err := pipe.sourceConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			log.Println("Error in conn. Waiting for new source.")
			<-sourceAdd
		}
		//log.Println("Done read message.")
		m := Message{
			message: mesg,
			mt:      mtype,
		}
		pipe.pipeChan <- m
		log.Println("Done  source read message.")
	}
}

func writeToSink() {
	<-sinkAdd
	for {
		m := <-pipe.pipeChan

		for i, c := range pipe.sinkConn {
			log.Printf("Start sink read message. %d\n", m.mt)
			err := c.WriteMessage(m.mt, m.message)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					pipe.sinkConn = append(pipe.sinkConn[:i], pipe.sinkConn[(i+1):]...)
					log.Printf("error: %v", err)
				}
				fmt.Printf("Here :%v\n", err)
				continue
			}
			log.Println("Done sink sending message.")
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