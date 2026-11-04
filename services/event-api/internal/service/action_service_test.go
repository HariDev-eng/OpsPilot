package service

import (
	"testing"
)

func TestDetermineActionPolicy(t *testing.T) {
	tests := []struct {
		name             string
		actionType       string
		expectedStatus   string
		expectedApproval bool
		expectedError    bool
	}{
		{
			name:             "investigation does not require approval",
			actionType:       "INVESTIGATION",
			expectedStatus:   "PROPOSED",
			expectedApproval: false,
		},
		{
			name:             "mitigation requires approval",
			actionType:       "MITIGATION",
			expectedStatus:   "PENDING_APPROVAL",
			expectedApproval: true,
		},
		{
			name:             "destructive action requires approval",
			actionType:       "DESTRUCTIVE",
			expectedStatus:   "PENDING_APPROVAL",
			expectedApproval: true,
		},
		{
			name:          "unsupported action type",
			actionType:    "UNKNOWN",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, requiresApproval, err :=
				determineActionPolicy(tt.actionType)

			if tt.expectedError {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if status != tt.expectedStatus {
				t.Errorf(
					"expected status %s, got %s",
					tt.expectedStatus,
					status,
				)
			}

			if requiresApproval != tt.expectedApproval {
				t.Errorf(
					"expected requiresApproval %t, got %t",
					tt.expectedApproval,
					requiresApproval,
				)
			}
		})
	}
}

func TestGenerateActionIdempotencyKey(t *testing.T) {
	key1 := generateActionIdempotencyKey(
		11,
		0,
		"MITIGATION",
		"Restart payment service",
	)

	key2 := generateActionIdempotencyKey(
		11,
		0,
		"MITIGATION",
		"Restart payment service",
	)

	if key1 != key2 {
		t.Fatal("identical inputs should generate identical keys")
	}

	key3 := generateActionIdempotencyKey(
		12,
		0,
		"MITIGATION",
		"Restart payment service",
	)

	if key1 == key3 {
		t.Fatal("different incident IDs should generate different keys")
	}

	key4 := generateActionIdempotencyKey(
		11,
		1,
		"MITIGATION",
		"Restart payment service",
	)

	if key1 == key4 {
		t.Fatal("different recommendation indexes should generate different keys")
	}

	key5 := generateActionIdempotencyKey(
		11,
		0,
		"INVESTIGATION",
		"Restart payment service",
	)

	if key1 == key5 {
		t.Fatal("different action types should generate different keys")
	}
}
