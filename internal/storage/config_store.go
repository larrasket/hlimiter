package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/larrasket/hlimiter/internal/config"
)

const configKeyPrefix = "rlconfig:"

func (r *RedisStore) RegisterService(ctx context.Context, serviceName string, apis []config.API) error {
	if serviceName == "" {
		return fmt.Errorf("service name cannot be empty")
	}
	
	for _, api := range apis {
		if api.Path == "" {
			return fmt.Errorf("api path cannot be empty")
		}
		if api.Algorithm != "sliding_window" && api.Algorithm != "token_bucket" {
			return fmt.Errorf("invalid algorithm: %s", api.Algorithm)
		}
		if api.Limit <= 0 {
			return fmt.Errorf("limit must be positive")
		}
		if api.WindowSeconds <= 0 {
			return fmt.Errorf("window_seconds must be positive")
		}
	}
	
	key := configKeyPrefix + serviceName
	
	data, err := json.Marshal(apis)
	if err != nil {
		return fmt.Errorf("marshal failed: %w", err)
	}

	return r.client.Set(ctx, key, data, 0).Err()
}

func (r *RedisStore) GetServiceConfig(ctx context.Context, serviceName string) ([]config.API, error) {
	key := configKeyPrefix + serviceName
	
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}

	var apis []config.API
	if err := json.Unmarshal(data, &apis); err != nil {
		return nil, fmt.Errorf("unmarshal failed: %w", err)
	}

	return apis, nil
}

func (r *RedisStore) GetAllServices(ctx context.Context) (map[string][]config.API, error) {
	result := make(map[string][]config.API)
	pattern := configKeyPrefix + "*"
	
	iter := r.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		serviceName := key[len(configKeyPrefix):]
		
		apis, err := r.GetServiceConfig(ctx, serviceName)
		if err != nil {
			continue
		}
		result[serviceName] = apis
	}
	
	if err := iter.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
