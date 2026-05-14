package api

import (
	"encoding/json"
	"net/http"

	"distributed-db/internal/config"
	"distributed-db/internal/models"
	"distributed-db/internal/storage"
)

type Server struct {
	config config.NodeConfig
	mux    *http.ServeMux
	store  *storage.Store
}

type healthResponse struct {
	Status string `json:"status"`
	Node   string `json:"node"`
}

type dbHealthResponse struct {
	Status string `json:"status"`
	Node   string `json:"node"`
	DBName string `json:"dbName"`
}

func NewServer(cfg config.NodeConfig, store *storage.Store) *Server {
	mux := http.NewServeMux()
	server := &Server{
		config: cfg,
		mux:    mux,
		store:  store,
	}

	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/db/health", server.handleDBHealth)
	mux.HandleFunc("/create-table", server.handleCreateTable)

	return server
}

func (s *Server) Start() error {
	return http.ListenAndServe(s.config.Address(), s.mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status: "ok",
		Node:   s.config.Role,
	})
}

func (s *Server) handleDBHealth(w http.ResponseWriter, _ *http.Request) {
	if err := s.store.Ping(); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	writeJSON(w, http.StatusOK, dbHealthResponse{
		Status: "ok",
		Node:   s.config.Role,
		DBName: s.config.DBName,
	})
}

func (s *Server) handleCreateTable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request models.CreateTableRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if err := s.store.CreateTable(request.Table, request.Columns); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, models.MessageResponse{Message: "table created"})
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
