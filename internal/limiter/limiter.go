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
	// TODO: add metrics/stats collection
}

type slidingWindow struct {
	mu        sync.Mutex
	requests  []int64 // storing timestamps
	limit     int
	windowSec int64
}

type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	limit      float64
	refillRate float64 // tokens per second
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

	var validReqs []int64
	for _, ts := range sw.requests {
		if ts > cutoff {
			validReqs = append(validReqs, ts)
		}
	}
	sw.requests = validReqs

	ok := len(sw.requests) < sw.limit
	rem := sw.limit - len(sw.requests)
	if ok {
		sw.requests = append(sw.requests, now)
		rem--
	}

	resetAt := now + sw.windowSec

	return ok, rem, resetAt
}

func (tb *tokenBucket) allow() (bool, int, int64) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()

	tb.tokens = min(tb.burst, tb.tokens+elapsed*tb.refillRate)
	tb.lastRefill = now

	ok := tb.tokens >= 1
	if ok {
		tb.tokens--
	}

	rem := int(tb.tokens)
	
	tokensNeeded := tb.burst - tb.tokens
	secondsUntilFull := tokensNeeded / tb.refillRate
	resetTime := now.Add(time.Duration(secondsUntilFull * float64(time.Second))).Unix()

	return ok, rem, resetTime
}

func (rl *RateLimiter) extractKey(req CheckRequest, api config.API) string {
	strat := api.KeyStrategy

	if strat == "ip" {
		return req.Service + ":" + api.Path + ":ip:" + req.IP
	}

	if strings.HasPrefix(strat, "header:") {
		hdrName := strings.TrimPrefix(strat, "header:")
		hdrVal := req.Headers[hdrName]
		if len(hdrVal) > 256 {
			hdrVal = hdrVal[:256]
		}
		return req.Service + ":" + api.Path + ":header:" + hdrName + ":" + hdrVal
	}

	return req.Service + ":" + api.Path + ":default"
}

func (rl *RateLimiter) Check(req CheckRequest) CheckResponse {
	fmt.Printf("[check] service=%s api=%s ip=%s\n", req.Service, req.API, req.IP)

	// find the matching service and api config
	// TODO: could optimize this lookup with a map instead of nested loops
	for _, svc := range rl.cfg.Services {
		if svc.Name != req.Service {
			continue
		}
		for _, api := range svc.APIs {
			if api.Path != req.API {
				continue
			}

			key := rl.extractKey(req, api)
			fmt.Printf("[check] using algorithm=%s key=%s\n", api.Algorithm, key)

			// handle sliding window algorithm
			if api.Algorithm == "sliding_window" {
				v, _ := rl.store.LoadOrStore(key, &slidingWindow{
					limit:     api.Limit,
					windowSec: int64(api.WindowSeconds),
				})
				sw := v.(*slidingWindow)
				ok, rem, reset := sw.allow()
				fmt.Printf("[check] sliding_window result: allowed=%v remaining=%d\n", ok, rem)
				return CheckResponse{Allowed: ok, Remaining: rem, ResetAt: reset}
			}

			// handle token bucket algorithm
			if api.Algorithm == "token_bucket" {
				burstSize := api.Burst
				if burstSize == 0 {
					burstSize = api.Limit // default burst = limit
				}
				v, _ := rl.store.LoadOrStore(key, &tokenBucket{
					tokens:     float64(burstSize),
					limit:      float64(api.Limit),
					refillRate: float64(api.Limit) / float64(api.WindowSeconds),
					burst:      float64(burstSize),
					lastRefill: time.Now(),
				})
				tb := v.(*tokenBucket)
				ok, rem, reset := tb.allow()
				fmt.Printf("[check] token_bucket result: allowed=%v remaining=%d\n", ok, rem)
				return CheckResponse{Allowed: ok, Remaining: rem, ResetAt: reset}
			}

			// TODO: add leaky bucket algorithm?
		}
	}

	// if service/api not found, allow by default
	// TODO: maybe we should deny unknown services instead?
	fmt.Printf("[check] service/api not found, allowing by default\n")
	return CheckResponse{Allowed: true, Remaining: -1}
}
