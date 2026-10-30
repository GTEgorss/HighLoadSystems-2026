package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"highload/ddia-hw-1/internal/app"
	"highload/ddia-hw-1/internal/metrics"
)

func main() {
	m := metrics.New(os.Getenv("DEPLOYMENT"), os.Getenv("DATACENTER"), os.Getenv("POD"))
	server := &http.Server{Addr: ":8080", Handler: app.New(m), ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(server.ListenAndServe())
}
