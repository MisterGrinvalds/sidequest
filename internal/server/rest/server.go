package rest

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/MisterGrinvalds/sidequest/internal/store"
)

// Server is a REST API server with full CRUD on items.
type Server struct {
	port   int
	store  *store.Store
	server *http.Server
}

// New creates a new REST API server.
func New(port int, s *store.Store) *Server {
	srv := &Server{port: port, store: s}
	mux := http.NewServeMux()

	// Routes: /api/v1/items and /api/v1/items/{id}
	mux.HandleFunc("/api/v1/items", srv.handleItems)
	mux.HandleFunc("/api/v1/items/", srv.handleItemByID)
	mux.HandleFunc("/health", srv.handleHealth)

	srv.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv
}

// Port returns the configured port.
func (s *Server) Port() int { return s.port }

// Start begins listening.
func (s *Server) Start() error { return s.server.ListenAndServe() }

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error { return s.server.Shutdown(ctx) }

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleItems(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listItems(w, r)
	case http.MethodPost:
		s.createItem(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Use GET or POST on /api/v1/items")
	}
}

func (s *Server) handleItemByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/items/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "MISSING_ID", "Item ID is required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getItem(w, r, id)
	case http.MethodPut:
		s.updateItem(w, r, id)
	case http.MethodPatch:
		s.upsertItem(w, r, id)
	case http.MethodDelete:
		s.deleteItem(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Use GET, PUT, PATCH, or DELETE")
	}
}

func (s *Server) listItems(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	sortField := q.Get("sort")

	// Parse label filters: ?label.env=prod&label.tier=frontend
	labels := make(map[string]string)
	for k, v := range q {
		if strings.HasPrefix(k, "label.") {
			labels[strings.TrimPrefix(k, "label.")] = v[0]
		}
	}

	result := s.store.List(store.ListOptions{
		Page:   page,
		Limit:  limit,
		Sort:   sortField,
		Labels: labels,
	})

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getItem(w http.ResponseWriter, r *http.Request, id string) {
	item, err := s.store.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	etag := computeETag(item)
	w.Header().Set("ETag", etag)

	// Conditional: If-None-Match
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	writeJSON(w, http.StatusOK, item)
}

func (s *Server) createItem(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels,omitempty"`
		Data   json.RawMessage   `json:"data,omitempty"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Field 'name' is required")
		return
	}

	item, err := s.store.Create(&store.Item{
		Name:   input.Name,
		Labels: input.Labels,
		Data:   input.Data,
	})
	if err != nil {
		writeError(w, http.StatusConflict, "CONFLICT", err.Error())
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/v1/items/%s", item.ID))
	w.Header().Set("ETag", computeETag(item))
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) updateItem(w http.ResponseWriter, r *http.Request, id string) {
	var input struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels,omitempty"`
		Data   json.RawMessage   `json:"data,omitempty"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	// Conditional: If-Match
	if match := r.Header.Get("If-Match"); match != "" {
		existing, err := s.store.Get(id)
		if err != nil {
			writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		if computeETag(existing) != match {
			writeError(w, http.StatusPreconditionFailed, "PRECONDITION_FAILED", "ETag mismatch")
			return
		}
	}

	item, err := s.store.Update(id, &store.Item{
		Name:   input.Name,
		Labels: input.Labels,
		Data:   input.Data,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	w.Header().Set("ETag", computeETag(item))
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) upsertItem(w http.ResponseWriter, r *http.Request, id string) {
	var input struct {
		Name   string            `json:"name,omitempty"`
		Labels map[string]string `json:"labels,omitempty"`
		Data   json.RawMessage   `json:"data,omitempty"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	item, created, err := s.store.Upsert(id, &store.Item{
		Name:   input.Name,
		Labels: input.Labels,
		Data:   input.Data,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.Header().Set("ETag", computeETag(item))
	if created {
		w.Header().Set("Location", fmt.Sprintf("/api/v1/items/%s", item.ID))
		writeJSON(w, http.StatusCreated, item)
	} else {
		writeJSON(w, http.StatusOK, item)
	}
}

func (s *Server) deleteItem(w http.ResponseWriter, _ *http.Request, id string) {
	if err := s.store.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func readJSON(r *http.Request, v interface{}) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("reading body: %w", err)
	}
	defer r.Body.Close()
	if len(body) == 0 {
		return fmt.Errorf("empty request body")
	}
	return json.Unmarshal(body, v)
}

func computeETag(item *store.Item) string {
	data := fmt.Sprintf("%s-%d-%s", item.ID, item.Version, item.UpdatedAt.Format(time.RFC3339Nano))
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf(`"%x"`, hash[:8])
}
