package main

import (
	"log"

	"distributed-db/internal/api"
	"distributed-db/internal/config"
	"distributed-db/internal/storage"
)

func main() {
	cfg := config.LoadSlaveConfig("slave1", "8081", "SLAVE1_MYSQL_DSN", "root:@tcp(localhost:3306)/distributed_slave1?parseTime=true")

	store, err := storage.NewStore(cfg.MySQLDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	server := api.NewServer(cfg, store, nil)

	log.Printf("starting %s node on %s", cfg.Role, cfg.Address())
	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}
