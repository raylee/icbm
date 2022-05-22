package main

// metrics and logging

import (
	"bytes"
	"compress/gzip"
	"fmt"
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
var StatsChan chan Sample = AggregateUpdates() // StatsChan aggregates samples written to it.

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

const (
	italicOn  = "\033[3m"
	italicOff = "\033[23m"
	resetAll  = "\033[0m"
)

type italic struct {
	*os.File
}

// Write implements an italic io.Writer interface.
func (i italic) Write(p []byte) (n int, err error) {
	defer i.File.Write([]byte(italicOff))
	// if p contains any resets, fix them up. Not speed critical as this is going to a console.
	len := 0
	for _, h := range bytes.Split(p, []byte(resetAll)) {
		i.File.Write([]byte(resetAll + italicOn))
		len, err = i.File.Write(h)
		n += len + 4
		if err != nil {
			return
		}
	}
	n -= 4 // we counted one extra reset
	return
}

// Italic wraps f to add italics if f is a terminal, otherwise will return f unchanged.
func Italic(f *os.File) io.Writer {
	fileInfo, err := f.Stat()
	if err != nil || fileInfo.Mode()&os.ModeCharDevice == 0 {
		return f
	}
	return &italic{f}
}

// localPath builds an absolute pathname, including the path which
// contains this executable, joined by []subdirs underneath it
func localPath(subdirs ...string) string {
	ex, _ := os.Executable()                           // get the path to this executable
	abs, _ := filepath.Abs(ex)                         // resolve it to the root
	folder := filepath.Dir(abs)                        // find the containing folder
	ee := append([]string{folder, "data"}, subdirs...) // full path to target

	filename := path.Join(ee...)
	dir := filepath.Dir(filename)
	os.MkdirAll(dir, os.ModeDir|0755) // ensure the whole path exists
	return filename
}

// logData saves the received JSON as a compressed file.
func logData(u ICBMUpdate, rawData []byte) {
	now := time.Now()
	filename := now.Format("20060102150405") + ".json"
	pathname := localPath(u.FridgeName, filename) + ".gz"

	f, e := os.Create(pathname)
	if e != nil {
		log.Println("Could not store json measurements for updated chartdata: " + pathname)
		metrics.Errors++
	}

	zw := gzip.NewWriter(f)
	// Setting the Header fields is optional, but polite.
	zw.Name = filename
	zw.Comment = "icbm telemetry for " + fmt.Sprintf(u.FridgeName)
	zw.ModTime = now

	if _, e := zw.Write(rawData); e != nil {
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
		"Average HTTP time", time.Duration(metrics.HTTPDuration/uint64(metrics.HTTP)),
		"Average HTTPS time", time.Duration(metrics.HTTPSDuration/uint64(metrics.HTTPS)),
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
