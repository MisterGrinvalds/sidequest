package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	pb "github.com/MisterGrinvalds/sidequest/proto/sidequest/v1"
)

// Client wraps a gRPC connection to the ItemService.
type Client struct {
	conn    *grpc.ClientConn
	service pb.ItemServiceClient
}

// Connect establishes a gRPC connection.
func Connect(addr string, plaintext bool, timeout time.Duration) (*Client, error) {
	opts := []grpc.DialOption{}
	if plaintext {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", addr, err)
	}

	return &Client{
		conn:    conn,
		service: pb.NewItemServiceClient(conn),
	}, nil
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Call invokes a gRPC method by name with JSON data, returning JSON.
func (c *Client) Call(ctx context.Context, method string, data string) (string, error) {
	switch method {
	case "GetItem", "sidequest.v1.ItemService/GetItem":
		req := &pb.GetItemRequest{}
		if err := protojson.Unmarshal([]byte(data), req); err != nil {
			return "", fmt.Errorf("parsing request: %w", err)
		}
		resp, err := c.service.GetItem(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalResponse(resp)

	case "ListItems", "sidequest.v1.ItemService/ListItems":
		req := &pb.ListItemsRequest{}
		if err := protojson.Unmarshal([]byte(data), req); err != nil {
			return "", fmt.Errorf("parsing request: %w", err)
		}
		resp, err := c.service.ListItems(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalResponse(resp)

	case "CreateItem", "sidequest.v1.ItemService/CreateItem":
		req := &pb.CreateItemRequest{}
		if err := protojson.Unmarshal([]byte(data), req); err != nil {
			// Try manual parsing for the data field.
			return "", fmt.Errorf("parsing request: %w", err)
		}
		resp, err := c.service.CreateItem(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalResponse(resp)

	case "UpdateItem", "sidequest.v1.ItemService/UpdateItem":
		req := &pb.UpdateItemRequest{}
		if err := protojson.Unmarshal([]byte(data), req); err != nil {
			return "", fmt.Errorf("parsing request: %w", err)
		}
		resp, err := c.service.UpdateItem(ctx, req)
		if err != nil {
			return "", err
		}
		return marshalResponse(resp)

	case "DeleteItem", "sidequest.v1.ItemService/DeleteItem":
		req := &pb.DeleteItemRequest{}
		if err := protojson.Unmarshal([]byte(data), req); err != nil {
			return "", fmt.Errorf("parsing request: %w", err)
		}
		_, err := c.service.DeleteItem(ctx, req)
		if err != nil {
			return "", err
		}
		return "{}", nil

	default:
		return "", fmt.Errorf("unknown method %q; available: GetItem, ListItems, CreateItem, UpdateItem, DeleteItem", method)
	}
}

// ListServices returns the list of gRPC services via server reflection.
func ListServices(ctx context.Context, addr string, plaintext bool, timeout time.Duration) ([]string, error) {
	opts := []grpc.DialOption{}
	if plaintext {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", addr, err)
	}
	defer conn.Close()

	client := rpb.NewServerReflectionClient(conn)
	stream, err := client.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("reflection: %w", err)
	}

	if err := stream.Send(&rpb.ServerReflectionRequest{
		MessageRequest: &rpb.ServerReflectionRequest_ListServices{ListServices: ""},
	}); err != nil {
		return nil, fmt.Errorf("sending reflection request: %w", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("receiving reflection response: %w", err)
	}

	listResp := resp.GetListServicesResponse()
	if listResp == nil {
		return nil, fmt.Errorf("unexpected reflection response type")
	}

	var services []string
	for _, svc := range listResp.Service {
		services = append(services, svc.Name)
	}
	return services, nil
}

// WatchItems opens a server-streaming call and prints events as they arrive.
func (c *Client) WatchItems(ctx context.Context, labels map[string]string) error {
	stream, err := c.service.WatchItems(ctx, &pb.WatchItemsRequest{Labels: labels})
	if err != nil {
		return fmt.Errorf("watch: %w", err)
	}

	for {
		event, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		out, _ := marshalResponse(event)
		fmt.Println(out)
	}
}

// CreateItemSimple is a convenience for creating an item with simple fields.
func (c *Client) CreateItemSimple(ctx context.Context, name string, labels map[string]string, data map[string]interface{}) (string, error) {
	req := &pb.CreateItemRequest{
		Name:   name,
		Labels: labels,
	}
	if data != nil {
		s, err := structpb.NewStruct(data)
		if err != nil {
			return "", fmt.Errorf("converting data: %w", err)
		}
		req.Data = s
	}

	resp, err := c.service.CreateItem(ctx, req)
	if err != nil {
		return "", err
	}
	return marshalResponse(resp)
}

func marshalResponse(msg proto.Message) (string, error) {
	marshaler := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		EmitUnpopulated: false,
	}
	b, err := marshaler.Marshal(msg)
	if err != nil {
		return "", err
	}
	// Re-indent for consistent output.
	var buf json.RawMessage = b
	indented, err := json.MarshalIndent(buf, "", "  ")
	if err != nil {
		return string(b), nil
	}
	return string(indented), nil
}
