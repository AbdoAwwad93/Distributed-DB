package replication

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"distributed-db/internal/models"
)

type pendingOperation struct {
	Query     string
	CreatedAt time.Time
	Attempts  int
}

type slaveState struct {
	URL         string
	Healthy     bool
	LastChecked time.Time
	LastError   string
	Pending     []pendingOperation
}

type Broadcaster struct {
	client *http.Client
	mu     sync.Mutex
	slaves map[string]*slaveState
}

func NewBroadcaster(slaveURLs []string) *Broadcaster {
	slaves := make(map[string]*slaveState, len(slaveURLs))
	for _, slaveURL := range slaveURLs {
		slaveURL = strings.TrimRight(strings.TrimSpace(slaveURL), "/")
		if slaveURL == "" {
			continue
		}

		slaves[slaveURL] = &slaveState{URL: slaveURL, Healthy: true}
	}

	return &Broadcaster{
		client: &http.Client{Timeout: 5 * time.Second},
		slaves: slaves,
	}
}

func (b *Broadcaster) StartHealthChecks(interval time.Duration) {
	if b == nil || interval <= 0 {
		return
	}

	go func() {
		b.CheckHealth()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			b.CheckHealth()
		}
	}()
}

func (b *Broadcaster) Broadcast(query string) []models.ReplicationResponse {
	if b == nil {
		return nil
	}

	slaveURLs := b.slaveURLs()
	if len(slaveURLs) == 0 {
		return nil
	}

	responses := make([]models.ReplicationResponse, len(slaveURLs))
	var wg sync.WaitGroup

	for i, slaveURL := range slaveURLs {
		wg.Add(1)
		go func(index int, baseURL string) {
			defer wg.Done()

			if !b.isHealthy(baseURL) {
				b.enqueue(baseURL, query, "slave is unhealthy")
				responses[index] = models.ReplicationResponse{Slave: baseURL, Status: "queued", Error: "slave is unhealthy"}
				return
			}

			result := b.send(baseURL, query)
			if result.Status != "ok" {
				b.enqueue(baseURL, query, result.Error)
			}
			responses[index] = result
		}(i, slaveURL)
	}

	wg.Wait()
	return responses
}

func (b *Broadcaster) RetryPending() []models.ReplicationResponse {
	if b == nil {
		return nil
	}

	slaveURLs := b.slaveURLs()
	responses := make([]models.ReplicationResponse, 0)
	for _, slaveURL := range slaveURLs {
		responses = append(responses, b.retrySlave(slaveURL)...)
	}

	return responses
}

func (b *Broadcaster) CheckHealth() []models.SlaveStatus {
	if b == nil {
		return nil
	}

	slaveURLs := b.slaveURLs()
	var wg sync.WaitGroup
	for _, slaveURL := range slaveURLs {
		wg.Add(1)
		go func(baseURL string) {
			defer wg.Done()
			b.checkSlave(baseURL)
		}(slaveURL)
	}
	wg.Wait()

	return b.Status()
}

func (b *Broadcaster) Status() []models.SlaveStatus {
	if b == nil {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	statuses := make([]models.SlaveStatus, 0, len(b.slaves))
	for _, slave := range b.slaves {
		statuses = append(statuses, models.SlaveStatus{
			URL:          slave.URL,
			Healthy:      slave.Healthy,
			LastChecked:  slave.LastChecked,
			LastError:    slave.LastError,
			PendingCount: len(slave.Pending),
		})
	}

	return statuses
}

func (b *Broadcaster) checkSlave(baseURL string) {
	url := strings.TrimRight(baseURL, "/") + "/health"
	response, err := b.client.Get(url)

	b.mu.Lock()
	slave := b.slaves[baseURL]
	if slave == nil {
		slave = &slaveState{URL: baseURL}
		b.slaves[baseURL] = slave
	}
	slave.LastChecked = time.Now()
	if err != nil {
		slave.Healthy = false
		slave.LastError = err.Error()
		b.mu.Unlock()
		return
	}
	defer response.Body.Close()

	if response.StatusCode >= 300 {
		slave.Healthy = false
		slave.LastError = fmt.Sprintf("status %d", response.StatusCode)
		b.mu.Unlock()
		return
	}

	slave.Healthy = true
	slave.LastError = ""
	pendingCount := len(slave.Pending)
	b.mu.Unlock()

	if pendingCount > 0 {
		log.Printf("slave %s is healthy; retrying %d queued replication operations", baseURL, pendingCount)
		b.retrySlave(baseURL)
	}
}

func (b *Broadcaster) retrySlave(baseURL string) []models.ReplicationResponse {
	responses := make([]models.ReplicationResponse, 0)

	for {
		operation, ok := b.nextPending(baseURL)
		if !ok {
			return responses
		}

		result := b.send(baseURL, operation.Query)
		responses = append(responses, result)
		if result.Status != "ok" {
			operation.Attempts++
			b.requeueFront(baseURL, operation, result.Error)
			return responses
		}

		b.markHealthy(baseURL)
	}
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
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return models.ReplicationResponse{Slave: baseURL, Status: "failed", Error: strings.TrimSpace(fmt.Sprintf("status %d %s", response.StatusCode, body))}
	}

	return models.ReplicationResponse{Slave: baseURL, Status: "ok"}
}

func (b *Broadcaster) slaveURLs() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	urls := make([]string, 0, len(b.slaves))
	for url := range b.slaves {
		urls = append(urls, url)
	}

	return urls
}

func (b *Broadcaster) isHealthy(baseURL string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	slave := b.slaves[baseURL]
	return slave != nil && slave.Healthy
}

func (b *Broadcaster) enqueue(baseURL, query, reason string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	slave := b.slaves[baseURL]
	if slave == nil {
		slave = &slaveState{URL: baseURL}
		b.slaves[baseURL] = slave
	}

	slave.Healthy = false
	slave.LastError = reason
	slave.Pending = append(slave.Pending, pendingOperation{Query: query, CreatedAt: time.Now()})
}

func (b *Broadcaster) nextPending(baseURL string) (pendingOperation, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	slave := b.slaves[baseURL]
	if slave == nil || len(slave.Pending) == 0 {
		return pendingOperation{}, false
	}

	operation := slave.Pending[0]
	slave.Pending = slave.Pending[1:]
	return operation, true
}

func (b *Broadcaster) requeueFront(baseURL string, operation pendingOperation, reason string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	slave := b.slaves[baseURL]
	if slave == nil {
		slave = &slaveState{URL: baseURL}
		b.slaves[baseURL] = slave
	}

	slave.Healthy = false
	slave.LastError = reason
	slave.Pending = append([]pendingOperation{operation}, slave.Pending...)
}

func (b *Broadcaster) markHealthy(baseURL string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	slave := b.slaves[baseURL]
	if slave != nil {
		slave.Healthy = true
		slave.LastError = ""
	}
}
