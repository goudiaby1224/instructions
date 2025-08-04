package grpc

import (
    "context"
    "your-module/internal/core/ports"
)

type GRPCServer struct {
    service ports.Service
}

func NewGRPCServer(service ports.Service) *GRPCServer {
    return &GRPCServer{service: service}
}

func (s *GRPCServer) CreateItem(ctx context.Context, req *CreateItemRequest) (*ItemResponse, error) {
    // ...implementation...
}
