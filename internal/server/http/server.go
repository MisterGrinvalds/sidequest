package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/MisterGrinvalds/sidequest/internal/server/ui"
)

// Server is an HTTP echo server for testing connectivity.
type Server struct {
	port        int
	server      *http.Server
	landingHTML []byte // pre-rendered landing page; nil = echo fallback
}

// New creates a new HTTP echo server on the given port.
// If landing is non-nil, "/" serves the rendered landing page instead of echo.
func New(port int, landing *ui.LandingData) *Server {
	s := &Server{port: port}

	// Pre-render the landing page if data was provided.
	if landing != nil {
		var buf bytes.Buffer
		if err := ui.RenderLanding(&buf, *landing); err == nil {
			s.landingHTML = buf.Bytes()
		}
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/echo", s.handleEcho)
	mux.HandleFunc("/echo/", s.handleEcho)
	mux.HandleFunc("/headers", s.handleHeaders)
	mux.HandleFunc("/ip", s.handleIP)
	mux.HandleFunc("/delay/", s.handleDelay)
	mux.HandleFunc("/status/", s.handleStatus)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.Handle("/static/", ui.StaticHandler())
	mux.HandleFunc("/", s.handleRoot)

	s.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s
}

// Port returns the configured port.
func (s *Server) Port() int {
	return s.port
}

// Start begins listening and serving. Blocks until the server stops.
func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// Close immediately stops the server.
func (s *Server) Close() error {
	return s.server.Close()
}

// handleRoot serves the landing page if available, otherwise falls back to echo.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" && s.landingHTML != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(s.landingHTML)
		return
	}
	s.handleEcho(w, r)
}

func (s *Server) handleEcho(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	defer r.Body.Close()

	resp := map[string]interface{}{
		"method":   r.Method,
		"path":     r.URL.Path,
		"query":    r.URL.Query(),
		"headers":  flattenHeaders(r.Header),
		"body":     string(body),
		"host":     r.Host,
		"remote":   r.RemoteAddr,
		"protocol": r.Proto,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleHeaders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, flattenHeaders(r.Header))
}

func (s *Server) handleIP(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	writeJSON(w, http.StatusOK, map[string]string{"ip": ip})
}

func (s *Server) handleDelay(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/delay/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "specify delay in seconds: /delay/5"})
		return
	}

	seconds, err := strconv.Atoi(parts[0])
	if err != nil || seconds < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid delay value"})
		return
	}
	if seconds > 60 {
		seconds = 60
	}

	time.Sleep(time.Duration(seconds) * time.Second)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"delayed": seconds,
		"unit":    "seconds",
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/status/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "specify status code: /status/404"})
		return
	}

	code, err := strconv.Atoi(parts[0])
	if err != nil || code < 100 || code > 599 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status code (100-599)"})
		return
	}

	writeJSON(w, code, map[string]interface{}{
		"status": code,
		"text":   http.StatusText(code),
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(data)
}

func flattenHeaders(h http.Header) map[string]string {
	flat := make(map[string]string, len(h))
	for k, v := range h {
		flat[k] = strings.Join(v, ", ")
	}
	return flat
}

func clientIP(r *http.Request) string {
	// Check X-Forwarded-For first.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-Ip"); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
