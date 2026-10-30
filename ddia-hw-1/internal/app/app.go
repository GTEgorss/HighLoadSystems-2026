package app

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"highload/ddia-hw-1/internal/metrics"
)

type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type store struct {
	mu    sync.Mutex
	items map[string]Item
	next  int
}

func New(m *metrics.Metrics) http.Handler {
	mux := http.NewServeMux()
	s := &store{items: map[string]Item{"1": {ID: "1", Name: "example"}}, next: 1}
	m.Handle(mux, "GET", "/items/{id}", s.get)
	m.Handle(mux, "POST", "/items", s.create)
	m.Handle(mux, "PUT", "/items/{id}", s.update)
	mux.Handle("GET /metrics", m.Handler())
	mux.HandleFunc("GET /endpoints", m.Endpoints)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	return mux
}

func (s *store) get(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	item, ok := s.items[r.PathValue("id")]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "item not found", 404)
		return
	}
	writeJSON(w, 200, item)
}

func readName(w http.ResponseWriter, r *http.Request) (string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		http.Error(w, "expected a non-empty name", 400)
		return "", false
	}
	return body.Name, true
}

func (s *store) create(w http.ResponseWriter, r *http.Request) {
	name, ok := readName(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	if len(s.items) >= 10000 {
		s.mu.Unlock()
		http.Error(w, "store is full", 409)
		return
	}
	s.next++
	item := Item{ID: strconv.Itoa(s.next), Name: name}
	s.items[item.ID] = item
	s.mu.Unlock()
	writeJSON(w, 201, item)
}

func (s *store) update(w http.ResponseWriter, r *http.Request) {
	name, ok := readName(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	s.mu.Lock()
	_, exists := s.items[id]
	item := Item{ID: id, Name: name}
	if exists {
		s.items[id] = item
	}
	s.mu.Unlock()
	if !exists {
		http.Error(w, "item not found", 404)
		return
	}
	writeJSON(w, 200, item)
}

func writeJSON(w http.ResponseWriter, status int, item Item) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(item)
}
