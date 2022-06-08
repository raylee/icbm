package main

// Handle the JSON reports from the fridge. Parsing, saving, and summarizing for charting.

import (
	"bytes"
	"encoding/json"
	"fmt"

	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"regexp"
	"sort"
	"sync"
	"time"

	"golang.org/x/exp/constraints"
)

type (
	// Sample holds any one individual sample from the fridge.
	Sample struct {
		PubFillRatio float64
		RawFillRatio float64
		RawMass      int
		Timestamp    time.Time
	}
	// ICBMreport corresponds to the fridge payload.
	ICBMreport struct {
		FridgeName    string
		RawMassFull   int
		RawMassTare   int
		RawSamples    []Sample // every second or two
		StableSamples []Sample // every minute

		sorted bool
		mu     *sync.Mutex
	}
)

// Append samples from the passed report to this one.
func (r *ICBMreport) Append(n ICBMreport) *ICBMreport {
	if r == nil {
		return &n
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.RawSamples = append(r.RawSamples, n.RawSamples...)
	r.StableSamples = append(r.StableSamples, n.StableSamples...)
	r.sorted = false
	return r
}

// sort the samples in this report by time. Requires the caller to hold the lock.
func (r *ICBMreport) sort() {
	if r == nil || r.sorted {
		return
	}

	sort.Slice(r.StableSamples, func(i, j int) bool {
		return r.StableSamples[i].Timestamp.Before(r.StableSamples[j].Timestamp)
	})
	sort.Slice(r.RawSamples, func(i, j int) bool {
		return r.RawSamples[i].Timestamp.Before(r.RawSamples[j].Timestamp)
	})
	r.sorted = true
}

// Save a compressed (.json.gz) version of this report to dataPath(fn) + .json.gz.
func (r *ICBMreport) Save(fn, comment string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sort()

	b, _ := json.Marshal(*r)
	fn = dataPath(r.FridgeName, fmt.Sprintf("%s.json.gz", fn))
	gzWrite(fn, comment, b)
}

// Trim the Samples to the latest max count.
func (r *ICBMreport) Trim(max int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sort()

	var once sync.Once
	if len(r.RawSamples) > max {
		once.Do(r.sort)
		r.RawSamples = last(r.RawSamples, max)
	}
	if len(r.StableSamples) > max {
		once.Do(r.sort)
		r.StableSamples = last(r.StableSamples, max)
	}
}

// KeepSince keeps only the samples more recent than the duration ago.
func (r *ICBMreport) KeepSince(d time.Duration) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sort()

	notBefore := time.Now().Add(-d)

	from := sort.Search(len(r.RawSamples), func(i int) bool {
		return r.RawSamples[i].Timestamp.After(notBefore)
	})
	r.RawSamples = last(r.RawSamples, len(r.RawSamples)-from)

	from = sort.Search(len(r.StableSamples), func(i int) bool {
		return r.StableSamples[i].Timestamp.After(notBefore)
	})
	r.StableSamples = last(r.StableSamples, len(r.StableSamples)-from)
}

// Rollup the data to one sample per duration.
func (r *ICBMreport) Rollup(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sort()

	harmonized := []Sample{}
	var t time.Time
	for _, s := range r.RawSamples {
		if s.Timestamp.After(t.Add(d)) {
			harmonized = append(harmonized, s)
			t = s.Timestamp
		}
	}
	r.RawSamples = harmonized
	harmonized = []Sample{}
	for _, s := range r.StableSamples {
		if s.Timestamp.After(t.Add(d)) {
			harmonized = append(harmonized, s)
			t = s.Timestamp
		}
	}
	r.StableSamples = harmonized
}

// clamp ensures that x is between low and high, for an orderable type.
func clamp[T constraints.Ordered](x, low, high T) T {
	if x < low {
		return low
	}
	if x > high {
		return high
	}
	return x
}

// processUpdate takes a set of samples and appends them to the correct history
func processUpdate(u ICBMreport) error {
	filename := dataPath(u.FridgeName + ".tsv")
	chartData := ""
	for _, s := range u.StableSamples {
		s.PubFillRatio = clamp(s.PubFillRatio, 0.0, 1.0)
		chartData += fmt.Sprintf("%d\t%g\n", s.Timestamp.Unix(), s.PubFillRatio)
		// metrics.DataPoints++
	}
	tapReport[u.FridgeName] = tapReport[u.FridgeName].Append(u)
	tapReport[u.FridgeName].KeepSince(maxAge)

	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// metrics.Errors++
		return fmt.Errorf("could not open data file for appending: %w", err)
	}
	if _, err := f.Write([]byte(chartData)); err != nil {
		// metrics.Errors++
		return fmt.Errorf("could not append chartdata: %w", err)
	}
	if err := f.Close(); err != nil {
		// metrics.Errors++
		return fmt.Errorf("could not close written file: %w", err)
	}
	return trimFile(filename, 10000)
}

var disallowed = regexp.MustCompile(`[^[:alnum:]-.]`)

// sanitize returns a copy of x without any disallowed characters
func sanitize(x string) string {
	return disallowed.ReplaceAllString(x, "")
}

// icbmUpdate handles the POST of new fridge data, including checking credentials
func icbmUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		http.Error(w, "Please send a request body", http.StatusBadRequest)
		return
	}
	user := getLogin(w, r)
	if user == nil {
		http.Error(w, "Fridge status not updated, please supply an authorized API key", http.StatusUnauthorized)
		return
	}
	if !user.Valid {
		http.Error(w, "Your account is disabled, please contact the administrator if you believe this is in error", http.StatusForbidden)
		return
	}
	var data ICBMreport
	rawRequest, _ := ioutil.ReadAll(r.Body)
	if err := json.NewDecoder(bytes.NewReader(rawRequest)).Decode(&data); err != nil {
		// metrics.BadJSON++
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data.FridgeName = sanitize(data.FridgeName)
	data.mu = &sync.Mutex{}

	if err := processUpdate(data); err != nil {
		log.Println("Error processing update:", err)
		// metrics.Errors++
		// Fallthrough to save the data regardless.
	}
	filename := time.Now().Format("20060102150405")
	data.Save(filename, "icbm update for "+data.FridgeName)
	io.WriteString(w, fmt.Sprintf("Fridge status updated for %s, thank you %s\n", data.FridgeName, user.Username))
}

// trimFile preserves the last N lines of contents of filename, removing all before.
func trimFile(filename string, n int) error {
	content, err := tail(filename, n)
	if err != nil {
		return err
	}
	tmpDir := "."
	if superfly() {
		tmpDir = "/data"
	}
	tmpfile, err := ioutil.TempFile(tmpDir, "icbm-data-")
	if err != nil {
		return fmt.Errorf("could not create tempfile from filename %s: %w", filename, err)
	}
	_, err = tmpfile.Write(content)
	if err != nil {
		tmpfile.Close()
		os.Remove(tmpfile.Name())
		return fmt.Errorf("could not write to tempfile %s: %w", tmpfile.Name(), err)
	}
	if err := tmpfile.Close(); err != nil {
		return fmt.Errorf("error closing tempfile %s: %w", tmpfile.Name(), err)
	}
	if err := os.Rename(tmpfile.Name(), filename); err != nil {
		return fmt.Errorf("error renaming tempfile %s to %s: %w", tmpfile.Name(), filename, err)
	}
	return nil
}

// tail acts like the unix command of the same name. Warning: this reads the
// whole file into memory. It's optimal for ICBM's use case but not in general.
func tail(filename string, lines int) ([]byte, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("couldn't open %s for reading: %w", filename, err)
	}
	idx := NthFromEnd(content, '\n', lines+1)
	return content[idx+1:], nil
}

// NthFromEnd returns the position of the nth needle in haystack, counting
// backward from the end.
func NthFromEnd(haystack []byte, needle byte, n int) int {
	if n < 0 {
		return -1
	}
	for i := len(haystack) - 1; i >= 0; i-- {
		if haystack[i] != needle {
			continue
		}
		n--
		if n == 0 {
			return i
		}
	}
	return -1
}

func last[T any](slice []T, keep int) []T {
	n := make([]T, keep)
	start := len(slice) - keep
	if start < 0 {
		start = 0
	}
	copy(n, slice[start:])
	return n
}

func mapf[T any, S any](slice []T, extractFunc func([]T, int) S) []S {
	col := make([]S, len(slice))
	for i := range slice {
		col[i] = extractFunc(slice, i)
	}
	return col
}

func stdev[T constraints.Float | constraints.Integer](slice []T) float64 {
	var sum, squared float64
	for i := range slice {
		f := float64(slice[i])
		sum += f
		squared += f * f
	}
	n := float64(len(slice))
	return math.Sqrt((squared - sum*sum/n) / n)
}

func average[T constraints.Float | constraints.Integer](slice []T) float64 {
	var sum float64
	for i := range slice {
		sum += float64(slice[i])
	}
	return sum / float64(len(slice))
}
