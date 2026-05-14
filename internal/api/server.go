package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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
	masterHTTP  *http.Client
	approvalMu  sync.Mutex
	approvals   map[string]*approvalRecord
	approvalSeq int64
	roleMu      sync.RWMutex
	role        string
}

type approvalRecord struct {
	ID          string
	Method      string
	Path        string
	RawQuery    string
	Body        []byte
	ContentType string
	RequestedBy string
	RequestRole string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DecisionBy  string
	Reason      string
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
		masterHTTP:  &http.Client{Timeout: 10 * time.Second},
		approvals:   make(map[string]*approvalRecord),
		role:        cfg.Role,
	}

	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/db/health", server.handleDBHealth)
	mux.HandleFunc("/cluster/status", server.handleClusterStatus)
	mux.HandleFunc("/approval-requests", server.handleApprovalRequests)
	mux.HandleFunc("/approval-requests/", server.handleApprovalDecision)
	mux.HandleFunc("/replication/retry", server.handleReplicationRetry)
	mux.HandleFunc("/promote", server.handlePromote)
	mux.HandleFunc("/create-table", server.handleCreateTable)
	mux.HandleFunc("/drop-table", server.handleDropTable)
	mux.HandleFunc("/drop-database", server.handleDropDatabase)
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
	if s.submitApprovalRequest(w, r) {
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
	if s.submitApprovalRequest(w, r) {
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

func (s *Server) handleDropTable(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodDelete) {
		return
	}
	if s.submitApprovalRequest(w, r) {
		return
	}

	var request models.DropTableRequest
	if !decodeJSON(w, r, &request) {
		return
	}

	query, err := s.store.DropTable(request.Table)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, writeResponse{
		Message:     "table dropped",
		Replication: s.broadcastIfMaster(query),
	})
}

func (s *Server) handleDropDatabase(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodDelete) {
		return
	}
	if s.submitApprovalRequest(w, r) {
		return
	}

	query, err := s.store.DropDatabase(s.config.DBName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, writeResponse{
		Message:     "database dropped",
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
	if s.submitApprovalRequest(w, r) {
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
	allowedPrefixes := []string{"CREATE TABLE", "INSERT", "UPDATE", "DELETE", "DROP TABLE", "DROP DATABASE"}
	for _, prefix := range allowedPrefixes {
		if hasSQLPrefix(query, prefix) {
			return true
		}
	}

	return false
}

func (s *Server) handleApprovalRequests(w http.ResponseWriter, r *http.Request) {
	if s.currentRole() != "master" {
		http.Error(w, "approval requests are only managed on master", http.StatusForbidden)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.listApprovalRequests(w, r)
	case http.MethodPost:
		s.createApprovalRequest(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleApprovalDecision(w http.ResponseWriter, r *http.Request) {
	if s.currentRole() != "master" {
		http.Error(w, "approval decisions are only available on master", http.StatusForbidden)
		return
	}
	if !requireMethod(w, r, http.MethodPut) {
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/approval-requests/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	var decision models.ApprovalDecisionRequest
	if r.ContentLength != 0 {
		if !decodeJSON(w, r, &decision) {
			return
		}
	}

	switch parts[1] {
	case "approve":
		s.approveRequest(w, parts[0], decision.Reason)
	case "reject":
		s.rejectRequest(w, parts[0], decision.Reason)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) listApprovalRequests(w http.ResponseWriter, r *http.Request) {
	statusFilter := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("status")))
	if statusFilter == "" {
		statusFilter = "pending"
	}

	s.approvalMu.Lock()
	requests := make([]models.ApprovalRequest, 0, len(s.approvals))
	for _, record := range s.approvals {
		if statusFilter != "all" && strings.ToLower(record.Status) != statusFilter {
			continue
		}
		requests = append(requests, record.toModel())
	}
	s.approvalMu.Unlock()

	sort.Slice(requests, func(i, j int) bool {
		if requests[i].CreatedAt.Equal(requests[j].CreatedAt) {
			return requests[i].ID < requests[j].ID
		}
		return requests[i].CreatedAt.Before(requests[j].CreatedAt)
	})

	writeJSON(w, http.StatusOK, models.ApprovalListResponse{
		Node:     s.currentRole(),
		Requests: requests,
	})
}

func (s *Server) createApprovalRequest(w http.ResponseWriter, r *http.Request) {
	var submission models.ApprovalSubmissionRequest
	if !decodeJSON(w, r, &submission) {
		return
	}
	if submission.Method == "" || submission.Path == "" || submission.RequestedBy == "" {
		http.Error(w, "method, path, and requestedBy are required", http.StatusBadRequest)
		return
	}
	if !isMasterManagedPath(submission.Path) {
		http.Error(w, "path is not eligible for master approval", http.StatusBadRequest)
		return
	}

	now := time.Now()
	record := &approvalRecord{
		ID:          s.nextApprovalID(),
		Method:      submission.Method,
		Path:        submission.Path,
		RawQuery:    submission.RawQuery,
		Body:        append([]byte(nil), submission.Body...),
		ContentType: submission.ContentType,
		RequestedBy: submission.RequestedBy,
		RequestRole: submission.RequestRole,
		Status:      "pending",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	s.approvalMu.Lock()
	s.approvals[record.ID] = record
	s.approvalMu.Unlock()

	writeJSON(w, http.StatusAccepted, models.ApprovalRequest{
		ID:          record.ID,
		Method:      record.Method,
		Path:        record.Path,
		RawQuery:    record.RawQuery,
		Body:        string(record.Body),
		ContentType: record.ContentType,
		RequestedBy: record.RequestedBy,
		RequestRole: record.RequestRole,
		Status:      record.Status,
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	})
}

func (s *Server) approveRequest(w http.ResponseWriter, id, reason string) {
	record, ok := s.getApprovalRecord(id)
	if !ok {
		http.Error(w, "approval request not found", http.StatusNotFound)
		return
	}
	if record.Status != "pending" {
		http.Error(w, "approval request is not pending", http.StatusConflict)
		return
	}

	response := httptest.NewRecorder()
	req := httptest.NewRequest(record.Method, record.Path, bytes.NewReader(record.Body))
	req.Header.Set("Content-Type", record.ContentType)
	if record.RawQuery != "" {
		req.URL.RawQuery = record.RawQuery
		req.RequestURI = record.Path + "?" + record.RawQuery
	}

	s.mux.ServeHTTP(response, req)
	result := response.Result()
	defer result.Body.Close()
	body, _ := io.ReadAll(result.Body)

	s.approvalMu.Lock()
	defer s.approvalMu.Unlock()
	current := s.approvals[id]
	if current == nil {
		http.Error(w, "approval request not found", http.StatusNotFound)
		return
	}
	if current.Status != "pending" {
		http.Error(w, "approval request is not pending", http.StatusConflict)
		return
	}

	current.UpdatedAt = time.Now()
	current.DecisionBy = s.currentRole()
	if reason != "" {
		current.Reason = reason
	}

	if result.StatusCode >= 200 && result.StatusCode < 300 {
		current.Status = "approved"
		writeJSON(w, http.StatusOK, map[string]any{
			"message":  "request approved and executed",
			"request":  current.toModel(),
			"response": parseResponseBody(body),
		})
		return
	}

	current.Status = "rejected"
	if current.Reason == "" {
		current.Reason = strings.TrimSpace(string(body))
	}
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"message":  "request approval failed during execution",
		"request":  current.toModel(),
		"response": parseResponseBody(body),
	})
}

func (s *Server) rejectRequest(w http.ResponseWriter, id, reason string) {
	s.approvalMu.Lock()
	defer s.approvalMu.Unlock()

	record := s.approvals[id]
	if record == nil {
		http.Error(w, "approval request not found", http.StatusNotFound)
		return
	}
	if record.Status != "pending" {
		http.Error(w, "approval request is not pending", http.StatusConflict)
		return
	}

	record.Status = "rejected"
	record.UpdatedAt = time.Now()
	record.DecisionBy = s.currentRole()
	record.Reason = reason
	if record.Reason == "" {
		record.Reason = "rejected by master"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "request rejected",
		"request": record.toModel(),
	})
}

func (s *Server) submitApprovalRequest(w http.ResponseWriter, r *http.Request) bool {
	if s.currentRole() == "master" {
		return false
	}
	if s.config.MasterURL == "" {
		http.Error(w, "master URL is not configured for this slave", http.StatusServiceUnavailable)
		return true
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return true
	}
	defer r.Body.Close()

	payload, err := json.Marshal(models.ApprovalSubmissionRequest{
		Method:      r.Method,
		Path:        r.URL.Path,
		RawQuery:    r.URL.RawQuery,
		Body:        body,
		ContentType: r.Header.Get("Content-Type"),
		RequestedBy: s.config.Address(),
		RequestRole: s.currentRole(),
	})
	if err != nil {
		http.Error(w, "failed to create approval payload", http.StatusInternalServerError)
		return true
	}

	masterURL := strings.TrimRight(s.config.MasterURL, "/") + "/approval-requests"
	forwardReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, masterURL, bytes.NewReader(payload))
	if err != nil {
		http.Error(w, "failed to create approval request", http.StatusInternalServerError)
		return true
	}
	forwardReq.Header.Set("Content-Type", "application/json")

	response, err := s.masterHTTP.Do(forwardReq)
	if err != nil {
		http.Error(w, "failed to contact master: "+err.Error(), http.StatusBadGateway)
		return true
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		http.Error(w, "failed to read master response", http.StatusBadGateway)
		return true
	}

	if contentType := response.Header.Get("Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("X-Master-Decision", "pending")
	w.Header().Set("X-Master-Node", strings.TrimRight(s.config.MasterURL, "/"))
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(responseBody)
	return true
}

func (s *Server) nextApprovalID() string {
	s.approvalMu.Lock()
	defer s.approvalMu.Unlock()
	s.approvalSeq++
	return "approval-" + strconv.FormatInt(s.approvalSeq, 10)
}

func (s *Server) getApprovalRecord(id string) (*approvalRecord, bool) {
	s.approvalMu.Lock()
	defer s.approvalMu.Unlock()

	record := s.approvals[id]
	if record == nil {
		return nil, false
	}

	copyRecord := *record
	copyRecord.Body = append([]byte(nil), record.Body...)
	return &copyRecord, true
}

func (r *approvalRecord) toModel() models.ApprovalRequest {
	return models.ApprovalRequest{
		ID:          r.ID,
		Method:      r.Method,
		Path:        r.Path,
		RawQuery:    r.RawQuery,
		Body:        string(r.Body),
		ContentType: r.ContentType,
		RequestedBy: r.RequestedBy,
		RequestRole: r.RequestRole,
		Status:      r.Status,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
		DecisionBy:  r.DecisionBy,
		Reason:      r.Reason,
	}
}

func isMasterManagedPath(path string) bool {
	allowed := map[string]bool{
		"/replication/retry": true,
		"/create-table":      true,
		"/drop-table":        true,
		"/drop-database":     true,
		"/insert":            true,
		"/update":            true,
		"/delete":            true,
	}
	return allowed[path]
}

func parseResponseBody(body []byte) any {
	if len(body) == 0 {
		return map[string]any{}
	}

	var decoded any
	if err := json.Unmarshal(body, &decoded); err == nil {
		return decoded
	}

	return strings.TrimSpace(string(body))
}
