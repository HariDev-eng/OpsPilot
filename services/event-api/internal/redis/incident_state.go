package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type IncidentStateStore struct {
	client *redis.Client
}

func NewIncidentStateStore(client *redis.Client) *IncidentStateStore {
	return &IncidentStateStore{
		client: client,
	}
}

func (s *IncidentStateStore) RecordEvent(
	ctx context.Context,
	key string,
	eventID string,
	timestamp time.Time,
) error {

	return s.client.ZAdd(
		ctx,
		key,
		redis.Z{
			Score:  float64(timestamp.UnixMilli()),
			Member: eventID,
		},
	).Err()
}
