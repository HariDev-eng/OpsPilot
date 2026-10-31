# OpsPilot — Bugs, Failures and Engineering Lessons

## 1. Why Keep a Bugs-and-Fixes Document?

The architecture is not just the final code.

The debugging process teaches important engineering concepts.

This document records failures that occurred during development and the reasoning used to fix them.

---

# 2. Bug: Using Transaction After Commit

## Symptom

The consumer produced:

```text
tx is closed
```

The logs looked approximately like:

```text
new incident created:
incident_id=8
service=projection-test-service
severity=HIGH

processing failed:
event_id=projection-test-5
attempt=1
error=tx is closed
```

The consumer retried.

---

## Root Cause

The code effectively did:

```go
tx.Commit(ctx)

incident, err := p.incidentRepository.GetByIDTx(
    ctx,
    tx,
    incidentID,
)
```

Once:

```go
tx.Commit(ctx)
```

returns successfully, the transaction is closed.

Therefore:

```text
tx
```

cannot be reused.

---

## Fix

We moved the read before commit:

```text
BEGIN
 ↓
create/update incident
 ↓
attach event
 ↓
read incident
 ↓
COMMIT
 ↓
Redis projection
```

The fixed code obtains:

```go
incident, err := p.incidentRepository.GetByIDTx(
    ctx,
    tx,
    incidentID,
)
```

before:

```go
tx.Commit(ctx)
```

---

## Lesson

A transaction object has a lifecycle:

```text
BEGIN
 ↓
ACTIVE
 ↓
COMMIT / ROLLBACK
 ↓
CLOSED
```

Never use the transaction after it has ended.

---

# 3. Bug: Redis Projection Missing

The original transaction bug prevented the processor from reaching the projection code correctly.

The intended flow was:

```text
incident created
 ↓
transaction commit
 ↓
Redis projection
```

But because the processor attempted to use the closed transaction:

```text
processing failed
 ↓
retry
```

the projection did not happen correctly.

After fixing the transaction lifecycle, the projection worked.

---

# 4. Bug: Duplicate Redis Projection Call

During processor development, the code contained duplicate:

```text
SetIncidentState()
```

calls.

This was unnecessary.

We cleaned the flow to:

```text
database transaction
       ↓
commit
       ↓
one projection
```

This made the code easier to reason about.

---

# 5. Failed Test Helper Assumption

A Redis test was written using:

```text
setupRedisTestClient
```

but that helper did not actually exist.

The test failed to compile.

Instead of creating an undocumented helper just to make the test pass, we corrected the test to use the actual repository/test infrastructure.

The lesson:

> Tests should reflect the real system, not imagined abstractions.

---

# 6. DLQ Topic Missing

During Redis failure testing, processing eventually exhausted retries.

The consumer attempted to publish to:

```text
ops.events.dlq
```

Kafka returned:

```text
Unknown Topic Or Partition
```

---

## What This Taught Us

The application had DLQ logic, but the infrastructure was not guaranteed to exist.

There is a difference between:

```text
application supports DLQ
```

and:

```text
deployment guarantees DLQ availability
```

Production deployment needs both.

---

# 7. Redis Failure Revealed an Architectural Dependency

We stopped Redis and sent failure events.

The events reached Kafka.

The consumer received them.

But incident detection could not maintain its sliding window.

Therefore:

```text
Kafka = healthy
PostgreSQL = healthy
Redis = unavailable
```

resulted in:

```text
incident detection failure
```

This was valuable because it exposed a hidden dependency.

---

# 8. Redis Projection Failure Was Different

We then tested an already-created incident.

Redis was stopped.

We acknowledged the incident.

PostgreSQL successfully became:

```text
ACKNOWLEDGED
```

Redis failed to update.

This was expected because Redis is a projection for this operation.

The difference is:

```text
Detection:
Redis failure → current system cannot detect

Projection:
Redis failure → state remains correct in PostgreSQL
```

---

# 9. Event Timestamp Testing

We tested failure events using timestamps.

When events were more than the configured window apart, the detector did not maintain the same failure count.

This initially looked suspicious.

But it was actually the expected sliding-window behavior.

For example:

```text
10-second window

event 1 → t=0
event 2 → t=2
event 3 → t=4
event 4 → t=6
event 5 → t=8

count = 5
```

But:

```text
event 1 → t=0
event 2 → t=12
```

should not be counted together.

This verified that the window is actually time-based rather than simply accumulating events forever.

---

# 10. Controlled Severity Test

We then used controlled timestamps so events fell inside the same window.

At:

```text
5 failures
```

the incident became:

```text
HIGH
```

At:

```text
20 failures
```

it became:

```text
CRITICAL
```

This validated both:

```text
threshold
```

and:

```text
severity escalation
```

---

# 11. Git Repository Mistake

Git was initially initialized in the wrong directory:

```text
~
```

instead of:

```text
~/OpsPilot
```

This was corrected by removing the accidental home-directory repository and initializing Git at the project root.

Correct repository boundary:

```text
~/OpsPilot
```

This matters because source control should represent the actual project boundary.

---

# 12. Architecture Lesson From These Bugs

Every bug exposed a deeper concept.

| Bug | Concept learned |
|---|---|
| tx is closed | Transaction lifecycle |
| Missing projection | Commit-before-projection |
| Duplicate projection | Clear responsibility boundaries |
| Missing test helper | Tests must match real infrastructure |
| Missing DLQ topic | Application + infrastructure contract |
| Redis detection failure | Critical dependency analysis |
| Redis projection failure | Source of truth vs derived state |
| Timestamp behavior | Sliding-window semantics |
| Severity test | Deterministic verification |
| Git root mistake | Repository boundaries |

---

# 13. Why These Bugs Are Valuable

These are not just mistakes to hide.

They demonstrate that the system was actually exercised.

A production engineer should be able to explain:

```text
What broke?
Why did it break?
How did you identify it?
What was the root cause?
Why did the fix work?
What architectural lesson did you learn?
```

OpsPilot documentation should preserve that reasoning.

---

# 14. Debugging Philosophy

When something fails, we should not immediately patch the symptom.

Instead:

```text
Observe
   ↓
Reproduce
   ↓
Identify boundary
   ↓
Find root cause
   ↓
Understand failure semantics
   ↓
Fix
   ↓
Retest
   ↓
Document
```

That is the development process used throughout OpsPilot.

---

# 15. Most Important Lesson So Far

The biggest lesson is that distributed systems are mostly about **failure boundaries**.

The happy path is:

```text
event
 ↓
DB
 ↓
Kafka
 ↓
consumer
 ↓
incident
 ↓
Redis
```

The real engineering challenge is:

```text
What happens if any arrow fails?
```

That question drives the rest of the architecture.
