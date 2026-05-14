package models

type CreateTableRequest struct {
	Table   string            `json:"table"`
	Columns map[string]string `json:"columns"`
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
