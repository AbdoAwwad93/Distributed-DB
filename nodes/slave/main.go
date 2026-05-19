package main

import (
	"log"

	"distributed-db/internal/api"
	"distributed-db/internal/cluster"
	"distributed-db/internal/config"
	"distributed-db/internal/console"
	"distributed-db/internal/storage"
)

func main() {
	cfg := config.LoadSlaveConfig()

	store, err := storage.NewStore(cfg.MySQLDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	server := api.NewServer(cfg, store, nil)
	menu := console.NewMenu(cfg)
	go cluster.RegisterSlaveLoop(cfg, store)

	log.Printf("starting %s node %s on %s", cfg.Role, cfg.NodeID, cfg.Address())
	go func() {
		if err := server.Start(); err != nil {
			log.Fatal(err)
		}
	}()

	menu.Run()
}
