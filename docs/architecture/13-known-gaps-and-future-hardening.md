# OpsPilot — Known Gaps and Future Hardening

## 1. Why This Document Exists

OpsPilot currently has a working foundation, but it is not yet a production-grade platform.

The purpose of this document is to explicitly separate:

```text
Implemented
```

from:

```text
Known limitation
```

and:

```text
Future hardening
```

We should not claim future features as already implemented.

---

# 2. Current Implemented Architecture

Currently implemented:

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
Redis Sliding Window
    ↓
Incident Creation
    ↓
Incident Lifecycle
    ↓
PostgreSQL + Redis Projection
```

This is the current foundation.

---

# 3. Gap: Stale PUBLISHING Outbox Records

Current lifecycle:

```text
PENDING
 ↓
PUBLISHING
 ↓
PUBLISHED
```

Failure case:

```text
PENDING
 ↓
PUBLISHING
 ↓
process crashes
```

The record can remain:

```text
PUBLISHING
```

without automatic recovery.

---

## Future Solution

Add:

```text
claimed_at
```

or equivalent lease metadata.

Then:

```text
PUBLISHING
+
lease expired
```

becomes:

```text
PENDING
```

A recovery worker can retry it.

---

# 4. Gap: Concurrent Incident Creation

Current logic:

```text
Find open incident
       ↓
if none
       ↓
Create
```

Two processors can potentially race:

```text
Consumer A → Find → none
Consumer B → Find → none

Consumer A → Create
Consumer B → Create
```

This can create duplicate incidents.

---

## Future Solution

Possible strategies:

```text
database uniqueness constraint
database advisory locks
service-level serialization
partitioning by service
transaction isolation
```

The final choice should be based on the desired concurrency model.

---

# 5. Gap: Redis Detection Availability

Current detector depends on Redis.

Therefore:

```text
Redis down
    ↓
detection unavailable
```

This is a significant reliability weakness.

---

## Possible Improvements

### Option 1 — Redis HA

Use:

```text
Redis Sentinel
```

or:

```text
Redis Cluster
```

### Option 2 — Local fallback

Maintain limited in-process state.

This introduces consistency and scaling complications.

### Option 3 — Durable detection state

Use PostgreSQL or another durable stream/state mechanism.

This improves durability but may increase database load.

The correct solution requires evaluating:

```text
event rate
latency
availability requirements
failure tolerance
cost
```

We should not blindly add complexity.

---

# 6. Gap: Redis Projection Reconciliation

Current projection:

```text
PostgreSQL
 ↓
Redis
```

If Redis is unavailable:

```text
PostgreSQL correct
Redis stale
```

But we currently do not automatically reconcile stale Redis state.

---

## Future Solution

A reconciliation process can periodically compare:

```text
PostgreSQL
vs
Redis
```

and repair missing/stale projections.

A stronger architecture is:

```text
PostgreSQL transaction
      ↓
incident lifecycle outbox
      ↓
Kafka
      ↓
projection consumer
      ↓
Redis
```

This makes projection updates durable and replayable.

---

# 7. Gap: incidents.event_id

Current incidents schema contains:

```text
event_id
```

but incidents can contain multiple events through:

```text
incident_events
```

Therefore the current field is semantically redundant.

---

## Future Improvement

Rename:

```text
event_id
```

to:

```text
trigger_event_id
```

if we want to retain the event that caused the incident.

Alternatively, remove it if the relationship is sufficiently represented through:

```text
incident_events
```

---

# 8. Gap: Event-Time Semantics

Current detection uses:

```text
event.Timestamp
```

This is useful but creates edge cases.

Examples:

```text
late event
future event
clock skew
incorrect timestamp
```

Future rules should define:

```text
maximum acceptable event age
maximum future timestamp
clock-skew tolerance
late-event policy
```

---

# 9. Gap: Correlation Is Currently Simple

Current correlation is mostly:

```text
same service
+
failure threshold
+
time window
```

Real incidents are more complex.

Example:

```text
payment-service
    ↓
database latency increases
    ↓
Redis timeout
    ↓
Kafka lag
    ↓
payment 500s
```

These events may belong to one underlying incident even though they involve different services/components.

---

# 10. Future Correlation Engine

A richer correlation engine could consider:

```text
service
dependency graph
event type
timestamp
trace ID
request ID
deployment ID
infrastructure
metrics
logs
Kubernetes events
Kafka lag
database latency
```

Then the incident graph might look like:

```text
Deployment
     ↓
DB latency
     ↓
connection pool exhaustion
     ↓
payment failures
     ↓
HTTP 500 spike
```

This richer evidence becomes especially valuable for AI.

---

# 11. AI Layer Is Not Yet Implemented

The AI portion of OpsPilot is planned but has not yet been built.

The intended architecture is:

```text
Incident
   ↓
AI Context Builder
   ↓
Evidence collection
   ↓
RAG
   ↓
LLM
   ↓
Structured analysis
   ↓
Recommendations
```

The AI must not be treated as an oracle.

---

# 12. AI Context Builder

The context builder should collect information such as:

```text
Incident metadata
Recent events
Related events
Service information
Recent deployments
Relevant metrics
Logs
Runbooks
Historical incidents
Dependency information
```

The goal is:

```text
raw incident
     ↓
structured evidence
```

before asking the LLM to reason.

---

# 13. RAG

RAG means:

```text
Retrieval-Augmented Generation
```

Instead of asking:

```text
"What is wrong?"
```

with no context, OpsPilot should retrieve relevant knowledge first.

Possible knowledge sources:

```text
runbooks
past incidents
service documentation
architecture documentation
operational procedures
known failure patterns
```

Then:

```text
retrieved evidence
       ↓
LLM context
       ↓
analysis
```

---

# 14. LLM Output Should Be Structured

We should not rely on free-form text only.

The eventual AI response should resemble:

```json
{
  "summary": "...",
  "likely_root_causes": [
    {
      "cause": "...",
      "confidence": 0.82,
      "evidence": [
        "..."
      ]
    }
  ],
  "recommended_actions": [
    {
      "action": "...",
      "risk": "medium",
      "requires_approval": true
    }
  ]
}
```

This makes AI output usable by software.

---

# 15. AI Reliability Gap

The AI layer will need safeguards against:

```text
hallucination
unsupported conclusions
irrelevant retrieval
bad tool calls
unsafe recommendations
prompt injection
incorrect confidence
stale knowledge
```

The architecture should therefore make evidence visible.

The AI should be able to say:

```text
Likely root cause
```

rather than pretending:

```text
Certain root cause
```

when evidence is incomplete.

---

# 16. Human Approval

OpsPilot should not immediately execute dangerous remediation.

For example:

```text
AI recommendation:
restart payment-service
```

should become:

```text
Recommendation
      ↓
Human approval
      ↓
Action execution
```

not:

```text
LLM
 ↓
restart production
```

This is a major safety boundary.

---

# 17. Observability Gap

OpsPilot itself needs observability.

We eventually need:

```text
structured logs
metrics
traces
health checks
readiness checks
latency metrics
Kafka consumer lag
outbox backlog
Redis latency
AI latency
LLM token usage
AI failure rate
```

The incident platform should itself be observable.

---

# 18. Deployment Gap

Current development uses:

```text
Docker Compose
```

Future deployment should include Kubernetes.

Expected production components:

```text
Kubernetes
 ├── event-api
 ├── consumer
 ├── publisher
 ├── PostgreSQL
 ├── Kafka
 ├── Redis
 └── AI services
```

We should add:

```text
ConfigMaps
Secrets
Services
Ingress
Liveness probes
Readiness probes
Resource requests
Resource limits
Autoscaling where appropriate
```

---

# 19. Security Gap

Future security work includes:

```text
authentication
authorization
service authentication
API rate limiting
secret management
TLS
audit logging
RBAC
AI tool authorization
approval authorization
```

Especially important:

```text
AI tool execution
```

must have strict permissions.

---

# 20. UI Gap

The final UI should become an OpsPilot Control Center.

Planned areas:

```text
Dashboard
Incident Details
AI Analysis
Event Explorer
Runbooks / Knowledge Base
```

Incident detail should eventually show:

```text
Severity
Status
Timeline
Related events
Evidence
AI analysis
Likely causes
Confidence
Recommendations
Approval actions
```

---

# 21. Testing Gap

Current testing covers important backend behavior.

Future testing needs to include:

```text
concurrency
failure recovery
Kafka restart
Redis restart
PostgreSQL restart
stale outbox recovery
projection reconciliation
AI evaluation
RAG retrieval quality
structured output validation
tool authorization
end-to-end deployment
```

---

# 22. Performance Gap

Future benchmarking should measure:

```text
event ingestion throughput
Kafka throughput
consumer throughput
incident detection latency
Redis latency
database query latency
AI context construction latency
RAG latency
LLM latency
end-to-end incident analysis latency
```

We should measure before optimizing.

---

# 23. Cost Gap

AI introduces new costs.

We should track:

```text
tokens
LLM calls
embedding calls
retrieval operations
context size
average incident cost
```

The system should avoid unnecessary LLM calls.

For example:

```text
every raw event
    ↓
LLM call
```

would be wasteful.

Better:

```text
many raw events
    ↓
incident correlation
    ↓
one meaningful AI investigation
```

---

# 24. Final Target Architecture

The long-term architecture is:

```text
                  External Systems
                         │
                         ▼
                  Event Ingestion
                         │
                         ▼
                  PostgreSQL
                  ┌──────┴──────┐
                  │             │
               Events         Outbox
                                │
                                ▼
                              Kafka
                                │
              ┌─────────────────┼─────────────────┐
              │                 │                 │
              ▼                 ▼                 ▼
        Incident Engine    Analytics       Notifications
              │
              ▼
            Redis
              │
              ▼
          PostgreSQL
              │
              ▼
       AI Context Builder
              │
       ┌──────┴────────┐
       │               │
       ▼               ▼
     RAG            Live Evidence
       │               │
       └──────┬────────┘
              ▼
             LLM
              │
              ▼
     Structured Analysis
              │
              ▼
       Recommendations
              │
              ▼
       Human Approval
              │
              ▼
        Action Executor
              │
              ▼
       Audit / Observability
```

---

# 25. What Is Actually Built vs Planned

## Built

```text
Event ingestion
PostgreSQL persistence
Transactional outbox
Kafka publishing
Kafka consumer
Consumer retries
Consumer idempotency
DLQ producer
Incident classification
Sliding-window detection
Severity calculation
Incident correlation
Incident creation
Incident escalation
Incident lifecycle
Redis detection state
Redis incident projection
Failure testing
Architecture documentation
```

## Not Yet Built

```text
AI context builder
Embeddings
Vector database
RAG pipeline
LLM integration
Structured AI analysis
AI evaluation
AI tool calling
Human approval workflow
Action executor
Advanced correlation
Production observability
Kubernetes deployment
Final UI
```

---

# 26. Engineering Principle

We should never say:

```text
OpsPilot has AI-powered root-cause analysis
```

until the AI pipeline actually exists.

Until then, the accurate description is:

```text
OpsPilot is an incident intelligence platform
with a backend foundation designed to support
AI-powered incident analysis and response.
```

Once RAG + LLM + evidence-backed analysis + approval are implemented and verified, the stronger claim becomes accurate.

---

# 27. Final Development Strategy

The remaining project should proceed in this order:

```text
Day 6
AI Context + RAG
        ↓
Day 7
LLM Incident Intelligence
        ↓
Day 8
AI Response + Human Approval
        ↓
Day 9
Production Hardening + Testing
        ↓
Day 10
Kubernetes Deployment
        ↓
Day 11
UI + Final Deployment + Portfolio
```

The important idea is that the AI layer is being built **on top of a reliable event and incident foundation**, rather than putting an LLM on top of an unreliable application.

That is what makes OpsPilot a serious backend + applied-AI engineering project rather than simply an LLM demo.
