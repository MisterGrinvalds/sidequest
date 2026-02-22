package grpc

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/MisterGrinvalds/sidequest/internal/store"
	pb "github.com/MisterGrinvalds/sidequest/proto/sidequest/v1"
)

const bufSize = 1024 * 1024

func newTestServer(t *testing.T) (pb.ItemServiceClient, func()) {
	t.Helper()

	s := store.New()
	srv := New(0, s)

	lis := bufconn.Listen(bufSize)
	go func() {
		srv.server.Serve(lis)
	}()

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}

	client := pb.NewItemServiceClient(conn)
	cleanup := func() {
		conn.Close()
		srv.Stop()
	}

	return client, cleanup
}

func TestGRPCCreateItem(t *testing.T) {
	client, cleanup := newTestServer(t)
	defer cleanup()

	item, err := client.CreateItem(context.Background(), &pb.CreateItemRequest{
		Name:   "grpc-test",
		Labels: map[string]string{"env": "test"},
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	if item.Id == "" {
		t.Error("Expected non-empty ID")
	}
	if item.Name != "grpc-test" {
		t.Errorf("Expected name 'grpc-test', got %q", item.Name)
	}
	if item.Version != 1 {
		t.Errorf("Expected version 1, got %d", item.Version)
	}
	if item.Labels["env"] != "test" {
		t.Error("Expected label env=test")
	}
}

func TestGRPCCreateItemValidation(t *testing.T) {
	client, cleanup := newTestServer(t)
	defer cleanup()

	_, err := client.CreateItem(context.Background(), &pb.CreateItemRequest{
		Name: "",
	})
	if err == nil {
		t.Fatal("Expected error for empty name")
	}
	if s, ok := status.FromError(err); ok {
		if s.Code() != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", s.Code())
		}
	}
}

func TestGRPCCreateItemWithData(t *testing.T) {
	client, cleanup := newTestServer(t)
	defer cleanup()

	data, _ := structpb.NewStruct(map[string]interface{}{
		"key": "value",
		"num": 42.0,
	})
	item, err := client.CreateItem(context.Background(), &pb.CreateItemRequest{
		Name: "with-data",
		Data: data,
	})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if item.Data == nil {
		t.Error("Expected data to be set")
	}
}

func TestGRPCGetItem(t *testing.T) {
	client, cleanup := newTestServer(t)
	defer cleanup()

	created, _ := client.CreateItem(context.Background(), &pb.CreateItemRequest{Name: "getme"})

	item, err := client.GetItem(context.Background(), &pb.GetItemRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if item.Name != "getme" {
		t.Errorf("Expected name 'getme', got %q", item.Name)
	}
}

func TestGRPCGetItemNotFound(t *testing.T) {
	client, cleanup := newTestServer(t)
	defer cleanup()

	_, err := client.GetItem(context.Background(), &pb.GetItemRequest{Id: "nonexistent"})
	if err == nil {
		t.Fatal("Expected error for nonexistent item")
	}
	if s, ok := status.FromError(err); ok {
		if s.Code() != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", s.Code())
		}
	}
}

func TestGRPCGetItemInvalidArgument(t *testing.T) {
	client, cleanup := newTestServer(t)
	defer cleanup()

	_, err := client.GetItem(context.Background(), &pb.GetItemRequest{Id: ""})
	if err == nil {
		t.Fatal("Expected error for empty ID")
	}
	if s, ok := status.FromError(err); ok {
		if s.Code() != codes.InvalidArgument {
			t.Errorf("Expected InvalidArgument, got %v", s.Code())
		}
	}
}

func TestGRPCListItems(t *testing.T) {
	client, cleanup := newTestServer(t)
	defer cleanup()

	for i := 0; i < 5; i++ {
		client.CreateItem(context.Background(), &pb.CreateItemRequest{Name: "list-item"})
	}

	resp, err := client.ListItems(context.Background(), &pb.ListItemsRequest{
		Page:  1,
		Limit: 3,
	})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if resp.Total != 5 {
		t.Errorf("Expected total 5, got %d", resp.Total)
	}
	if len(resp.Items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(resp.Items))
	}
	if resp.Pages != 2 {
		t.Errorf("Expected 2 pages, got %d", resp.Pages)
	}
}

func TestGRPCListItemsWithLabelFilter(t *testing.T) {
	client, cleanup := newTestServer(t)
	defer cleanup()

	client.CreateItem(context.Background(), &pb.CreateItemRequest{
		Name: "prod", Labels: map[string]string{"env": "prod"},
	})
	client.CreateItem(context.Background(), &pb.CreateItemRequest{
		Name: "dev", Labels: map[string]string{"env": "dev"},
	})

	resp, err := client.ListItems(context.Background(), &pb.ListItemsRequest{
		Labels: map[string]string{"env": "prod"},
	})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if resp.Total != 1 {
		t.Errorf("Expected 1 prod item, got %d", resp.Total)
	}
}

func TestGRPCUpdateItem(t *testing.T) {
	client, cleanup := newTestServer(t)
	defer cleanup()

	created, _ := client.CreateItem(context.Background(), &pb.CreateItemRequest{Name: "original"})

	updated, err := client.UpdateItem(context.Background(), &pb.UpdateItemRequest{
		Id:   created.Id,
		Name: "updated",
	})
	if err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}
	if updated.Name != "updated" {
		t.Errorf("Expected 'updated', got %q", updated.Name)
	}
	if updated.Version != 2 {
		t.Errorf("Expected version 2, got %d", updated.Version)
	}
}

func TestGRPCUpdateItemNotFound(t *testing.T) {
	client, cleanup := newTestServer(t)
	defer cleanup()

	_, err := client.UpdateItem(context.Background(), &pb.UpdateItemRequest{
		Id: "nonexistent", Name: "x",
	})
	if err == nil {
		t.Fatal("Expected error")
	}
	if s, ok := status.FromError(err); ok {
		if s.Code() != codes.NotFound {
			t.Errorf("Expected NotFound, got %v", s.Code())
		}
	}
}

func TestGRPCDeleteItem(t *testing.T) {
	client, cleanup := newTestServer(t)
	defer cleanup()

	created, _ := client.CreateItem(context.Background(), &pb.CreateItemRequest{Name: "deleteme"})

	_, err := client.DeleteItem(context.Background(), &pb.DeleteItemRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}

	// Verify deleted.
	_, err = client.GetItem(context.Background(), &pb.GetItemRequest{Id: created.Id})
	if err == nil {
		t.Fatal("Expected NotFound after delete")
	}
}

func TestGRPCDeleteItemNotFound(t *testing.T) {
	client, cleanup := newTestServer(t)
	defer cleanup()

	_, err := client.DeleteItem(context.Background(), &pb.DeleteItemRequest{Id: "nonexistent"})
	if err == nil {
		t.Fatal("Expected error")
	}
}

func TestGRPCWatchItems(t *testing.T) {
	client, cleanup := newTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := client.WatchItems(ctx, &pb.WatchItemsRequest{})
	if err != nil {
		t.Fatalf("WatchItems: %v", err)
	}

	// Create an item — should emit an event.
	go func() {
		client.CreateItem(context.Background(), &pb.CreateItemRequest{Name: "watched"})
	}()

	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if event.Type != pb.EventType_EVENT_TYPE_CREATED {
		t.Errorf("Expected CREATED, got %v", event.Type)
	}
	if event.Item.Name != "watched" {
		t.Errorf("Expected name 'watched', got %q", event.Item.Name)
	}
}
