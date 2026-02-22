package http

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer() *httptest.Server {
	srv := New(0)
	// Extract the handler from our server.
	return httptest.NewServer(srv.server.Handler)
}

func TestEchoEndpoint(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	// GET with query params.
	resp, err := http.Get(ts.URL + "/echo?foo=bar&baz=qux")
	if err != nil {
		t.Fatalf("GET /echo failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["method"] != "GET" {
		t.Errorf("Expected method GET, got %v", result["method"])
	}
	if result["path"] != "/echo" {
		t.Errorf("Expected path /echo, got %v", result["path"])
	}

	query := result["query"].(map[string]interface{})
	if query["foo"] == nil {
		t.Error("Expected query param 'foo'")
	}
}

func TestEchoWithBody(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	body := `{"hello": "world"}`
	resp, err := http.Post(ts.URL+"/echo", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /echo failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["method"] != "POST" {
		t.Errorf("Expected method POST, got %v", result["method"])
	}
	if result["body"] != body {
		t.Errorf("Expected body %q, got %v", body, result["body"])
	}
}

func TestHeadersEndpoint(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/headers", nil)
	req.Header.Set("X-Custom", "test-value")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /headers failed: %v", err)
	}
	defer resp.Body.Close()

	var headers map[string]string
	json.NewDecoder(resp.Body).Decode(&headers)

	if headers["X-Custom"] != "test-value" {
		t.Errorf("Expected X-Custom=test-value, got %q", headers["X-Custom"])
	}
}

func TestIPEndpoint(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ip")
	if err != nil {
		t.Fatalf("GET /ip failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)

	if result["ip"] == "" {
		t.Error("Expected non-empty IP")
	}
}

func TestIPWithXForwardedFor(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/ip", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /ip failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)

	if result["ip"] != "1.2.3.4" {
		t.Errorf("Expected IP 1.2.3.4, got %q", result["ip"])
	}
}

func TestStatusEndpoint(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	tests := []struct {
		code     string
		expected int
	}{
		{"200", 200},
		{"404", 404},
		{"418", 418},
		{"500", 500},
	}

	for _, tt := range tests {
		resp, err := http.Get(ts.URL + "/status/" + tt.code)
		if err != nil {
			t.Fatalf("GET /status/%s failed: %v", tt.code, err)
		}
		resp.Body.Close()

		if resp.StatusCode != tt.expected {
			t.Errorf("/status/%s: expected %d, got %d", tt.code, tt.expected, resp.StatusCode)
		}
	}
}

func TestStatusEndpointInvalid(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/status/abc")
	if err != nil {
		t.Fatalf("GET /status/abc failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("Expected 400 for invalid status, got %d", resp.StatusCode)
	}
}

func TestStatusEndpointOutOfRange(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/status/999")
	if err != nil {
		t.Fatalf("GET /status/999 failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("Expected 400 for out-of-range status, got %d", resp.StatusCode)
	}
}

func TestDelayEndpoint(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	// Test with 0 second delay (fast).
	resp, err := http.Get(ts.URL + "/delay/0")
	if err != nil {
		t.Fatalf("GET /delay/0 failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["delayed"] != float64(0) {
		t.Errorf("Expected delayed=0, got %v", result["delayed"])
	}
}

func TestDelayEndpointInvalid(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/delay/abc")
	if err != nil {
		t.Fatalf("GET /delay/abc failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("Expected 400 for invalid delay, got %d", resp.StatusCode)
	}
}

func TestHealthEndpoint(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)

	if result["status"] != "ok" {
		t.Errorf("Expected status=ok, got %q", result["status"])
	}
}

func TestReadyEndpoint(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ready")
	if err != nil {
		t.Fatalf("GET /ready failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)

	if result["status"] != "ready" {
		t.Errorf("Expected status=ready, got %q", result["status"])
	}
}

func TestContentTypeJSON(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %q", ct)
	}
}

func TestRootFallsBackToEcho(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/anything")
	if err != nil {
		t.Fatalf("GET /anything failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)

	// Root handler falls back to echo.
	if result["method"] != "GET" {
		t.Errorf("Expected echo fallback, got %v", result)
	}
}
