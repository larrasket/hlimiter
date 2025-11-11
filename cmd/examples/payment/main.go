package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

type PaymentService struct {
	limiterURL string
	httpClient *http.Client
}

type rateLimitRequest struct {
	Service string            `json:"service"`
	API     string            `json:"api"`
	IP      string            `json:"ip"`
	Headers map[string]string `json:"headers"`
}

type rateLimitResponse struct {
	Allowed   bool  `json:"allowed"`
	Remaining int   `json:"remaining"`
	ResetAt   int64 `json:"reset_at"`
}

func (p *PaymentService) checkLimit(svc, path, ip string, hdrs map[string]string) (*rateLimitResponse, error) {
	reqBody := rateLimitRequest{
		Service: svc,
		API:     path,
		IP:      ip,
		Headers: hdrs,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Post(p.limiterURL+"/check", "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res rateLimitResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	return &res, nil
}

func (p *PaymentService) handleProcess(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	fmt.Printf("[payment/process] sess=%s\n", sessionID)

	result, err := p.checkLimit("payment-service", "/payment/process", r.RemoteAddr, map[string]string{"X-Session-ID": sessionID})
	if err != nil {
		log.Printf("limiter check failed: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if !result.Allowed {
		fmt.Printf("[payment/process] RATE LIMITED\n")
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", result.Remaining))
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

	result, err := p.checkLimit("payment-service", "/payment/validate", ip, nil)
	if err != nil {
		log.Printf("rate check error: %v", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}

	if !result.Allowed {
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", result.Remaining))
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}

	w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", result.Remaining))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{
		"valid": true,
	})
}

func main() {
	limiterURL := os.Getenv("LIMITER_URL")
	if limiterURL == "" {
		panic("LIMITER_URL environment variable is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		panic("PORT environment variable is required")
	}

	svc := &PaymentService{
		limiterURL: limiterURL,
		httpClient: &http.Client{},
	}

	http.HandleFunc("/payment/process", svc.handleProcess)
	http.HandleFunc("/payment/validate", svc.handleValidate)

	fmt.Printf("payment service on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
