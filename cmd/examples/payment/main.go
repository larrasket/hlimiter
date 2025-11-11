package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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

	fmt.Printf("[payment/process] sess=%s\n", sessionID)

	allowed, remaining, _, err := p.checkLimit("payment-service", "/payment/process", r.RemoteAddr, map[string]string{"X-Session-ID": sessionID})
	if err != nil {
		log.Printf("limiter check failed: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if !allowed {
		fmt.Printf("[payment/process] RATE LIMITED\n")
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}

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
		log.Printf("rate check error: %v", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}

	if !allowed {
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}

	w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{
		"valid": true,
	})
}

func main() {
	grpcAddr := os.Getenv("LIMITER_GRPC_ADDR")
	if grpcAddr == "" {
		panic("LIMITER_GRPC_ADDR environment variable is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		panic("PORT environment variable is required")
	}

	fmt.Printf("[grpc] connecting to %s\n", grpcAddr)
	conn, err := grpc.NewClient(grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(1024*1024),
			grpc.MaxCallSendMsgSize(1024*1024),
		),
	)
	if err != nil {
		log.Fatalf("grpc dial failed: %v", err)
	}
	defer conn.Close()

	client := pb.NewRateLimiterClient(conn)

	fmt.Printf("[register] registering with rate limiter\n")
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
		log.Fatalf("registration failed: %v", err)
	}
	if !regResp.Success {
		log.Fatalf("registration failed: %s", regResp.Message)
	}
	fmt.Printf("[register] %s\n", regResp.Message)

	svc := &PaymentService{
		grpcClient: client,
		conn:       conn,
	}

	http.HandleFunc("/payment/process", svc.handleProcess)
	http.HandleFunc("/payment/validate", svc.handleValidate)

	fmt.Printf("payment service on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
