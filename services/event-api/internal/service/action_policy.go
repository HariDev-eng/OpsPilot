package service

import "errors"

var (
	ErrOperationNotAllowed = errors.New(
		"operation is not registered in the server allowlist",
	)
)

const (
	OperationInspectPaymentLogs = "payment.inspect_logs"
	OperationRestartPayment     = "payment.restart"
)

type OperationPolicy struct {
	OperationKey     string
	ActionType       string
	RequiresApproval bool
	Description      string
}

// This registry is owned by the backend.
// The LLM cannot add or modify entries.
var operationPolicies = map[string]OperationPolicy{
	OperationInspectPaymentLogs: {
		OperationKey:     OperationInspectPaymentLogs,
		ActionType:       "INVESTIGATION",
		RequiresApproval: false,
		Description:      "Inspect payment service application logs",
	},

	OperationRestartPayment: {
		OperationKey:     OperationRestartPayment,
		ActionType:       "MITIGATION",
		RequiresApproval: true,
		Description:      "Restart the payment service",
	},
}

// LookupOperationPolicy returns a registered server-side policy.
func LookupOperationPolicy(
	operationKey string,
) (OperationPolicy, error) {

	policy, exists := operationPolicies[operationKey]

	if !exists {
		return OperationPolicy{}, ErrOperationNotAllowed
	}

	return policy, nil
}
