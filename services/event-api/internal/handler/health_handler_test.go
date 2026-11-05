package handler

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestLiveDoesNotDependOnServices(t *testing.T) {
	h := NewHealthHandler(fakePinger{err: errors.New("down")}, fakePinger{err: errors.New("down")}, func(context.Context) error {
		return errors.New("down")
	})

	recorder := httptest.NewRecorder()
	h.Live(recorder, httptest.NewRequest("GET", "/health/live", nil))

	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
}

func TestReadyReturnsOKWhenDependenciesHealthy(t *testing.T) {
	h := NewHealthHandler(fakePinger{}, fakePinger{}, func(context.Context) error { return nil })
	recorder := httptest.NewRecorder()
	h.Ready(recorder, httptest.NewRequest("GET", "/health/ready", nil))

	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestReadyReturns503WhenDependencyUnavailable(t *testing.T) {
	h := NewHealthHandler(fakePinger{}, fakePinger{err: errors.New("redis unavailable")}, func(context.Context) error { return nil })
	recorder := httptest.NewRecorder()
	h.Ready(recorder, httptest.NewRequest("GET", "/health/ready", nil))

	if recorder.Code != 503 {
		t.Fatalf("expected 503, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
