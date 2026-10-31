# OpsPilot — Incident Lifecycle

## 1. Why Incidents Need a Lifecycle

An incident is not simply:

```text
created = true
```

Real operational incidents move through states.

OpsPilot currently uses:

```text
OPEN
  ↓
ACKNOWLEDGED
  ↓
RESOLVED
```

This represents the basic operational workflow.

---

## 2. OPEN

An incident becomes OPEN when the detection system determines that the failure threshold has been reached.

Example:

```text
payment-service

5 failures
within 10 seconds
```

Detection:

```text
Triggered = true
Severity = HIGH
```

The system creates:

```text
Incident
status = OPEN
severity = HIGH
```

---

## 3. ACKNOWLEDGED

Acknowledgement means:

> An engineer has taken ownership/awareness of the incident.

The transition is:

```text
OPEN
 ↓
ACKNOWLEDGED
```

The database records:

```text
acknowledged_at
```

This provides an operational timestamp.

---

## 4. RESOLVED

Resolution means:

> The incident has been handled and is no longer considered active.

The transition is:

```text
ACKNOWLEDGED
 ↓
RESOLVED
```

The database records:

```text
resolved_at
```

---

## 5. State Transition Rules

Current intended lifecycle:

```text
OPEN
  │
  │ acknowledge
  ▼
ACKNOWLEDGED
  │
  │ resolve
  ▼
RESOLVED
```

We do not allow:

```text
OPEN → RESOLVED
```

through the current service operation.

The repository enforces lifecycle conditions.

For example, acknowledgement only applies to:

```text
OPEN
```

Resolution only applies to:

```text
ACKNOWLEDGED
```

---

## 6. Why Enforce State in the Database?

The service can validate:

```text
if status != OPEN
```

but concurrent requests could still race.

The database should enforce the final state transition condition.

This is another example of:

> Application validation is not the same as database authority.

---

## 7. Incident Repository

Code:

```text
internal/repository/incident_repository.go
```

Responsibilities include:

```text
CreateTx
AttachEventTx
FindOpenByServiceTx
Acknowledge
Resolve
EscalateSeverityTx
GetByIDTx
GetByID
```

This separates database operations from business orchestration.

---

## 8. Incident Service

Code:

```text
internal/service/incident_service.go
```

The service handles lifecycle workflows.

For acknowledgement:

```text
repository.Acknowledge()
        ↓
repository.GetByID()
        ↓
projectIncident()
```

For resolution:

```text
repository.Resolve()
        ↓
repository.GetByID()
        ↓
projectIncident()
```

---

## 9. Incident Detection and Lifecycle Are Different

Detection answers:

> Should an incident exist?

Lifecycle answers:

> What is the current state of that incident?

For example:

```text
Detection:
5 failures → create incident
```

Then:

```text
Lifecycle:
OPEN → ACKNOWLEDGED
```

Then:

```text
Lifecycle:
ACKNOWLEDGED → RESOLVED
```

Keeping these responsibilities separate makes the system easier to reason about.

---

## 10. Severity Is Also Separate

Severity answers:

> How serious is the incident?

Current levels:

```text
HIGH
CRITICAL
```

So an incident has independent dimensions:

```text
Incident
├── status
│   ├── OPEN
│   ├── ACKNOWLEDGED
│   └── RESOLVED
│
└── severity
    ├── HIGH
    └── CRITICAL
```

For example:

```text
status = ACKNOWLEDGED
severity = CRITICAL
```

is valid.

---

## 11. Escalation

Suppose:

```text
5 failures
```

creates:

```text
HIGH
```

Later:

```text
20 failures
```

arrive.

The existing open incident can become:

```text
CRITICAL
```

This is not a new incident.

It is:

```text
existing incident
       ↓
severity escalation
```

---

## 12. Redis Projection

After the PostgreSQL lifecycle update succeeds:

```text
PostgreSQL
     ↓
Redis projection
```

Example:

```text
OPEN
```

becomes:

```text
ACKNOWLEDGED
```

in Redis.

Then:

```text
ACKNOWLEDGED
```

becomes:

```text
RESOLVED
```

---

## 13. Real Incident #9 Verification

We created:

```text
Incident #9
service = projection-fixed-service
severity = HIGH
status = OPEN
```

Redis contained:

```json
{
  "incident_id": 9,
  "service": "projection-fixed-service",
  "severity": "HIGH",
  "status": "OPEN"
}
```

Then:

```http
POST /api/v1/incidents/9/acknowledge
```

Result:

```text
PostgreSQL:
ACKNOWLEDGED

Redis:
ACKNOWLEDGED
```

Then:

```http
POST /api/v1/incidents/9/resolve
```

Result:

```text
PostgreSQL:
RESOLVED

Redis:
RESOLVED
```

---

## 14. Failure Test With Incident #10

We created incident #10.

Then Redis was stopped.

We acknowledged the incident.

PostgreSQL:

```text
10 | HIGH | ACKNOWLEDGED
```

Redis projection:

```text
failed
```

This verified that PostgreSQL remains authoritative.

---

## 15. Why This Matters for the Future AI Layer

The AI system will need to know:

```text
Is this incident still active?
Has somebody acknowledged it?
Has it been resolved?
How severe is it?
```

These lifecycle states will influence AI behavior.

For example:

```text
OPEN
→ AI can investigate and recommend action

ACKNOWLEDGED
→ AI can continue providing analysis

RESOLVED
→ AI can generate post-incident analysis
```

The AI should not treat every historical incident as an active incident.

---

## 16. Where to See the Code

Lifecycle service:

```text
internal/service/incident_service.go
```

Repository:

```text
internal/repository/incident_repository.go
```

Handler:

```text
internal/handler/incident_handler.go
```

Model:

```text
internal/model/incidents.go
```

Migrations:

```text
migration/005_create_incidents.sql
migration/007_incident_lifecycle.sql
```

---

## 17. Main Lifecycle Model

```text
                ┌──────────┐
                │   OPEN   │
                └────┬─────┘
                     │
               acknowledge
                     │
                     ▼
             ┌───────────────┐
             │ ACKNOWLEDGED  │
             └───────┬───────┘
                     │
                   resolve
                     │
                     ▼
              ┌────────────┐
              │  RESOLVED  │
              └────────────┘
```

Severity exists independently:

```text
HIGH
 ↓
CRITICAL
```

This gives OpsPilot a clear operational state machine.
