package main

import (
	"log"

	"distributed-db/internal/api"
	"distributed-db/internal/config"
	"distributed-db/internal/storage"
)

func main() {
	cfg := config.LoadMasterConfig()

	store, err := storage.NewStore(cfg.MySQLDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	server := api.NewServer(cfg, store)

	log.Printf("starting %s node on %s", cfg.Role, cfg.Address())
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}
