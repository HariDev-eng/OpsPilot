# 15 — Action Management, Human Approval & Execution Safety

## 1. Objective

Introduce a controlled action-management and execution layer that converts AI-generated incident recommendations into governed, auditable operations.

The system must ensure that:

- AI recommendations cannot directly execute system operations.
- Operations are assigned through a backend-controlled allowlist.
- Sensitive operations require human approval.
- Execution state transitions are enforced by the backend.
- Execution attempts are recorded and auditable.
- Worker crashes do not automatically trigger potentially unsafe retries.
- Unknown execution outcomes require operator reconciliation.

The LLM recommends actions. The backend determines whether those actions are permitted, and an authorized operator controls approval and reconciliation.

## 2. Architecture

```text
              Incident Analysis
                     |
                     v
             Recommended Actions
                     |
                     v
              ActionService
                     |
              Policy Validation
                     |
                     v
             Action Repository
                     |
                     v
              Action Management
                     |
          +----------+----------+
          |                     |
     Investigation           Mitigation
     No approval             Approval required
          |                     |
          |                Human Approval
          |                     |
          +----------+----------+
                     |
                     v
             Operation Assignment
                     |
                     v
              Execution Claim
                     |
                     v
              ExecutionService
                     |
                     v
               Executor
                     |
              +------+------+
              |             |
           Success        Failure
              |             |
              v             v
          EXECUTED        FAILED

              Worker Crash
                     |
                     v
       RECONCILIATION_REQUIRED
                     |
                     v
          Operator Reconciliation
                     |
          +----------+----------+
          |          |          |
       Success    Not Done    Unknown
          |          |          |
          v          v          v
       EXECUTED   APPROVED/  RECONCILIATION
                  PROPOSED    REQUIRED
```

## 3. Core Components

### 3.1 ActionService

Responsible for application-level action management.

Responsibilities:

- Create actions from validated AI recommendations.
- Determine action policies.
- Generate action idempotency keys.
- Approve or reject actions.
- Assign permitted operations.
- Validate operation parameters.
- Resolve reconciliation decisions.

### 3.2 ActionRepository

Responsible for persistent action state and transactional audit operations.

Responsibilities:

- Create action records.
- Retrieve and list actions.
- Approve and reject actions.
- Assign operations.
- Claim execution.
- Record execution success or failure.
- Recover stale executions.
- Resolve reconciliation decisions.
- Persist audit records transactionally.

### 3.3 ActionHandler

Exposes HTTP endpoints for operator actions.

Implemented routes:

| Method | Endpoint | Responsibility |
|---|---|---|
| POST | `/api/v1/actions/{id}/approve` | Approve an action |
| POST | `/api/v1/actions/{id}/reject` | Reject an action |
| POST | `/api/v1/actions/{id}/reconcile` | Record an operator reconciliation decision |

The V1 HTTP routes require the shared `OPSPILOT_OPERATOR_SECRET` Bearer credential and attach the configured `OPSPILOT_OPERATOR_ID` audit label to the request context. The default label is `portfolio-operator`; set a specific operator label in the deployment environment. This is a single-operator gate, not multi-user authentication or RBAC.

### 3.4 ExecutionService

Coordinates execution requests.

Responsibilities:

- Request an execution claim from the repository.
- Verify that the action is eligible for execution.
- Invoke the registered executor.
- Record execution results.

The executor is invoked only after the repository successfully claims the action.

## 4. Action Classification and Approval Policy

Actions are classified into three categories.

| Type | Meaning | Approval |
|---|---|---|
| INVESTIGATION | Collect or inspect information | Not required |
| MITIGATION | Change system state to reduce impact | Required |
| DESTRUCTIVE | Potentially high-impact operation | Required |

Classification is validated by the backend, but natural-language classification alone is not a sufficient safety boundary.

The operation registry independently controls which operations can actually be assigned.

### Current registered operations

| Operation | Category | Approval |
|---|---|---|
| `payment.inspect_logs` | INVESTIGATION | No |
| `payment.restart` | MITIGATION | Yes |

Unknown operation keys are rejected.

Both currently registered operations accept empty parameter maps only.

The registry is intentionally restrictive. New operations require explicit backend registration and parameter validation.

## 5. Action Lifecycle

Supported action statuses:

| Status | Meaning |
|---|---|
| PROPOSED | Action has been recommended but not executed |
| PENDING_APPROVAL | Action requires operator approval |
| APPROVED | Action is approved and eligible for execution |
| REJECTED | Operator rejected the action |
| EXECUTING | Execution has been claimed |
| EXECUTED | Successful execution has been recorded or confirmed |
| FAILED | Execution returned a definite failure |
| RECONCILIATION_REQUIRED | Execution outcome is unknown and requires investigation |

### Typical transitions

```text
INVESTIGATION:

PROPOSED
   |
   v
EXECUTING
   |
   +----> EXECUTED
   |
   +----> FAILED


MITIGATION:

PENDING_APPROVAL
   |
   +----> REJECTED
   |
   +----> APPROVED
              |
              v
          EXECUTING
              |
              +----> EXECUTED
              |
              +----> FAILED
```

A stale execution can transition from EXECUTING to RECONCILIATION_REQUIRED.

The repository enforces state-dependent updates so that an action cannot be executed simply by changing its status through an API request.

## 6. Database Design

### 6.1 incident_actions

The `incident_actions` table stores action state and execution metadata.

Important fields:

| Field | Purpose |
|---|---|
| `id` | Action identifier |
| `incident_id` | Related incident |
| `description` | Recommended action description |
| `action_type` | INVESTIGATION, MITIGATION or DESTRUCTIVE |
| `status` | Current action lifecycle state |
| `requires_approval` | Approval policy |
| `idempotency_key` | Unique action creation key |
| `operation_key` | Assigned backend operation |
| `parameters` | Operation parameters |
| `approved_by` | Approving operator |
| `rejected_by` | Rejecting operator |
| `execution_started_at` | Execution claim timestamp |
| `executed_at` | Successful execution or reconciliation confirmation timestamp |
| `execution_error` | Recorded execution error |
| `execution_attempts` | Number of claimed execution attempts |
| `created_at` | Creation timestamp |
| `updated_at` | Last update timestamp |

The table uses constraints to restrict valid action types and statuses and enforce approval policy consistency.

The execution status index covers APPROVED, EXECUTING, FAILED and RECONCILIATION_REQUIRED records.

### 6.2 action_audit_logs

Stores action lifecycle and operator activity.

Important fields:

- `action_id`
- `actor_id`
- `action`
- `details`
- `created_at`

Audit entries are created within the same database transaction as the associated state change.

This prevents an action transition from committing without its corresponding audit record.

## 7. Operation Assignment

Operation assignment is controlled by a backend allowlist.

Example:

```text
AI recommendation
       |
       v
ActionService
       |
       v
Validate operation key
       |
       v
Check operation policy
       |
       v
Validate parameters
       |
       v
Assign operation
       |
       v
Persist audit record
```

The repository uses a row lock during assignment.

Repeated identical assignments are idempotent. Conflicting assignments are rejected.

The LLM cannot introduce arbitrary operation keys or execute arbitrary commands.

## 8. Execution Claiming

Execution is claimed through a conditional database update.

The repository checks:

- The action exists.
- The assigned operation matches.
- The action type and approval requirements match the operation policy.
- The action is in an eligible state.
- Approval has been granted when required.

The status changes to EXECUTING, the execution start timestamp is recorded, and the attempt counter increments.

An EXECUTION_STARTED audit record is written in the same transaction.

Only after a successful claim does the ExecutionService invoke the executor.

This prevents multiple workers from successfully claiming the same action through the normal execution path.

## 9. Execution Failure Handling

Execution outcomes must distinguish known failure from unknown outcome.

### Definite failure

When the executor returns a known error:

```text
EXECUTING
    |
    v
FAILED
```

The failure reason and audit record are persisted.

### Unknown outcome

A worker can crash after the external operation succeeds but before OpsPilot records the result.

Example:

1. OpsPilot claims an action.
2. The external operation succeeds.
3. The worker crashes before recording success.
4. PostgreSQL still contains EXECUTING.
5. OpsPilot cannot determine whether the operation completed.

Blindly retrying could duplicate the operation.

Therefore, unknown outcomes must not be treated as ordinary failures.

## 10. Stale Execution Recovery

The recovery process identifies stale EXECUTING records.

Current implementation:

- Recovery timeout is supplied by the caller.
- The recovery query selects up to 20 eligible records.
- PostgreSQL row locking uses `FOR UPDATE SKIP LOCKED`.
- Stale actions transition to RECONCILIATION_REQUIRED.
- An `EXECUTION_OUTCOME_UNKNOWN` audit record is created.
- Repeated recovery skips actions that are no longer EXECUTING.

The current recovery implementation uses an execution timestamp and fixed timeout.

Execution leases and heartbeats remain future hardening work because a fixed timeout can incorrectly classify a valid long-running operation as stale.

## 11. Operator Reconciliation

Reconciliation allows an operator to resolve an action whose external execution outcome is unknown.

Endpoint:

```http
POST /api/v1/actions/{id}/reconcile
```

Request:

```json
{
  "decision": "CONFIRMED_SUCCESS",
  "reason": "Verified successful completion using external provider logs."
}
```

Supported decisions:

| Decision | Result |
|---|---|
| CONFIRMED_SUCCESS | EXECUTED |
| CONFIRMED_NOT_PERFORMED | APPROVED if approval is required; otherwise PROPOSED |
| UNKNOWN | Remains RECONCILIATION_REQUIRED |

### 11.1 Confirmed success

```text
RECONCILIATION_REQUIRED
          |
          v
   CONFIRMED_SUCCESS
          |
          v
       EXECUTED
```

The system records the confirmation timestamp and reconciliation audit entry.

The timestamp represents when OpsPilot accepted the operator's confirmation, not necessarily the exact time the external operation completed.

### 11.2 Confirmed not performed

```text
RECONCILIATION_REQUIRED
          |
          v
 CONFIRMED_NOT_PERFORMED
          |
          +---- Approval required ----> APPROVED
          |
          +---- No approval required -> PROPOSED
```

Reconciliation does not automatically execute the action again.

A separate execution request is necessary.

### 11.3 Unknown

```text
RECONCILIATION_REQUIRED
          |
          v
         UNKNOWN
          |
          v
RECONCILIATION_REQUIRED
```

The operator's review is audited, but the action remains unresolved.

### 11.4 Concurrency protection

Reconciliation uses a row-level database lock.

When two operators attempt to reconcile the same action concurrently:

1. One transaction obtains the lock.
2. It validates the current state.
3. It updates the status and writes the audit record.
4. It commits.
5. The second transaction observes that the action is no longer awaiting reconciliation and is rejected.

This prevents conflicting operator decisions.

## 12. API Validation and Error Handling

The reconciliation endpoint validates:

- Positive action identifier.
- Request body size.
- Valid UTF-8.
- JSON structure.
- Unknown JSON fields.
- Additional JSON values.
- Supported decision.
- Non-empty reason.
- Maximum reason length of 2,000 characters.

Expected HTTP responses:

| HTTP status | Meaning |
|---|---|
| 200 | Reconciliation accepted |
| 400 | Invalid request |
| 404 | Action not found |
| 409 | Action is not awaiting reconciliation |
| 500 | Unexpected internal failure |

## 13. Verification and Testing

The implementation was verified using Go integration commands and a PostgreSQL-backed reconciliation test.

### Build verification

```bash
gofmt -w .
go test ./...
go build ./...
```

### Reconciliation scenarios

| Scenario | Expected result | Result |
|---|---|---|
| Confirmed success | EXECUTED | Passed |
| Mitigation not performed | APPROVED | Passed |
| Investigation not performed | PROPOSED | Passed |
| Unknown outcome | RECONCILIATION_REQUIRED | Passed |
| Invalid decision | Rejected | Passed |
| Repeated resolution | Rejected | Passed |

### Execution re-entry

Verified that:

- A mitigation can return to APPROVED after confirmation that it was not performed.
- A second execution claim succeeds.
- The execution attempt count increments from 1 to 2.
- The fake executor is called exactly once during the second attempt.
- The final action status becomes EXECUTED.

### Concurrent reconciliation

Verified that two simultaneous decisions against the same action produce:

- Exactly one successful reconciliation.
- Exactly one rejected decision.
- Exactly one reconciliation audit entry.

### HTTP verification

| Scenario | Expected HTTP status | Result |
|---|---:|---|
| Valid reconciliation | 200 | Passed |
| Invalid decision | 400 | Passed |
| Invalid action ID | 400 | Passed |
| Nonexistent action | 404 | Passed |
| Already-resolved action | 409 | Passed |
| Missing reason | 400 | Passed |
| Unknown JSON field | 400 | Passed |

All reconciliation integration scenarios passed against the PostgreSQL database.

## 14. Known Limitations and Future Hardening

1. **Authentication:** Action mutations are protected by a shared Bearer secret in V1. Production authentication, user sessions and RBAC are not implemented.
2. **External idempotency:** Real executors must use downstream idempotency mechanisms to prevent duplicate external operations.
3. **Execution leases:** Fixed-timeout recovery should be upgraded with leases or heartbeats.
4. **Reconciliation evidence:** The operator's reason is recorded, but OpsPilot does not independently verify external provider state.
5. **Operation coverage:** Only `payment.inspect_logs` and `payment.restart` are currently registered.
6. **Parameter validation:** Both registered operations currently accept empty parameter maps only.
7. **Destructive operations:** The classification exists, but a complete production-grade destructive-operation policy and execution registry are not implemented.
8. **Authorization:** The current implementation does not provide production-grade role separation between action creators, approvers and reconcilers.

## 15. Next Phase

Day 9 — Production Hardening and Reliability.

Planned work:

- Outbox stale-record recovery.
- Kafka retry and DLQ verification.
- Consumer failure handling.
- Incident event-window completeness.
- Execution leases and heartbeats.
- Authentication and RBAC.
- Structured logging and metrics.
- Configuration and secrets management.
- AI evidence and validation hardening.

The objective is to move OpsPilot from a working distributed-system prototype toward a reliable, observable and deployable incident intelligence platform.
