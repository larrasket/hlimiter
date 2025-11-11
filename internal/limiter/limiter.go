package limiter

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/larrasket/hlimiter/internal/config"
)

type CheckRequest struct {
	Service string            `json:"service"`
	API     string            `json:"api"`
	IP      string            `json:"ip"`
	Headers map[string]string `json:"headers"`
}

type CheckResponse struct {
	Allowed   bool  `json:"allowed"`
	Remaining int   `json:"remaining"`
	ResetAt   int64 `json:"reset_at"`
}

type RateLimiter struct {
	cfg   *config.Config
	store sync.Map
}

type slidingWindow struct {
	mu        sync.Mutex
	requests  []int64
	limit     int
	windowSec int64
}

type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	limit      float64
	refillRate float64
	burst      float64
	lastRefill time.Time
}

func New(cfg *config.Config) *RateLimiter {
	fmt.Printf("[limiter] initializing with %d services\n", len(cfg.Services))
	return &RateLimiter{cfg: cfg}
}

func (sw *slidingWindow) allow() (bool, int, int64) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now().Unix()
	cutoff := now - sw.windowSec

	var valid []int64
	for _, ts := range sw.requests {
		if ts > cutoff {
			valid = append(valid, ts)
		}
	}
	sw.requests = valid

	allowed := len(sw.requests) < sw.limit
	remaining := sw.limit - len(sw.requests)
	if allowed {
		sw.requests = append(sw.requests, now)
		remaining--
	}

	resetAt := now + sw.windowSec

	return allowed, remaining, resetAt
}

func (tb *tokenBucket) allow() (bool, int, int64) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()

	tb.tokens = min(tb.burst, tb.tokens+elapsed*tb.refillRate)
	tb.lastRefill = now

	allowed := tb.tokens >= 1
	if allowed {
		tb.tokens--
	}

	remaining := int(tb.tokens)
	needed := tb.burst - tb.tokens
	secsUntilFull := needed / tb.refillRate
	resetTime := now.Add(time.Duration(secsUntilFull * float64(time.Second))).Unix()

	return allowed, remaining, resetTime
}

func (rl *RateLimiter) buildKey(req CheckRequest, api config.API) string {
	strategy := api.KeyStrategy

	if strategy == "ip" {
		return req.Service + ":" + api.Path + ":ip:" + req.IP
	}

	if strings.HasPrefix(strategy, "header:") {
		headerName := strings.TrimPrefix(strategy, "header:")
		val := req.Headers[headerName]
		if len(val) > 256 {
			val = val[:256]
		}
		return req.Service + ":" + api.Path + ":header:" + headerName + ":" + val
	}

	return req.Service + ":" + api.Path + ":default"
}

func (rl *RateLimiter) Check(req CheckRequest) CheckResponse {
	fmt.Printf("[check] service=%s api=%s ip=%s\n", req.Service, req.API, req.IP)

	for _, svc := range rl.cfg.Services {
		if svc.Name != req.Service {
			continue
		}
		for _, api := range svc.APIs {
			if api.Path != req.API {
				continue
			}

			key := rl.buildKey(req, api)
			fmt.Printf("[check] algo=%s key=%s\n", api.Algorithm, key)

			if api.Algorithm == "sliding_window" {
				v, _ := rl.store.LoadOrStore(key, &slidingWindow{
					limit:     api.Limit,
					windowSec: int64(api.WindowSeconds),
				})
				sw := v.(*slidingWindow)
				allowed, remaining, reset := sw.allow()
				fmt.Printf("[check] sliding_window: allowed=%v remaining=%d\n", allowed, remaining)
				return CheckResponse{Allowed: allowed, Remaining: remaining, ResetAt: reset}
			}

			if api.Algorithm == "token_bucket" {
				burst := api.Burst
				if burst == 0 {
					burst = api.Limit
				}
				v, _ := rl.store.LoadOrStore(key, &tokenBucket{
					tokens:     float64(burst),
					limit:      float64(api.Limit),
					refillRate: float64(api.Limit) / float64(api.WindowSeconds),
					burst:      float64(burst),
					lastRefill: time.Now(),
				})
				tb := v.(*tokenBucket)
				allowed, remaining, reset := tb.allow()
				fmt.Printf("[check] token_bucket: allowed=%v remaining=%d\n", allowed, remaining)
				return CheckResponse{Allowed: allowed, Remaining: remaining, ResetAt: reset}
			}
		}
	}

	fmt.Printf("[check] no config found, defaulting to allow\n")
	return CheckResponse{Allowed: true, Remaining: -1}
}
