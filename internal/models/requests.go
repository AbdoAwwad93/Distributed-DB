package models

import "time"

type CreateTableRequest struct {
	Table   string            `json:"table"`
	Columns map[string]string `json:"columns"`
}

type DropTableRequest struct {
	Table string `json:"table"`
}

type QueryRequest struct {
	Query string `json:"query"`
}

type MessageResponse struct {
	Message string `json:"message"`
}

type SelectResponse struct {
	Rows []map[string]any `json:"rows"`
}

type ReplicationResponse struct {
	Slave  string `json:"slave"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type SlaveStatus struct {
	URL          string    `json:"url"`
	Healthy      bool      `json:"healthy"`
	LastChecked  time.Time `json:"lastChecked,omitempty"`
	LastError    string    `json:"lastError,omitempty"`
	PendingCount int       `json:"pendingCount"`
}

type ClusterStatusResponse struct {
	Node   string        `json:"node"`
	Slaves []SlaveStatus `json:"slaves"`
}
