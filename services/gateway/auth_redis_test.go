package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisAuthLimiterIntegration(t *testing.T) {
	redisURL := os.Getenv("REDIS_TEST_URL")
	if redisURL == "" {
		t.Skip("REDIS_TEST_URL is not configured")
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("parse Redis URL: %v", err)
	}
	client := redis.NewClient(options)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping Redis: %v", err)
	}

	key := "integration:" + newRequestID()
	defer client.Del(context.Background(), redisAuthLimitPrefix+key)
	limiter := newRedisAuthLimiter(client)
	for attempt := 1; attempt <= 2; attempt++ {
		allowed, retry, err := limiter.Allow(ctx, key, 2, time.Minute, time.Now())
		if err != nil || !allowed || retry <= 0 || retry > time.Minute {
			t.Fatalf("attempt %d: allowed=%v retry=%s err=%v", attempt, allowed, retry, err)
		}
	}
	allowed, retry, err := limiter.Allow(ctx, key, 2, time.Minute, time.Now())
	if err != nil || allowed || retry <= 0 || retry > time.Minute {
		t.Fatalf("limited attempt: allowed=%v retry=%s err=%v", allowed, retry, err)
	}
}
