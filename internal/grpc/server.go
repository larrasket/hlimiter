package grpc

import (
	"context"
	"fmt"

	"github.com/larrasket/hlimiter/internal/config"
	"github.com/larrasket/hlimiter/internal/limiter"
	pb "github.com/larrasket/hlimiter/proto"
)

type Server struct {
	pb.UnimplementedRateLimiterServer
	limiter *limiter.RedisRateLimiter
}

func NewServer(rl *limiter.RedisRateLimiter) *Server {
	return &Server{limiter: rl}
}

func (s *Server) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	var apis []config.API
	for _, apiCfg := range req.Apis {
		apis = append(apis, config.API{
			Path:          apiCfg.Path,
			Algorithm:     apiCfg.Algorithm,
			KeyStrategy:   apiCfg.KeyStrategy,
			Limit:         int(apiCfg.Limit),
			WindowSeconds: int(apiCfg.WindowSeconds),
			Burst:         int(apiCfg.Burst),
		})
	}

	if err := s.limiter.Register(ctx, req.Service, apis); err != nil {
		return &pb.RegisterResponse{
			Success: false,
			Message: fmt.Sprintf("registration failed: %v", err),
		}, nil
	}

	return &pb.RegisterResponse{
		Success: true,
		Message: fmt.Sprintf("registered %d APIs for %s", len(apis), req.Service),
	}, nil
}

func (s *Server) Check(ctx context.Context, req *pb.CheckRequest) (*pb.CheckResponse, error) {
	limReq := limiter.CheckRequest{
		Service: req.Service,
		API:     req.Api,
		IP:      req.Ip,
		Headers: req.Headers,
	}

	resp, err := s.limiter.Check(ctx, limReq)
	if err != nil {
		return nil, err
	}

	return &pb.CheckResponse{
		Allowed:   resp.Allowed,
		Remaining: int32(resp.Remaining),
		ResetAt:   resp.ResetAt,
	}, nil
}
