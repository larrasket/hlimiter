package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/larrasket/hlimiter/proto"
)

type PaymentService struct {
	grpcClient pb.RateLimiterClient
	conn       *grpc.ClientConn
}

func (p *PaymentService) checkLimit(svc, path, ip string, hdrs map[string]string) (bool, int32, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	resp, err := p.grpcClient.Check(ctx, &pb.CheckRequest{
		Service: svc,
		Api:     path,
		Ip:      ip,
		Headers: hdrs,
	})
	if err != nil {
		return false, 0, 0, err
	}

	return resp.Allowed, resp.Remaining, resp.ResetAt, nil
}

func (p *PaymentService) handleProcess(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	slog.Info("processing payment", "session_id", sessionID, "ip", r.RemoteAddr)

	allowed, remaining, _, err := p.checkLimit("payment-service", "/payment/process", r.RemoteAddr, map[string]string{"X-Session-ID": sessionID})
	if err != nil {
		slog.Error("rate limit check failed", "error", err, "session_id", sessionID)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if !allowed {
		slog.Warn("rate limit exceeded", "session_id", sessionID, "remaining", remaining)
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(int(remaining)))
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	slog.Info("payment processed successfully", "session_id", sessionID, "remaining", remaining)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"txn_id": "txn_123456",
		"amount": 99.99,
	})
}

func (p *PaymentService) handleValidate(w http.ResponseWriter, r *http.Request) {
	ip := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ip = forwarded
	}

	allowed, remaining, _, err := p.checkLimit("payment-service", "/payment/validate", ip, nil)
	if err != nil {
		slog.Error("rate limit check failed", "error", err, "ip", ip)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}

	if !allowed {
		slog.Warn("validation rate limited", "ip", ip, "remaining", remaining)
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(int(remaining)))
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}

	slog.Debug("validation successful", "ip", ip, "remaining", remaining)
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(int(remaining)))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{
		"valid": true,
	})
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	grpcAddr := os.Getenv("LIMITER_GRPC_ADDR")
	if grpcAddr == "" {
		slog.Error("LIMITER_GRPC_ADDR environment variable is required")
		os.Exit(1)
	}

	port := os.Getenv("PORT")
	if port == "" {
		slog.Error("PORT environment variable is required")
		os.Exit(1)
	}

	slog.Info("connecting to rate limiter", "addr", grpcAddr)
	conn, err := grpc.NewClient(grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(1024*1024),
			grpc.MaxCallSendMsgSize(1024*1024),
		),
	)
	if err != nil {
		slog.Error("grpc dial failed", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	client := pb.NewRateLimiterClient(conn)

	slog.Info("registering with rate limiter")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	regResp, err := client.Register(ctx, &pb.RegisterRequest{
		Service: "payment-service",
		Apis: []*pb.APIConfig{
			{
				Path:          "/payment/process",
				Algorithm:     "sliding_window",
				KeyStrategy:   "header:X-Session-ID",
				Limit:         10,
				WindowSeconds: 300,
			},
			{
				Path:          "/payment/validate",
				Algorithm:     "token_bucket",
				KeyStrategy:   "ip",
				Limit:         50,
				WindowSeconds: 60,
				Burst:         10,
			},
		},
	})
	if err != nil {
		slog.Error("registration failed", "error", err)
		os.Exit(1)
	}
	if !regResp.Success {
		slog.Error("registration rejected", "message", regResp.Message)
		os.Exit(1)
	}
	slog.Info("registration successful", "message", regResp.Message)

	svc := &PaymentService{
		grpcClient: client,
		conn:       conn,
	}

	http.HandleFunc("/payment/process", svc.handleProcess)
	http.HandleFunc("/payment/validate", svc.handleValidate)

	server := &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("payment service starting", "port", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down payment service")
	
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
	slog.Info("shutdown complete")
}
