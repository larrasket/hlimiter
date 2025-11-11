package limiter

import (
	"context"
	"fmt"
	"strings"

	"github.com/larrasket/hlimiter/internal/config"
	"github.com/larrasket/hlimiter/internal/storage"
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

type RedisRateLimiter struct {
	store *storage.RedisStore
}

func NewRedis(store *storage.RedisStore) *RedisRateLimiter {
	fmt.Printf("[limiter] redis backend initialized\n")
	return &RedisRateLimiter{store: store}
}

func (rl *RedisRateLimiter) Register(ctx context.Context, serviceName string, apis []config.API) error {
	if err := rl.store.RegisterService(ctx, serviceName, apis); err != nil {
		return err
	}
	fmt.Printf("[limiter] registered service: %s with %d APIs\n", serviceName, len(apis))
	return nil
}

func sanitizeKeyPart(s string) string {
	s = strings.ReplaceAll(s, ":", "_")
	s = strings.ReplaceAll(s, " ", "_")
	if len(s) > 256 {
		s = s[:256]
	}
	return s
}

func (rl *RedisRateLimiter) buildKey(req CheckRequest, api config.API) string {
	strategy := api.KeyStrategy
	service := sanitizeKeyPart(req.Service)
	path := sanitizeKeyPart(api.Path)

	if strategy == "ip" {
		ip := sanitizeKeyPart(req.IP)
		return service + ":" + path + ":ip:" + ip
	}

	if strings.HasPrefix(strategy, "header:") {
		headerName := strings.TrimPrefix(strategy, "header:")
		val := req.Headers[headerName]
		val = sanitizeKeyPart(val)
		return service + ":" + path + ":header:" + headerName + ":" + val
	}

	return service + ":" + path + ":default"
}

func (rl *RedisRateLimiter) Check(ctx context.Context, req CheckRequest) (CheckResponse, error) {
	fmt.Printf("[check] service=%s api=%s ip=%s\n", req.Service, req.API, req.IP)

	apis, err := rl.store.GetServiceConfig(ctx, req.Service)
	if err != nil {
		fmt.Printf("[check] service not registered: %s\n", req.Service)
		return CheckResponse{Allowed: true, Remaining: -1}, nil
	}

	for _, api := range apis {
		if api.Path != req.API {
			continue
		}

		key := rl.buildKey(req, api)
		fmt.Printf("[check] algo=%s key=%s\n", api.Algorithm, key)

		if api.Algorithm == "sliding_window" {
			allowed, remaining, reset, err := rl.store.SlidingWindow(ctx, key, api.Limit, int64(api.WindowSeconds))
			if err != nil {
				return CheckResponse{}, err
			}
			fmt.Printf("[check] sliding_window: allowed=%v remaining=%d\n", allowed, remaining)
			return CheckResponse{Allowed: allowed, Remaining: remaining, ResetAt: reset}, nil
		}

		if api.Algorithm == "token_bucket" {
			burst := api.Burst
			if burst == 0 {
				burst = api.Limit
			}
			allowed, remaining, reset, err := rl.store.TokenBucket(ctx, key, api.Limit, burst, int64(api.WindowSeconds))
			if err != nil {
				return CheckResponse{}, err
			}
			fmt.Printf("[check] token_bucket: allowed=%v remaining=%d\n", allowed, remaining)
			return CheckResponse{Allowed: allowed, Remaining: remaining, ResetAt: reset}, nil
		}
	}

	fmt.Printf("[check] no api config found for %s, defaulting to allow\n", req.API)
	return CheckResponse{Allowed: true, Remaining: -1}, nil
}
