# OpsPilot — PostgreSQL Architecture

## 1. Why PostgreSQL?

PostgreSQL is the **source of truth** for OpsPilot.

We considered the role of a database from the actual requirements of the system.

OpsPilot needs relationships between:

```text
Event
  ↓
Incident
  ↓
Incident Events
  ↓
AI Analysis
  ↓
Recommendations
  ↓
Audit History
```

This is strongly relational.

For example:

```text
One incident
    ├── many events
    ├── one current status
    ├── one severity
    ├── acknowledgement time
    └── resolution time
```

PostgreSQL gives us:

- Transactions
- Foreign keys
- Unique constraints
- Indexes
- JSONB
- Strong consistency
- SQL
- Row locking
- Reliable durability

We chose PostgreSQL over MongoDB for the core system because the most important relationships are relational.

However, event payloads can still be flexible because PostgreSQL supports JSONB.

---

## 2. PostgreSQL's Role

PostgreSQL stores durable business state.

```text
PostgreSQL
│
├── events
├── outbox
├── processed_events
├── incidents
└── incident_events
```

The rule is:

> If losing the information would make the system incorrect, it belongs in durable storage.

Redis is not the authority.

Kafka is not our business database.

PostgreSQL is the source of truth.

---

## 3. Events Table

Migration:

```text
services/event-api/migration/001_create_events.sql
```

Schema:

```sql
CREATE TABLE events (
    event_id TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    service TEXT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Meaning

```text
event_id
```

Unique identifier of the event.

```text
event_type
```

Example:

```text
payment.error
database.timeout
http.5xx
```

```text
service
```

The service that produced the event.

```text
timestamp
```

The event's timestamp.

```text
payload
```

Flexible event-specific data.

Example:

```json
{
  "status": 500,
  "endpoint": "/payments",
  "message": "connection timeout"
}
```

---

## 4. Why JSONB?

Different event types may have different payloads.

A payment event might contain:

```json
{
  "payment_id": "p123",
  "status": "failed"
}
```

A Kubernetes event might contain:

```json
{
  "pod": "payment-api-123",
  "restart_count": 7
}
```

A database event might contain:

```json
{
  "query": "SELECT ...",
  "latency_ms": 3200
}
```

Creating a separate relational column for every possible field would make the schema unnecessarily rigid.

Therefore:

```text
stable metadata → normal columns

variable event data → JSONB
```

---

## 5. Outbox Table

Migration:

```text
services/event-api/migration/002_create_outbox.sql
services/event-api/migration/003_add_outbox_event_fields.sql
```

The outbox stores events that still need to be published.

Important fields:

```text
event_id
topic
payload
status
attempts
last_error
next_retry_at
created_at
published_at
```

Example:

```text
event_id = payment-error-123
topic = ops.events
status = PENDING
attempts = 0
```

---

## 6. Processed Events

Migration:

```text
services/event-api/migration/004_create_processed_events.sql
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

The important design is:

```text
(event_id, consumer_name)
```

not just:

```text
event_id
```

because different consumers independently process the same Kafka event.

For example:

```text
event-123 + incident-consumer
event-123 + analytics-consumer
event-123 + notification-consumer
```

are different processing records.

---

## 7. Incidents

Migration:

```text
services/event-api/migration/005_create_incidents.sql
services/event-api/migration/007_incident_lifecycle.sql
```

Current schema:

```text
id
event_id
service
event_type
severity
status
created_at
acknowledged_at
resolved_at
```

Current statuses:

```text
OPEN
ACKNOWLEDGED
RESOLVED
```

One schema improvement is already identified:

```text
incidents.event_id
```

is semantically questionable because one incident can contain many events.

A better future name would be:

```text
trigger_event_id
```

or the column could eventually be removed if the relationship is represented entirely through `incident_events`.

This is documented as a known gap rather than silently changing the current implementation.

---

## 8. Incident Events

Migration:

```text
services/event-api/migration/006_incident_events.sql
```

Schema:

```sql
CREATE TABLE incident_events (
    incident_id BIGINT NOT NULL,
    event_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (incident_id, event_id),

    FOREIGN KEY (incident_id)
        REFERENCES incidents(id)
        ON DELETE CASCADE,

    FOREIGN KEY (event_id)
        REFERENCES events(event_id)
        ON DELETE CASCADE
);
```

This gives us:

```text
Incident 9
   │
   ├── event-1
   ├── event-2
   ├── event-3
   ├── event-4
   └── event-5
```

This is important for AI later because the AI context builder can retrieve all events associated with an incident.

---

## 9. Database Transaction

The most important transaction currently exists in:

```text
internal/repository/event_repository.go
```

The logic is:

```text
BEGIN
   │
   ├── INSERT events
   │
   ├── INSERT outbox
   │
COMMIT
```

If the first succeeds but the second fails:

```text
ROLLBACK
```

If the first fails:

```text
ROLLBACK
```

If both succeed:

```text
COMMIT
```

This gives us atomicity.

---

## 10. Why We Do Not Publish Kafka Inside the Transaction

A tempting design would be:

```text
BEGIN DB TRANSACTION

INSERT event

PUBLISH Kafka

COMMIT
```

This is dangerous.

Suppose:

```text
INSERT succeeds
Kafka succeeds
COMMIT fails
```

Kafka has the event but PostgreSQL transaction was rolled back.

Now the two systems disagree.

Or:

```text
INSERT succeeds
Kafka fails
```

What should happen to the database transaction?

This is a distributed transaction problem.

We deliberately avoid it.

Instead:

```text
DB transaction
    ↓
event + outbox
    ↓
commit
    ↓
publisher
    ↓
Kafka
```

---

## 11. Connection Pool

The application uses:

```text
pgxpool
```

The application creates one pool for its lifecycle.

Conceptually:

```text
Application
     │
     ▼
  pgxpool
  ┌──┼──┐
  │  │  │
 DB DB DB
```

We do not create a new database connection for every request.

The pool manages reusable connections.

---

## 12. Context Propagation

Database calls receive:

```go
context.Context
```

For example:

```go
func (r *EventRepository) Create(
    ctx context.Context,
    event model.Event,
) error
```

This allows:

```text
HTTP request
    ↓
service
    ↓
repository
    ↓
PostgreSQL
```

to share cancellation and deadlines.

If the request is cancelled, downstream work can stop.

---

## 13. A Critical Bug We Encountered

We discovered this during incident processing:

```go
tx.Commit(ctx)

incident, err := repository.GetByIDTx(
    ctx,
    tx,
    incidentID,
)
```

This is wrong.

After:

```go
tx.Commit(ctx)
```

the transaction is closed.

Using:

```go
tx
```

after that resulted in:

```text
tx is closed
```

The correct sequence became:

```text
BEGIN
   ↓
process database changes
   ↓
read required data
   ↓
COMMIT
   ↓
use returned data outside transaction
```

This was an important practical lesson:

> A transaction object has a lifecycle. Commit or rollback ends that lifecycle.

---

## 14. Database Is the Authority

For incident lifecycle:

```text
POST /incidents/9/acknowledge
             ↓
       PostgreSQL update
             ↓
          COMMIT
             ↓
       Redis projection
```

If Redis fails:

```text
PostgreSQL = correct
Redis      = stale
```

We accept that temporary inconsistency because Redis is derived state.

---

## 15. Where to See PostgreSQL Code

```text
internal/database/database.go

internal/repository/event_repository.go
internal/repository/outbox_repository.go
internal/repository/processed_event_repository.go
internal/repository/incident_repository.go
```

Migrations:

```text
migration/001_create_events.sql
migration/002_create_outbox.sql
migration/003_add_outbox_event_fields.sql
migration/004_create_processed_events.sql
migration/005_create_incidents.sql
migration/006_incident_events.sql
migration/007_incident_lifecycle.sql
```

---

## 16. Main PostgreSQL Design Principles

```text
PostgreSQL = source of truth

JSONB = flexible event payload

Transactions = atomic business changes

Unique constraints = final authority for uniqueness

Foreign keys = relationship integrity

Indexes = efficient lookup

pgxpool = connection management

context.Context = cancellation/deadline propagation
```

This database foundation supports the rest of OpsPilot.
