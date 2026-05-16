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

type RegisterSlaveRequest struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type RegisterSlaveResponse struct {
	Message string `json:"message"`
	ID      string `json:"id"`
	URL     string `json:"url"`
}

type ApprovalSubmissionRequest struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	RawQuery    string `json:"rawQuery,omitempty"`
	Body        []byte `json:"body,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	RequestedBy string `json:"requestedBy"`
	RequestRole string `json:"requestRole"`
}

type ApprovalDecisionRequest struct {
	Reason string `json:"reason,omitempty"`
}

type ApprovalRequest struct {
	ID          string    `json:"id"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	RawQuery    string    `json:"rawQuery,omitempty"`
	Body        string    `json:"body,omitempty"`
	ContentType string    `json:"contentType,omitempty"`
	RequestedBy string    `json:"requestedBy"`
	RequestRole string    `json:"requestRole"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	DecisionBy  string    `json:"decisionBy,omitempty"`
	Reason      string    `json:"reason,omitempty"`
}

type ApprovalListResponse struct {
	Node     string            `json:"node"`
	Requests []ApprovalRequest `json:"requests"`
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
	ID           string    `json:"id,omitempty"`
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
