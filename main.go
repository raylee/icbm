package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

var ( // These three are set at buildtime.
	Version   string // The version number of this executable, including git commit id.
	BuildTime string // Time this executable was built.
	Builder   string // Email address of the builder.
)

var usage = `
Usage:
	icbm
	icbm [--http <address:port>]

Options:
	-http address         the http endpoint address (default: :80)
	-help                 this message

Example:
	./icbm -http 0.0.0.0:8080   # listen on all interfaces on port 8080
`

var (
	httpaddr = flag.String("http", "0.0.0.0:80", "serve http on address:port")
)

func main() {
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	platform := platform()
	if superfly() {
		platform = fmt.Sprintf("%s / %s / %s", os.Getenv("FLY_APP_NAME"), os.Getenv("FLY_ALLOC_ID"), os.Getenv("FLY_REGION"))
	}
	log.Println("hostnames", platform, "http", *httpaddr,
		"version", Version, "buildtime", BuildTime, "builder", Builder)

	servers := serve(*httpaddr)
	processSignals()
	shutdown(servers)
}
