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
	// Get a sorted list of entries under data/{fridge}/ .
	ff, err := os.ReadDir(dataPath(fridge, "."))
	if err != nil {
		log.Printf("couldn't read data archive %s: %s\n", dataPath(fridge, "."), err)
	}
	// Report files are in the format yyyymmddhhmmss.json.gz .
	var report = regexp.MustCompile("[0-9]{14}.json.gz")

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

	// Bundle the data by era (herein, a single day per first 8 of yyyymmddhhmmss).
	const resolution = 8
	era := ""
	var writeEra = func() {
		if era == "" {
			return
		}
		fn := dataPath(fridge, fmt.Sprintf("%s.json.gz", era))
		log.Printf("writing summary %s\n", fn)
		iu := ICBMreport{
			FridgeName:    fridge,
			RawSamples:    sorted(iu.RawSamples),
			StableSamples: sorted(iu.StableSamples),
		}
		b, _ := json.Marshal(iu)
		gzWrite(fn, "rollup for era "+fn, b)
	}

	// For each entry, extract the era, and record the data.
	for i := range ff {
		if !report.MatchString(ff[i].Name()) {
			continue
		}
		fn := ff[i].Name()
		fera := fn[:resolution]
		if fera != era {
			writeEra()
			iu.RawSamples = []Sample{}
			iu.StableSamples = []Sample{}
			era = fera
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
	writeEra()

	// write out new tsv
	processUpdate(iu)
}
