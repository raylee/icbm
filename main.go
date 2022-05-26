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
	icbm [--http <address:port>] [--https <address:port>]

Options:
	-hostnames h1,h2,...  a comma-separated list of hostnames for autocert to honor
	-http address         the http endpoint address (default: :80)
	-https address        the https endpoint address (default: :443)
	-help                 this message

Examples:
	./icbm -http :7080 -https :7443 -hostnames test.icbm.api.evq.io
	./icbm -install
	./icbm -http 127.0.0.99:80 -https 127.0.0.99:443 -hostnames icbm.api.evq.io,INSTANCE.ZONE.c.PROJ.internal
`

var (
	httpaddr  = flag.String("http", ":80", "serve http on address:port")
	httpsaddr = flag.String("https", ":443", "serve https on address:port")
	tlsnames  = flag.String("hostnames", "localhost", "a comma-separated list of our TLS hostnames")
)

func logError(err error) {
	if err != nil {
		log.Println(err)
	}
}

func main() {
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	platform := platform()
	if superfly() {
		platform = fmt.Sprintf("%s / %s / %s", os.Getenv("FLY_APP_NAME"), os.Getenv("FLY_ALLOC_ID"), os.Getenv("FLY_REGION"))
	}
	log.Println("hostnames", platform, "http", *httpaddr, "https", *httpsaddr,
		"version", Version, "buildtime", BuildTime, "builder", Builder)

	servers := serve(*tlsnames, *httpaddr, *httpsaddr)
	processSignals()
	shutdown(servers)
}
