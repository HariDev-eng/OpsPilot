# OpsPilot — System Overview

## 1. What Are We Building?

OpsPilot is an **AI-powered incident intelligence and response platform**.

The goal is not simply to collect logs or create alerts. OpsPilot should eventually be able to:

1. Receive operational events from production systems.
2. Persist those events reliably.
3. Publish events asynchronously.
4. Process events independently.
5. Detect abnormal behavior.
6. Correlate multiple events into incidents.
7. Track incident lifecycle.
8. Maintain fast incident state using Redis.
9. Build context from multiple sources.
10. Retrieve relevant runbooks and historical incidents.
11. Ask an LLM to analyze the incident.
12. Produce evidence-backed root-cause analysis.
13. Recommend response actions.
14. Require human approval before dangerous actions.
15. Provide a UI where engineers can investigate and respond.

The final system should look conceptually like:

```text
                         ┌──────────────────────┐
                         │   External Systems   │
                         │                      │
                         │ Payment Service      │
                         │ Database             │
                         │ Kubernetes           │
                         │ CI/CD                │
                         │ Monitoring           │
                         └──────────┬───────────┘
                                    │
                                    │ Events
                                    ▼
                         ┌──────────────────────┐
                         │   Event Ingestion    │
                         │        API           │
                         └──────────┬───────────┘
                                    │
                           Transaction
                                    │
                    ┌───────────────┴───────────────┐
                    │                               │
                    ▼                               ▼
             ┌────────────┐                 ┌────────────┐
             │ PostgreSQL │                 │   Outbox   │
             │   events   │                 │   records   │
             └────────────┘                 └─────┬──────┘
                                                   │
                                                   ▼
                                           ┌──────────────┐
                                           │    Outbox    │
                                           │   Publisher  │
                                           └──────┬───────┘
                                                  │
                                                  ▼
                                           ┌──────────────┐
                                           │    Kafka     │
                                           │  ops.events  │
                                           └──────┬───────┘
                                                  │
                         ┌────────────────────────┼────────────────────┐
                         │                        │                    │
                         ▼                        ▼                    ▼
                 ┌───────────────┐       ┌──────────────┐      ┌─────────────┐
                 │   Incident    │       │  Analytics   │      │Notification │
                 │   Detection   │       │   Consumer   │      │  Consumer   │
                 └───────┬───────┘       └──────────────┘      └─────────────┘
                         │
                         ▼
                 ┌───────────────┐
                 │    Redis      │
                 │ Correlation   │
                 │   State      │
                 └───────┬───────┘
                         │
                         ▼
                 ┌───────────────┐
                 │  PostgreSQL   │
                 │   Incident    │
                 └───────┬───────┘
                         │
                         ▼
                 ┌──────────────────────┐
                 │    AI Context        │
                 │      Builder         │
                 └──────────┬───────────┘
                            │
                            ▼
                 ┌──────────────────────┐
                 │        RAG           │
                 │ Runbooks / History   │
                 └──────────┬───────────┘
                            │
                            ▼
                 ┌──────────────────────┐
                 │        LLM           │
                 │ Incident Intelligence│
                 └──────────┬───────────┘
                            │
                            ▼
                 ┌──────────────────────┐
                 │ Structured Analysis  │
                 │ Evidence + Confidence│
                 └──────────┬───────────┘
                            │
                            ▼
                 ┌──────────────────────┐
                 │ Recommendations      │
                 │ + Human Approval     │
                 └──────────────────────┘
```

---

## 2. Why We Chose This Architecture

A production incident platform has several different requirements.

The ingestion API needs to be reliable.

The database needs to provide durable storage.

Event processing needs to be asynchronous.

Multiple consumers should be able to independently react to the same event.

Incident detection needs temporary/high-speed state.

AI processing should not block event ingestion.

Therefore, we intentionally separated the system into different responsibilities.

### PostgreSQL

PostgreSQL is our **source of truth**.

It stores:

- Events
- Outbox records
- Processed events
- Incidents
- Incident-event relationships
- Incident lifecycle timestamps

### Transactional Outbox

The outbox solves this problem:

```text
Database write succeeds
        +
Kafka publish fails
```

Without an outbox, the system could permanently lose the event.

With the outbox:

```text
DB transaction
    ├── event
    └── outbox record
```

Both succeed or both fail.

### Kafka

Kafka is our asynchronous event backbone.

We selected Kafka because OpsPilot benefits from:

- Event retention
- Replay
- Multiple independent consumers
- Partitioning
- Consumer groups
- High-throughput event processing

### Redis

Redis has two roles.

#### Role 1 — Incident detection

We use a Redis sorted set to implement a sliding time window.

Example:

```text
incident:failures:payment-service
```

It allows us to efficiently ask:

```text
How many failure events happened in the last 10 seconds?
```

#### Role 2 — Incident state projection

We store a fast representation such as:

```text
incident:9
```

containing:

```json
{
  "incident_id": 9,
  "service": "payment-service",
  "severity": "HIGH",
  "status": "OPEN"
}
```

PostgreSQL remains authoritative.

Redis is derived state.

---

## 3. Current Event Flow

A request such as:

```json
{
  "event_id": "payment-error-123",
  "event_type": "payment.error",
  "service": "payment-service",
  "timestamp": "2026-10-30T10:00:00Z",
  "payload": {
    "status": 500,
    "message": "database connection timeout"
  }
}
```

enters:

```text
HTTP API
   ↓
Event Handler
   ↓
Event Service
   ↓
Event Repository
   ↓
PostgreSQL Transaction
   ├── events
   └── outbox
```

Then:

```text
Outbox Publisher
      ↓
Kafka
      ↓
Incident Consumer
      ↓
Incident Processor
      ↓
Incident Detection
      ↓
Redis correlation state
      ↓
PostgreSQL incident
      ↓
Redis incident projection
```

---

## 4. Where Can I See This in the Code?

### Event ingestion

```text
services/event-api/internal/handler/event_handler.go
services/event-api/internal/service/event_service.go
services/event-api/internal/repository/event_repository.go
```

### Database

```text
services/event-api/internal/database/database.go
```

### Event model

```text
services/event-api/internal/model/events.go
```

### Outbox

```text
services/event-api/internal/publisher/outbox_publisher.go
services/event-api/internal/repository/outbox_repository.go
```

### Kafka

```text
services/event-api/internal/kafka/producer.go
services/event-api/cmd/kafka-test/main.go
```

### Consumer

```text
services/event-api/cmd/consumer/main.go
services/event-api/internal/consumer/incident_consumer.go
```

### Idempotency

```text
services/event-api/internal/repository/processed_event_repository.go
```

### Incident detection

```text
services/event-api/internal/service/event_classifier.go
services/event-api/internal/service/incident_detection.go
services/event-api/internal/service/incident_severity.go
```

### Incident processing

```text
services/event-api/internal/service/incident_processor.go
```

### Incident lifecycle

```text
services/event-api/internal/service/incident_service.go
services/event-api/internal/repository/incident_repository.go
```

### Redis

```text
services/event-api/internal/redis/client.go
services/event-api/internal/redis/incident_state.go
```

### Database migrations

```text
services/event-api/migration/
```

---

## 5. Current Repository Structure

```text
OpsPilot/
├── docker-compose.yaml
├── Initial_architecture.png
├── .env
├── docs/
│   └── architecture/
│       ├── 01-system-overview.md
│       ├── 02-postgresql.md
│       ├── 03-transactional-outbox.md
│       ├── 04-kafka.md
│       ├── 05-consumer-idempotency.md
│       ├── 06-incident-detection.md
│       ├── 07-redis.md
│       ├── 08-incident-state-projection.md
│       ├── 09-incident-lifecycle.md
│       ├── 10-failure-modes.md
│       ├── 11-bugs-and-fixes.md
│       ├── 12-testing-and-verification.md
│       └── 13-known-gaps-and-future-hardening.md
└── services/
    └── event-api/
        ├── cmd/
        ├── internal/
        ├── migration/
        ├── go.mod
        └── go.sum
```

---

## 6. Important Architecture Principle

OpsPilot deliberately separates:

```text
Durability
    PostgreSQL

Transport
    Kafka

Fast temporary state
    Redis

Business processing
    Go services

AI reasoning
    Context Builder + RAG + LLM

Human decision
    Approval layer
```

This separation makes the system easier to reason about, test, scale, and recover.

---

## 7. Current vs Future Architecture

### Currently implemented

```text
Event API
   ↓
PostgreSQL
   ↓
Transactional Outbox
   ↓
Kafka
   ↓
Consumer
   ↓
Idempotency
   ↓
Incident Detection
   ↓
Redis
   ↓
Incident Lifecycle
   ↓
PostgreSQL + Redis Projection
```

### Future implementation

```text
Incident
   ↓
Context Builder
   ↓
Evidence collection
   ↓
RAG
   ↓
LLM
   ↓
Structured incident analysis
   ↓
Recommendations
   ↓
Human approval
   ↓
Safe action execution
```

We should not claim the AI layer is implemented yet. It is the next major stage of OpsPilot.

---

## 8. Major Engineering Decisions

| Decision | Reason |
|---|---|
| PostgreSQL | Durable relational source of truth |
| JSONB | Flexible event payload |
| Transactional Outbox | Prevent DB/event publication inconsistency |
| Kafka | Replay, retention, independent consumers |
| Kafka key = service | Preserve ordering for events from same service |
| At-least-once processing | Practical distributed-system delivery model |
| processed_events | Consumer idempotency |
| Redis ZSET | Sliding-window incident detection |
| Redis projection | Fast incident reads |
| PostgreSQL first | Business state must remain durable |
| AI separated from ingestion | AI should not block event acceptance |
| Human approval | Prevent unsafe automated remediation |

---

## 9. The Final Product Story

The final demo should be understandable in one flow:

```text
A production service starts failing
        ↓
OpsPilot receives events
        ↓
Events are durably stored
        ↓
Kafka distributes them
        ↓
Incident engine correlates failures
        ↓
Incident is created
        ↓
AI gathers evidence
        ↓
RAG finds relevant runbooks/history
        ↓
LLM analyzes the incident
        ↓
OpsPilot explains likely root cause
        ↓
Evidence is shown
        ↓
Recommended remediation is generated
        ↓
Engineer approves
        ↓
Safe action is executed
        ↓
Everything is auditable
```

That is the architecture we are building toward.
