package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/handler"

	"github.com/MisterGrinvalds/sidequest/internal/store"
)

// Server is a GraphQL API server.
type Server struct {
	port   int
	store  *store.Store
	server *http.Server
	schema graphql.Schema
}

// New creates a new GraphQL server.
func New(port int, s *store.Store) (*Server, error) {
	srv := &Server{port: port, store: s}

	schema, err := srv.buildSchema()
	if err != nil {
		return nil, fmt.Errorf("building schema: %w", err)
	}
	srv.schema = schema

	mux := http.NewServeMux()

	// GraphQL endpoint.
	h := handler.New(&handler.Config{
		Schema:   &schema,
		Pretty:   true,
		GraphiQL: false,
	})
	mux.Handle("/graphql", h)

	// GraphiQL playground.
	playgroundH := handler.New(&handler.Config{
		Schema:   &schema,
		Pretty:   true,
		GraphiQL: true,
	})
	mux.Handle("/playground", playgroundH)

	// Health endpoint.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	srv.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return srv, nil
}

// Port returns the configured port.
func (s *Server) Port() int { return s.port }

// Start begins listening.
func (s *Server) Start() error { return s.server.ListenAndServe() }

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error { return s.server.Shutdown(ctx) }

func (s *Server) buildSchema() (graphql.Schema, error) {
	// Label type.
	labelType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Label",
		Fields: graphql.Fields{
			"key":   &graphql.Field{Type: graphql.String},
			"value": &graphql.Field{Type: graphql.String},
		},
	})

	// Item type.
	itemType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Item",
		Fields: graphql.Fields{
			"id":   &graphql.Field{Type: graphql.NewNonNull(graphql.ID)},
			"name": &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"labels": &graphql.Field{
				Type: graphql.NewList(labelType),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					item, ok := p.Source.(*store.Item)
					if !ok {
						return nil, nil
					}
					var labels []map[string]string
					for k, v := range item.Labels {
						labels = append(labels, map[string]string{"key": k, "value": v})
					}
					return labels, nil
				},
			},
			"data": &graphql.Field{
				Type: graphql.String,
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					item, ok := p.Source.(*store.Item)
					if !ok {
						return nil, nil
					}
					if item.Data == nil {
						return nil, nil
					}
					return string(item.Data), nil
				},
			},
			"createdAt": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					item := p.Source.(*store.Item)
					return item.CreatedAt.Format(time.RFC3339), nil
				},
			},
			"updatedAt": &graphql.Field{
				Type: graphql.NewNonNull(graphql.String),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					item := p.Source.(*store.Item)
					return item.UpdatedAt.Format(time.RFC3339), nil
				},
			},
			"version": &graphql.Field{Type: graphql.NewNonNull(graphql.Int)},
		},
	})

	// ItemConnection type (pagination wrapper).
	itemConnectionType := graphql.NewObject(graphql.ObjectConfig{
		Name: "ItemConnection",
		Fields: graphql.Fields{
			"items": &graphql.Field{Type: graphql.NewList(itemType)},
			"total": &graphql.Field{Type: graphql.Int},
			"page":  &graphql.Field{Type: graphql.Int},
			"limit": &graphql.Field{Type: graphql.Int},
			"pages": &graphql.Field{Type: graphql.Int},
		},
	})

	// Queries.
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"item": &graphql.Field{
				Type: itemType,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					id := p.Args["id"].(string)
					item, err := s.store.Get(id)
					if err != nil {
						return nil, err
					}
					return item, nil
				},
			},
			"items": &graphql.Field{
				Type: itemConnectionType,
				Args: graphql.FieldConfigArgument{
					"page":  &graphql.ArgumentConfig{Type: graphql.Int, DefaultValue: 1},
					"limit": &graphql.ArgumentConfig{Type: graphql.Int, DefaultValue: 20},
					"sort":  &graphql.ArgumentConfig{Type: graphql.String, DefaultValue: ""},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					page := p.Args["page"].(int)
					limit := p.Args["limit"].(int)
					sortField := p.Args["sort"].(string)

					result := s.store.List(store.ListOptions{
						Page:  page,
						Limit: limit,
						Sort:  sortField,
					})
					return result, nil
				},
			},
		},
	})

	// Mutations.
	mutationType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Mutation",
		Fields: graphql.Fields{
			"createItem": &graphql.Field{
				Type: itemType,
				Args: graphql.FieldConfigArgument{
					"name":   &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
					"labels": &graphql.ArgumentConfig{Type: graphql.String, Description: "JSON object of key-value labels"},
					"data":   &graphql.ArgumentConfig{Type: graphql.String, Description: "JSON data payload"},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					name := p.Args["name"].(string)
					item := &store.Item{Name: name}

					if labelsStr, ok := p.Args["labels"].(string); ok && labelsStr != "" {
						var labels map[string]string
						if err := json.Unmarshal([]byte(labelsStr), &labels); err != nil {
							return nil, fmt.Errorf("invalid labels JSON: %w", err)
						}
						item.Labels = labels
					}

					if dataStr, ok := p.Args["data"].(string); ok && dataStr != "" {
						item.Data = json.RawMessage(dataStr)
					}

					return s.store.Create(item)
				},
			},
			"updateItem": &graphql.Field{
				Type: itemType,
				Args: graphql.FieldConfigArgument{
					"id":     &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
					"name":   &graphql.ArgumentConfig{Type: graphql.String},
					"labels": &graphql.ArgumentConfig{Type: graphql.String},
					"data":   &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					id := p.Args["id"].(string)
					item := &store.Item{}

					if name, ok := p.Args["name"].(string); ok {
						item.Name = name
					}
					if labelsStr, ok := p.Args["labels"].(string); ok && labelsStr != "" {
						var labels map[string]string
						if err := json.Unmarshal([]byte(labelsStr), &labels); err != nil {
							return nil, fmt.Errorf("invalid labels JSON: %w", err)
						}
						item.Labels = labels
					}
					if dataStr, ok := p.Args["data"].(string); ok && dataStr != "" {
						item.Data = json.RawMessage(dataStr)
					}

					return s.store.Update(id, item)
				},
			},
			"deleteItem": &graphql.Field{
				Type: graphql.Boolean,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.ID)},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					id := p.Args["id"].(string)
					if err := s.store.Delete(id); err != nil {
						return false, err
					}
					return true, nil
				},
			},
		},
	})

	return graphql.NewSchema(graphql.SchemaConfig{
		Query:    queryType,
		Mutation: mutationType,
	})
}
