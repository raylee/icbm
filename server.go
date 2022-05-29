// icbm is the Internet Connected Beverage Monitor (server side).
package main

import (
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
)

// AssetFS holds the contents of the static/** and template/** folders.
//go:embed static template
var AssetFS embed.FS

// cors takes a Handler and a list of allowed origin domains and returns a new
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

func fileSrvPicky(path string) http.Handler {
	return maybeCompress(http.FileServer(http.Dir(path)))
}

func maybeCompress(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ".gz") {
			h = gziphandler.GzipHandler(h)
		}
		h.ServeHTTP(w, r)
	})
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
	mux.Handle("/", assetSrv("static"))
	// mux.HandleFunc("/", BeverageStatus("Lunarville"))
	mux.HandleFunc("/bev", BeverageStatus("Lunarville"))
	mux.HandleFunc("/bevbeta", BeverageStatus("Lunarville-beta"))
	mux.HandleFunc("/icbm/v1", icbmUpdate)
	willServeFor := []string{
		"http://lunarville.org",
		"http://www.lunarville.org",
		"https://lunarville.org",
		"https://www.lunarville.org",
		"https://api.evq.io",
		"https://icbm.api.evq.io",
		"http://localhost:*",
		"https://icbm.fly.dev",
	}
	mux.Handle("/data/", http.StripPrefix("/data/", cors(fileSrv("/data"), willServeFor...)))
	mux.Handle("/datanz/", http.StripPrefix("/datanz/", cors(fileSrvPicky("/data"), willServeFor...)))
	mux.Handle("/static/", http.StripPrefix("/static/", assetSrv("static")))
	mux.HandleFunc("/version", icbmVersion)
	return mux
}

func ident() string {
	return fmt.Sprintf("host:       %s\n"+
		"http:       %s\n"+
		"version     %s\n"+
		"buildtime:  %s\n"+
		"builder:    %s\n",
		platform(),
		*httpaddr,
		Version,
		BuildTime,
		Builder)
}

func icbmVersion(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, ident())
}

func serve(httpaddr string) *http.Server {
	logger := log.New(FilteredHTTPLogger(Italic(os.Stderr)), "", log.LstdFlags)
	logUnsualClose := func(e error) {
		if e != http.ErrServerClosed {
			logger.Print(e)
		}
	}
	srv := &http.Server{
		Addr:     httpaddr,
		ErrorLog: logger,
		Handler:  onHit(Routes(), &metrics.HTTP, &metrics.HTTPDuration),
	}
	startHTTP := func(srv *http.Server) { logUnsualClose(srv.ListenAndServe()) }
	go startHTTP(srv)
	return srv
}

func processSignals() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGUSR1)
	for {
		switch <-c {
		case syscall.SIGINT:
			return
		}
	}
}

func shutdown(srv *http.Server) {
	// Attempt a graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go srv.Shutdown(ctx)
}

// superfly returns whether we're running on a fly.io instance
func superfly() bool {
	return os.Getenv("FLY_ALLOC_ID") != ""
}

func flyNeighbors() string {
	if !superfly() {
		return ""
	}

	// Every instance of every application in your organization now has an additional IPv6 address — its “6PN address”, in /etc/hosts as fly-local-6pn. That address is reachable only within your organization. Bind services to it that you want to run privately.

	// It’s pretty inefficient to connect two IPv6 endpoints by randomly guessing IPv6 addresses, so we use the DNS to make some introductions. Each of your Fly apps now has an internal DNS zone. If your application is fearsome-bagel-43, its DNS zone is fearsome-bagel-43.internal — that DNS resolves to all the IPv6 6PN addresses deployed for the application. You can find hosts by region: nrt.fearsome-bagel-43.internal are your instances in Japan. You can find all the regions for your application: the TXT record at regions.fearsom-bagel-43.internal. And you can find the “sibling” apps in your organization with the TXT record at _apps.internal.

	peers, _ := net.LookupHost("icbm.internal")
	regions, _ := net.LookupTXT("icbm.internal")
	siblings, _ := net.LookupHost("_apps.internal")
	return fmt.Sprintf("peers: %s\nregions: %s\nsiblings: %s\n", peers, regions, siblings)
}

// platform returns the platform name and details (for fly.io) or the
// hostname of the hosting virtual machine.
func platform() string {
	if superfly() {
		return fmt.Sprintf(
			"host:   %s.fly.dev\n"+
				"id:     %s\n"+
				"region: %s\n",
			os.Getenv("FLY_APP_NAME"),
			os.Getenv("FLY_ALLOC_ID"),
			os.Getenv("FLY_REGION"),
		) + flyNeighbors()
	}
	hostname, _ := os.Hostname()
	return hostname
}

// getLogin looks for a validated API key and returns the credentials
// or nil on failure.
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

var users = map[string]User{} // The key is the API key.

func loadUserList() error {
	userdb := os.Getenv("ICBMUserDb")
	j := strings.NewReader(userdb)
	dec := json.NewDecoder(j)
	return dec.Decode(&users)
}

func init() {
	if err := loadUserList(); err != nil {
		log.Println("Could not load user database, server will be read-only:", err)
	} else {
		log.Printf("User database loaded, %d entries\n", len(users))
	}
}
