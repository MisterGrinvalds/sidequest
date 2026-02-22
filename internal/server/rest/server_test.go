package rest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MisterGrinvalds/sidequest/internal/store"
)

func newTestServer() (*httptest.Server, *store.Store) {
	s := store.New()
	srv := New(0, s)
	return httptest.NewServer(srv.server.Handler), s
}

func TestCreateItem(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/items",
		"application/json",
		strings.NewReader(`{"name":"test","labels":{"env":"prod"},"data":{"key":"val"}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		t.Fatalf("Expected 201, got %d", resp.StatusCode)
	}

	// Check Location header.
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/api/v1/items/") {
		t.Errorf("Expected Location header, got %q", loc)
	}

	// Check ETag header.
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Error("Expected ETag header")
	}

	var item store.Item
	json.NewDecoder(resp.Body).Decode(&item)
	if item.ID == "" {
		t.Error("Expected non-empty ID")
	}
	if item.Name != "test" {
		t.Errorf("Expected name 'test', got %q", item.Name)
	}
	if item.Version != 1 {
		t.Errorf("Expected version 1, got %d", item.Version)
	}
}

func TestCreateItemValidation(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	// Missing name.
	resp, _ := http.Post(ts.URL+"/api/v1/items", "application/json", strings.NewReader(`{"labels":{}}`))
	if resp.StatusCode != 400 {
		t.Errorf("Expected 400 for missing name, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Empty body.
	resp, _ = http.Post(ts.URL+"/api/v1/items", "application/json", strings.NewReader(""))
	if resp.StatusCode != 400 {
		t.Errorf("Expected 400 for empty body, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestGetItem(t *testing.T) {
	ts, s := newTestServer()
	defer ts.Close()

	item, _ := s.Create(&store.Item{Name: "getme"})

	resp, err := http.Get(ts.URL + "/api/v1/items/" + item.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Error("Expected ETag header")
	}

	var got store.Item
	json.NewDecoder(resp.Body).Decode(&got)
	if got.Name != "getme" {
		t.Errorf("Expected name 'getme', got %q", got.Name)
	}
}

func TestGetItemNotFound(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/api/v1/items/nonexistent")
	if resp.StatusCode != 404 {
		t.Errorf("Expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestGetItemConditional304(t *testing.T) {
	ts, s := newTestServer()
	defer ts.Close()

	item, _ := s.Create(&store.Item{Name: "conditional"})

	// First request to get ETag.
	resp, _ := http.Get(ts.URL + "/api/v1/items/" + item.ID)
	etag := resp.Header.Get("ETag")
	resp.Body.Close()

	// Second request with If-None-Match.
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/items/"+item.ID, nil)
	req.Header.Set("If-None-Match", etag)
	resp, _ = http.DefaultClient.Do(req)

	if resp.StatusCode != 304 {
		t.Errorf("Expected 304, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestListItems(t *testing.T) {
	ts, s := newTestServer()
	defer ts.Close()

	for i := 0; i < 25; i++ {
		s.Create(&store.Item{Name: "item"})
	}

	resp, _ := http.Get(ts.URL + "/api/v1/items?page=1&limit=10")
	defer resp.Body.Close()

	var result store.ListResult
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Total != 25 {
		t.Errorf("Expected total 25, got %d", result.Total)
	}
	if len(result.Items) != 10 {
		t.Errorf("Expected 10 items, got %d", len(result.Items))
	}
	if result.Pages != 3 {
		t.Errorf("Expected 3 pages, got %d", result.Pages)
	}
}

func TestListItemsLabelFilter(t *testing.T) {
	ts, s := newTestServer()
	defer ts.Close()

	s.Create(&store.Item{Name: "prod", Labels: map[string]string{"env": "prod"}})
	s.Create(&store.Item{Name: "dev", Labels: map[string]string{"env": "dev"}})

	resp, _ := http.Get(ts.URL + "/api/v1/items?label.env=prod")
	defer resp.Body.Close()

	var result store.ListResult
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Total != 1 {
		t.Errorf("Expected 1 prod item, got %d", result.Total)
	}
}

func TestListItemsSort(t *testing.T) {
	ts, s := newTestServer()
	defer ts.Close()

	s.Create(&store.Item{Name: "banana"})
	s.Create(&store.Item{Name: "apple"})

	resp, _ := http.Get(ts.URL + "/api/v1/items?sort=name")
	defer resp.Body.Close()

	var result store.ListResult
	json.NewDecoder(resp.Body).Decode(&result)

	if len(result.Items) < 2 {
		t.Fatal("Expected at least 2 items")
	}
	if result.Items[0].Name != "apple" {
		t.Errorf("Expected first item 'apple', got %q", result.Items[0].Name)
	}
}

func TestUpdateItem(t *testing.T) {
	ts, s := newTestServer()
	defer ts.Close()

	item, _ := s.Create(&store.Item{Name: "original"})

	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/items/"+item.ID,
		strings.NewReader(`{"name":"updated"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	var updated store.Item
	json.NewDecoder(resp.Body).Decode(&updated)
	if updated.Name != "updated" {
		t.Errorf("Expected name 'updated', got %q", updated.Name)
	}
	if updated.Version != 2 {
		t.Errorf("Expected version 2, got %d", updated.Version)
	}
}

func TestUpdateItemNotFound(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/items/nonexistent",
		strings.NewReader(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)

	if resp.StatusCode != 404 {
		t.Errorf("Expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestUpdateItemIfMatch(t *testing.T) {
	ts, s := newTestServer()
	defer ts.Close()

	item, _ := s.Create(&store.Item{Name: "etag-test"})

	// Get current ETag.
	getResp, _ := http.Get(ts.URL + "/api/v1/items/" + item.ID)
	etag := getResp.Header.Get("ETag")
	getResp.Body.Close()

	// Update with correct ETag.
	req, _ := http.NewRequest("PUT", ts.URL+"/api/v1/items/"+item.ID,
		strings.NewReader(`{"name":"etag-updated"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", etag)
	resp, _ := http.DefaultClient.Do(req)

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200 with correct ETag, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update with stale ETag (should fail).
	req, _ = http.NewRequest("PUT", ts.URL+"/api/v1/items/"+item.ID,
		strings.NewReader(`{"name":"should-fail"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", etag) // Stale ETag.
	resp, _ = http.DefaultClient.Do(req)

	if resp.StatusCode != 412 {
		t.Errorf("Expected 412 with stale ETag, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestUpsertItemCreate(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	req, _ := http.NewRequest("PATCH", ts.URL+"/api/v1/items/new-id",
		strings.NewReader(`{"name":"upserted"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		t.Fatalf("Expected 201 for upsert-create, got %d", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if loc != "/api/v1/items/new-id" {
		t.Errorf("Expected Location /api/v1/items/new-id, got %q", loc)
	}
}

func TestUpsertItemUpdate(t *testing.T) {
	ts, s := newTestServer()
	defer ts.Close()

	s.Create(&store.Item{ID: "existing", Name: "original"})

	req, _ := http.NewRequest("PATCH", ts.URL+"/api/v1/items/existing",
		strings.NewReader(`{"name":"merged"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Expected 200 for upsert-update, got %d", resp.StatusCode)
	}
}

func TestDeleteItem(t *testing.T) {
	ts, s := newTestServer()
	defer ts.Close()

	item, _ := s.Create(&store.Item{Name: "deleteme"})

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/items/"+item.ID, nil)
	resp, _ := http.DefaultClient.Do(req)

	if resp.StatusCode != 204 {
		t.Errorf("Expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify deleted.
	resp, _ = http.Get(ts.URL + "/api/v1/items/" + item.ID)
	if resp.StatusCode != 404 {
		t.Errorf("Expected 404 after delete, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDeleteItemNotFound(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	req, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/items/nonexistent", nil)
	resp, _ := http.DefaultClient.Do(req)

	if resp.StatusCode != 404 {
		t.Errorf("Expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestMethodNotAllowed(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	// PATCH on /api/v1/items (collection) is not allowed.
	req, _ := http.NewRequest("PATCH", ts.URL+"/api/v1/items", nil)
	resp, _ := http.DefaultClient.Do(req)

	if resp.StatusCode != 405 {
		t.Errorf("Expected 405, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestErrorResponseFormat(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/api/v1/items/nonexistent")
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(body, &errResp)

	if errResp.Error.Code != "NOT_FOUND" {
		t.Errorf("Expected error code NOT_FOUND, got %q", errResp.Error.Code)
	}
	if errResp.Error.Message == "" {
		t.Error("Expected non-empty error message")
	}
}

func TestHealthEndpoint(t *testing.T) {
	ts, _ := newTestServer()
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/health")
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}
