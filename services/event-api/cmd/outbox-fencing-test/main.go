package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/database"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
)

const testEventID = "outbox-fencing-test-001"

func run() error {
	ctx := context.Background()

	pool, err := database.NewPool()
	if err != nil {
		return err
	}
	defer pool.Close()

	repo := repository.NewOutboxRepository(pool)

	// Ensure this test owns only its dedicated record.
	_, err = pool.Exec(ctx, `
		DELETE FROM outbox
		WHERE event_id = $1
	`, testEventID)
	if err != nil {
		return err
	}

	// Insert a dedicated test record.
	err = pool.QueryRow(ctx, `
		INSERT INTO outbox (
			event_id, event_type, service, timestamp,
			topic, payload, status, attempts
		)
		VALUES (
			$1, 'payment.failed', 'payment-service', NOW(),
			'ops.events', '{"test": true}'::jsonb, 'PENDING', 0
		)
		RETURNING id
	`, testEventID).Scan(new(int64))
	if err != nil {
		return err
	}

	// Cleanup only our synthetic record.
	defer func() {
		_, cleanupErr := pool.Exec(
			context.Background(),
			`DELETE FROM outbox WHERE event_id = $1`,
			testEventID,
		)
		if cleanupErr != nil {
			log.Printf("test cleanup failed: %v", cleanupErr)
		}
	}()

	fmt.Println("1. Worker A claims the event")

	workerA, err := repo.ClaimPending(ctx, 1)
	if err != nil {
		return err
	}

	if len(workerA) != 1 || workerA[0].EventID != testEventID {
		return fmt.Errorf("worker A did not claim the test event")
	}

	tokenA := workerA[0].ClaimToken
	if tokenA == "" {
		return fmt.Errorf("worker A received an empty token")
	}

	fmt.Printf("   Worker A token: %s\n", tokenA)

	fmt.Println("2. Simulating an expired lease")

	_, err = pool.Exec(ctx, `
		UPDATE outbox
		SET claimed_at = NOW() - INTERVAL '10 minutes'
		WHERE event_id = $1
	`, testEventID)
	if err != nil {
		return err
	}

	recovered, err := repo.RecoverStalePublishing(
		ctx,
		5*time.Minute,
	)
	if err != nil {
		return err
	}

	if recovered < 1 {
		return fmt.Errorf("expected at least one recovered record, got %d", recovered)
	}

	var recoveredStatus string
	var recoveredToken *string

	err = pool.QueryRow(ctx, `
    SELECT status, claim_token::text
    FROM outbox
    WHERE event_id = $1
	`, testEventID).Scan(
		&recoveredStatus,
		&recoveredToken,
	)
	if err != nil {
		return err
	}

	if recoveredStatus != "PENDING" || recoveredToken != nil {
		return fmt.Errorf(
			"test event recovery failed: status=%s token=%v",
			recoveredStatus,
			recoveredToken,
		)
	}

	fmt.Println("   Record recovered")

	fmt.Println("3. Worker B claims the recovered event")

	workerB, err := repo.ClaimPending(ctx, 1)
	if err != nil {
		return err
	}

	if len(workerB) != 1 || workerB[0].EventID != testEventID {
		return fmt.Errorf("worker B did not claim the test event")
	}

	tokenB := workerB[0].ClaimToken

	if tokenB == "" || tokenA == tokenB {
		return fmt.Errorf("worker B did not receive a new token")
	}

	fmt.Printf("   Worker B token: %s\n", tokenB)

	fmt.Println("4. Worker A tries to mark the event published")

	err = repo.MarkPublished(ctx, workerA[0].ID, tokenA)

	if !errors.Is(err, repository.ErrOutboxClaimLost) {
		return fmt.Errorf(
			"expected ErrOutboxClaimLost, got %v",
			err,
		)
	}

	fmt.Println("   Correctly rejected stale worker A")

	fmt.Println("5. Worker A tries to mark the event failed")

	err = repo.MarkFailed(
		ctx,
		workerA[0].ID,
		tokenA,
		"simulated stale worker failure",
		time.Now().Add(time.Second),
	)

	if !errors.Is(err, repository.ErrOutboxClaimLost) {
		return fmt.Errorf(
			"expected ErrOutboxClaimLost, got %v",
			err,
		)
	}

	fmt.Println("   Correctly rejected stale failure update")

	fmt.Println("6. Worker B marks the event published")

	err = repo.MarkPublished(
		ctx,
		workerB[0].ID,
		tokenB,
	)
	if err != nil {
		return fmt.Errorf("worker B failed to publish: %w", err)
	}

	var status string
	var attempts int
	var claimToken *string

	err = pool.QueryRow(ctx, `
		SELECT status, attempts, claim_token::text
		FROM outbox
		WHERE event_id = $1
	`, testEventID).Scan(
		&status,
		&attempts,
		&claimToken,
	)
	if err != nil {
		return err
	}

	if status != "PUBLISHED" || attempts != 2 || claimToken != nil {
		return fmt.Errorf(
			"unexpected final state: status=%s attempts=%d token=%v",
			status,
			attempts,
			claimToken,
		)
	}

	fmt.Printf(
		"   Final state: %s, attempts: %d\n",
		status,
		attempts,
	)

	fmt.Println("\nALL OUTBOX FENCING TESTS PASSED")
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
