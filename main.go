package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

var usage = `
Usage:
	icbm
	icbm [--http <address:port>]

Options:
	-http address         the http endpoint address (default: :8080)
	-help                 this message

Example:
	./icbm -http :8080   # listen on all interfaces on port 8080
`

var (
	httpaddr = flag.String("http", ":8080", "serve http on address:port")
)

func main() {
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()
	log.Println(platform())

	server := serve(*httpaddr)
	processSignals()
	shutdown(server)
}
