// icbm is the Internet Connected Beverage Monitor (server side).
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
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
	c := corsMid.New(corsMid.Options{
		AllowedOrigins:   origins,
		AllowCredentials: true,
		MaxAge:           300,
	})
	return c.Handler(next)
}

func assetSrv(path string) http.Handler {
	fsys, err := fs.Sub(AssetFS, path)
	if err != nil {
		return http.NotFoundHandler()
	}
	return maybeCompress(http.FileServer(http.FS(fsys)))
}

func fileSrv(path string) http.Handler {
	return maybeCompress(http.FileServer(http.Dir(path)))
}

func maybeCompress(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.ToLower(filepath.Ext(r.URL.Path)) {
		case ".html", ".css", ".tsv", ".txt", ".js", ".md":
			h = noCache(gziphandler.GzipHandler(h))
		}
		h.ServeHTTP(w, r)
	})
}

var (
	noCacheHeaders = map[string]string{
		"Expires":         time.Unix(0, 0).Format(time.RFC1123),
		"Cache-Control":   "no-cache, no-store, no-transform, must-revalidate, private, max-age=0",
		"Pragma":          "no-cache",
		"X-Accel-Expires": "0",
	}
	etagHeaders = []string{
		"ETag",
		"If-Modified-Since",
		"If-Match",
		"If-None-Match",
		"If-Range",
		"If-Unmodified-Since",
	}
)

// noCache configures the return headers to request not caching the results.
// per https://github.com/go-chi/chi/blob/master/middleware/nocache.go
func noCache(h http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delete any ETag headers.
		for _, v := range etagHeaders {
			if r.Header.Get(v) != "" {
				r.Header.Del(v)
			}
		}
		// Set our headers.
		for k, v := range noCacheHeaders {
			w.Header().Set(k, v)
		}
		h.ServeHTTP(w, r)
	})
}

var willServeFor = []string{
	"http://lunarville.org",
	"http://www.lunarville.org",
	"https://lunarville.org",
	"https://www.lunarville.org",
	"https://api.evq.io",
	"https://icbm.api.evq.io",
	"http://localhost:*",
	"https://icbm.fly.dev",
}

// Routes returns the mappings for handling web requests.
func Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/", assetSrv("static"))
	// mux.HandleFunc("/", BeverageStatus("Lunarville"))
	mux.Handle("/b/", http.StripPrefix("/b/", http.HandlerFunc(tapStatus)))
	mux.HandleFunc("/bev", BeverageStatus("Lunarville"))
	mux.HandleFunc("/bevbeta", BeverageStatus("Lunarville-beta"))
	mux.HandleFunc("/icbm/v1", icbmUpdate)
	mux.Handle("/data/", http.StripPrefix("/data/", cors(fileSrv("/data"), willServeFor...)))
	mux.Handle("/static/", http.StripPrefix("/static/", assetSrv("static")))
	mux.HandleFunc("/version", icbmVersion)
	return mux
}

func serve(httpaddr string) *http.Server {
	logger := log.New(FilteredHTTPLogger(os.Stderr), "", log.LstdFlags)
	srv := &http.Server{
		Addr:     httpaddr,
		ErrorLog: logger,
		Handler:  Routes(),
	}
	startHTTP := func(srv *http.Server) {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			logger.Print(err)
		}
	}
	go startHTTP(srv)
	return srv
}

func processSignals() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGUSR1)
	for {
		sig := <-c
		if sig == syscall.SIGINT {
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

// superfly returns whether we're running on a fly.io instance.
func superfly() bool {
	return os.Getenv("FLY_ALLOC_ID") != ""
}

// platform returns the platform name and details (for fly.io) or the
// hostname of the hosting virtual machine.
func platform() string {
	if superfly() {
		var x string
		peers, _ := net.LookupHost("icbm.internal")
		regions, _ := net.LookupTXT("regions.icbm.internal")
		siblings, _ := net.LookupTXT("_apps.internal")
		x += fmt.Sprintf("host:     %s.fly.dev\n", os.Getenv("FLY_APP_NAME"))
		x += fmt.Sprintf("listen:   %s\n", *httpaddr)
		x += fmt.Sprintf("id:       %s\n", os.Getenv("FLY_ALLOC_ID"))
		x += fmt.Sprintf("region:   %s\n", os.Getenv("FLY_REGION"))
		x += fmt.Sprintf("peers:    %s\n", peers)
		x += fmt.Sprintf("regions:  %s\n", regions)
		x += fmt.Sprintf("siblings: %s\n", siblings)
		return x
	}
	hostname, _ := os.Hostname()
	return fmt.Sprintf("%s, %s\n", hostname, *httpaddr)
}

// getLogin looks for a validated API key and returns the credentials
// or nil on failure.
func getLogin(w http.ResponseWriter, r *http.Request) *User {
	apikey := r.Header.Get("x-icbm-api-key")
	creds, found := users[apikey]
	if !found || !creds.Valid {
		// metrics.BadLogins++
		return nil
	}
	// metrics.APILogins++
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
