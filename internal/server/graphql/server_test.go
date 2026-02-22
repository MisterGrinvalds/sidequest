package graphql

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MisterGrinvalds/sidequest/internal/store"
)

type gqlRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	s := store.New()
	srv, err := New(0, s)
	if err != nil {
		t.Fatalf("creating server: %v", err)
	}
	return httptest.NewServer(srv.server.Handler), s
}

func doGraphQL(t *testing.T, ts *httptest.Server, query string) gqlResponse {
	t.Helper()
	body, _ := json.Marshal(gqlRequest{Query: query})
	resp, err := http.Post(ts.URL+"/graphql", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /graphql: %v", err)
	}
	defer resp.Body.Close()

	var result gqlResponse
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

func TestQueryItems(t *testing.T) {
	ts, s := newTestServer(t)
	defer ts.Close()

	s.Create(&store.Item{Name: "alpha"})
	s.Create(&store.Item{Name: "beta"})

	result := doGraphQL(t, ts, `{ items { items { id name version } total } }`)

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	var data struct {
		Items struct {
			Items []struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Version int    `json:"version"`
			} `json:"items"`
			Total int `json:"total"`
		} `json:"items"`
	}
	json.Unmarshal(result.Data, &data)

	if data.Items.Total != 2 {
		t.Errorf("Expected total 2, got %d", data.Items.Total)
	}
	if len(data.Items.Items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(data.Items.Items))
	}
}

func TestQueryItemByID(t *testing.T) {
	ts, s := newTestServer(t)
	defer ts.Close()

	item, _ := s.Create(&store.Item{Name: "findme"})

	result := doGraphQL(t, ts, `{ item(id: "`+item.ID+`") { id name version } }`)

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	var data struct {
		Item struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Version int    `json:"version"`
		} `json:"item"`
	}
	json.Unmarshal(result.Data, &data)

	if data.Item.Name != "findme" {
		t.Errorf("Expected name 'findme', got %q", data.Item.Name)
	}
}

func TestQueryItemNotFound(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	result := doGraphQL(t, ts, `{ item(id: "nonexistent") { id name } }`)

	// GraphQL returns null for not found (with errors).
	if len(result.Errors) == 0 {
		var data struct {
			Item *struct{} `json:"item"`
		}
		json.Unmarshal(result.Data, &data)
		if data.Item != nil {
			t.Error("Expected null item for nonexistent ID")
		}
	}
}

func TestMutationCreateItem(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	result := doGraphQL(t, ts, `mutation { createItem(name: "mutated") { id name version } }`)

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	var data struct {
		CreateItem struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Version int    `json:"version"`
		} `json:"createItem"`
	}
	json.Unmarshal(result.Data, &data)

	if data.CreateItem.Name != "mutated" {
		t.Errorf("Expected name 'mutated', got %q", data.CreateItem.Name)
	}
	if data.CreateItem.ID == "" {
		t.Error("Expected non-empty ID")
	}
	if data.CreateItem.Version != 1 {
		t.Errorf("Expected version 1, got %d", data.CreateItem.Version)
	}
}

func TestMutationCreateItemWithLabels(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	result := doGraphQL(t, ts, `mutation { createItem(name: "labeled", labels: "{\"env\":\"prod\"}") { id name labels { key value } } }`)

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	var data struct {
		CreateItem struct {
			Name   string `json:"name"`
			Labels []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"labels"`
		} `json:"createItem"`
	}
	json.Unmarshal(result.Data, &data)

	if len(data.CreateItem.Labels) != 1 {
		t.Errorf("Expected 1 label, got %d", len(data.CreateItem.Labels))
	}
}

func TestMutationUpdateItem(t *testing.T) {
	ts, s := newTestServer(t)
	defer ts.Close()

	item, _ := s.Create(&store.Item{Name: "original"})

	result := doGraphQL(t, ts, `mutation { updateItem(id: "`+item.ID+`", name: "updated") { name version } }`)

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	var data struct {
		UpdateItem struct {
			Name    string `json:"name"`
			Version int    `json:"version"`
		} `json:"updateItem"`
	}
	json.Unmarshal(result.Data, &data)

	if data.UpdateItem.Name != "updated" {
		t.Errorf("Expected 'updated', got %q", data.UpdateItem.Name)
	}
}

func TestMutationDeleteItem(t *testing.T) {
	ts, s := newTestServer(t)
	defer ts.Close()

	item, _ := s.Create(&store.Item{Name: "deleteme"})

	result := doGraphQL(t, ts, `mutation { deleteItem(id: "`+item.ID+`") }`)

	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	// Verify deleted.
	if s.Count() != 0 {
		t.Errorf("Expected 0 items after delete, got %d", s.Count())
	}
}

func TestQueryWithPagination(t *testing.T) {
	ts, s := newTestServer(t)
	defer ts.Close()

	for i := 0; i < 15; i++ {
		s.Create(&store.Item{Name: "item"})
	}

	result := doGraphQL(t, ts, `{ items(page: 2, limit: 5) { items { id } total page limit pages } }`)

	var data struct {
		Items struct {
			Items []struct{ ID string } `json:"items"`
			Total int                   `json:"total"`
			Page  int                   `json:"page"`
			Limit int                   `json:"limit"`
			Pages int                   `json:"pages"`
		} `json:"items"`
	}
	json.Unmarshal(result.Data, &data)

	if data.Items.Total != 15 {
		t.Errorf("Expected total 15, got %d", data.Items.Total)
	}
	if len(data.Items.Items) != 5 {
		t.Errorf("Expected 5 items on page 2, got %d", len(data.Items.Items))
	}
}

func TestHealthEndpoint(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}

func TestPlaygroundEndpoint(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/playground")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}
