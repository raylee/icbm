package main

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
	"time"
)

type (
	// ICBMUpdate corresponds to the fridge payload.
	ICBMUpdate struct {
		FridgeName    string
		RawMassFull   int
		RawMassTare   int
		RawSamples    []Sample
		StableSamples []Sample
	}

	// Sample holds any one individual sample from the fridge.
	Sample struct {
		PubFillRatio float64
		RawFillRatio float64
		RawMass      int
		Timestamp    time.Time
	}
)

func clamp(x, low, high float64) float64 {
	x = math.Max(low, x)
	return math.Min(x, high)
}

// processUpdate takes a set of samples and appends them to the correct history
func processUpdate(u ICBMUpdate) error {
	filename := localPath(u.FridgeName + ".tsv")
	// data := loadHistory(filename)
	chartData := ""
	for _, s := range u.StableSamples {
		History[u.FridgeName] = append(History[u.FridgeName], s)
		x := clamp(s.PubFillRatio, 0.0, 1.0)
		chartData += fmt.Sprintf("%d\t%g\n", s.Timestamp.Unix(), x)
		metrics.DataPoints++
		StatsChan <- s
	}
	excess := len(History[u.FridgeName]) - maxHistory
	if excess > 0 {
		for i := 0; i < maxHistory; i++ {
			History[u.FridgeName][i] = History[u.FridgeName][i+excess]
		}
		History[u.FridgeName] = History[u.FridgeName][0:maxHistory]
	}
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		metrics.Errors++
		return fmt.Errorf("could not open data file for appending: %w", err)
	}
	if _, err := f.Write([]byte(chartData)); err != nil {
		metrics.Errors++
		return fmt.Errorf("could not append chartdata: %w", err)
	}
	if err := f.Close(); err != nil {
		metrics.Errors++
		return fmt.Errorf("could not close written file: %w", err)
	}
	return trimFile(filename, 10000)
}

// icbmUpdate handles the POST of new fridge data, including checking credentials
func icbmUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		http.Error(w, "Please send a request body", http.StatusBadRequest)
		return
	}
	user := getLogin(w, r)
	if user == nil {
		http.Error(w, "Fridge status not updated, please supply an API key", http.StatusUnauthorized)
		return
	}
	if !user.Valid {
		http.Error(w, "Your account is disabled, please contact the administrator if you believe this is in error", http.StatusForbidden)
		return
	}
	var data ICBMUpdate
	rawRequest, _ := ioutil.ReadAll(r.Body)
	if err := json.NewDecoder(bytes.NewReader(rawRequest)).Decode(&data); err != nil {
		metrics.BadJSON++
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err := processUpdate(data)
	if err != nil {
		log.Println("Error processing update:", err)
	}
	logData(data, rawRequest)
	io.WriteString(w, fmt.Sprintf("Fridge status updated for %s, thank you %s\n", data.FridgeName, user.Username))
}

// trimFile preserves the last N lines of contents of filename, removing all before.
func trimFile(filename string, N int) error {
	content, err := tail(filename, N)
	if err != nil {
		return err
	}
	tmpfile, err := ioutil.TempFile("", "icbm-data-")
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
		return fmt.Errorf("Error renaming tempfile " + tmpfile.Name() + " to " + filename)
	}
	return nil
}

// tail acts like the unix command of the same name. Warning: this reads the
// whole file into memory. It's optimum for ICBM's use case but not in general.
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