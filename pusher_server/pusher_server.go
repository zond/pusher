package main

import (
	"flag"
	"fmt"
	"github.com/zond/pusher/hub"
	"net/http"
)

func main() {
	port := flag.Int("port", 2222, "What port to listen to")
	flag.Parse()
	addr := fmt.Sprintf("0.0.0.0:%d", *port)
	fmt.Println("Listening on", addr)
	http.ListenAndServe(addr, hub.NewServer())
}
