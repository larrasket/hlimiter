package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/larrasket/hlimiter/internal/limiter"
)

type Handler struct {
	rl *limiter.RateLimiter
	// TODO: add request logging?
}

func New(rl *limiter.RateLimiter) *Handler {
	return &Handler{rl: rl}
}

func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	// only accept POST requests
	if r.Method != http.MethodPost {
		fmt.Printf("[handler] invalid method: %s\n", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req limiter.CheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// TODO: log the error details?
		fmt.Printf("[handler] failed to decode request: %v\n", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// fmt.Printf("[handler] received check request: %+v\n", req)

	// check rate limit
	resp := h.rl.Check(req)
	
	// always return json
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
