package api

import (
	"encoding/json"
	"net/http"

	"distributed-db/internal/config"
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

	return server
}

func (s *Server) Start() error {
	return http.ListenAndServe(s.config.Address(), s.mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	response := healthResponse{
		Status: "ok",
		Node:   s.config.Role,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleDBHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := s.store.Ping(); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	response := dbHealthResponse{
		Status: "ok",
		Node:   s.config.Role,
		DBName: s.config.DBName,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
