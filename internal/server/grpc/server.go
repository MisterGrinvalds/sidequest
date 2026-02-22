package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/MisterGrinvalds/sidequest/internal/store"
	pb "github.com/MisterGrinvalds/sidequest/proto/sidequest/v1"
)

// Server wraps the gRPC server and item store.
type Server struct {
	pb.UnimplementedItemServiceServer
	port   int
	store  *store.Store
	server *grpc.Server
}

// New creates a new gRPC server.
func New(port int, s *store.Store) *Server {
	srv := &Server{port: port, store: s}

	grpcServer := grpc.NewServer()
	pb.RegisterItemServiceServer(grpcServer, srv)

	// Enable server reflection for discoverability.
	reflection.Register(grpcServer)

	// Health check service.
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthSrv)
	healthSrv.SetServingStatus("sidequest.v1.ItemService", healthpb.HealthCheckResponse_SERVING)

	srv.server = grpcServer
	return srv
}

// Port returns the configured port.
func (s *Server) Port() int { return s.port }

// Start begins listening.
func (s *Server) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	return s.server.Serve(lis)
}

// GracefulStop gracefully stops the gRPC server.
func (s *Server) GracefulStop() {
	s.server.GracefulStop()
}

// Stop immediately stops the gRPC server.
func (s *Server) Stop() {
	s.server.Stop()
}

// --- RPC Implementations ---

func (s *Server) GetItem(_ context.Context, req *pb.GetItemRequest) (*pb.Item, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	item, err := s.store.Get(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "item %q not found", req.Id)
	}
	return itemToProto(item), nil
}

func (s *Server) ListItems(_ context.Context, req *pb.ListItemsRequest) (*pb.ListItemsResponse, error) {
	result := s.store.List(store.ListOptions{
		Page:   int(req.Page),
		Limit:  int(req.Limit),
		Sort:   req.Sort,
		Labels: req.Labels,
	})

	items := make([]*pb.Item, len(result.Items))
	for i, item := range result.Items {
		items[i] = itemToProto(item)
	}

	return &pb.ListItemsResponse{
		Items: items,
		Total: int32(result.Total),
		Page:  int32(result.Page),
		Limit: int32(result.Limit),
		Pages: int32(result.Pages),
	}, nil
}

func (s *Server) CreateItem(_ context.Context, req *pb.CreateItemRequest) (*pb.Item, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	item := &store.Item{
		Name:   req.Name,
		Labels: req.Labels,
	}
	if req.Data != nil {
		data, err := json.Marshal(req.Data.AsMap())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid data: %v", err)
		}
		item.Data = data
	}

	created, err := s.store.Create(item)
	if err != nil {
		return nil, status.Errorf(codes.AlreadyExists, "%v", err)
	}
	return itemToProto(created), nil
}

func (s *Server) UpdateItem(_ context.Context, req *pb.UpdateItemRequest) (*pb.Item, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	item := &store.Item{
		Name:   req.Name,
		Labels: req.Labels,
	}
	if req.Data != nil {
		data, err := json.Marshal(req.Data.AsMap())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid data: %v", err)
		}
		item.Data = data
	}

	updated, err := s.store.Update(req.Id, item)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return itemToProto(updated), nil
}

func (s *Server) DeleteItem(_ context.Context, req *pb.DeleteItemRequest) (*emptypb.Empty, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if err := s.store.Delete(req.Id); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) WatchItems(req *pb.WatchItemsRequest, stream pb.ItemService_WatchItemsServer) error {
	ch := s.store.Subscribe()
	defer s.store.Unsubscribe(ch)

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return nil
			}

			// Apply label filter if specified.
			if len(req.Labels) > 0 && !matchLabels(event.Item, req.Labels) {
				continue
			}

			pbEvent := &pb.ItemEvent{
				Item: itemToProto(event.Item),
			}
			switch event.Type {
			case store.EventCreated:
				pbEvent.Type = pb.EventType_EVENT_TYPE_CREATED
			case store.EventUpdated:
				pbEvent.Type = pb.EventType_EVENT_TYPE_UPDATED
			case store.EventDeleted:
				pbEvent.Type = pb.EventType_EVENT_TYPE_DELETED
			}

			if err := stream.Send(pbEvent); err != nil {
				return err
			}

		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}
}

// --- Helpers ---

func itemToProto(item *store.Item) *pb.Item {
	pbItem := &pb.Item{
		Id:        item.ID,
		Name:      item.Name,
		Labels:    item.Labels,
		CreatedAt: timestamppb.New(item.CreatedAt),
		UpdatedAt: timestamppb.New(item.UpdatedAt),
		Version:   int32(item.Version),
	}

	if item.Data != nil {
		var m map[string]interface{}
		if err := json.Unmarshal(item.Data, &m); err == nil {
			if s, err := structpb.NewStruct(m); err == nil {
				pbItem.Data = s
			}
		}
	}

	return pbItem
}

func matchLabels(item *store.Item, labels map[string]string) bool {
	for k, v := range labels {
		if item.Labels[k] != v {
			return false
		}
	}
	return true
}
