package watch

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

//go:embed web/index.html
var pageFS embed.FS

// keepalive keeps proxies and idle-connection reapers from closing a quiet
// stream. A run can sit silent for minutes while one agent thinks.
const keepalive = 15 * time.Second

type Server struct {
	sess  Session
	graph *Graph
	hub   *Hub
}

func NewServer(sess Session, g *Graph, h *Hub) *Server {
	return &Server{sess: sess, graph: g, hub: h}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.page)
	mux.HandleFunc("GET /events", s.events)
	mux.HandleFunc("GET /session", s.session)
	return mux
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	b, err := pageFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "page missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id":    s.sess.ID,
		"repo":  s.sess.Repo,
		"lead":  s.sess.LeadLabel,
		"crew":  s.sess.SubagentDir,
		"trail": s.sess.LeadPath,
	})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Subscribe before snapshotting: a delta landing between the two would
	// otherwise be lost. The reverse order only risks a duplicate, and every
	// message is an idempotent upsert.
	ch, unsubscribe := s.hub.Subscribe()
	defer unsubscribe()

	for _, m := range s.graph.Snapshot() {
		if b, err := json.Marshal(m); err == nil {
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
	}
	flusher.Flush()

	tick := time.NewTicker(keepalive)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case b, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		case <-tick.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
