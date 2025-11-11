package main

import (
	"fmt"
	"log"
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
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		panic("CONFIG_PATH environment variable is required")
	}
	fmt.Printf("[main] loading config: %s\n", configPath)

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("config load failed: %v", err)
	}

	fmt.Printf("[redis] connecting to %s\n", cfg.Redis.Addr)
	store, err := storage.NewRedis(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB, cfg.Redis.PoolSize)
	if err != nil {
		log.Fatalf("redis connection failed: %v", err)
	}
	defer store.Close()

	rl := limiter.NewRedis(store)

	grpcAddr := cfg.GRPC.Addr
	fmt.Printf("[grpc] starting on %s\n", grpcAddr)
	
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
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
			log.Fatalf("grpc serve failed: %v", err)
		}
	}()

	fmt.Printf("[grpc] ready for service registration\n")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\nshutting down...")
	s.GracefulStop()
}
