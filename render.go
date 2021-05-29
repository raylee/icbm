package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
)

const (
	maxHistory = 100 // maximum number of samples to keep per fridge in memory
)

var (
	tmpl    *template.Template
	History = map[string][]Sample{} // History is a cache of the most recent data per fridge.
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
		Title string
		Items []string
	}{Title: "Lunarville beer collective - " + fridge}

	// just show the last five samples, descending order
	samples := len(History[fridge])
	for i := 1; i <= 5 && i < samples; i++ {
		data.Items = append(data.Items, fmt.Sprintf("%v", History[fridge][samples-i]))
	}

	var res bytes.Buffer
	err := tmpl.ExecuteTemplate(&res, fridge+".tmpl", data)
	if err != nil {
		log.Println("Could not execute template")
	}
	return res.String()
}
