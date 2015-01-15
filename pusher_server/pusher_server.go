package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"runtime"

	"github.com/soundtrackyourbrand/pusher/hub"
)

func main() {
	port := flag.Int("port", 2222, "What port to listen to")
	loglevel := flag.Int("loglevel", 0, "How much to log")
	flag.Parse()
	addr := fmt.Sprintf("0.0.0.0:%d", *port)
	if *loglevel > 0 {
		fmt.Println("Listening on", addr)
	}
	runtime.GOMAXPROCS(runtime.NumCPU())
	if *loglevel > 1 {
		fmt.Printf("Processes: %d\n", runtime.GOMAXPROCS(-1))
	}
	// http.Handle("/", http.FileServer(http.Dir("./js")))
	http.Handle("/", http.FileServer(http.Dir("./")))
	http.Handle("/ws", hub.NewServer().Loglevel(*loglevel))
	log.Fatal(http.ListenAndServe(addr, nil))
}
