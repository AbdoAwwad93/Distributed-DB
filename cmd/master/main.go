package main

import (
	"log"

	"distributed-db/internal/api"
	"distributed-db/internal/config"
)

func main() {
	cfg := config.LoadMasterConfig()

	server := api.NewServer(cfg)

	log.Printf("starting %s node on %s", cfg.Role, cfg.Address())
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}
