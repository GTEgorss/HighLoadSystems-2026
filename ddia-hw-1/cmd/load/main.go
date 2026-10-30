package main

import (
	"flag"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"
)

func main() {
	url := flag.String("url", "http://app:8080", "application URL")
	targetRPS := flag.Float64("rps", 10, "target requests per second")
	ramp := flag.Duration("ramp", 30*time.Second, "ramp-up duration")
	duration := flag.Duration("duration", 5*time.Minute, "load duration")
	flag.Parse()
	if *targetRPS <= 0 || *ramp <= 0 || *duration <= 0 {
		log.Fatal("parameters must be positive")
	}
	client := &http.Client{Timeout: 3 * time.Second}
	start := time.Now()
	requests, failures := 0, 0
	for time.Since(start) < *duration {
		requestStart := time.Now()
		method, path, body := "GET", "/items/1", ""
		switch requests % 3 {
		case 1:
			method, path, body = "POST", "/items", `{"name":"example"}`
		case 2:
			method, body = "PUT", `{"name":"updated"}`
		}
		if rand.IntN(5) == 0 {
			if method == "GET" {
				path = "/items/missing"
			} else {
				body = `{"name":""}`
			}
		}
		req, err := http.NewRequest(method, *url+path, strings.NewReader(body))
		if err != nil {
			log.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		requests++
		if resp.StatusCode >= 400 {
			failures++
		}
		if requests%100 == 0 {
			log.Printf("requests=%d errors=%d", requests, failures)
		}
		progress := time.Since(start).Seconds() / ramp.Seconds()
		rps := *targetRPS * min(1, .1+.9*progress)
		pause := time.Duration(float64(time.Second)/rps) - time.Since(requestStart)
		if pause > 0 {
			time.Sleep(pause)
		}
	}
	log.Printf("finished: requests=%d errors=%d", requests, failures)
}
