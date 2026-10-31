# OpsPilot — Redis Architecture

## 1. Why Redis?

Redis is used in OpsPilot for **fast state**, not as the primary database.

We currently use Redis for two different problems:

```text
Redis
│
├── Incident detection state
│
└── Incident state projection
```

These should be considered separate responsibilities.

---

## 2. Redis Role #1 — Sliding Window

For incident detection we need to answer:

> How many failure events happened for this service during the last 10 seconds?

PostgreSQL could theoretically do this, but doing it repeatedly for high-frequency operational events would put unnecessary load on the primary database.

Redis is better suited for fast ephemeral state.

---

## 3. Redis Sorted Set

We use a Redis:

```text
ZSET
```

for each service.

Key:

```text
incident:failures:<service>
```

Example:

```text
incident:failures:payment-service
```

Members:

```text
event-101
event-102
event-103
event-104
event-105
```

Scores:

```text
event timestamp
```

So conceptually:

```text
ZSET

event-101 → 1001
event-102 → 1003
event-103 → 1005
event-104 → 1007
event-105 → 1009
```

---

## 4. Sliding Window

Current window:

```text
10 seconds
```

Suppose current event time is:

```text
1009
```

Cutoff:

```text
999
```

Events older than the cutoff are removed.

The operation is:

```text
ZADD
 ↓
ZREMRANGEBYSCORE
 ↓
EXPIRE
 ↓
ZCARD
```

This gives:

```text
number of recent events
```

---

## 5. Lua Script

The code is:

```text
internal/redis/client.go
```

The Redis operations are wrapped in a Lua script.

Conceptually:

```lua
ZADD key timestamp event_id

ZREMRANGEBYSCORE key -inf cutoff

EXPIRE key ttl

return ZCARD key
```

The reason for Lua is atomic execution.

We want the sequence:

```text
add event
remove old events
refresh expiration
count
```

to behave as one logical operation.

---

## 6. TTL

The sorted-set key gets a TTL slightly longer than the detection window.

Current logic:

```text
TTL ≈ window + 5 seconds
```

The purpose is to prevent unused service keys from living forever.

For example:

```text
10-second window
+
5-second buffer
=
15-second TTL
```

---

## 7. Redis Role #2 — Incident Projection

The second Redis use case is:

```text
incident:<incidentID>
```

Example:

```text
incident:9
```

Stored as JSON:

```json
{
  "incident_id": 9,
  "service": "projection-fixed-service",
  "severity": "HIGH",
  "status": "OPEN"
}
```

Code:

```text
internal/redis/incident_state.go
```

---

## 8. Why Projection?

PostgreSQL is the source of truth.

But a UI may repeatedly ask:

```text
What is the current state of incident 9?
```

A Redis projection can provide fast access.

The architecture becomes:

```text
PostgreSQL
    │
    │ source of truth
    ▼
Redis
    │
    │ derived fast read model
    ▼
UI/API
```

---

## 9. Projection Is Not the Source of Truth

Suppose:

```text
PostgreSQL update → SUCCESS
Redis update → FAILURE
```

The incident is still correct.

We have:

```text
PostgreSQL = current
Redis      = stale
```

This is an intentional consistency model.

We do not roll back a successful business operation merely because a cache/projection failed.

---

## 10. Failure Test

We deliberately stopped Redis.

Then we acknowledged an existing incident.

PostgreSQL showed:

```text
incident 10
HIGH
ACKNOWLEDGED
```

while Redis could not update because it was unavailable.

The application logged:

```text
failed to update incident state projection
```

This proved:

```text
PostgreSQL lifecycle operation
        ↓
still succeeds
```

even when:

```text
Redis projection
        ↓
fails
```

This validated the source-of-truth design.

---

## 11. Different Reliability Requirements

This is one of the most important lessons from Day 5.

### Detection Redis

```text
Redis failure
    ↓
currently detection fails
```

This means Redis is currently a critical dependency for that path.

### Projection Redis

```text
Redis failure
    ↓
PostgreSQL remains correct
    ↓
projection becomes stale
```

This path is best-effort.

---

## 12. Redis Client

Code:

```text
internal/redis/client.go
```

The application creates:

```go
redis.NewClient(...)
```

and checks connectivity with:

```go
Ping(...)
```

The client is reused rather than creating a new Redis connection for every operation.

---

## 13. Incident State API

The Redis client exposes:

```text
SetIncidentState()
GetIncidentState()
DeleteIncidentState()
```

This keeps Redis implementation details inside the Redis package.

The rest of the application does not need to know the exact Redis command structure.

---

## 14. Key Naming

Current keys:

```text
incident:failures:<service>
incident:<incidentID>
```

These represent two different domains.

A future improvement could namespace keys more explicitly:

```text
opspilot:incident:failures:<service>
opspilot:incident:<id>
```

This is not currently implemented.

---

## 15. Test Failure We Encountered

A Redis test was previously suggested using a helper:

```text
setupRedisTestClient
```

That helper did not exist in the repository.

This caused a test compilation failure.

We removed the dependency rather than inventing infrastructure that wasn't actually present.

The existing Redis tests then passed.

The lesson:

> Tests must reflect actual repository infrastructure, not assumed helper functions.

---

## 16. Redis and PostgreSQL Relationship

The intended hierarchy is:

```text
PostgreSQL
    ↓
authoritative state

Redis
    ↓
derived/temporary state
```

Not:

```text
Redis
    ↓
PostgreSQL
```

---

## 17. Future Redis Hardening

Potential improvements:

```text
Redis Sentinel / Cluster
Redis persistence strategy
Connection retry
Circuit breaker
Detection fallback
Projection reconciliation
Key namespacing
Metrics
Redis health monitoring
```

A particularly important improvement is to separate detection reliability from projection reliability.

---

## 18. Where to See the Code

Redis connection:

```text
internal/redis/client.go
```

Sliding window:

```text
internal/redis/client.go
```

Incident state:

```text
internal/redis/incident_state.go
```

Detection:

```text
internal/service/incident_detection.go
```

Processor:

```text
internal/service/incident_processor.go
```

---

## 19. Main Lesson

Redis is not simply:

> A faster PostgreSQL.

In OpsPilot it has specific roles:

```text
Redis ZSET
    ↓
fast temporary correlation state

Redis JSON
    ↓
fast incident state projection
```

while:

```text
PostgreSQL
    ↓
durable source of truth
```

The reliability requirements of each Redis role are different, and that distinction must remain explicit.
