package main

import (
	"context"
	"flag"
	"log"

	"highload/ddia-hw-1/internal/grafana"
)

func main() {
	initPassword := flag.Bool("init", false, "create the Grafana admin password")
	flag.Parse()
	if *initPassword {
		if err := grafana.InitPassword("/run/admin/password"); err != nil {
			log.Fatal(err)
		}
		return
	}
	password, err := grafana.ReadSecret("/run/admin/password")
	if err != nil {
		log.Fatal(err)
	}
	client := grafana.New("http://grafana:3000")
	client.Password = password
	if err := client.Bootstrap(context.Background(), "/run/token/token"); err != nil {
		log.Fatal(err)
	}
	log.Print("service account token is ready")
}
