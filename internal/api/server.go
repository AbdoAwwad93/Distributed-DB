package api
import (
	"net/http"
	"distributed-db/internal/config"
	"encoding/json"
)

type Server struct{
	config config.NodeConfig
	mux  *http.ServeMux
}
type healthResponse struct{
	Status string 
	Node string 
}
func NewServer(cfg config.NodeConfig) *Server{
	mux := http.NewServeMux();
	server:= &Server{
		config: cfg,
		mux: mux,
	}
	mux.HandleFunc("/health",server.handleHealth)
	return  server;
}
func (s *Server) Start() error{
	return http.ListenAndServe(s.config.Address(),s.mux);
}

func (s *Server) handleHealth(w http.ResponseWriter,_ *http.Request){
	w.Header().Set("Content-Type","application/json")
	response:= healthResponse{
		Status: "ok",
		Node: s.config.Role,
	}
	if err:= json.NewEncoder(w).Encode(response); err!= nil{
		http.Error(w,err.Error(),http.StatusInternalServerError);
	}
}