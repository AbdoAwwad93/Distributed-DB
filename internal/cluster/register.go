package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"distributed-db/internal/config"
	"distributed-db/internal/models"
	"distributed-db/internal/storage"
)

func RegisterSlaveLoop(cfg config.NodeConfig, store *storage.Store) {
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
			if store == nil {
				log.Printf("registered slave %s at %s with master %s", payload.ID, payload.URL, cfg.MasterURL)
				return
			}

			if err := syncReplicaFromMaster(client, cfg, store); err != nil {
				log.Printf("slave bootstrap sync failed after registration: %v; retrying in 5s", err)
			} else {
				log.Printf("registered slave %s at %s with master %s and synced replica", payload.ID, payload.URL, cfg.MasterURL)
				return
			}
		}
		if err != nil {
			log.Printf("slave registration failed: %v; retrying in 5s", err)
		} else {
			log.Printf("slave registration returned status %d; retrying in 5s", response.StatusCode)
		}

		time.Sleep(5 * time.Second)
	}
}

func syncReplicaFromMaster(client *http.Client, cfg config.NodeConfig, store *storage.Store) error {
	endpoint := strings.TrimRight(cfg.MasterURL, "/") + "/replication/snapshot"
	response, err := client.Get(endpoint)
	if err != nil {
		return fmt.Errorf("fetch snapshot: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf("fetch snapshot status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var snapshot models.SnapshotResponse
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		return fmt.Errorf("decode snapshot: %w", err)
	}

	if err := store.ReplaceWithSnapshot(snapshot.Queries); err != nil {
		return fmt.Errorf("apply snapshot: %w", err)
	}

	return nil
}
