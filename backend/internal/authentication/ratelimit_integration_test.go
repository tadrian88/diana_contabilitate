//go:build integration

package authentication

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestIntegrationRedisLoginRateLimitIsShared(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(options)
	defer client.Close()
	h := NewHTTP(nil, client, Config{LoginLimit: 2, LoginWindow: time.Minute}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	key := "diana:auth:integration:" + time.Now().Format("20060102150405.000000000")
	defer client.Del(context.Background(), key)
	if !h.allowLogin(context.Background(), key) || !h.allowLogin(context.Background(), key) || h.allowLogin(context.Background(), key) {
		t.Fatal("Redis limit did not stop the third attempt")
	}
}
