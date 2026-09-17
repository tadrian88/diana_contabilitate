package main

import (
	"context"
	"errors"
	"github.com/redis/go-redis/v9"
	"testing"
)

type databaseProbe struct{ err error }

func (p databaseProbe) Ping(context.Context) error { return p.err }
func TestReadinessFailsForDatabaseOrRedisUnavailable(t *testing.T) {
	failure := errors.New("database unavailable")
	if err := (dependencyReadiness{database: databaseProbe{failure}}).Ping(context.Background()); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0", MaxRetries: -1})
	defer client.Close()
	if err := (dependencyReadiness{database: databaseProbe{}, redis: client}).Ping(context.Background()); err == nil {
		t.Fatal("unavailable Redis accepted")
	}
}
