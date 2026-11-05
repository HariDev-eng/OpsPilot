package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/database"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/redis"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/repository"
	"github.com/HariDev-eng/opspilot/services/event-api/internal/service"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Connect to PostgreSQL.
	pool, err := database.NewPool()
	if err != nil {
		log.Fatalf("PostgreSQL connection failed: %v", err)
	}
	defer pool.Close()

	// Connect to Redis.
	redisClient, err := redis.NewClient("localhost:6379")
	if err != nil {
		log.Fatalf("Redis connection failed: %v", err)
	}
	defer redisClient.Close()

	// Initialize repositories and services.
	eventRepository := repository.NewEventRepository(pool)
	processedRepository := repository.NewProcessedEventRepository(pool)
	incidentRepository := repository.NewIncidentRepository(pool)

	detector := service.NewIncidentDetector(redisClient)

	processor := service.NewIncidentProcessor(
		pool,
		processedRepository,
		incidentRepository,
		detector,
		redisClient,
	)

	// Generate unique test identifiers.
	serviceName := fmt.Sprintf(
		"incident-window-test-%d",
		time.Now().UnixNano(),
	)

	eventIDs := make([]string, 0, 5)

	// Clean up only this test's database records.
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cleanupCancel()

		_, err := pool.Exec(
			cleanupCtx,
			`DELETE FROM incidents WHERE service = $1`,
			serviceName,
		)
		if err != nil {
			log.Printf("Incident cleanup failed: %v", err)
		}

		_, err = pool.Exec(
			cleanupCtx,
			`DELETE FROM processed_events
			 WHERE event_id = ANY($1::text[])`,
			eventIDs,
		)
		if err != nil {
			log.Printf("Processed event cleanup failed: %v", err)
		}

		_, err = pool.Exec(
			cleanupCtx,
			`DELETE FROM outbox
			 WHERE event_id = ANY($1::text[])`,
			eventIDs,
		)
		if err != nil {
			log.Printf("Outbox cleanup failed: %v", err)
		}

		_, err = pool.Exec(
			cleanupCtx,
			`DELETE FROM events WHERE service = $1`,
			serviceName,
		)
		if err != nil {
			log.Printf("Event cleanup failed: %v", err)
		}
	}()

	fmt.Println("Starting incident window integration test...")
	fmt.Println("Service:", serviceName)

	// Create five failure events inside a 10-second window.
	baseTime := time.Now().UTC()

	for i := 0; i < 5; i++ {
		eventID := fmt.Sprintf("%s-event-%d", serviceName, i+1)

		event := model.Event{
			EventID:   eventID,
			EventType: "payment.failed",
			Service:   serviceName,
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			Payload:   []byte(`{"message":"Payment failure"}`),
		}

		// Persist event and outbox record.
		if err := eventRepository.Create(ctx, event); err != nil {
			log.Fatalf("Event creation failed: %v", err)
		}

		eventIDs = append(eventIDs, eventID)

		// Process event through the actual incident processor.
		if err := processor.Process(ctx, event); err != nil {
			log.Fatalf(
				"Processing event %s failed: %v",
				eventID,
				err,
			)
		}

		fmt.Printf("Processed event %d/5: %s\n", i+1, eventID)
	}

	// Verify exactly one incident exists.
	var incidentCount int

	err = pool.QueryRow(
		ctx,
		`SELECT COUNT(*)
		 FROM incidents
		 WHERE service = $1`,
		serviceName,
	).Scan(&incidentCount)

	if err != nil {
		log.Fatalf("Incident count query failed: %v", err)
	}

	if incidentCount != 1 {
		log.Fatalf(
			"Expected 1 incident, got %d",
			incidentCount,
		)
	}

	// Verify severity and attached event count.
	var incidentID int64
	var severity string
	var attachedCount int

	err = pool.QueryRow(
		ctx,
		`
		SELECT
			i.id,
			i.severity,
			COUNT(ie.event_id)
		FROM incidents i
		LEFT JOIN incident_events ie
			ON ie.incident_id = i.id
		WHERE i.service = $1
		GROUP BY i.id, i.severity
		`,
		serviceName,
	).Scan(
		&incidentID,
		&severity,
		&attachedCount,
	)

	if err != nil {
		log.Fatalf("Incident verification failed: %v", err)
	}

	if severity != "HIGH" {
		log.Fatalf("Expected HIGH severity, got %s", severity)
	}

	if attachedCount != 5 {
		log.Fatalf(
			"Expected 5 attached events, got %d",
			attachedCount,
		)
	}

	fmt.Println("\nIncident verification passed:")
	fmt.Println("Incident ID:", incidentID)
	fmt.Println("Severity:", severity)
	fmt.Println("Attached events:", attachedCount)

	// Verify that all five expected event IDs are attached.
	rows, err := pool.Query(
		ctx,
		`
		SELECT ie.event_id
		FROM incident_events ie
		WHERE ie.incident_id = $1
		`,
		incidentID,
	)
	if err != nil {
		log.Fatalf("Attached event query failed: %v", err)
	}

	foundEvents := make(map[string]bool)

	for rows.Next() {
		var eventID string

		if err := rows.Scan(&eventID); err != nil {
			rows.Close()
			log.Fatalf("Event scan failed: %v", err)
		}

		foundEvents[eventID] = true
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		log.Fatalf("Event iteration failed: %v", err)
	}
	rows.Close()

	for _, eventID := range eventIDs {
		if !foundEvents[eventID] {
			log.Fatalf("Missing attached event: %s", eventID)
		}
	}

	fmt.Println("All five event IDs verified.")

	// Reprocess the triggering event to verify idempotency.
	if err := processor.Process(ctx, model.Event{
		EventID:   eventIDs[4],
		EventType: "payment.failed",
		Service:   serviceName,
		Timestamp: baseTime.Add(4 * time.Second),
		Payload:   []byte(`{"message":"Payment failure"}`),
	}); err != nil {
		log.Fatalf("Duplicate processing failed: %v", err)
	}

	var finalIncidentCount int
	var finalAttachedCount int

	err = pool.QueryRow(
		ctx,
		`
		SELECT
			COUNT(DISTINCT i.id),
			COUNT(ie.event_id)
		FROM incidents i
		LEFT JOIN incident_events ie
			ON ie.incident_id = i.id
		WHERE i.service = $1
		`,
		serviceName,
	).Scan(
		&finalIncidentCount,
		&finalAttachedCount,
	)

	if err != nil {
		log.Fatalf("Final verification failed: %v", err)
	}

	if finalIncidentCount != 1 || finalAttachedCount != 5 {
		log.Fatalf(
			"Idempotency failed: incidents=%d attached_events=%d",
			finalIncidentCount,
			finalAttachedCount,
		)
	}

	fmt.Println("\nIdempotency verification passed.")
	fmt.Println("No duplicate incident or event associations created.")
	fmt.Println("\nALL INCIDENT WINDOW TESTS PASSED!")
}
