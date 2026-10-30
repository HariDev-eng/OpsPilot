package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	client *redis.Client
}

func NewClient(address string) (*Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr: address,
	})

	ctx := context.Background()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()

		return nil, fmt.Errorf(
			"failed to connect to redis: %w",
			err,
		)
	}

	return &Client{
		client: client,
	}, nil
}

func (c *Client) Close() error {
	return c.client.Close()
}

var slidingWindowScript = redis.NewScript(`
local key = KEYS[1]
local event_id = ARGV[1]
local timestamp = tonumber(ARGV[2])
local cutoff = tonumber(ARGV[3])
local ttl_seconds = tonumber(ARGV[4])

redis.call("ZADD", key, timestamp, event_id)

redis.call(
    "ZREMRANGEBYSCORE",
    key,
    "-inf",
    cutoff
)

redis.call(
    "EXPIRE",
    key,
    ttl_seconds
)

return redis.call("ZCARD", key)
`)

func (c *Client) RecordAndCountRecentEvents(
	ctx context.Context,
	key string,
	eventID string,
	timestamp time.Time,
	window time.Duration,
) (int64, error) {

	now := timestamp.UnixMilli()
	cutoff := timestamp.Add(-window).UnixMilli()

	ttl := int64(window.Seconds()) + 5

	result, err := slidingWindowScript.Run(
		ctx,
		c.client,
		[]string{key},
		eventID,
		now,
		cutoff,
		ttl,
	).Int64()

	if err != nil {
		return 0, err
	}

	return result, nil
}
