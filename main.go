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

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	if superfly() {
		log.SetFlags(0)
	}
}

func main() {
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	log.Print(platform())
	log.Print(buildInfo())

	go servePrometheus()

	if fridge := os.Getenv("ICBMRepack"); fridge != "" {
		go repack(fridge)
	}

	server := serve(*httpaddr)
	processSignals()
	shutdown(server)
}
