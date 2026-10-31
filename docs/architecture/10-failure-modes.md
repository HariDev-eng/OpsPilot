# OpsPilot — Failure Modes and Recovery

## 1. Why We Document Failures

A production architecture is not defined only by the happy path.

The important questions are:

```text
What happens when Kafka dies?

What happens when Redis dies?

What happens when PostgreSQL fails?

What happens when the process crashes?

What happens after a successful external operation but before local state is updated?

What happens when the same event arrives twice?
```

OpsPilot was intentionally tested against several of these cases.

---

# 2. Kafka Unavailable

Normal path:

```text
Outbox
 ↓
Kafka
 ↓
Consumer
```

If Kafka is unavailable:

```text
Outbox
 ↓
publish attempt
 ↓
FAIL
```

The outbox remains retryable.

Relevant fields:

```text
status
attempts
last_error
next_retry_at
```

The event is not lost because it remains in PostgreSQL.

---

# 3. Kafka Publish Succeeds but MarkPublished Fails

This is a classic distributed-system failure window.

```text
Outbox
 ↓
Kafka publish SUCCESS
 ↓
application crashes
 ↓
MarkPublished never happens
```

The database may still think:

```text
PUBLISHING
```

or the record may become retryable.

The publisher can publish again.

Therefore:

```text
Kafka may receive duplicate
```

This is why consumers must be idempotent.

---

# 4. Process Crashes During PUBLISHING

Another failure:

```text
PENDING
 ↓
PUBLISHING
 ↓
process crashes
```

The record may remain:

```text
PUBLISHING
```

forever.

Current implementation does not yet have a full stale-claim recovery mechanism.

Future design:

```text
PUBLISHING
+
claimed_at
+
lease duration
```

If:

```text
now > claimed_at + lease
```

then:

```text
PUBLISHING → PENDING
```

and another publisher can retry.

---

# 5. Duplicate Kafka Event

Suppose:

```text
event-123
```

is delivered twice.

Without idempotency:

```text
event-123 → incident creation
event-123 → second incident creation
```

With processed events:

```text
event-123 + incident-consumer
```

is recorded once.

Second delivery:

```text
ON CONFLICT DO NOTHING
```

returns:

```text
already processed
```

and business processing is skipped.

---

# 6. Redis Projection Failure

Suppose:

```text
POST /incidents/10/acknowledge
```

PostgreSQL succeeds:

```text
ACKNOWLEDGED
```

Redis fails:

```text
connection refused
```

The application logs the projection failure.

The authoritative state remains:

```text
PostgreSQL = ACKNOWLEDGED
```

This is acceptable because Redis is derived state.

---

# 7. Redis Detection Failure

This is more serious.

The detection algorithm currently requires Redis:

```text
event
 ↓
Redis sliding window
 ↓
failure count
 ↓
incident
```

If Redis is unavailable:

```text
event
 ↓
Redis failure
 ↓
detection fails
```

During testing, events were published successfully but the incident was not created while Redis was unavailable.

This revealed:

> Redis is currently a critical dependency for incident detection.

That is different from projection Redis.

---

# 8. DLQ Unavailable

During Redis outage testing, events eventually exhausted retries.

The consumer attempted:

```text
ops.events.dlq
```

but Kafka returned:

```text
Unknown Topic Or Partition
```

This exposed a deployment/configuration dependency.

A DLQ must itself be treated as infrastructure.

Production must guarantee:

```text
DLQ topic exists
DLQ permissions exist
DLQ has correct partitions
DLQ is monitored
```

---

# 9. PostgreSQL Failure

If PostgreSQL is unavailable during event ingestion:

```text
HTTP
 ↓
DB transaction
 ↓
FAIL
```

The event cannot be durably accepted.

The API should not claim:

```text
accepted
```

when the durable write failed.

This is why the ingestion endpoint returns:

```text
202 Accepted
```

only after the event and outbox record have been successfully committed.

---

# 10. Transaction Failure

Suppose:

```text
INSERT events
   ↓
SUCCESS

INSERT outbox
   ↓
FAILURE
```

Because both belong to one transaction:

```text
ROLLBACK
```

The event does not remain partially committed.

This is the core value of the transactional outbox.

---

# 11. Redis Projection vs Detection

This distinction is important enough to repeat.

| Redis role | Failure consequence |
|---|---|
| Detection ZSET | Incident detection currently fails |
| Incident projection | PostgreSQL remains correct; Redis becomes stale |

Therefore, future architecture should make detection more resilient.

---

# 12. Transaction Used After Commit

We encountered:

```text
tx.Commit(ctx)

GetByIDTx(ctx, tx, ...)
```

which produced:

```text
tx is closed
```

The fix was:

```text
BEGIN
 ↓
process
 ↓
read required data
 ↓
COMMIT
 ↓
projection
```

This was a correctness bug, not just a coding style issue.

---

# 13. Duplicate Projection Calls

During debugging we also found duplicate:

```text
SetIncidentState()
```

calls in the processor.

The implementation was cleaned up so projection happens once after successful database commit.

This reduced unnecessary Redis operations and made the control flow easier to understand.

---

# 14. Event Timestamp Problems

The current detector uses event timestamps.

Potential problems:

```text
event arrives late
event timestamp is wrong
clock skew
future timestamps
old events
```

These can affect sliding-window correlation.

Future hardening should define:

```text
maximum event age
allowed clock skew
late-event policy
future timestamp policy
```

---

# 15. Concurrent Incident Creation

There is another known race.

Two events from the same service may be processed concurrently:

```text
Consumer A
    ↓
Find open incident
    ↓
none

Consumer B
    ↓
Find open incident
    ↓
none
```

Both could attempt:

```text
CREATE INCIDENT
```

This can create duplicate incidents unless the database or transaction design prevents it.

A stronger future design could use:

```text
database constraint
advisory lock
serialization strategy
```

or another concurrency-control mechanism.

---

# 16. Failure Philosophy

OpsPilot should follow these principles:

```text
1. Never silently lose committed events.

2. Assume messages can be duplicated.

3. Make consumers idempotent.

4. Keep PostgreSQL authoritative.

5. Treat Redis as recoverable derived state.

6. Retry transient failures.

7. Move permanently failing messages to DLQ.

8. Recover stale work.

9. Make failures observable.

10. Test failure paths deliberately.
```

---

# 17. Why Failure Testing Matters

A system that works only when:

```text
Kafka is healthy
Redis is healthy
PostgreSQL is healthy
network is healthy
process never crashes
```

is not production-ready.

The actual engineering skill is understanding:

```text
what fails
+
what state remains
+
what retries
+
what duplicates
+
what is authoritative
+
how recovery occurs
```

That is the mindset OpsPilot is being built to demonstrate.
