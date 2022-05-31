package main

// This handles maintenance of history files.

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	maxAge = 31 * 24 * time.Hour // maximum number of samples to keep per fridge in memory
)

var tapReport = make(map[string]*ICBMreport) // Records the most recent data per fridge.

func readReport(fn string) (rep ICBMreport, err error) {
	var r io.Reader
	r, err = os.Open(fn)
	if err != nil {
		return rep, fmt.Errorf("couldn't open %s: %w", fn, err)
	}
	if strings.HasSuffix(fn, ".gz") {
		r, err = gzip.NewReader(r)
		if err != nil {
			return rep, fmt.Errorf("couldn't wrap gunzip: %w", err)
		}
	}
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return rep, fmt.Errorf("couldn't gunzip: %w", err)
	}
	json.Unmarshal(b, &rep)
	return rep, nil
}

func fridgeReports(fridge string, bundles bool) (fs []fs.DirEntry) {
	fridgeDataPath := dataPath(fridge, ".")
	// Get a sorted list of entries under data/{fridge}/ .
	ff, err := os.ReadDir(fridgeDataPath)
	if err != nil {
		log.Printf("couldn't read data archive %s: %s\n", fridgeDataPath, err)
	}

	// report checks whether the string matches the ICBM report format, yyyymmddhhmmss.json.gz .
	report := regexp.MustCompile("[0-9]{14}.json.gz").MatchString
	if bundles {
		report = regexp.MustCompile("[0-9]{8,14}.json.gz").MatchString
	}
	for i := range ff {
		if !report(ff[i].Name()) {
			continue
		}
		fs = append(fs, ff[i])
	}
	return fs
}

func allTaps() (s []string) {
	ff, err := os.ReadDir(dataPath("."))
	if err != nil {
		log.Printf("couldn't read list taps %s: %s\n", dataPath("."), err)
		return
	}
	for i := range ff {
		if ff[i].IsDir() {
			s = append(s, ff[i].Name())
		}
	}
	return
}

func reportTime(rp string) time.Time {
	pi := func(s string) int { i, _ := strconv.Atoi(s); return i }
	y, m, d := pi(rp[0:4]), pi(rp[4:6]), pi(rp[6:8])
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

func loadTapReports() {
	first := time.Now().Add(-maxAge)
	for _, tap := range allTaps() {
		reports := fridgeReports(tap, true)
		for i := range reports {
			name := reports[i].Name()
			tm := reportTime(name)
			if tm.Before(first) {
				continue // too old, don't bother
			}

			log.Println("loading", reports[i].Name())
			src := dataPath(tap, reports[i].Name())
			rep, err := readReport(src)
			if err != nil {
				log.Println(err)
				continue
			}
			tapReport[tap] = tapReport[tap].Append(rep)
		}
		if t := tapReport[tap]; t != nil {
			t.Since(maxAge)
			log.Printf("tap report %s : (%d, %d) raw, stable samples loaded\n", t.FridgeName, len(t.RawSamples), len(t.StableSamples))
		}
	}
}

func init() {
	go loadTapReports()
}

func repack(fridge string) {
	// filesystem path to this fridge's archives
	fridgeDataPath := dataPath(fridge, ".")
	archive := path.Join(fridgeDataPath, "archive")
	if err := os.MkdirAll(archive, 0700); err != nil {
		log.Printf("Couldn't create archive folder: %s", err)
	}

	// Get a sorted list of entries under data/{fridge}/ .
	ff := fridgeReports(fridge, false)

	// Bundle the data by era (herein, a single day per first 8 of yyyymmddhhmmss).

	var bundle *ICBMreport

	eraFmt := "20060102150405"
	const resolution = 8
	now := time.Now().Format(eraFmt[:resolution])
	era := ""

	// A scan conversion, as the files are in order. For each entry,
	// extract the era, and record the data.

	for i := range ff {
		fn := ff[i].Name()
		d, _ := ff[i].Info()
		d.ModTime()

		fera := fn[:resolution]
		if fera == now {
			break // Don't bundle the current era, it's not finished yet!
		}
		if fera != era {
			bundle.Save(era, "rollup for "+era)
			era = fera
			bundle = nil
		}

		src := dataPath(fridge, fn)
		rep, err := readReport(src)
		if err != nil {
			log.Println(err)
			continue
		}

		bundle = bundle.Append(rep)
		dst := path.Join(archive, ff[i].Name())
		_ = os.Rename(src, dst)
	}
	bundle.Save(era, "rollup for "+era)
}
