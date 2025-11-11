package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client *redis.Client
}

func NewRedis(addr, password string, db, poolSize int) (*RedisStore, error) {
	if poolSize <= 0 {
		poolSize = 100
	}
	
	client := redis.NewClient(&redis.Options{
		Addr:            addr,
		Password:        password,
		DB:              db,
		PoolSize:        poolSize,
		MinIdleConns:    10,
		MaxRetries:      3,
		DialTimeout:     2 * time.Second,
		ReadTimeout:     1 * time.Second,
		WriteTimeout:    1 * time.Second,
		PoolTimeout:     2 * time.Second,
		ConnMaxIdleTime: 5 * time.Minute,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &RedisStore{client: client}, nil
}

var slidingWindowScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local reqid = ARGV[4]
local cutoff = now - window

redis.call('ZREMRANGEBYSCORE', key, 0, cutoff)

local count = redis.call('ZCARD', key)
if count < limit then
	redis.call('ZADD', key, now, reqid)
	redis.call('EXPIRE', key, math.ceil(window * 1.5))
	return {1, limit - count - 1, now + window}
else
	redis.call('EXPIRE', key, math.ceil(window * 1.5))
	return {0, 0, now + window}
end
`)

func (r *RedisStore) SlidingWindow(ctx context.Context, key string, limit int, window int64) (bool, int, int64, error) {
	now := time.Now()
	nowUnix := now.Unix()
	reqID := fmt.Sprintf("%d:%d", nowUnix, now.UnixNano())
	
	result, err := slidingWindowScript.Run(ctx, r.client, []string{key}, nowUnix, window, limit, reqID).Int64Slice()
	if err != nil {
		return false, 0, 0, err
	}

	allowed := result[0] == 1
	remaining := int(result[1])
	resetAt := result[2]

	return allowed, remaining, resetAt, nil
}

var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local burst = tonumber(ARGV[3])
local window = tonumber(ARGV[4])

local bucket = redis.call('HMGET', key, 'tokens', 'last')
local tokens = tonumber(bucket[1])
local last = tonumber(bucket[2])

if tokens == nil then
	tokens = burst
	last = now
else
	local elapsed = now - last
	tokens = math.min(burst, tokens + elapsed * rate)
end

local allowed = 0
local remaining = math.floor(tokens)
if tokens >= 1 then
	tokens = tokens - 1
	allowed = 1
	remaining = math.floor(tokens)
end

redis.call('HMSET', key, 'tokens', tokens, 'last', now)
redis.call('EXPIRE', key, math.ceil(window * 1.5))

local needed = burst - tokens
local secs_until_full = needed / rate
local reset_at = now + math.ceil(secs_until_full)

return {allowed, remaining, reset_at}
`)

func (r *RedisStore) TokenBucket(ctx context.Context, key string, limit, burst int, window int64) (bool, int, int64, error) {
	now := time.Now().Unix()
	rate := float64(limit) / float64(window)

	result, err := tokenBucketScript.Run(ctx, r.client, []string{key}, now, rate, burst, window).Int64Slice()
	if err != nil {
		return false, 0, 0, err
	}

	allowed := result[0] == 1
	remaining := int(result[1])
	resetAt := result[2]

	return allowed, remaining, resetAt, nil
}

func (r *RedisStore) Close() error {
	return r.client.Close()
}
