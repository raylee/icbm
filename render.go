package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"
)

var (
	tmpl *template.Template
)

func init() {
	tmpl = template.Must(template.ParseFS(AssetFS, "template/*.tmpl"))
}

// BeverageStatus takes a fridge name and returns an httpHandleFunc which
// renders the page for that fridge.
func BeverageStatus(endpoint string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, renderPage(endpoint))
	}
}

// renderPage takes a template corresponding to the fridge name and renders out
// the most recent data.
func renderPage(fridge string) string {
	data := struct {
		Title       string
		Items       []string
		Pop         int
		FillPercent float64
		Report      *ICBMreport
		LastTime    time.Time
	}{}
	data.Title = fridge + " status"
	data.Report = tapReport[fridge]

	if data.Report == nil {
		return "Not found!" // make a 404 page
	}
	count := len(tapReport[fridge].StableSamples)
	s := tapReport[fridge].StableSamples[count-1]
	data.FillPercent = s.PubFillRatio
	data.LastTime = s.Timestamp

	maxCount := int(maxAge / (300 * time.Second))
	fracMissing := 1.0 - float64(count)/float64(maxCount)
	data.Pop = int(math.Floor(12.0 * fracMissing))
	data.Pop = clamp(data.Pop, 0, 12)
	log.Println(fracMissing, data.Pop, count, maxCount)

	var res bytes.Buffer
	err := tmpl.ExecuteTemplate(&res, fridge+".tmpl", data)
	if err != nil {
		log.Println("Could not execute template:", err)
	}
	return res.String()
}

func tapStatus(w http.ResponseWriter, r *http.Request) {
	path := strings.Split(r.URL.Path, "/")
	fridge := path[0]
	t, found := tapReport[fridge]
	if !found || t == nil {
		http.NotFound(w, r)
		return
	}

	// emit stats from the report
	ss := mapf(t.StableSamples, func(s []Sample, i int) float64 { return s[i].PubFillRatio })
	ts := mapf(t.StableSamples, func(s []Sample, i int) time.Time { return s[i].Timestamp })
	fmt.Fprintf(w, "average: %0.3g ± %0.3g%%\n", average(ss)*100, stdev(ss)*100)
	if len(ts) > 1 {
		span := ts[len(ts)-1].Sub(ts[0])
		days := int(span.Hours() / 24)
		hours := int(math.Mod(span.Hours(), 24))
		mins := int(math.Mod(span.Minutes(), 60))
		fmt.Fprintf(w, "cached-range: %dd%dh%dm\n", days, hours, mins)
	}
}

func icbmVersion(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, platform())
}
