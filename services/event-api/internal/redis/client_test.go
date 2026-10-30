package redis

import (
	"context"
	"testing"
	"time"
)

func TestSlidingWindow(t *testing.T) {

	ctx := context.Background()

	client, err := NewClient("localhost:6379")
	if err != nil {
		t.Fatalf("failed to connect to redis: %v", err)
	}

	defer client.Close()

	key := "test:payment-service:errors"

	t.Cleanup(func() {
		client.client.Del(ctx, key)
	})

	now := time.Now()

	err = client.RecordEvent(
		ctx,
		key,
		"event-1",
		now.Add(-9*time.Second),
	)
	if err != nil {
		t.Fatalf("failed to record event-1: %v", err)
	}

	err = client.RecordEvent(
		ctx,
		key,
		"event-2",
		now.Add(-5*time.Second),
	)
	if err != nil {
		t.Fatalf("failed to record event-2: %v", err)
	}

	err = client.RecordEvent(
		ctx,
		key,
		"event-3",
		now.Add(-2*time.Second),
	)
	if err != nil {
		t.Fatalf("failed to record event-3: %v", err)
	}

	count, err := client.CountRecentEvents(
		ctx,
		key,
		10*time.Second,
		now,
	)

	if err != nil {
		t.Fatalf("failed to count recent events: %v", err)
	}

	if count != 3 {
		t.Fatalf(
			"expected 3 events, got %d",
			count,
		)
	}
}

func TestSlidingWindowRemovesOldEvents(t *testing.T) {

	ctx := context.Background()

	client, err := NewClient("localhost:6379")
	if err != nil {
		t.Fatalf("failed to connect to redis: %v", err)
	}

	defer client.Close()

	key := "test:payment-service:old-events"

	t.Cleanup(func() {
		client.client.Del(ctx, key)
	})

	now := time.Now()

	err = client.RecordEvent(
		ctx,
		key,
		"old-event",
		now.Add(-20*time.Second),
	)
	if err != nil {
		t.Fatalf("failed to record old event: %v", err)
	}

	err = client.RecordEvent(
		ctx,
		key,
		"recent-event",
		now.Add(-2*time.Second),
	)
	if err != nil {
		t.Fatalf("failed to record recent event: %v", err)
	}

	count, err := client.CountRecentEvents(
		ctx,
		key,
		10*time.Second,
		now,
	)

	if err != nil {
		t.Fatalf("failed to count recent events: %v", err)
	}

	if count != 1 {
		t.Fatalf(
			"expected 1 recent event, got %d",
			count,
		)
	}
}
