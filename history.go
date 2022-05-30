package main

// This file handles maintenance of history files.

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func readReport(fn string) (iu ICBMreport, err error) {
	var r io.Reader
	r, err = os.Open(fn)
	if err != nil {
		return iu, fmt.Errorf("couldn't open %s: %w", fn, err)
	}
	if strings.HasSuffix(fn, ".gz") {
		r, err = gzip.NewReader(r)
		if err != nil {
			return iu, fmt.Errorf("couldn't wrap gunzip: %w", err)
		}
	}
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return iu, fmt.Errorf("couldn't gunzip: %w", err)
	}
	json.Unmarshal(b, &iu)
	return iu, nil
}

func repack(fridge string) {
	// get a sorted list of entries under data/{fridge}/20220529172056.json.gz
	ff, err := os.ReadDir(dataPath(fridge, "."))
	if err != nil {
		log.Printf("couldn't read data archive %s: %s\n", dataPath(fridge, "."), err)
	}
	// report files are in the format yyyymmddhhmmss.json.gz
	var report = regexp.MustCompile("[0-9]{14}.json.gz")

	year, month := 0, 0
	iu := ICBMreport{
		FridgeName:    fridge,
		RawSamples:    []Sample{},
		StableSamples: []Sample{},
	}

	var sorted = func(s []Sample) []Sample {
		sort.Slice(s, func(i, j int) bool {
			return s[i].Timestamp.Before(s[j].Timestamp)
		})
		return s
	}

	var writeMonth = func() {
		if year == 0 {
			return
		}
		fn := dataPath(fridge, fmt.Sprintf("%04d-%02d.json.gz", year, month))
		log.Printf("writing summary %s\n", fn)
		iu := ICBMreport{
			FridgeName:    fridge,
			RawSamples:    sorted(iu.RawSamples),
			StableSamples: sorted(iu.StableSamples),
		}
		b, _ := json.Marshal(iu)
		go gzWrite(fn, "monthly rollup", b)
	}

	// For each entry, extract the year and month, and record the data.
	for i := range ff {
		if !report.MatchString(ff[i].Name()) {
			continue
		}
		fn := ff[i].Name()
		ny, _ := strconv.Atoi(fn[0:4])
		nm, _ := strconv.Atoi(fn[4:6])
		if ny != year || month != nm {
			writeMonth()
			iu.RawSamples = iu.RawSamples[:0]
			iu.StableSamples = iu.StableSamples[:0]
			month, year = nm, ny
		}
		iup, err := readReport(dataPath(fridge, fn))
		if err != nil {
			log.Println(err)
			continue
		}
		iu.RawSamples = append(iu.RawSamples, iup.RawSamples...)
		iu.StableSamples = append(iu.StableSamples, iup.StableSamples...)
		iu.RawMassFull = iup.RawMassFull
		iu.RawMassTare = iup.RawMassTare
	}
	writeMonth()

	// write out new tsv
	processUpdate(iu)
}
