# OpsPilot — Incident Detection, Correlation and Severity

## 1. Event Is Not an Incident

This distinction is fundamental.

An event means:

> Something happened.

An incident means:

> A significant undesirable condition exists that requires investigation or response.

For example:

```text
payment.error
```

is an event.

Five payment errors within ten seconds may indicate:

```text
Payment service is experiencing an outage
```

which can become an incident.

---

## 2. Why We Need Correlation

If every failure event created an incident:

```text
100 errors
    ↓
100 incidents
```

That would create alert noise.

Instead we want:

```text
100 related failures
       ↓
1 incident
```

This is correlation.

---

## 3. Event Classification

Code:

```text
internal/service/event_classifier.go
```

Current failure event types include:

```text
payment.failed
payment.error
database.error
database.timeout
service.error
service.timeout
http.5xx
```

The classifier answers:

```text
Is this event relevant to incident detection?
```

It does not decide whether an incident exists by itself.

---

## 4. Detection

Code:

```text
internal/service/incident_detection.go
```

Current configuration:

```text
Failure window = 10 seconds
Failure threshold = 5
```

Conceptually:

```text
Count failure events
within last 10 seconds
```

If:

```text
count < 5
```

then:

```text
no incident
```

If:

```text
count >= 5
```

then:

```text
incident triggered
```

---

## 5. Redis Sliding Window

Redis is used because we need fast temporary state.

For each service:

```text
incident:failures:<service>
```

The data structure is a:

```text
Sorted Set
```

Each event is stored using:

```text
member = event_id
score  = event timestamp
```

Example:

```text
incident:failures:payment-service

event-1 → 10:00:01
event-2 → 10:00:03
event-3 → 10:00:05
event-4 → 10:00:07
event-5 → 10:00:09
```

When processing an event at:

```text
10:00:09
```

we remove events older than:

```text
10:00: - 10 seconds
```

and count the remaining members.

---

## 6. Why Redis Sorted Set?

A sorted set is useful because every member has a numeric score.

We use:

```text
timestamp = score
```

This gives us efficient time-based operations.

The operations are:

```text
ZADD
```

to insert the event.

```text
ZREMRANGEBYSCORE
```

to remove old events.

```text
ZCARD
```

to count remaining events.

---

## 7. Atomic Lua Script

Code:

```text
internal/redis/client.go
```

The operations are executed through a Redis Lua script:

```text
ZADD
   ↓
remove old events
   ↓
EXPIRE
   ↓
ZCARD
```

This is important because otherwise multiple Redis commands could interleave with other operations.

The script gives us an atomic unit of execution inside Redis.

---

## 8. Event-Time Semantics

The detector currently uses:

```go
event.Timestamp
```

rather than blindly using server wall-clock time.

This means the sliding window is based on the event's timestamp.

That was an intentional decision for the current implementation.

However, event-time systems introduce future questions:

- What if an event arrives late?
- What if timestamps are incorrect?
- What if clocks are unsynchronized?
- Should extremely old events be accepted?

These are documented as future hardening areas.

---

## 9. Severity

Code:

```text
internal/service/incident_severity.go
```

Current rules:

```text
< 5
    no incident

5–19
    HIGH

20+
    CRITICAL
```

So:

```text
5 failures
    ↓
HIGH

20 failures
    ↓
CRITICAL
```

---

## 10. Existing Incident vs New Incident

Suppose the first five failures happen:

```text
count = 5
```

OpsPilot creates:

```text
Incident #20
HIGH
OPEN
```

Then more failures arrive.

Instead of creating:

```text
Incident #21
Incident #22
Incident #23
```

the processor looks for an existing open incident for that service.

Conceptually:

```text
failure
  ↓
trigger detection
  ↓
find open incident for service
  ├── exists → update/escalate
  └── doesn't exist → create
```

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

Then the count reaches:

```text
20
```

The existing incident becomes:

```text
CRITICAL
```

We therefore distinguish:

```text
incident creation
```

from:

```text
incident escalation
```

---

## 12. Incident Processing Flow

Code:

```text
internal/service/incident_processor.go
```

Flow:

```text
Kafka event
     ↓
Incident Processor
     ↓
Idempotency check
     ↓
Incident Detection
     ↓
Threshold?
   ┌───────┴───────┐
   │               │
  No              Yes
   │               │
 Commit      Find open incident
               │
          ┌────┴────┐
          │         │
        None      Exists
          │         │
       Create     Escalate
          │         │
          └────┬────┘
               │
          Attach event
               │
             Commit
               │
        Redis projection
```

---

## 13. A Critical Bug We Found

Originally the processor did:

```go
tx.Commit(ctx)

incident, err := p.incidentRepository.GetByIDTx(
    ctx,
    tx,
    incidentID,
)
```

This caused:

```text
tx is closed
```

The event was retried even though the database work had already happened.

This was a very important bug because retry behavior could interact badly with idempotency and incident state.

We fixed the order:

```text
BEGIN
 ↓
process
 ↓
read incident
 ↓
COMMIT
 ↓
Redis projection
```

The incident is fetched while the transaction is still open.

---

## 14. Verification

We tested the threshold behavior.

With five failures for a controlled service:

```text
HIGH
```

With enough failures to reach twenty:

```text
CRITICAL
```

This verified:

```text
sliding-window counting
+
threshold detection
+
severity escalation
```

---

## 15. Redis Failure Test

We deliberately stopped Redis.

Then events were published.

The incident detection process could no longer access its sliding-window state.

As a result:

```text
events published
    ↓
consumer receives
    ↓
Redis detection fails
    ↓
incident not created
```

This exposed an important architectural truth:

> Redis is currently a critical dependency for detection.

This is different from Redis's projection role.

---

## 16. Detection Redis vs Projection Redis

Redis has two different responsibilities.

### Detection

```text
Redis unavailable
    ↓
incident detection currently fails
```

### Projection

```text
PostgreSQL succeeds
Redis unavailable
    ↓
business state remains correct
    ↓
projection becomes stale
```

These two roles have different reliability requirements.

This distinction is important for future hardening.

---

## 17. Where to See the Code

Classifier:

```text
internal/service/event_classifier.go
```

Detection:

```text
internal/service/incident_detection.go
```

Severity:

```text
internal/service/incident_severity.go
```

Processor:

```text
internal/service/incident_processor.go
```

Redis sliding window:

```text
internal/redis/client.go
```

Incident repository:

```text
internal/repository/incident_repository.go
```

---

## 18. Current Detection Model

The current model is intentionally simple:

```text
Event classification
       ↓
10-second sliding window
       ↓
failure count
       ↓
threshold
       ↓
severity
       ↓
incident correlation
```

It is not yet sophisticated AI-based anomaly detection.

That comes later.

The current detection layer gives the AI system a reliable incident boundary and evidence set to work from.
