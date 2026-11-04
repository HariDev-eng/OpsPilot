package service

import (
	"errors"
	"testing"
)

func TestLookupOperationPolicy(t *testing.T) {
	tests := []struct {
		name             string
		operationKey     string
		expectedType     string
		expectedApproval bool
		expectedError    bool
	}{
		{
			name:             "inspect payment logs",
			operationKey:     OperationInspectPaymentLogs,
			expectedType:     "INVESTIGATION",
			expectedApproval: false,
		},
		{
			name:             "restart payment service",
			operationKey:     OperationRestartPayment,
			expectedType:     "MITIGATION",
			expectedApproval: true,
		},
		{
			name:          "unregistered operation",
			operationKey:  "database.drop",
			expectedError: true,
		},
		{
			name:          "empty operation",
			operationKey:  "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := LookupOperationPolicy(
				tt.operationKey,
			)

			if tt.expectedError {
				if !errors.Is(err, ErrOperationNotAllowed) {
					t.Fatalf(
						"expected ErrOperationNotAllowed, got %v",
						err,
					)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if policy.ActionType != tt.expectedType {
				t.Errorf(
					"expected action type %s, got %s",
					tt.expectedType,
					policy.ActionType,
				)
			}

			if policy.RequiresApproval != tt.expectedApproval {
				t.Errorf(
					"expected approval %t, got %t",
					tt.expectedApproval,
					policy.RequiresApproval,
				)
			}

			if policy.OperationKey != tt.operationKey {
				t.Errorf(
					"expected operation key %s, got %s",
					tt.operationKey,
					policy.OperationKey,
				)
			}
		})
	}
}

func TestValidateOperationParameters(t *testing.T) {
	tests := []struct {
		name          string
		operationKey  string
		parameters    map[string]any
		expectedError error
	}{
		{
			name:         "inspect logs with no parameters",
			operationKey: OperationInspectPaymentLogs,
		},
		{
			name:         "restart payment with no parameters",
			operationKey: OperationRestartPayment,
		},
		{
			name:         "inspect logs with unexpected parameters",
			operationKey: OperationInspectPaymentLogs,
			parameters: map[string]any{
				"command": "delete logs",
			},
			expectedError: ErrInvalidOperationParameters,
		},
		{
			name:         "restart with unexpected parameters",
			operationKey: OperationRestartPayment,
			parameters: map[string]any{
				"force": true,
			},
			expectedError: ErrInvalidOperationParameters,
		},
		{
			name:          "unregistered operation",
			operationKey:  "database.drop",
			expectedError: ErrOperationNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOperationParameters(
				tt.operationKey,
				tt.parameters,
			)

			if !errors.Is(err, tt.expectedError) {
				t.Fatalf(
					"expected error %v, got %v",
					tt.expectedError,
					err,
				)
			}
		})
	}
}
