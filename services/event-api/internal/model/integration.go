package model

import "time"

type Integration struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	ServiceName string     `json:"service_name"`
	KeyPrefix   string     `json:"key_prefix"`
	KeyHash     string     `json:"-"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	RevokedAt   *time.Time `json:"revoked_at"`
}
