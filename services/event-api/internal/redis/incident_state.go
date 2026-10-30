package redis

import (
	"context"
	"encoding/json"
	"fmt"

	redisclient "github.com/redis/go-redis/v9"
)

type IncidentState struct {
	IncidentID int64  `json:"incident_id"`
	Service    string `json:"service"`
	Severity   string `json:"severity"`
	Status     string `json:"status"`
}

func incidentStateKey(incidentID int64) string {
	return fmt.Sprintf("incident:%d", incidentID)
}

func (c *Client) SetIncidentState(
	ctx context.Context,
	state IncidentState,
) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal incident state: %w", err)
	}

	key := incidentStateKey(state.IncidentID)

	return c.client.Set(
		ctx,
		key,
		data,
		0,
	).Err()
}

func (c *Client) GetIncidentState(
	ctx context.Context,
	incidentID int64,
) (*IncidentState, error) {
	key := incidentStateKey(incidentID)

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redisclient.Nil {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to get incident state: %w", err)
	}

	var state IncidentState

	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal incident state: %w", err)
	}

	return &state, nil
}

func (c *Client) DeleteIncidentState(
	ctx context.Context,
	incidentID int64,
) error {
	key := incidentStateKey(incidentID)

	return c.client.Del(ctx, key).Err()
}
