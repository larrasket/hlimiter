package limiter

import (
	"context"
	"log/slog"
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
	slog.Info("redis backend initialized")
	return &RedisRateLimiter{store: store}
}

func (rl *RedisRateLimiter) Register(ctx context.Context, serviceName string, apis []config.API) error {
	if err := rl.store.RegisterService(ctx, serviceName, apis); err != nil {
		return err
	}
	slog.Info("service registered", "service", serviceName, "apis", len(apis))
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
	slog.Debug("rate limit check", "service", req.Service, "api", req.API, "ip", req.IP)

	apis, err := rl.store.GetServiceConfig(ctx, req.Service)
	if err != nil {
		slog.Warn("service not registered, allowing request", "service", req.Service)
		return CheckResponse{Allowed: true, Remaining: -1}, nil
	}

	for _, api := range apis {
		if api.Path != req.API {
			continue
		}

		key := rl.buildKey(req, api)
		slog.Debug("checking rate limit", "algorithm", api.Algorithm, "key", key)

		if api.Algorithm == "sliding_window" {
			allowed, remaining, reset, err := rl.store.SlidingWindow(ctx, key, api.Limit, int64(api.WindowSeconds))
			if err != nil {
				slog.Error("sliding window check failed", "error", err, "key", key)
				return CheckResponse{}, err
			}
			slog.Info("rate limit check", "algorithm", "sliding_window", "allowed", allowed, "remaining", remaining)
			return CheckResponse{Allowed: allowed, Remaining: remaining, ResetAt: reset}, nil
		}

		if api.Algorithm == "token_bucket" {
			burst := api.Burst
			if burst == 0 {
				burst = api.Limit
			}
			allowed, remaining, reset, err := rl.store.TokenBucket(ctx, key, api.Limit, burst, int64(api.WindowSeconds))
			if err != nil {
				slog.Error("token bucket check failed", "error", err, "key", key)
				return CheckResponse{}, err
			}
			slog.Info("rate limit check", "algorithm", "token_bucket", "allowed", allowed, "remaining", remaining)
			return CheckResponse{Allowed: allowed, Remaining: remaining, ResetAt: reset}, nil
		}
	}

	slog.Warn("no api config found, allowing request", "api", req.API)
	return CheckResponse{Allowed: true, Remaining: -1}, nil
}
