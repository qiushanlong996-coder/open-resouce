package main

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisAuthLimitPrefix = "open-resouce:auth-limit:"

var redisFixedWindowScript = redis.NewScript(`
local count = redis.call("INCR", KEYS[1])
if count == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
local ttl = redis.call("PTTL", KEYS[1])
return {count, ttl}
`)

type redisAuthLimiter struct {
	client redis.UniversalClient
}

func newRedisAuthLimiter(client redis.UniversalClient) *redisAuthLimiter {
	return &redisAuthLimiter{client: client}
}

func (limiter *redisAuthLimiter) Allow(
	ctx context.Context, key string, limit int, window time.Duration, _ time.Time,
) (bool, time.Duration, error) {
	result, err := redisFixedWindowScript.Run(
		ctx, limiter.client, []string{redisAuthLimitPrefix + key}, window.Milliseconds(),
	).Slice()
	if err != nil {
		return false, 0, fmt.Errorf("run fixed-window script: %w", err)
	}
	if len(result) != 2 {
		return false, 0, fmt.Errorf("unexpected fixed-window result length %d", len(result))
	}
	count, ok := result[0].(int64)
	if !ok {
		return false, 0, fmt.Errorf("unexpected fixed-window count type %T", result[0])
	}
	ttlMilliseconds, ok := result[1].(int64)
	if !ok {
		return false, 0, fmt.Errorf("unexpected fixed-window ttl type %T", result[1])
	}
	retryAfter := time.Duration(ttlMilliseconds) * time.Millisecond
	if retryAfter < 0 {
		retryAfter = window
	}
	return count <= int64(limit), retryAfter, nil
}
