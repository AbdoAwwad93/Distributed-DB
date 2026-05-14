package models

type CreateTableRequest struct {
	Table   string            `json:"table"`
	Columns map[string]string `json:"columns"`
}

type MessageResponse struct {
	Message string `json:"message"`
}
