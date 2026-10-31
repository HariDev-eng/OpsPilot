# OpsPilot — Consumer Idempotency

## 1. Why Idempotency Is Necessary

OpsPilot uses at-least-once delivery.

That means this can happen:

```text
event-123
   ↓
consumer processes
   ↓
success
   ↓
Kafka offset commit fails
   ↓
Kafka redelivers event-123
```

The consumer sees:

```text
event-123
```

again.

If our processing is not idempotent, we could create duplicate incidents.

---

## 2. Duplicate Event vs Duplicate Incident

These are different problems.

### Duplicate event

The same event is delivered twice.

```text
event-123
event-123
```

### Duplicate incident

Two incidents are created for what should be one incident.

```text
incident 10
incident 11
```

A duplicate event does not automatically mean duplicate incident.

Our system must protect against both.

---

## 3. Processed Events Table

Migration:

```text
migration/004_create_processed_events.sql
```

Schema:

```sql
CREATE TABLE processed_events (
    event_id TEXT NOT NULL,
    consumer_name TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (event_id, consumer_name)
);
```

This creates an idempotency key:

```text
event_id + consumer_name
```

---

## 4. Why Consumer Name Is Part of the Key

Suppose:

```text
event-123
```

is processed by:

```text
incident-consumer
```

and:

```text
analytics-consumer
```

Both are legitimate.

Therefore:

```text
(event-123, incident-consumer)
```

and:

```text
(event-123, analytics-consumer)
```

are different records.

---

## 5. Idempotency Operation

The repository uses:

```text
internal/repository/processed_event_repository.go
```

Core idea:

```sql
INSERT INTO processed_events (...)
VALUES (...)
ON CONFLICT (...)
DO NOTHING
```

If the insert happens:

```text
new event
```

If the row already exists:

```text
already processed
```

The database unique constraint is the final authority.

---

## 6. Why Not SELECT First?

A naive approach is:

```text
SELECT processed_events
        ↓
if not found
        ↓
INSERT
```

This has a race condition.

Two consumers could do:

```text
Consumer A → SELECT → not found
Consumer B → SELECT → not found

Consumer A → INSERT
Consumer B → INSERT
```

The better design is:

```text
INSERT
ON CONFLICT DO NOTHING
```

The database decides atomically.

---

## 7. Processing Transaction

For incident processing, idempotency is part of the same database transaction.

Conceptually:

```text
BEGIN
   ↓
mark event processed
   ↓
detect/process incident
   ↓
attach event
   ↓
COMMIT
```

This is important.

We don't want:

```text
mark processed
COMMIT

then incident creation fails
```

because the system would believe the event was processed even though the business operation failed.

---

## 8. Duplicate Event Behavior

If the event has already been processed:

```text
ErrAlreadyProcessed
```

The processor logs:

```text
event already processed
```

and returns success.

The Kafka consumer can then commit the message.

This is intentional.

A duplicate is not a processing failure.

---

## 9. Real Test

We deliberately redelivered an event.

The system originally processed it and created an incident.

When the same event was processed again, the logs showed:

```text
event already processed
```

and no second incident was created.

This verified the idempotency mechanism.

---

## 10. Why This Matters for Distributed Systems

Consider the chain:

```text
Kafka
 ↓
Consumer
 ↓
PostgreSQL
```

There are multiple failure windows.

For example:

```text
DB transaction succeeds
        ↓
application crashes
        ↓
Kafka offset not committed
        ↓
event redelivered
```

Exactly this type of situation is normal in distributed systems.

Therefore, good consumers assume:

```text
I may receive this message again.
```

---

## 11. Idempotency and Transactional Outbox

These patterns work together.

### Outbox

Protects:

```text
Database → Kafka
```

### Idempotent consumer

Protects:

```text
Kafka → Database/business processing
```

Together:

```text
DB
 ↓
Outbox
 ↓
Kafka
 ↓
Idempotent Consumer
 ↓
DB
```

This is a robust at-least-once architecture.

---

## 12. Important Distinction

Transactional Outbox does not eliminate duplicates.

It guarantees that a committed database event has a durable publication record.

But the publisher may publish successfully and crash before recording that publication.

Therefore:

```text
Outbox
    ↓
possible duplicate
    ↓
Idempotent consumer
```

This is expected.

---

## 13. Where to See the Code

Idempotency repository:

```text
internal/repository/processed_event_repository.go
```

Main processor:

```text
internal/service/incident_processor.go
```

Kafka consumer:

```text
internal/consumer/incident_consumer.go
```

Database migration:

```text
migration/004_create_processed_events.sql
```

---

## 14. Failure We Designed For

```text
Kafka redelivery
      ↓
same event
      ↓
processed_events conflict
      ↓
ErrAlreadyProcessed
      ↓
no duplicate business operation
```

---

## 15. Main Lesson

At-least-once systems require idempotent consumers.

The important mindset is:

> Do not design a consumer assuming a message arrives exactly once.

Design it assuming:

```text
zero, one, or multiple deliveries
```

and make repeated processing safe.
