# OpsPilot — Transactional Outbox

## 1. The Problem We Needed to Solve

OpsPilot receives an event.

We need to:

```text
1. Store the event
2. Publish the event to Kafka
```

At first glance this looks simple:

```text
HTTP
 ↓
PostgreSQL
 ↓
Kafka
```

But there is a reliability problem.

Imagine:

```text
INSERT event → SUCCESS

PUBLISH Kafka → FAILURE
```

Now the event exists in PostgreSQL but Kafka never receives it.

The event can be lost from the processing pipeline.

The opposite problem also exists:

```text
Kafka publish → SUCCESS

DB commit → FAILURE
```

Now Kafka contains an event that the database does not consider committed.

We needed a reliable boundary.

---

## 2. The Solution

We introduced the **Transactional Outbox Pattern**.

Instead of directly publishing Kafka from the request:

```text
HTTP
 ↓
DB
 ↓
Kafka
```

we do:

```text
HTTP
 ↓
PostgreSQL transaction
 ├── events
 └── outbox
       ↓
     COMMIT
       ↓
Outbox Publisher
       ↓
Kafka
```

The event and its outbox record are committed together.

---

## 3. Transaction

The code lives in:

```text
services/event-api/internal/repository/event_repository.go
```

Core logic:

```go
tx, err := r.pool.Begin(ctx)
if err != nil {
    return err
}

defer tx.Rollback(ctx)
```

Then:

```go
INSERT INTO events (...)
```

Then:

```go
INSERT INTO outbox (...)
```

Finally:

```go
return tx.Commit(ctx)
```

Therefore:

```text
BEGIN
   │
   ├── events INSERT
   │
   ├── outbox INSERT
   │
COMMIT
```

---

## 4. Why Two Tables?

The `events` table represents:

> What happened?

The `outbox` table represents:

> What still needs to be published externally?

These are related but have different responsibilities.

For example:

```text
events
--------------------------------
event_id = e123
event_type = payment.error
service = payment-service
```

and:

```text
outbox
--------------------------------
event_id = e123
topic = ops.events
status = PENDING
attempts = 0
```

---

## 5. Outbox Lifecycle

We introduced states:

```text
PENDING
   ↓
PUBLISHING
   ↓
PUBLISHED
```

If publication fails:

```text
PUBLISHING
   ↓
PENDING
   ↓
retry
```

Conceptually:

```text
                 ┌──────────────┐
                 │    PENDING   │
                 └──────┬───────┘
                        │
                     claim
                        │
                        ▼
                 ┌──────────────┐
                 │  PUBLISHING  │
                 └──────┬───────┘
                        │
              ┌─────────┴─────────┐
              │                   │
           success              failure
              │                   │
              ▼                   ▼
        ┌───────────┐         PENDING
        │ PUBLISHED │
        └───────────┘
```

---

## 6. Why PUBLISHING Exists

Imagine two publisher instances:

```text
Publisher A
Publisher B
```

Both query the database at the same time.

Without locking, both could select:

```text
outbox id = 123
```

and publish it.

We use:

```sql
FOR UPDATE SKIP LOCKED
```

This allows one publisher to claim the row while other publishers skip it.

The pattern is:

```text
Publisher A
   ↓
claims row 123

Publisher B
   ↓
skips locked row 123
```

This supports multiple publisher instances.

---

## 7. Outbox Publisher

Code:

```text
internal/publisher/outbox_publisher.go
```

Repository:

```text
internal/repository/outbox_repository.go
```

The publisher periodically polls the database.

Current behavior:

```text
every ~2 seconds
batch size = 10
```

It asks:

```text
Give me publishable outbox records.
```

Then:

```text
Claim
  ↓
Publish Kafka
  ↓
MarkPublished
```

---

## 8. Retry

Current retry delays are:

```text
2 seconds
5 seconds
10 seconds
30 seconds
```

The outbox stores:

```text
attempts
last_error
next_retry_at
```

Therefore, if Kafka temporarily fails:

```text
PENDING
 ↓
attempt 1
 ↓
failure
 ↓
next_retry_at
 ↓
attempt 2
```

The database remembers the retry state.

---

## 9. Why Kafka Is Not Inside the DB Transaction

This is one of the most important decisions.

We do NOT do:

```text
BEGIN
   INSERT
   Kafka Publish
COMMIT
```

Instead:

```text
BEGIN
   INSERT event
   INSERT outbox
COMMIT

Later:

Outbox Publisher
   ↓
Kafka
```

The DB transaction only controls database state.

Kafka publication is a separate asynchronous workflow.

---

## 10. Delivery Guarantee

Transactional Outbox gives us strong protection against losing committed events.

But it does **not** automatically provide exactly-once delivery.

Consider:

```text
Publisher
   ↓
Kafka publish SUCCESS
   ↓
application crashes
   ↓
MarkPublished never executes
```

The outbox record still looks publishable.

The publisher may retry.

Kafka can receive the event twice.

Therefore:

```text
Transactional Outbox
        +
Idempotent Consumers
```

is the practical architecture.

---

## 11. Failure Window

One important failure case is:

```text
Kafka receives event
        ↓
DB MarkPublished fails
        ↓
record remains PUBLISHING/PENDING
        ↓
retry
        ↓
Kafka receives duplicate
```

This is why consumer idempotency is required.

We deliberately designed for:

```text
at-least-once delivery
```

rather than pretending the entire distributed workflow is exactly once.

---

## 12. Another Failure We Identified

A process can crash here:

```text
PENDING
 ↓
PUBLISHING
 ↓
CRASH
```

Now the row may remain:

```text
PUBLISHING
```

forever unless recovery exists.

This is currently a known gap.

Future hardening should introduce a lease/timeout such as:

```text
PUBLISHING
   +
claimed_at
   +
lease expiration
```

Then a recovery worker can safely return stale rows to:

```text
PENDING
```

---

## 13. Producer vs Outbox Publisher

This distinction is important.

### Kafka Producer

Its responsibility is:

> How do I send a message to Kafka?

Code:

```text
internal/kafka/producer.go
```

### Outbox Publisher

Its responsibility is:

> Which database records should be sent, when should they be retried, and how should publication state be updated?

Code:

```text
internal/publisher/outbox_publisher.go
internal/repository/outbox_repository.go
```

Therefore:

```text
Outbox Publisher
      ↓
Kafka Producer
      ↓
Kafka
```

They are not the same abstraction.

---

## 14. Where to See the Data Flow

```text
HTTP Request
     ↓
EventRepository.Create()
     ↓
BEGIN
     ↓
events INSERT
     ↓
outbox INSERT
     ↓
COMMIT
     ↓
OutboxPublisher
     ↓
ClaimPending()
     ↓
Kafka Producer
     ↓
ops.events
     ↓
Incident Consumer
```

---

## 15. Why This Matters for OpsPilot

If an incident event disappears, the entire AI analysis chain can fail.

For example:

```text
payment.error
```

could eventually become:

```text
incident
   ↓
AI context
   ↓
RAG
   ↓
LLM analysis
```

If the original event disappears, the AI may never know the incident happened.

Therefore, reliability starts before AI.

---

## 16. Main Lesson

The Transactional Outbox gives us:

```text
Atomic DB persistence
        +
Reliable asynchronous publication
        +
Retry
        +
At-least-once delivery
```

It does not give us:

```text
Exactly-once end-to-end processing
```

That distinction is fundamental to the design.
