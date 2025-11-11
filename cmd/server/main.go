package main

import (
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/larrasket/hlimiter/internal/config"
	grpcserver "github.com/larrasket/hlimiter/internal/grpc"
	"github.com/larrasket/hlimiter/internal/limiter"
	"github.com/larrasket/hlimiter/internal/storage"
	pb "github.com/larrasket/hlimiter/proto"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		slog.Error("CONFIG_PATH environment variable is required")
		os.Exit(1)
	}
	slog.Info("loading config", "path", configPath)

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	slog.Info("connecting to redis", "addr", cfg.Redis.Addr)
	store, err := storage.NewRedis(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB, cfg.Redis.PoolSize)
	if err != nil {
		slog.Error("redis connection failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	rl := limiter.NewRedis(store)

	grpcAddr := cfg.GRPC.Addr
	slog.Info("starting grpc server", "addr", grpcAddr)
	
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		slog.Error("failed to listen", "addr", grpcAddr, "error", err)
		os.Exit(1)
	}

	s := grpc.NewServer(
		grpc.MaxConcurrentStreams(1000),
		grpc.MaxRecvMsgSize(1024*1024),
		grpc.MaxSendMsgSize(1024*1024),
	)
	pb.RegisterRateLimiterServer(s, grpcserver.NewServer(rl))
	reflection.Register(s)

	go func() {
		if err := s.Serve(lis); err != nil {
			slog.Error("grpc serve failed", "error", err)
			os.Exit(1)
		}
	}()

	slog.Info("grpc server ready for service registration")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down gracefully")
	s.GracefulStop()
	slog.Info("shutdown complete")
}
