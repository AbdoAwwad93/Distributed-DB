package main

import (
	"log"

	"distributed-db/internal/api"
	"distributed-db/internal/cluster"
	"distributed-db/internal/config"
	"distributed-db/internal/storage"
)

func main() {
	cfg := config.LoadSlaveConfig("slave2", "8082", "SLAVE2_MYSQL_DSN", "root:@tcp(localhost:3306)/distributed_slave2?parseTime=true")

	store, err := storage.NewStore(cfg.MySQLDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	server := api.NewServer(cfg, store, nil)
	go cluster.RegisterSlaveLoop(cfg)

	log.Printf("starting %s node %s on %s", cfg.Role, cfg.NodeID, cfg.Address())
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}
