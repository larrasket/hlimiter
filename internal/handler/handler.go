package handler

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/larrasket/hlimiter/internal/limiter"
)

type Handler struct {
	rl *limiter.RateLimiter
}

func New(rl *limiter.RateLimiter) *Handler {
	return &Handler{rl: rl}
}

func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fmt.Printf("[handler] bad method: %s\n", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req limiter.CheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fmt.Printf("[handler] decode err: %v\n", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if req.IP != "" {
		if host, _, err := net.SplitHostPort(req.IP); err == nil {
			req.IP = host
		}
	}

	result := h.rl.Check(req)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
