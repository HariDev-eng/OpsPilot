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

func (c *Client) RecordEvent(
	ctx context.Context,
	key string,
	eventID string,
	timestamp time.Time,
) error {

	return c.client.ZAdd(
		ctx,
		key,
		redis.Z{
			Score:  float64(timestamp.UnixMilli()),
			Member: eventID,
		},
	).Err()
}

func (c *Client) CountRecentEvents(
	ctx context.Context,
	key string,
	window time.Duration,
	now time.Time,
) (int64, error) {

	cutoff := now.Add(-window).UnixMilli()

	_, err := c.client.ZRemRangeByScore(
		ctx,
		key,
		"-inf",
		fmt.Sprintf("%d", cutoff),
	).Result()

	if err != nil {
		return 0, err
	}

	return c.client.ZCard(ctx, key).Result()
}
