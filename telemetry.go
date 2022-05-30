package main

// metrics and logging

import (
	"bytes"
	"compress/gzip"
	"io"
	"log"
	"math"
	"os"
	"path"
	"path/filepath"
	"time"
)

// Metrics keeps some basic stats about our health and usage for the logs.
type Metrics struct {
	TCPResets     int64
	DataPoints    int64
	APILogins     int64
	BadLogins     int64
	BadJSON       int64
	Errors        int64
	HTTP          int64
	HTTPDuration  uint64
	HTTPS         int64
	HTTPSDuration uint64
	ReadTimeout   int64
	CorsListed    int64
	CorsUnlisted  int64
}

var metrics = Metrics{}
var statsChan chan Sample = AggregateUpdates() // StatsChan aggregates samples written to it.

type filter struct {
	needle  []byte
	counter *int64
}

// filterWriter suppresses logging specific errors and converts them to metrics instead
type filterWriter struct {
	out     io.Writer
	filters []filter
}

// filterWriter's Write silently suppresses lines which contain any needles in fw.filters.
func (fw *filterWriter) Write(p []byte) (n int, err error) {
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
	return &filterWriter{w, []filter{
		{[]byte("http: TLS handshake error from"), &metrics.TCPResets},
		{[]byte("server: error reading preface from client"), &metrics.ReadTimeout},
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

func gzWrite(fn, comment string, data []byte) {
	f, e := os.Create(fn)
	if e != nil {
		log.Printf("Could not create output file %s: %s\n", fn, e)
		metrics.Errors++
	}

	zw := gzip.NewWriter(f)
	// Setting the Header fields is optional, but polite.
	zw.Name = fn
	zw.Comment = comment
	zw.ModTime = time.Now()

	if _, e := zw.Write(data); e != nil {
		log.Print(e)
	}
	if e := zw.Close(); e != nil {
		log.Print(e)
	}
}

// logStats is racy, but good enough for rough stats. Add a mutex to the Metrics
// struct to close the window.
func logStats(ss []Sample) {
	type Summary struct{ Min, Max, Avg float64 }
	s := Summary{math.Inf(1), math.Inf(-1), 0.0}

	for i := range ss {
		sample := ss[i].PubFillRatio
		s.Max = math.Max(s.Max, sample)
		s.Min = math.Min(s.Min, sample)
		s.Avg += sample
	}

	r := func(x float64) float64 { return math.Round(x*1000) / 10 }
	s.Avg /= float64(len(ss))
	s.Min, s.Max, s.Avg = r(s.Min), r(s.Max), r(s.Avg)
	log.Println("Last hour stats", "Percent full", s, "Metrics", metrics)
	log.Println(
		"Average HTTP time", time.Duration(metrics.HTTPDuration/uint64(metrics.HTTP+1)),
		"Average HTTPS time", time.Duration(metrics.HTTPSDuration/uint64(metrics.HTTPS+1)),
	)
	metrics = Metrics{} // reset the metrics
}

// AggregateUpdates returns a channel which this reads and summarizes hourly.
func AggregateUpdates() chan Sample {
	var Samples []Sample
	hour := time.Tick(time.Hour)
	watcher := func(ch chan Sample) {
		for {
			select {
			case s := <-ch:
				Samples = append(Samples, s)
			case <-hour:
				logStats(Samples)
				Samples = Samples[:0]
			}
		}
	}
	sampleChan := make(chan Sample)
	go watcher(sampleChan)
	return sampleChan
}
