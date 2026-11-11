package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOperatorAuthMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		secret         string
		actorID        string
		authorization  string
		wantStatus     int
		wantActor      string
	}{
		{
			name:       "missing server secret fails closed",
			actorID:    "operator-1",
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "missing actor id fails closed",
			secret:     "test-secret",
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:          "missing authorization",
			secret:        "test-secret",
			actorID:       "operator-1",
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "wrong bearer token",
			secret:        "test-secret",
			actorID:       "operator-1",
			authorization: "Bearer wrong-secret",
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "wrong scheme",
			secret:        "test-secret",
			actorID:       "operator-1",
			authorization: "Basic test-secret",
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "bearer token containing whitespace rejected",
			secret:        "test-secret",
			actorID:       "operator-1",
			authorization: "Bearer test-secret extra",
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "valid bearer token",
			secret:        "test-secret",
			actorID:       "operator-1",
			authorization: "Bearer test-secret",
			wantStatus:    http.StatusNoContent,
			wantActor:     "operator-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if got := OperatorIDFromContext(r); got != tt.wantActor {
					t.Errorf("OperatorIDFromContext() = %q, want %q", got, tt.wantActor)
				}
				w.WriteHeader(http.StatusNoContent)
			})

			handler := OperatorAuthMiddleware(tt.secret, tt.actorID, next)
			request := httptest.NewRequest(http.MethodPost, "/actions/1/approve", nil)
			if tt.authorization != "" {
				request.Header.Set("Authorization", tt.authorization)
			}
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s",
					recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if called != (tt.wantStatus == http.StatusNoContent) {
				t.Fatalf("next handler called = %t, want %t",
					called, tt.wantStatus == http.StatusNoContent)
			}
		})
	}
}
