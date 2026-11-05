package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/database"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
)

func run() error {
	ctx := context.Background()

	pool, err := database.NewPool()
	if err != nil {
		return err
	}
	defer pool.Close()

	repo := repository.NewOutboxRepository(pool)

	fmt.Println("Starting outbox recovery test...")

	recovered, err := repo.RecoverStalePublishing(
		ctx,
		5*time.Minute,
	)
	if err != nil {
		return err
	}

	fmt.Printf("Recovered records: %d\n", recovered)

	var status string
	var claimIsNull bool

	err = pool.QueryRow(
		ctx,
		`
		SELECT status, claimed_at IS NULL
		FROM outbox
		WHERE id = $1
		`,
		int64(122),
	).Scan(&status, &claimIsNull)

	if err != nil {
		return err
	}

	fmt.Printf("Record status: %s\n", status)
	fmt.Printf("Claim timestamp cleared: %t\n", claimIsNull)

	if recovered != 1 {
		return fmt.Errorf("expected 1 recovered record, got %d", recovered)
	}

	if status != "PENDING" || !claimIsNull {
		return fmt.Errorf("recovery verification failed")
	}

	fmt.Println("OUTBOX RECOVERY TEST PASSED")
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
