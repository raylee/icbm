package main

// metrics and logging

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"
)

// // Metrics keeps some basic stats about our health and usage for the logs.
// type Metrics struct {
// 	TCPResets     int64
// 	DataPoints    int64
// 	APILogins     int64
// 	BadLogins     int64
// 	BadJSON       int64
// 	Errors        int64
// 	HTTP          int64
// 	HTTPDuration  uint64
// 	HTTPS         int64
// 	HTTPSDuration uint64
// 	ReadTimeout   int64
// 	CorsListed    int64
// 	CorsUnlisted  int64
// }

// var metrics = Metrics{}
// var statsChan chan Sample = AggregateUpdates() // StatsChan aggregates samples written to it.

type denoise struct {
	needle  []byte
	counter *int64
}

// denoiseWriter suppresses logging specific errors and converts them to metrics instead
type denoiseWriter struct {
	out     io.Writer
	filters []denoise
}

// filterWriter's Write silently suppresses lines which contain any needles in fw.filters.
func (fw *denoiseWriter) Write(p []byte) (n int, err error) {
	for _, f := range fw.filters {
		if bytes.Contains(p, f.needle) {
			if f.counter != nil {
				*f.counter++ // increment the associated metric
			}
			return len(p), nil
		}
	}
	return fw.out.Write(p)
}

// FilteredHTTPLogger removes a bunch of noise from the logs and converts them to metrics instead.
func FilteredHTTPLogger(w io.Writer) io.Writer {
	// My assumption is that these are due to random port scans.
	return &denoiseWriter{w, []denoise{
		{[]byte("http: TLS handshake error from"), nil /*&metrics.TCPResets*/},
		{[]byte("server: error reading preface from client"), nil /*&metrics.ReadTimeout*/},
	}}
}

// dataPath returns a path in the /data folder joined by []subdirs underneath it.
func dataPath(subdirs ...string) string {
	root := "data"
	if superfly() {
		root = "/data"
	}
	ee := append([]string{root}, subdirs...) // full path to target

	filename := path.Join(ee...)
	dir := filepath.Dir(filename)
	os.MkdirAll(dir, os.ModeDir|0755) // ensure the whole path exists
	return filename
}

func gzWriter(w io.WriteCloser, name, comment string, modTime time.Time) gzip.Writer {
	zw := gzip.NewWriter(w)
	// Setting the Header fields is optional, but polite.
	zw.Name = name
	zw.Comment = comment
	zw.ModTime = modTime
	return *zw
}

func gzWrite(fn, comment string, data []byte) (err error) {
	f, err := os.Create(fn)
	if err != nil {
		// metrics.Errors++
		return
	}
	zw := gzWriter(io.WriteCloser(f), fn, comment, time.Now())
	if _, e := zw.Write(data); e != nil {
		// metrics.Errors++
		return
	}
	if e := zw.Close(); e != nil {
		// metrics.Errors++
		return
	}
	return
}

func servePrometheus() {
	// prom := http.NewServeMux()
	// prom.Handle("/metrics", promhttp.Handler())
	// http.ListenAndServe(":9091", prom)
}
