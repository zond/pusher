package main

import (
	"flag"
	"fmt"
	"github.com/zond/pusher/hub"
	"log"
	"net/http"
	"runtime"
)

func main() {
	port := flag.Int("port", 2222, "What port to listen to")
	flag.Parse()
	addr := fmt.Sprintf("0.0.0.0:%d", *port)
	fmt.Println("Listening on", addr)
	runtime.GOMAXPROCS(runtime.NumCPU())
	fmt.Printf("Processes: %d\n", runtime.GOMAXPROCS(-1))
	log.Fatal(http.ListenAndServe(addr, hub.NewServer()))
}
