package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type Client struct {
	client *goredis.Client
}

func NewClient(connection string) (*Client, error) {
	if connection == "" {
		return nil, fmt.Errorf("redis connection is required")
	}

	var options *goredis.Options
	var err error

	if strings.HasPrefix(connection, "redis://") ||
		strings.HasPrefix(connection, "rediss://") {

		options, err = goredis.ParseURL(connection)
		if err != nil {
			return nil, fmt.Errorf("invalid redis connection URL: %w", err)
		}
	} else {
		options = &goredis.Options{
			Addr: connection,
		}
	}

	client := goredis.NewClient(options)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &Client{client: client}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *Client) Close() error {
	return c.client.Close()
}

var slidingWindowScript = goredis.NewScript(`
local key = KEYS[1]
local event_id = ARGV[1]
local timestamp = tonumber(ARGV[2])
local cutoff = tonumber(ARGV[3])
local ttl_seconds = tonumber(ARGV[4])

redis.call("ZADD", key, timestamp, event_id)
redis.call("ZREMRANGEBYSCORE", key, "-inf", cutoff)
redis.call("EXPIRE", key, ttl_seconds)

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
		ctx, c.client, []string{key}, eventID, now, cutoff, ttl,
	).Int64()
	if err != nil {
		return 0, err
	}
	return result, nil
}

// RecordAndGetRecentEventIDs records the event and returns all event IDs
// currently inside the sliding failure window. The operation is atomic.
func (c *Client) RecordAndGetRecentEventIDs(
	ctx context.Context,
	key string,
	eventID string,
	timestamp time.Time,
	window time.Duration,
) ([]string, error) {
	now := timestamp.UnixMilli()
	cutoff := timestamp.Add(-window).UnixMilli()
	ttl := int64(window.Seconds()) + 5

	script := goredis.NewScript(`
		local key = KEYS[1]
		local event_id = ARGV[1]
		local timestamp = tonumber(ARGV[2])
		local cutoff = tonumber(ARGV[3])
		local ttl_seconds = tonumber(ARGV[4])

		redis.call("ZADD", key, timestamp, event_id)
		redis.call("ZREMRANGEBYSCORE", key, "-inf", cutoff)
		redis.call("EXPIRE", key, ttl_seconds)

		return redis.call("ZRANGEBYSCORE", key, "(" .. cutoff, "+inf")
	`)

	eventIDs, err := script.Run(
		ctx, c.client, []string{key}, eventID, now, cutoff, ttl,
	).StringSlice()
	if err != nil {
		return nil, err
	}
	return eventIDs, nil
}
