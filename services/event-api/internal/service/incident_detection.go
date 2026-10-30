package service

import (
	"context"
	"time"

	"github.com/HariDev-eng/opspilot/services/event-api/internal/model"
	redisStore "github.com/HariDev-eng/opspilot/services/event-api/internal/redis"
)

type IncidentDetectionResult struct {
	Triggered bool
	Count     int64
	Threshold int64
	Severity  string
}

const (
	failureWindow    = 10 * time.Second
	failureThreshold = 5
)

type IncidentDetector struct {
	redis *redisStore.Client
}

func NewIncidentDetector(
	redisClient *redisStore.Client,
) *IncidentDetector {
	return &IncidentDetector{
		redis: redisClient,
	}
}

func (d *IncidentDetector) Detect(
	ctx context.Context,
	event model.Event,
) (IncidentDetectionResult, error) {

	if !IsFailureEvent(event) {
		return IncidentDetectionResult{
			Triggered: false,
		}, nil
	}

	key := "incident:failures:" + event.Service

	if err := d.redis.RecordEvent(
		ctx,
		key,
		event.EventID,
		event.Timestamp,
	); err != nil {
		return IncidentDetectionResult{}, err
	}

	count, err := d.redis.CountRecentEvents(
		ctx,
		key,
		failureWindow,
		event.Timestamp,
	)
	if err != nil {
		return IncidentDetectionResult{}, err
	}

	severity := CalculateSeverity(count)

	return IncidentDetectionResult{
		Triggered: count >= failureThreshold,
		Count:     count,
		Threshold: failureThreshold,
		Severity:  severity,
	}, nil
}
