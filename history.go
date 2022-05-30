package main

// This handles maintenance of history files.

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
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
	fridgeDataPath := dataPath(fridge, ".") // filesystem path to this fridge's archives
	// Get a sorted list of entries under data/{fridge}/ .
	ff, err := os.ReadDir(fridgeDataPath)
	if err != nil {
		log.Printf("couldn't read data archive %s: %s\n", fridgeDataPath, err)
	}
	// report checks whether the string matches the ICBM report format, yyyymmddhhmmss.json.gz .
	var report = regexp.MustCompile("[0-9]{14}.json.gz").MatchString
	var sorted = func(s []Sample) []Sample {
		sort.Slice(s, func(i, j int) bool {
			return s[i].Timestamp.Before(s[j].Timestamp)
		})
		return s
	}

	var bundle *ICBMreport

	// Bundle the data by era (herein, a single day per first 8 of yyyymmddhhmmss).
	const resolution = 8
	era := ""
	var writeEra = func() {
		if bundle == nil {
			return
		}
		fn := dataPath(fridge, fmt.Sprintf("%s.json.gz", era))
		log.Printf("writing summary %s\n", fn)
		b, _ := json.Marshal(ICBMreport{
			FridgeName:    fridge,
			RawSamples:    sorted(bundle.RawSamples),
			StableSamples: sorted(bundle.StableSamples),
		})
		gzWrite(fn, "rollup for era "+fn, b)
	}

	// A scan conversion, as the files are in order. For each entry,
	// extract the era, and record the data.
	eraFmt := "20060102150405"
	now := time.Now().Format(eraFmt[:resolution])
	archive := path.Join(fridgeDataPath, "archive")
	if err = os.MkdirAll(archive, 0700); err != nil {
		log.Printf("Couldn't create archive folder: %s", err)
	}
	log.Println("archiving to", archive)

	latest := &ICBMreport{}

	for i := range ff {
		if !report(ff[i].Name()) {
			continue
		}
		fn := ff[i].Name()
		fera := fn[:resolution]
		if fera == now {
			break // Don't bundle the current era, it's not finished yet!
		}
		if fera != era {
			writeEra()
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
		latest = latest.Append(rep)
		dst := path.Join(archive, ff[i].Name())
		_ = os.Rename(src, dst)
		if rand.Float32() > 0.95 {
			latest.Trim(31 * 24 * 60 / 5)
		}
	}
	writeEra()

	// write out new tsv
	processUpdate(*latest)
	log.Println("archive finished")
}
