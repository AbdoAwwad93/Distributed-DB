package main

import (
	"log"

	"distributed-db/internal/api"
	"distributed-db/internal/config"
	"distributed-db/internal/console"
	"distributed-db/internal/replication"
	"distributed-db/internal/storage"
)

func main() {
	cfg := config.LoadMasterConfig()

	store, err := storage.NewStore(cfg.MySQLDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	broadcaster := replication.NewBroadcaster(cfg.SlaveURLs)
	broadcaster.StartHealthChecks(cfg.HealthCheckInterval)

	server := api.NewServer(cfg, store, broadcaster)
	menu := console.NewMenu(cfg)

	log.Printf("starting %s node on %s", cfg.Role, cfg.Address())
	go func() {
		if err := server.Start(); err != nil {
			log.Fatal(err)
		}
	}()

	menu.Run()
}
