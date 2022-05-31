package main

import (
	"bytes"
	_ "embed"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
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
	fracMissing := float64(maxCount-count) / float64(maxCount)

	data.Pop = 12 - int(math.Floor(12.0*fracMissing))
	if data.Pop < 0 {
		data.Pop = 0
	}
	if data.Pop > 12 {
		data.Pop = 12
	}
	log.Println(fracMissing, data.Pop, count, maxCount)

	var res bytes.Buffer
	err := tmpl.ExecuteTemplate(&res, fridge+".tmpl", data)
	if err != nil {
		log.Println("Could not execute template:", err)
	}
	return res.String()
}
