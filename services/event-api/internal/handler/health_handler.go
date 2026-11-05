package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

const healthCheckTimeout = time.Second

type dependencyChecker interface {
	Ping(context.Context) error
}

type HealthHandler struct {
	postgres dependencyChecker
	redis    dependencyChecker
	kafka    func(context.Context) error
}

type componentHealth struct {
	Status string `json:"status"`
}

type healthResponse struct {
	Status     string                     `json:"status"`
	Timestamp  time.Time                  `json:"timestamp"`
	Components map[string]componentHealth `json:"components"`
}

func NewHealthHandler(
	postgres dependencyChecker,
	redis dependencyChecker,
	kafkaCheck func(context.Context) error,
) *HealthHandler {
	return &HealthHandler{
		postgres: postgres,
		redis:    redis,
		kafka:    kafkaCheck,
	}
}

// Live reports whether the HTTP process is alive. It deliberately does not
// inspect dependencies, so a database outage does not cause restart loops.
func (h *HealthHandler) Live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
}

// Ready checks the dependencies required for the API to process events.
// It returns 503 if any required dependency cannot be reached.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	checks := map[string]func(context.Context) error{
		"postgres": h.postgres.Ping,
		"redis":    h.redis.Ping,
		"kafka":    h.kafka,
	}

	components := make(map[string]componentHealth, len(checks))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for name, check := range checks {
		name, check := name, check
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
			defer cancel()

			status := "healthy"
			if check == nil || check(ctx) != nil {
				status = "unhealthy"
			}

			mu.Lock()
			components[name] = componentHealth{Status: status}
			mu.Unlock()
		}()
	}
	wg.Wait()

	status := "healthy"
	for _, component := range components {
		if component.Status != "healthy" {
			status = "unhealthy"
			break
		}
	}

	response := healthResponse{
		Status:     status,
		Timestamp:  time.Now().UTC(),
		Components: components,
	}

	httpStatus := http.StatusOK
	if status != "healthy" {
		httpStatus = http.StatusServiceUnavailable
	}
	writeJSON(w, httpStatus, response)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
