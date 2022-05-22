// icbm is the Internet Connected Beverage Monitor (server side).
package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/NYTimes/gziphandler"
	corsMid "github.com/rs/cors"
	"golang.org/x/crypto/acme/autocert"
)

// AssetFS holds our static web server assets.
//go:embed static template
var AssetFS embed.FS

// cors take a Handler and a list of allowed origin domains and returns a new
// Handler that enforces CORS to those sources.
func cors(next http.Handler, origins ...string) http.Handler {
	c := corsMid.New(corsMid.Options{AllowedOrigins: origins, AllowCredentials: true, MaxAge: 300})
	return c.Handler(next)
}

func assetSrv(path string) http.Handler {
	fsys, err := fs.Sub(AssetFS, path)
	if err != nil {
		return http.NotFoundHandler()
	}
	return gziphandler.GzipHandler(http.FileServer(http.FS(fsys)))
}

func fileSrv(path string) http.Handler {
	return gziphandler.GzipHandler(http.FileServer(http.Dir(path)))
}

func onHit(h http.Handler, counter *int64, duration *uint64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(counter, 1)
		start := time.Now()
		h.ServeHTTP(w, r)
		t := time.Since(start)
		atomic.AddUint64(duration, uint64(t))
	})
}

// Routes returns the mappings for handling web requests.
func Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", BeverageStatus("Lunarville"))
	mux.HandleFunc("/beta", BeverageStatus("Lunarville-beta"))
	mux.HandleFunc("/icbm/v1", icbmUpdate)
	willServeFor := []string{
		"http://lunarville.org",
		"http://www.lunarville.org",
		"https://lunarville.org",
		"https://www.lunarville.org",
		"https://api.evq.io",
		"https://icbm.api.evq.io",
		"http://localhost:*",
	}
	mux.Handle("/data/", http.StripPrefix("/data/", cors(fileSrv("data"), willServeFor...)))
	mux.Handle("/static/", http.StripPrefix("/static/", assetSrv("static")))
	mux.HandleFunc("/version", icbmVersion)
	return mux
}

func icbmVersion(w http.ResponseWriter, r *http.Request) {
	v := " hostname: %s\n" +
		"     http: %s\n" +
		"    https: %s\n" +
		"  version: %s\n" +
		"buildtime: %s\n" +
		"  builder: %s\n"
	io.WriteString(w, fmt.Sprintf(v, fqdns(), *httpaddr, *httpsaddr, Version, BuildTime, Builder))
}

func serve(tlsnames, httpaddr, httpsaddr string) (servers []*http.Server) {
	logger := log.New(FilteredHTTPLogger(Italic(os.Stderr)), "", log.LstdFlags)
	logUnsualClose := func(e error) {
		if e != http.ErrServerClosed {
			logger.Print(e)
		}
	}
	startHTTP := func(srv *http.Server) { logUnsualClose(srv.ListenAndServe()) }
	startTLS := func(srv *http.Server) { logUnsualClose(srv.ListenAndServeTLS("", "")) }

	if tlsnames == "localhost" || strings.HasPrefix(httpaddr, "0") {
		srv := &http.Server{
			Addr:     httpaddr,
			ErrorLog: logger,
			Handler:  onHit(Routes(), &metrics.HTTP, &metrics.HTTPDuration),
		}
		go startHTTP(srv)
		servers = append(servers, srv)
		return
	}

	if httpsaddr == "" {
		return
	}

	// Otherwise, we're an https server with autocert.
	autocertmgr := &autocert.Manager{
		Cache:      autocert.DirCache("secret"),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(strings.Split(tlsnames, ",")...),
	}
	// With a nil Handler HTTP will only answer Let's Encrypt challenges and
	// redirect clients to https on 443.
	srv := &http.Server{
		Addr:     httpaddr,
		ErrorLog: logger,
		Handler:  onHit(autocertmgr.HTTPHandler(nil), &metrics.HTTP, &metrics.HTTPDuration),
	}
	go startHTTP(srv)
	servers = append(servers, srv)

	srv = &http.Server{
		Addr:      httpsaddr,
		TLSConfig: autocertmgr.TLSConfig(),
		ErrorLog:  logger,
		Handler:   onHit(Routes(), &metrics.HTTPS, &metrics.HTTPSDuration),
	}
	go startTLS(srv)
	servers = append(servers, srv)
	return
}

func processSignals() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGUSR1)
	for {
		switch <-c {
		// case syscall.SIGUSR1:
		// log.Infof("Received SIGUSR1, reloading certificate and key from %s and %s", cert, key)
		// if err := srv.ReloadCerts(); err != nil {
		// 	log.Errorf("Could not update certificates: %v", err)
		// }
		case syscall.SIGINT:
			return
		}
	}
}

func shutdown(servers []*http.Server) {
	// Attempt a graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, srv := range servers {
		go srv.Shutdown(ctx)
	}
}

// fqdns returns all the fully qualified domain names found on this system.
func fqdns() []string {
	// todo: collect names into two lists, internal and external.
	// return append(external, internal) to give priority to global IP names.
	var hostnames []string
	hostname, err := os.Hostname()
	if err != nil {
		return append(hostnames, "(unknown)")
	}

	addrs, err := net.LookupIP(hostname)
	if err != nil {
		return append(hostnames, hostname)
	}

	for _, addr := range addrs {
		if ipv4 := addr.To4(); ipv4 != nil {
			ip, err := ipv4.MarshalText()
			if err != nil {
				continue
			}
			hosts, err := net.LookupAddr(string(ip))
			if err != nil || len(hosts) == 0 {
				continue
			}
			for _, h := range hosts {
				if strings.Count(h, ".") > 1 {
					hostnames = append(hostnames, strings.TrimSuffix(h, "."))
				}
			}
		}
	}

	return hostnames
}

// superfly returns whether we're running on a fly.io instance
func superfly() bool {
	return os.Getenv("FLY_ALLOC_ID") != ""
}

// platform returns the platform name (fly.io) or an effectively random
// hostname from the set of valid fully qualified domain names on the
// hosting virtual machine.
func platform() string {
	if superfly() {
		return fmt.Sprintf("fly.io: %s / %s / %s", os.Getenv("FLY_APP_NAME"), os.Getenv("FLY_ALLOC_ID"), os.Getenv("FLY_REGION"))
	}

	fqs := fqdns()
	if len(fqs) > 0 {
		return fqdns()[0]
	}
	return ""
}

// getLogin looks for a validated API key and returns the credentials
func getLogin(w http.ResponseWriter, r *http.Request) *User {
	apikey := r.Header.Get("x-icbm-api-key")
	creds, found := users[apikey]
	if !found || !creds.Valid {
		metrics.BadLogins++
		return nil
	}

	metrics.APILogins++
	return &creds
}

// User is a simple on-off scheme per API client.
type User struct {
	Username string
	Valid    bool
}

//go:embed icbmuserdb.json
var userdbjson []byte
var users = map[string]User{} // The key is the API key.

func loadUserList() error {
	j := bytes.NewReader(userdbjson)
	dec := json.NewDecoder(j)
	return dec.Decode(&users)
}

func init() {
	if err := loadUserList(); err != nil {
		log.Println("Could not load user database, server will be read-only:", err)
	}
}
