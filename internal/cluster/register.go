package cluster

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"distributed-db/internal/config"
	"distributed-db/internal/models"
)

func RegisterSlaveLoop(cfg config.NodeConfig) {
	if cfg.MasterURL == "" || cfg.PublicURL == "" {
		log.Printf("slave registration skipped: MASTER_URL or public URL is empty")
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	endpoint := strings.TrimRight(cfg.MasterURL, "/") + "/register-slave"
	payload := models.RegisterSlaveRequest{
		ID:  cfg.NodeID,
		URL: strings.TrimRight(cfg.PublicURL, "/"),
	}

	for {
		body, err := json.Marshal(payload)
		if err != nil {
			log.Printf("slave registration payload error: %v", err)
			return
		}

		response, err := client.Post(endpoint, "application/json", bytes.NewReader(body))
		if err == nil && response != nil {
			response.Body.Close()
		}
		if err == nil && response.StatusCode >= 200 && response.StatusCode < 300 {
			log.Printf("registered slave %s at %s with master %s", payload.ID, payload.URL, cfg.MasterURL)
			return
		}
		if err != nil {
			log.Printf("slave registration failed: %v; retrying in 5s", err)
		} else {
			log.Printf("slave registration returned status %d; retrying in 5s", response.StatusCode)
		}

		time.Sleep(5 * time.Second)
	}
}
