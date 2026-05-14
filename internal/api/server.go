package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"distributed-db/internal/config"
	"distributed-db/internal/models"
	"distributed-db/internal/replication"
	"distributed-db/internal/storage"
)

type Server struct {
	config      config.NodeConfig
	mux         *http.ServeMux
	store       *storage.Store
	broadcaster *replication.Broadcaster
	roleMu      sync.RWMutex
	role        string
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

type writeResponse struct {
	Message     string                       `json:"message"`
	Replication []models.ReplicationResponse `json:"replication,omitempty"`
}

func NewServer(cfg config.NodeConfig, store *storage.Store, broadcaster *replication.Broadcaster) *Server {
	mux := http.NewServeMux()
	server := &Server{
		config:      cfg,
		mux:         mux,
		store:       store,
		broadcaster: broadcaster,
		role:        cfg.Role,
	}

	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/db/health", server.handleDBHealth)
	mux.HandleFunc("/cluster/status", server.handleClusterStatus)
	mux.HandleFunc("/replication/retry", server.handleReplicationRetry)
	mux.HandleFunc("/promote", server.handlePromote)
	mux.HandleFunc("/create-table", server.handleCreateTable)
	mux.HandleFunc("/insert", server.handleInsert)
	mux.HandleFunc("/update", server.handleUpdate)
	mux.HandleFunc("/delete", server.handleDelete)
	mux.HandleFunc("/select", server.handleSelect)
	mux.HandleFunc("/replicate", server.handleReplicate)

	return server
}

func (s *Server) Start() error {
	return http.ListenAndServe(s.config.Address(), s.mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status: "ok",
		Node:   s.currentRole(),
	})
}

func (s *Server) handleDBHealth(w http.ResponseWriter, _ *http.Request) {
	if err := s.store.Ping(); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	writeJSON(w, http.StatusOK, dbHealthResponse{
		Status: "ok",
		Node:   s.currentRole(),
		DBName: s.config.DBName,
	})
}

func (s *Server) handleClusterStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if s.broadcaster == nil {
		writeJSON(w, http.StatusOK, models.ClusterStatusResponse{Node: s.currentRole()})
		return
	}

	writeJSON(w, http.StatusOK, models.ClusterStatusResponse{
		Node:   s.currentRole(),
		Slaves: s.broadcaster.Status(),
	})
}

func (s *Server) handleReplicationRetry(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if s.broadcaster == nil {
		http.Error(w, "replication retry is only available on a master with slaves", http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, writeResponse{
		Message:     "retry completed",
		Replication: s.broadcaster.RetryPending(),
	})
}

func (s *Server) handlePromote(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPut) {
		return
	}

	s.setRole("master")
	writeJSON(w, http.StatusOK, models.MessageResponse{Message: "node promoted to master"})
}

func (s *Server) handleCreateTable(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if s.currentRole() != "master" {
		http.Error(w, "table creation is only allowed on master", http.StatusForbidden)
		return
	}

	var request models.CreateTableRequest
	if !decodeJSON(w, r, &request) {
		return
	}

	query, err := s.store.CreateTable(request.Table, request.Columns)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, writeResponse{
		Message:     "table created",
		Replication: s.broadcastIfMaster(query),
	})
}

func (s *Server) handleInsert(w http.ResponseWriter, r *http.Request) {
	s.handleWriteQuery(w, r, http.MethodPost, "INSERT", "row inserted")
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	s.handleWriteQuery(w, r, http.MethodPut, "UPDATE", "row updated")
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	s.handleWriteQuery(w, r, http.MethodDelete, "DELETE", "row deleted")
}

func (s *Server) handleSelect(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		var request models.QueryRequest
		if !decodeJSON(w, r, &request) {
			return
		}
		query = request.Query
	}

	if !hasSQLPrefix(query, "SELECT") {
		http.Error(w, "only SELECT queries are allowed", http.StatusBadRequest)
		return
	}

	rows, err := s.store.Select(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, models.SelectResponse{Rows: rows})
}

func (s *Server) handleReplicate(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if s.currentRole() == "master" {
		http.Error(w, "master does not accept replication writes", http.StatusForbidden)
		return
	}

	var request models.QueryRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if !isReplicationQuery(request.Query) {
		http.Error(w, "query is not allowed for replication", http.StatusBadRequest)
		return
	}

	if err := s.store.Exec(request.Query); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, models.MessageResponse{Message: "replication applied"})
}

func (s *Server) handleWriteQuery(w http.ResponseWriter, r *http.Request, method, prefix, message string) {
	if !requireMethod(w, r, method) {
		return
	}
	if s.currentRole() != "master" {
		http.Error(w, "writes are only allowed on master", http.StatusForbidden)
		return
	}

	var request models.QueryRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if !hasSQLPrefix(request.Query, prefix) {
		http.Error(w, fmt.Sprintf("only %s queries are allowed", prefix), http.StatusBadRequest)
		return
	}

	if err := s.store.Exec(request.Query); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, writeResponse{
		Message:     message,
		Replication: s.broadcastIfMaster(request.Query),
	})
}

func (s *Server) broadcastIfMaster(query string) []models.ReplicationResponse {
	if s.currentRole() != "master" || s.broadcaster == nil {
		return nil
	}

	return s.broadcaster.Broadcast(query)
}

func (s *Server) currentRole() string {
	s.roleMu.RLock()
	defer s.roleMu.RUnlock()
	return s.role
}

func (s *Server) setRole(role string) {
	s.roleMu.Lock()
	defer s.roleMu.Unlock()
	s.role = role
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}

	return true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(value); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return false
	}

	return true
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func hasSQLPrefix(query, prefix string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query)), prefix)
}

func isReplicationQuery(query string) bool {
	allowedPrefixes := []string{"CREATE TABLE", "INSERT", "UPDATE", "DELETE"}
	for _, prefix := range allowedPrefixes {
		if hasSQLPrefix(query, prefix) {
			return true
		}
	}

	return false
}
