# OpsPilot — Kafka Architecture

## 1. Why Kafka?

OpsPilot needs an asynchronous event backbone.

We could have used RabbitMQ, and RabbitMQ would be a valid choice for many workloads.

We selected Kafka because OpsPilot benefits from:

- Event retention
- Replay
- Multiple independent consumers
- Partitioning
- Consumer groups
- High-throughput event streams

The important requirement is not:

> Kafka is better than RabbitMQ.

The requirement is:

> OpsPilot needs durable event streams that multiple independent consumers can process.

---

## 2. Kafka's Role

Kafka sits between event publication and event processing.

```text
PostgreSQL Outbox
       ↓
Outbox Publisher
       ↓
Kafka
       ↓
Consumers
```

Current topic:

```text
ops.events
```

DLQ topic:

```text
ops.events.dlq
```

---

## 3. Topic Creation

The main topic was created with:

```bash
docker exec opspilot-kafka /opt/kafka/bin/kafka-topics.sh \
  --create --topic ops.events \
  --bootstrap-server localhost:9092 \
  --partitions 3 \
  --replication-factor 1
```

We currently use:

```text
3 partitions
```

because we want to demonstrate partitioning and independent consumption.

---

## 4. Partitions

Kafka does not provide global ordering across all partitions.

For example:

```text
Topic: ops.events

Partition 0
   event A
   event D

Partition 1
   event B
   event E

Partition 2
   event C
```

Kafka guarantees ordering within an individual partition.

It does not guarantee:

```text
A < B < C < D < E
```

across the whole topic.

---

## 5. Kafka Message Key

Our producer uses:

```text
service
```

as the Kafka message key.

For example:

```text
payment-service
payment-service
payment-service
```

will hash to the same partition under the same partitioning strategy.

This gives us ordering for events belonging to the same service.

Conceptually:

```text
payment-service
       ↓
same partition
       ↓
ordered service events
```

This was an intentional decision because incident correlation often cares about the sequence of events from the same service.

---

## 6. Producer

Code:

```text
internal/kafka/producer.go
```

The producer uses `kafka-go`.

We configured:

```text
RequiredAcks = kafka.RequireAll
```

This asks Kafka for stronger acknowledgement semantics.

The producer is used by the outbox publisher.

Architecture:

```text
Outbox Publisher
      ↓
Kafka Producer
      ↓
Kafka Broker
```

---

## 7. Why Require All Acknowledgements?

We want the producer to receive acknowledgement from the required in-sync replicas.

In our local environment:

```text
replication-factor = 1
```

so this is not providing production-grade redundancy.

The configuration is still useful because the code expresses the intended producer reliability model.

In production, replication would need to be greater than one.

---

## 8. Consumer Groups

We deliberately want different processing responsibilities to be independent.

Conceptually:

```text
                    ops.events
                        │
          ┌─────────────┼─────────────┐
          │             │             │
          ▼             ▼             ▼
   Incident Group  Analytics Group  Notification Group
```

If the incident consumer processes an event, that does not mean the analytics consumer has processed it.

Each consumer group maintains its own offsets.

---

## 9. Current Consumer

Code:

```text
cmd/consumer/main.go
internal/consumer/incident_consumer.go
```

The consumer:

```text
FetchMessage
      ↓
Process
      ↓
CommitMessages
```

The important rule is:

> Commit the Kafka offset only after successful processing.

---

## 10. Retry

Current processing retries:

```text
Attempt 1
   ↓
1 second
Attempt 2
   ↓
2 seconds
Attempt 3
   ↓
4 seconds
```

If processing eventually succeeds:

```text
Commit Kafka offset
```

If processing permanently fails:

```text
Send to DLQ
   ↓
Commit Kafka offset
```

The event is not allowed to block the partition forever.

---

## 11. Dead Letter Queue

The DLQ topic is:

```text
ops.events.dlq
```

The purpose is to preserve events that could not be processed successfully after retries.

Instead of:

```text
event
 ↓
failure forever
 ↓
consumer stuck
```

we use:

```text
event
 ↓
retry
 ↓
retry
 ↓
retry
 ↓
DLQ
 ↓
commit offset
```

This allows the main consumer pipeline to continue.

---

## 12. DLQ Failure We Encountered

During a Redis outage, events eventually exhausted retries.

The consumer attempted to publish to:

```text
ops.events.dlq
```

but Kafka returned:

```text
Unknown Topic Or Partition
```

This exposed an important operational lesson:

> A DLQ is only useful if the DLQ infrastructure itself is available and correctly provisioned.

The application code alone is not enough.

Production deployment should ensure:

```text
main topic exists
DLQ topic exists
partitions are correct
permissions are correct
monitoring exists
```

---

## 13. Host vs Docker Networking

When the Go application runs on the host:

```text
localhost:9092
```

is used.

When the application itself runs inside Docker Compose, it should use the Kafka service name:

```text
kafka:9092
```

because containers communicate through the Docker network.

This distinction is important.

```text
Host → Kafka
localhost:9092

Docker container → Kafka
kafka:9092
```

---

## 14. Kafka Delivery Guarantee

Our system is designed around:

```text
At-least-once delivery
```

This means the same event can potentially be delivered more than once.

We therefore combine:

```text
Kafka
+
processed_events
+
idempotent processing
```

This is more realistic than assuming exactly-once end-to-end behavior.

---

## 15. Kafka and Incident Detection

The event flow is:

```text
event-api
   ↓
PostgreSQL
   ↓
outbox
   ↓
outbox publisher
   ↓
ops.events
   ↓
incident consumer
   ↓
incident processor
```

The consumer does not directly query the HTTP API.

Kafka decouples ingestion from processing.

---

## 16. Why This Decoupling Matters

Imagine incident detection suddenly becomes slow.

Without Kafka:

```text
HTTP request
    ↓
slow incident detection
    ↓
request waits
```

With Kafka:

```text
HTTP request
    ↓
persist event
    ↓
202 Accepted
```

Processing happens asynchronously.

This provides:

- Decoupling
- Buffering
- Backpressure
- Independent scaling
- Failure isolation

---

## 17. Where to See Kafka Code

```text
internal/kafka/producer.go
internal/kafka/dlq_producer.go
internal/publisher/outbox_publisher.go
internal/consumer/incident_consumer.go
cmd/consumer/main.go
```

Kafka configuration:

```text
docker-compose.yaml
```

---

## 18. Main Kafka Lesson

Kafka is not simply:

> A queue.

For OpsPilot it is an event backbone that provides:

```text
Durable event stream
        +
Partitioning
        +
Consumer groups
        +
Retention
        +
Replay capability
        +
Independent consumers
```

The system is designed around Kafka's actual semantics rather than pretending it provides global ordering or exactly-once processing.
