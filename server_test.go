package main

import (
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func payload(fridge string) string {
	return `{
    "FridgeName":"` + fridge + `",
    "RawMassFull": 800000, 
    "RawMassTare": 300000, 
    "RawSamples": [ 
        { "PubFillRatio": 0.48296266666666665, "RawFillRatio": 0.612222, "RawMass": 606111, "Timestamp": "2018-09-13T05:11:32Z" },
        { "PubFillRatio": 0.491112, "RawFillRatio": 0.618334, "RawMass": 609167, "Timestamp": "2018-09-13T05:11:33Z" },
        { "PubFillRatio": 0.4992586666666667, "RawFillRatio": 0.624444, "RawMass": 612222, "Timestamp": "2018-09-13T05:11:34Z" },
        { "PubFillRatio": 0.507408, "RawFillRatio": 0.630556, "RawMass": 615278, "Timestamp": "2018-09-13T05:11:35Z" },
        { "PubFillRatio": 0.5155546666666667, "RawFillRatio": 0.636666, "RawMass": 618333, "Timestamp": "2018-09-13T05:11:36Z" },
        { "PubFillRatio": 0.523704, "RawFillRatio": 0.642778, "RawMass": 621389, "Timestamp": "2018-09-13T05:11:37Z" }
    ],
    "StableSamples": [
        { "PubFillRatio": 0.5033333333333333, "RawFillRatio": 0.6275, "RawMass": 613750, "Timestamp": "2018-09-13T05:11:37Z" }
    ]
}
`
}

func randhex(bytes int) string {
	r := make([]byte, bytes)
	rand.Read(r)
	s := ""
	for _, x := range r {
		s += fmt.Sprintf("%x", x)
	}
	return s
}

func TestServer(t *testing.T) {
	servers := serve("localhost", "0.0.0.0:8081", "")
	defer shutdown(servers)

	body := strings.NewReader(payload("Lunarville-beta"))
	req, err := http.NewRequest("POST", "http://localhost:8081/icbm/v1", body)
	if err != nil {
		t.Error("Error building request")
	}
	apikey := randhex(32)
	users[apikey] = User{Username: "testbot", Valid: true}
	req.Header.Set("X-Icbm-Api-Key", apikey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	c := &http.Client{
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 5 * time.Second,
			}).Dial,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal("could not post data", err)
	}
	res, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Error("could not read response body")
	}
	if string(res) != "Fridge status updated for Lunarville-beta, thank you testbot\n" {
		t.Errorf("\nReceived: %s\nExpected: %s\n", strings.TrimSpace(string(res)), "Fridge status updated for Lunarville-beta, thank you testbot")
	}
}
