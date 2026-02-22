package store

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestCreateAndGet(t *testing.T) {
	s := New()

	item, err := s.Create(&Item{
		Name:   "test-item",
		Labels: map[string]string{"env": "prod"},
		Data:   json.RawMessage(`{"key": "value"}`),
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if item.ID == "" {
		t.Fatal("Expected generated ID")
	}
	if item.Name != "test-item" {
		t.Fatalf("Expected name 'test-item', got %q", item.Name)
	}
	if item.Version != 1 {
		t.Fatalf("Expected version 1, got %d", item.Version)
	}
	if item.CreatedAt.IsZero() {
		t.Fatal("Expected non-zero CreatedAt")
	}

	got, err := s.Get(item.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Name != "test-item" {
		t.Fatalf("Expected name 'test-item', got %q", got.Name)
	}
}

func TestCreateDuplicate(t *testing.T) {
	s := New()

	_, err := s.Create(&Item{ID: "dup", Name: "first"})
	if err != nil {
		t.Fatalf("First create failed: %v", err)
	}

	_, err = s.Create(&Item{ID: "dup", Name: "second"})
	if err == nil {
		t.Fatal("Expected error on duplicate create")
	}
}

func TestGetNotFound(t *testing.T) {
	s := New()
	_, err := s.Get("nonexistent")
	if err == nil {
		t.Fatal("Expected error on get nonexistent")
	}
}

func TestUpdate(t *testing.T) {
	s := New()

	item, _ := s.Create(&Item{Name: "original"})

	updated, err := s.Update(item.ID, &Item{
		Name: "updated",
		Data: json.RawMessage(`{"new": true}`),
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.Name != "updated" {
		t.Fatalf("Expected name 'updated', got %q", updated.Name)
	}
	if updated.Version != 2 {
		t.Fatalf("Expected version 2, got %d", updated.Version)
	}
	if updated.CreatedAt != item.CreatedAt {
		t.Fatal("CreatedAt should not change on update")
	}
}

func TestUpdateNotFound(t *testing.T) {
	s := New()
	_, err := s.Update("nonexistent", &Item{Name: "x"})
	if err == nil {
		t.Fatal("Expected error on update nonexistent")
	}
}

func TestUpsertCreate(t *testing.T) {
	s := New()

	item, created, err := s.Upsert("new-id", &Item{Name: "upserted"})
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if !created {
		t.Fatal("Expected created=true for new item")
	}
	if item.ID != "new-id" {
		t.Fatalf("Expected ID 'new-id', got %q", item.ID)
	}
}

func TestUpsertUpdate(t *testing.T) {
	s := New()

	s.Create(&Item{ID: "existing", Name: "original", Labels: map[string]string{"env": "dev"}})

	item, created, err := s.Upsert("existing", &Item{
		Name:   "merged",
		Labels: map[string]string{"tier": "frontend"},
	})
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if created {
		t.Fatal("Expected created=false for existing item")
	}
	if item.Name != "merged" {
		t.Fatalf("Expected name 'merged', got %q", item.Name)
	}
	// Original label should be preserved, new one added.
	if item.Labels["env"] != "dev" {
		t.Fatal("Expected original label 'env=dev' to be preserved")
	}
	if item.Labels["tier"] != "frontend" {
		t.Fatal("Expected new label 'tier=frontend'")
	}
	if item.Version != 2 {
		t.Fatalf("Expected version 2, got %d", item.Version)
	}
}

func TestDelete(t *testing.T) {
	s := New()

	item, _ := s.Create(&Item{Name: "to-delete"})

	if err := s.Delete(item.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := s.Get(item.ID)
	if err == nil {
		t.Fatal("Expected error after delete")
	}

	if s.Count() != 0 {
		t.Fatalf("Expected count 0, got %d", s.Count())
	}
}

func TestDeleteNotFound(t *testing.T) {
	s := New()
	if err := s.Delete("nonexistent"); err == nil {
		t.Fatal("Expected error on delete nonexistent")
	}
}

func TestListPagination(t *testing.T) {
	s := New()

	for i := 0; i < 25; i++ {
		s.Create(&Item{Name: "item"})
	}

	result := s.List(ListOptions{Page: 1, Limit: 10})
	if result.Total != 25 {
		t.Fatalf("Expected total 25, got %d", result.Total)
	}
	if len(result.Items) != 10 {
		t.Fatalf("Expected 10 items on page 1, got %d", len(result.Items))
	}
	if result.Pages != 3 {
		t.Fatalf("Expected 3 pages, got %d", result.Pages)
	}

	result = s.List(ListOptions{Page: 3, Limit: 10})
	if len(result.Items) != 5 {
		t.Fatalf("Expected 5 items on page 3, got %d", len(result.Items))
	}
}

func TestListLabelFilter(t *testing.T) {
	s := New()

	s.Create(&Item{Name: "prod-1", Labels: map[string]string{"env": "prod"}})
	s.Create(&Item{Name: "dev-1", Labels: map[string]string{"env": "dev"}})
	s.Create(&Item{Name: "prod-2", Labels: map[string]string{"env": "prod"}})

	result := s.List(ListOptions{
		Labels: map[string]string{"env": "prod"},
	})
	if result.Total != 2 {
		t.Fatalf("Expected 2 prod items, got %d", result.Total)
	}
}

func TestListSort(t *testing.T) {
	s := New()

	s.Create(&Item{Name: "banana"})
	time.Sleep(time.Millisecond) // Ensure different timestamps.
	s.Create(&Item{Name: "apple"})
	time.Sleep(time.Millisecond)
	s.Create(&Item{Name: "cherry"})

	result := s.List(ListOptions{Sort: "name"})
	if result.Items[0].Name != "apple" {
		t.Fatalf("Expected first item 'apple', got %q", result.Items[0].Name)
	}

	result = s.List(ListOptions{Sort: "-name"})
	if result.Items[0].Name != "cherry" {
		t.Fatalf("Expected first item 'cherry', got %q", result.Items[0].Name)
	}
}

func TestEvents(t *testing.T) {
	s := New()
	ch := s.Subscribe()
	defer s.Unsubscribe(ch)

	// Create
	item, _ := s.Create(&Item{Name: "event-test"})
	event := <-ch
	if event.Type != EventCreated {
		t.Fatalf("Expected CREATED event, got %s", event.Type)
	}

	// Update
	s.Update(item.ID, &Item{Name: "updated"})
	event = <-ch
	if event.Type != EventUpdated {
		t.Fatalf("Expected UPDATED event, got %s", event.Type)
	}

	// Delete
	s.Delete(item.ID)
	event = <-ch
	if event.Type != EventDeleted {
		t.Fatalf("Expected DELETED event, got %s", event.Type)
	}
}

func TestConcurrency(t *testing.T) {
	s := New()
	var wg sync.WaitGroup

	// Concurrent creates.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Create(&Item{Name: "concurrent"})
		}()
	}
	wg.Wait()

	if s.Count() != 100 {
		t.Fatalf("Expected 100 items, got %d", s.Count())
	}
}
