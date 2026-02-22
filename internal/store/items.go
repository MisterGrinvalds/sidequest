package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Item represents a resource in the shared data store.
type Item struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels,omitempty"`
	Data      json.RawMessage   `json:"data,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Version   int               `json:"version"`
}

// EventType describes the kind of change that occurred.
type EventType string

const (
	EventCreated EventType = "CREATED"
	EventUpdated EventType = "UPDATED"
	EventDeleted EventType = "DELETED"
)

// ItemEvent is emitted when an item changes.
type ItemEvent struct {
	Type EventType `json:"type"`
	Item *Item     `json:"item"`
}

// ListOptions controls filtering, sorting, and pagination for List operations.
type ListOptions struct {
	Page   int
	Limit  int
	Sort   string // field name, prefix with "-" for descending
	Labels map[string]string
}

// ListResult contains a page of items plus pagination metadata.
type ListResult struct {
	Items []*Item `json:"items"`
	Total int     `json:"total"`
	Page  int     `json:"page"`
	Limit int     `json:"limit"`
	Pages int     `json:"pages"`
}

// Store is a thread-safe in-memory item store with event emission.
type Store struct {
	mu         sync.RWMutex
	items      map[string]*Item
	listeners  []chan ItemEvent
	listenerMu sync.Mutex
}

// New creates a new empty Store.
func New() *Store {
	return &Store{
		items: make(map[string]*Item),
	}
}

// Subscribe returns a channel that receives item events.
// The caller should call Unsubscribe when done.
func (s *Store) Subscribe() chan ItemEvent {
	s.listenerMu.Lock()
	defer s.listenerMu.Unlock()
	ch := make(chan ItemEvent, 64)
	s.listeners = append(s.listeners, ch)
	return ch
}

// Unsubscribe removes a listener channel and closes it.
func (s *Store) Unsubscribe(ch chan ItemEvent) {
	s.listenerMu.Lock()
	defer s.listenerMu.Unlock()
	for i, l := range s.listeners {
		if l == ch {
			s.listeners = append(s.listeners[:i], s.listeners[i+1:]...)
			close(ch)
			return
		}
	}
}

func (s *Store) emit(event ItemEvent) {
	s.listenerMu.Lock()
	listeners := make([]chan ItemEvent, len(s.listeners))
	copy(listeners, s.listeners)
	s.listenerMu.Unlock()

	for _, ch := range listeners {
		select {
		case ch <- event:
		default:
			// Drop event if listener is not keeping up.
		}
	}
}

// Create adds a new item to the store. If item.ID is empty, a UUID is generated.
func (s *Store) Create(item *Item) (*Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item.ID == "" {
		item.ID = uuid.New().String()
	}

	if _, exists := s.items[item.ID]; exists {
		return nil, fmt.Errorf("item %q already exists", item.ID)
	}

	now := time.Now().UTC()
	item.CreatedAt = now
	item.UpdatedAt = now
	item.Version = 1

	if item.Labels == nil {
		item.Labels = make(map[string]string)
	}

	stored := clone(item)
	s.items[stored.ID] = stored

	s.emit(ItemEvent{Type: EventCreated, Item: clone(stored)})
	return clone(stored), nil
}

// Get retrieves a single item by ID.
func (s *Store) Get(id string) (*Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, ok := s.items[id]
	if !ok {
		return nil, fmt.Errorf("item %q not found", id)
	}
	return clone(item), nil
}

// Update performs a full replace of an existing item.
func (s *Store) Update(id string, item *Item) (*Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.items[id]
	if !ok {
		return nil, fmt.Errorf("item %q not found", id)
	}

	item.ID = id
	item.CreatedAt = existing.CreatedAt
	item.UpdatedAt = time.Now().UTC()
	item.Version = existing.Version + 1

	if item.Labels == nil {
		item.Labels = make(map[string]string)
	}

	stored := clone(item)
	s.items[id] = stored

	s.emit(ItemEvent{Type: EventUpdated, Item: clone(stored)})
	return clone(stored), nil
}

// Upsert creates or updates an item. Returns the item and whether it was created.
func (s *Store) Upsert(id string, item *Item) (*Item, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.items[id]

	now := time.Now().UTC()
	item.ID = id

	if item.Labels == nil {
		item.Labels = make(map[string]string)
	}

	if exists {
		// Merge: only overwrite non-zero fields.
		if item.Name != "" {
			existing.Name = item.Name
		}
		if item.Data != nil {
			existing.Data = item.Data
		}
		for k, v := range item.Labels {
			existing.Labels[k] = v
		}
		existing.UpdatedAt = now
		existing.Version++

		stored := clone(existing)
		s.items[id] = stored
		s.emit(ItemEvent{Type: EventUpdated, Item: clone(stored)})
		return clone(stored), false, nil
	}

	// Create new.
	item.CreatedAt = now
	item.UpdatedAt = now
	item.Version = 1

	stored := clone(item)
	s.items[id] = stored
	s.emit(ItemEvent{Type: EventCreated, Item: clone(stored)})
	return clone(stored), true, nil
}

// Delete removes an item by ID.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.items[id]
	if !ok {
		return fmt.Errorf("item %q not found", id)
	}

	deleted := clone(item)
	delete(s.items, id)

	s.emit(ItemEvent{Type: EventDeleted, Item: deleted})
	return nil
}

// List returns a paginated, sorted, filtered list of items.
func (s *Store) List(opts ListOptions) *ListResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.Limit < 1 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}

	// Collect all items, applying label filter.
	var filtered []*Item
	for _, item := range s.items {
		if matchLabels(item, opts.Labels) {
			filtered = append(filtered, clone(item))
		}
	}

	// Sort.
	sortItems(filtered, opts.Sort)

	// Paginate.
	total := len(filtered)
	pages := (total + opts.Limit - 1) / opts.Limit
	if pages < 1 {
		pages = 1
	}

	start := (opts.Page - 1) * opts.Limit
	if start > total {
		start = total
	}
	end := start + opts.Limit
	if end > total {
		end = total
	}

	return &ListResult{
		Items: filtered[start:end],
		Total: total,
		Page:  opts.Page,
		Limit: opts.Limit,
		Pages: pages,
	}
}

// Count returns the total number of items in the store.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

func matchLabels(item *Item, labels map[string]string) bool {
	for k, v := range labels {
		if item.Labels[k] != v {
			return false
		}
	}
	return true
}

func sortItems(items []*Item, field string) {
	if field == "" {
		field = "created_at"
	}

	desc := false
	if strings.HasPrefix(field, "-") {
		desc = true
		field = field[1:]
	}

	sort.Slice(items, func(i, j int) bool {
		var less bool
		switch field {
		case "name":
			less = items[i].Name < items[j].Name
		case "updated_at":
			less = items[i].UpdatedAt.Before(items[j].UpdatedAt)
		case "id":
			less = items[i].ID < items[j].ID
		default: // created_at
			less = items[i].CreatedAt.Before(items[j].CreatedAt)
		}
		if desc {
			return !less
		}
		return less
	})
}

func clone(item *Item) *Item {
	c := *item
	if item.Labels != nil {
		c.Labels = make(map[string]string, len(item.Labels))
		for k, v := range item.Labels {
			c.Labels[k] = v
		}
	}
	if item.Data != nil {
		c.Data = make(json.RawMessage, len(item.Data))
		copy(c.Data, item.Data)
	}
	return &c
}
