package handler

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
)

type operatorContextKey struct{}

// OperatorAuthMiddleware protects an operator-only route with a shared
// bearer secret and places the configured audit actor label in the request
// context. This is a single-operator V1 gate, not multi-user authentication
// or role-based authorization.
func OperatorAuthMiddleware(
	secret string,
	actorID string,
	next http.Handler,
) http.Handler {
	secret = strings.TrimSpace(secret)
	actorID = strings.TrimSpace(actorID)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if secret == "" || actorID == "" {
			http.Error(
				w,
				"operator authentication is not configured",
				http.StatusServiceUnavailable,
			)
			return
		}

		scheme, token, found := strings.Cut(
			strings.TrimSpace(r.Header.Get("Authorization")),
			" ",
		)
		token = strings.TrimSpace(token)

		if !found ||
			!strings.EqualFold(scheme, "Bearer") ||
			token == "" ||
			strings.ContainsAny(token, " \t\r\n") ||
			subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), operatorContextKey{}, actorID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OperatorIDFromContext returns the audit actor assigned by
// OperatorAuthMiddleware. Empty means the request was not authenticated
// through that middleware.
func OperatorIDFromContext(r *http.Request) string {
	actorID, _ := r.Context().Value(operatorContextKey{}).(string)
	return actorID
}
