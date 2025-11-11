package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/larrasket/hlimiter/internal/config"
)

const configKeyPrefix = "rlconfig:"

func (r *RedisStore) RegisterService(ctx context.Context, serviceName string, apis []config.API) error {
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
	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pattern := configKeyPrefix + "*"
	keys, err := r.client.Keys(ctxTimeout, pattern).Result()
	if err != nil {
		return nil, err
	}

	result := make(map[string][]config.API)
	for _, key := range keys {
		serviceName := key[len(configKeyPrefix):]
		apis, err := r.GetServiceConfig(ctx, serviceName)
		if err != nil {
			continue
		}
		result[serviceName] = apis
	}

	return result, nil
}
