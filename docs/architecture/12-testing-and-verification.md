# OpsPilot — Testing and Verification

## 1. Why Testing Is Part of the Architecture

OpsPilot is a distributed system.

Testing only individual functions is not enough.

We need to verify:

```text
HTTP
 ↓
PostgreSQL
 ↓
Outbox
 ↓
Kafka
 ↓
Consumer
 ↓
Incident Detection
 ↓
Redis
```

and we also need to deliberately break parts of the system.

---

# 2. Basic Go Verification

After changes we run:

```bash
gofmt -w .
```

Then:

```bash
go test ./...
```

Then:

```bash
go build ./...
```

These provide three basic levels:

```text
gofmt
 ↓
formatting

go test
 ↓
automated correctness

go build
 ↓
compilation
```

---

# 3. Event Ingestion Test

We verify that a valid event can be submitted.

Example structure:

```json
{
  "event_id": "test-123",
  "event_type": "payment.error",
  "service": "payment-service",
  "timestamp": "2026-10-30T10:00:00Z",
  "payload": {
    "status": 500
  }
}
```

Expected:

```text
HTTP 202 Accepted
```

with:

```json
{
  "event_id": "test-123",
  "status": "accepted"
}
```

---

# 4. Duplicate Event Test

Submit the same:

```text
event_id
```

again.

Expected behavior:

```text
HTTP 200
status = already_received
```

The important point is that duplicate submission is treated as idempotent acceptance, not as a server failure.

---

# 5. Transactional Outbox Verification

After submitting an event, PostgreSQL should contain:

```text
events
```

and:

```text
outbox
```

for the same event.

Conceptually:

```text
events.event_id
        =
outbox.event_id
```

This verifies that the transaction created both records.

---

# 6. Kafka Verification

The outbox publisher should move:

```text
PENDING
```

to:

```text
PUBLISHING
```

and eventually:

```text
PUBLISHED
```

Kafka should contain:

```text
ops.events
```

with the event.

---

# 7. Consumer Verification

The consumer should:

```text
FetchMessage
 ↓
Process
 ↓
Commit
```

If processing fails:

```text
retry
```

If the event remains permanently invalid:

```text
DLQ
```

---

# 8. Idempotency Verification

We intentionally caused an event to be redelivered.

The system logged:

```text
event already processed
```

and did not create a second incident.

This verified:

```text
Kafka at-least-once
+
database idempotency
```

---

# 9. Incident Detection Test

Current configuration:

```text
window = 10 seconds
threshold = 5
```

We sent failure events for the same service.

Expected:

```text
1
2
3
4
```

No incident.

At:

```text
5
```

an incident should be created.

---

# 10. Severity Test

At:

```text
5–19
```

expected:

```text
HIGH
```

At:

```text
20+
```

expected:

```text
CRITICAL
```

This was verified using controlled event timestamps.

---

# 11. Sliding Window Test

We verified that events outside the time window stop contributing to the count.

Example:

```text
t=0
t=2
t=4
t=6
t=8
```

All five are inside a 10-second window.

But:

```text
t=0
t=12
```

should not be counted together.

This verifies that Redis is implementing a real sliding window rather than a simple counter.

---

# 12. Incident Correlation Test

The important behavior is:

```text
first threshold crossing
        ↓
create incident
```

Then:

```text
additional failure
        ↓
find existing open incident
        ↓
attach event
```

rather than:

```text
additional failure
        ↓
create another incident
```

---

# 13. Incident Lifecycle Test

We verified:

```text
OPEN
 ↓
ACKNOWLEDGED
 ↓
RESOLVED
```

For incident #9:

```text
PostgreSQL:
OPEN
```

then:

```text
ACKNOWLEDGED
```

then:

```text
RESOLVED
```

---

# 14. Redis Projection Test

After lifecycle changes, Redis should reflect PostgreSQL.

For incident #9:

```text
PostgreSQL:
OPEN

Redis:
OPEN
```

Then:

```text
PostgreSQL:
ACKNOWLEDGED

Redis:
ACKNOWLEDGED
```

Then:

```text
PostgreSQL:
RESOLVED

Redis:
RESOLVED
```

---

# 15. Redis Failure Test

We deliberately stopped Redis.

Then:

```text
PostgreSQL lifecycle update
```

was performed.

Expected:

```text
PostgreSQL = successful
Redis = projection failure
```

This verified that PostgreSQL is authoritative.

---

# 16. Redis Detection Failure Test

We stopped Redis before sending failure events.

Expected:

```text
event ingestion → successful
outbox → successful
Kafka → successful
consumer → receives
detection → fails because Redis unavailable
```

This revealed the current architecture's dependency on Redis for detection.

This is an important test because it discovered an architectural weakness rather than merely testing the happy path.

---

# 17. DLQ Failure Test

We allowed events to exhaust consumer retries.

The consumer attempted:

```text
ops.events.dlq
```

and the infrastructure returned:

```text
Unknown Topic Or Partition
```

This showed that the DLQ infrastructure itself must be tested.

---

# 18. Build Verification

After fixing the transaction bug and related code:

```bash
gofmt -w .
go test ./...
go build ./...
```

passed.

This gave confidence that:

```text
formatting
+
tests
+
compilation
```

were clean.

---

# 19. Test Categories

OpsPilot should eventually have:

```text
Unit tests
Integration tests
Repository tests
Kafka tests
Redis tests
API tests
End-to-end tests
Failure tests
```

---

# 20. Future AI Testing

Once the AI layer exists, testing must expand.

We will need to verify:

```text
retrieval relevance
structured output validity
hallucination resistance
citation/evidence correctness
confidence behavior
prompt regression
tool-call correctness
approval boundaries
latency
cost
failure fallback
```

For example, an AI response should not say:

```text
Root cause: database connection pool exhaustion
```

without evidence.

The evaluation should ask:

```text
Did the retrieved evidence actually support that conclusion?
```

---

# 21. Production Test Philosophy

For every major feature, we should ask:

```text
Happy path?
Duplicate?
Timeout?
Dependency unavailable?
Crash?
Retry?
Partial success?
Concurrent request?
Invalid input?
Recovery?
```

This is how OpsPilot should be developed.

---

# 22. Where Tests Live

Current Redis tests:

```text
internal/redis/client_test.go
```

Future tests should remain close to the component they verify where practical.

Integration tests can live under dedicated test packages/directories as the project grows.

---

# 23. Main Lesson

Testing OpsPilot is not only:

```text
Does the function return the right value?
```

It is:

```text
Does the system remain correct
when components succeed,
fail,
retry,
duplicate,
restart,
or become temporarily unavailable?
```

That is the level of testing required for the architecture we are building.
