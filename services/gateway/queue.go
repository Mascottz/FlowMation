package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"
)

type redisQueue struct { client *redis.Client }

func openQueue() *redisQueue {
	url := os.Getenv("REDIS_URL")
	if url == "" { url = "redis://localhost:6379/0" }
	options, err := redis.ParseURL(url)
	if err != nil { return nil }
	return &redisQueue{client: redis.NewClient(options)}
}

func (q *redisQueue) ping(ctx context.Context) error { return q.client.Ping(ctx).Err() }

func (q *redisQueue) enqueue(ctx context.Context, run Run, workflow Workflow) error {
	payload, err := json.Marshal(map[string]interface{}{"run": run, "workflow": workflow})
	if err != nil { return fmt.Errorf("encode queue payload: %w", err) }
	return q.client.RPush(ctx, "flowmation:runs", payload).Err()
}
