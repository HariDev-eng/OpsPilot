package service

import "github.com/HariDev-eng/opspilot/services/event-api/internal/model"

func IsFailureEvent(event model.Event) bool {
	switch event.EventType {
	case
		"payment.failed",
		"payment.error",
		"database.error",
		"database.timeout",
		"service.error",
		"service.timeout",
		"http.5xx":
		return true

	default:
		return false
	}
}
