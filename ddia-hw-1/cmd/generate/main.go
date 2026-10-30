package main

import (
	"context"
	"flag"
	"log"

	"highload/ddia-hw-1/internal/dashboard"
	"highload/ddia-hw-1/internal/grafana"
	"highload/ddia-hw-1/internal/metrics"
)

func main() {
	appURL := flag.String("app-url", "http://app:8080", "application URL")
	grafanaURL := flag.String("grafana-url", "http://grafana:3000", "Grafana URL")
	tokenPath := flag.String("token-file", "/run/token/token", "service account token file")
	flag.Parse()
	ctx := context.Background()
	var routes []metrics.Route
	if err := grafana.New(*appURL).Do(ctx, "GET", "/endpoints", nil, &routes); err != nil {
		log.Fatal(err)
	}
	if len(routes) == 0 {
		log.Fatal("no endpoints found")
	}
	token, err := grafana.ReadSecret(*tokenPath)
	if err != nil {
		log.Fatal(err)
	}
	client := grafana.New(*grafanaURL)
	client.Token = token
	body := map[string]any{"dashboard": dashboard.Build(routes), "overwrite": true}
	if err := client.Do(ctx, "POST", "/api/dashboards/db", body, nil); err != nil {
		log.Fatal(err)
	}
	log.Printf("dashboard created: %d endpoints", len(routes))
}
