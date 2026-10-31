# OpsPilot — Incident State Projection

## 1. What Is a Projection?

A projection is a derived representation of authoritative data optimized for a particular use case.

In OpsPilot:

```text
PostgreSQL
    ↓
source of truth

Redis
    ↓
derived projection
```

The Redis representation is not the canonical incident record.

---

## 2. Why Do We Need It?

Suppose the UI displays:

```text
Active incidents
Critical incidents
Current incident status
```

These reads can happen frequently.

Instead of forcing every read through complex PostgreSQL queries, we can maintain a fast representation in Redis.

For example:

```text
incident:9
```

contains:

```json
{
  "incident_id": 9,
  "service": "projection-fixed-service",
  "severity": "HIGH",
  "status": "OPEN"
}
```

---

## 3. Current Architecture

Current flow:

```text
Business operation
       ↓
PostgreSQL
       ↓
COMMIT
       ↓
Redis projection
```

The important rule is:

> PostgreSQL is updated first.

---

## 4. Why PostgreSQL First?

Imagine:

```text
Redis update succeeds
PostgreSQL update fails
```

Then Redis would show:

```text
ACKNOWLEDGED
```

while PostgreSQL would show:

```text
OPEN
```

Now the derived state has become more authoritative than the source.

That is dangerous.

Instead:

```text
PostgreSQL
    ↓
commit
    ↓
Redis
```

If Redis fails:

```text
PostgreSQL = correct
Redis = stale
```

That is much easier to recover from.

---

## 5. Current Code

Incident lifecycle:

```text
internal/service/incident_service.go
```

Projection:

```go
func (s *IncidentService) projectIncident(
    ctx context.Context,
    incident *model.Incident,
) {
    state := redis.IncidentState{
        IncidentID: incident.ID,
        Service:    incident.Service,
        Severity:   incident.Severity,
        Status:     incident.Status,
    }

    if err := s.redisClient.SetIncidentState(
        ctx,
        state,
    ); err != nil {
        log.Printf(
            "failed to update incident state projection: incident_id=%d error=%v",
            incident.ID,
            err,
        )
    }
}
```

---

## 6. Acknowledge Flow

When:

```http
POST /api/v1/incidents/9/acknowledge
```

is called:

```text
HTTP
 ↓
Incident Handler
 ↓
Incident Service
 ↓
PostgreSQL
 ↓
UPDATE incident
 ↓
COMMIT
 ↓
Get updated incident
 ↓
Redis projection
```

---

## 7. Resolve Flow

Resolve follows the same pattern:

```text
POST /api/v1/incidents/9/resolve
        ↓
PostgreSQL
        ↓
COMMIT
        ↓
Redis projection
```

The lifecycle is:

```text
OPEN
 ↓
ACKNOWLEDGED
 ↓
RESOLVED
```

---

## 8. Real Verification

Incident #9 was tested.

Initially:

```text
PostgreSQL

9 | HIGH | OPEN
```

Redis:

```json
{
  "incident_id": 9,
  "service": "projection-fixed-service",
  "severity": "HIGH",
  "status": "OPEN"
}
```

After acknowledgement:

```text
PostgreSQL

9 | HIGH | ACKNOWLEDGED
```

Redis:

```json
{
  "incident_id": 9,
  "service": "projection-fixed-service",
  "severity": "HIGH",
  "status": "ACKNOWLEDGED"
}
```

After resolution:

```text
PostgreSQL

9 | HIGH | RESOLVED
```

Redis:

```json
{
  "incident_id": 9,
  "service": "projection-fixed-service",
  "severity": "HIGH",
  "status": "RESOLVED"
}
```

This verified the projection path.

---

## 9. Failure Verification

We then stopped Redis.

PostgreSQL lifecycle updates still succeeded.

Redis projection failed.

The log showed:

```text
failed to update incident state projection:
incident_id=10
error=dial tcp 127.0.0.1:6379:
connect: connection refused
```

PostgreSQL still contained:

```text
10 | HIGH | ACKNOWLEDGED
```

This was exactly the behavior we wanted from the current architecture.

---

## 10. Why We Don't Roll Back PostgreSQL

A common mistake would be:

```text
PostgreSQL succeeds
Redis fails
↓
rollback PostgreSQL
```

But PostgreSQL and Redis are separate systems.

There is no simple local database transaction spanning both.

More importantly, Redis is derived state.

The correct strategy is:

```text
accept authoritative state
        ↓
repair projection later
```

---

## 11. Current Limitation

The current projection is:

```text
best effort
```

If Redis fails, we log the problem.

We do not currently have a durable projection queue or automatic reconciliation mechanism.

This is a known gap.

---

## 12. Better Future Architecture

A stronger design would be:

```text
Incident transaction
       ↓
PostgreSQL
       ├── incident update
       └── lifecycle outbox event
                    ↓
                  Kafka
                    ↓
          Projection Consumer
                    ↓
                  Redis
```

Then:

```text
PostgreSQL
    ↓
durable lifecycle event
    ↓
Kafka
    ↓
Redis projection
```

If Redis is temporarily unavailable:

```text
Kafka event remains available
        ↓
consumer retries
        ↓
projection eventually catches up
```

This is significantly more durable.

---

## 13. Why This Is Important for AI

Later the AI layer will need incident state.

For example:

```text
Incident #9
Service: payment-service
Severity: CRITICAL
Status: OPEN
```

The AI context builder can retrieve the authoritative incident from PostgreSQL.

Redis can help provide fast operational views.

The AI should not blindly trust a potentially stale cache when authoritative data is available.

---

## 14. Where to See the Code

Lifecycle:

```text
internal/service/incident_service.go
```

Redis projection:

```text
internal/redis/incident_state.go
```

Incident repository:

```text
internal/repository/incident_repository.go
```

Handlers:

```text
internal/handler/incident_handler.go
```

---

## 15. Main Principle

The architecture follows:

```text
WRITE
  ↓
PostgreSQL
  ↓
COMMIT
  ↓
PROJECT
  ↓
Redis
```

rather than:

```text
WRITE
  ↓
Redis
  ↓
PostgreSQL
```

The first model protects correctness.

The second risks making a cache look authoritative.

---

## 16. Future Principle

The long-term goal is:

```text
Authoritative state
       ↓
Durable event
       ↓
Derived projections
```

This same pattern will become useful for:

```text
Redis
Search index
Analytics
AI context indexes
Vector database
UI read models
```

The source of truth remains PostgreSQL.
