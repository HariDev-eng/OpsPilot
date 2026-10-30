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

	count, err := client.RecordAndCountRecentEvents(
		ctx,
		key,
		"event-1",
		now.Add(-9*time.Second),
		10*time.Second,
	)
	if err != nil {
		t.Fatalf("failed to record event-1: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected 1 event, got %d", count)
	}

	count, err = client.RecordAndCountRecentEvents(
		ctx,
		key,
		"event-2",
		now.Add(-5*time.Second),
		10*time.Second,
	)
	if err != nil {
		t.Fatalf("failed to record event-2: %v", err)
	}

	if count != 2 {
		t.Fatalf("expected 2 events, got %d", count)
	}

	count, err = client.RecordAndCountRecentEvents(
		ctx,
		key,
		"event-3",
		now.Add(-2*time.Second),
		10*time.Second,
	)
	if err != nil {
		t.Fatalf("failed to record event-3: %v", err)
	}

	if count != 3 {
		t.Fatalf("expected 3 events, got %d", count)
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

	count, err := client.RecordAndCountRecentEvents(
		ctx,
		key,
		"old-event",
		now.Add(-20*time.Second),
		10*time.Second,
	)
	if err != nil {
		t.Fatalf("failed to record old event: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected 1 event after old event, got %d", count)
	}

	count, err = client.RecordAndCountRecentEvents(
		ctx,
		key,
		"recent-event",
		now.Add(-2*time.Second),
		10*time.Second,
	)
	if err != nil {
		t.Fatalf("failed to record recent event: %v", err)
	}

	if count != 1 {
		t.Fatalf(
			"expected 1 recent event, got %d",
			count,
		)
	}
}

func TestSlidingWindowDoesNotCountDuplicateEvent(t *testing.T) {
	ctx := context.Background()

	client, err := NewClient("localhost:6379")
	if err != nil {
		t.Fatalf("failed to connect to redis: %v", err)
	}

	defer client.Close()

	key := "test:payment-service:duplicates"

	t.Cleanup(func() {
		client.client.Del(ctx, key)
	})

	now := time.Now()

	count, err := client.RecordAndCountRecentEvents(
		ctx,
		key,
		"event-1",
		now,
		10*time.Second,
	)
	if err != nil {
		t.Fatalf("failed to record event-1: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected 1 event, got %d", count)
	}

	count, err = client.RecordAndCountRecentEvents(
		ctx,
		key,
		"event-1",
		now,
		10*time.Second,
	)
	if err != nil {
		t.Fatalf("failed to record duplicate event-1: %v", err)
	}

	if count != 1 {
		t.Fatalf(
			"expected duplicate event to keep count at 1, got %d",
			count,
		)
	}

	count, err = client.RecordAndCountRecentEvents(
		ctx,
		key,
		"event-2",
		now,
		10*time.Second,
	)
	if err != nil {
		t.Fatalf("failed to record event-2: %v", err)
	}

	if count != 2 {
		t.Fatalf(
			"expected 2 unique events, got %d",
			count,
		)
	}
}

func TestSlidingWindowSetsTTL(t *testing.T) {
	ctx := context.Background()

	client, err := NewClient("localhost:6379")
	if err != nil {
		t.Fatalf("failed to connect to redis: %v", err)
	}

	defer client.Close()

	key := "test:payment-service:ttl"

	t.Cleanup(func() {
		client.client.Del(ctx, key)
	})

	now := time.Now()

	_, err = client.RecordAndCountRecentEvents(
		ctx,
		key,
		"event-1",
		now,
		10*time.Second,
	)
	if err != nil {
		t.Fatalf("failed to record event: %v", err)
	}

	ttl, err := client.client.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("failed to read TTL: %v", err)
	}

	if ttl <= 0 {
		t.Fatalf(
			"expected positive TTL, got %v",
			ttl,
		)
	}

	if ttl > 15*time.Second {
		t.Fatalf(
			"expected TTL <= 15 seconds, got %v",
			ttl,
		)
	}
}
