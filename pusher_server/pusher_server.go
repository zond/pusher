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
	log.Fatal(http.ListenAndServe(addr, hub.NewServer().Loglevel(*loglevel)))
}
