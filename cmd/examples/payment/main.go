package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/larrasket/hlimiter/pkg/client"
)

type PaymentService struct {
	limiterClient *client.Client
	// TODO: add database connection
}

func (p *PaymentService) handleProcess(w http.ResponseWriter, r *http.Request) {
	sessID := r.Header.Get("X-Session-ID")
	if sessID == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}

	fmt.Printf("[payment/process] session=%s\n", sessID)

	res, err := p.limiterClient.Check(client.CheckRequest{
		Service: "payment-service",
		API:     "/payment/process",
		IP:      r.RemoteAddr,
		Headers: map[string]string{"X-Session-ID": sessID},
	})
	if err != nil {
		log.Printf("rate limit check error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if !res.Allowed {
		fmt.Printf("[payment/process] rate limited!\n")
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", res.Remaining))
		http.Error(w, "too many payment attempts", http.StatusTooManyRequests)
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
	clientIP := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		clientIP = fwd
	}

	res, err := p.limiterClient.Check(client.CheckRequest{
		Service: "payment-service",
		API:     "/payment/validate",
		IP:      clientIP,
	})
	if err != nil {
		log.Printf("limiter err: %v", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}

	if !res.Allowed {
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", res.Remaining))
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}

	w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", res.Remaining))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{
		"valid": true,
	})
}

func main() {
	limiterUrl := os.Getenv("LIMITER_URL")
	if limiterUrl == "" {
		panic("LIMITER_URL environment variable is required")
	}

	svc := &PaymentService{
		limiterClient: client.New(limiterUrl),
	}

	http.HandleFunc("/payment/process", svc.handleProcess)
	http.HandleFunc("/payment/validate", svc.handleValidate)

	port := os.Getenv("PORT")
	if port == "" {
		panic("PORT environment variable is required")
	}

	fmt.Printf("payment service on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
