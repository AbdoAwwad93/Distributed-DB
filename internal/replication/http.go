package replication

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"distributed-db/internal/models"
)

type Broadcaster struct {
	client    *http.Client
	slaveURLs []string
}

func NewBroadcaster(slaveURLs []string) *Broadcaster {
	return &Broadcaster{
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		slaveURLs: slaveURLs,
	}
}

func (b *Broadcaster) Broadcast(query string) []models.ReplicationResponse {
	if len(b.slaveURLs) == 0 {
		return nil
	}

	responses := make([]models.ReplicationResponse, len(b.slaveURLs))
	var wg sync.WaitGroup

	for i, slaveURL := range b.slaveURLs {
		wg.Add(1)
		go func(index int, baseURL string) {
			defer wg.Done()
			responses[index] = b.send(baseURL, query)
		}(i, slaveURL)
	}

	wg.Wait()
	return responses
}

func (b *Broadcaster) send(baseURL, query string) models.ReplicationResponse {
	payload, err := json.Marshal(models.QueryRequest{Query: query})
	if err != nil {
		return models.ReplicationResponse{Slave: baseURL, Status: "failed", Error: err.Error()}
	}

	url := strings.TrimRight(baseURL, "/") + "/replicate"
	response, err := b.client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return models.ReplicationResponse{Slave: baseURL, Status: "failed", Error: err.Error()}
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		return models.ReplicationResponse{Slave: baseURL, Status: "failed", Error: fmt.Sprintf("status %d", response.StatusCode)}
	}

	return models.ReplicationResponse{Slave: baseURL, Status: "ok"}
}
