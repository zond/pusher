package main

import (
	"flag"
	"fmt"
	"github.com/zond/pusher/hub"
	"net/http"
)

func main() {
	port := flag.Int("port", 8080, "What port to listen to")
	flag.Parse()
	http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", *port), hub.NewServer())
}
