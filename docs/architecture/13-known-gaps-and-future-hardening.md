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

# 11. AI Incident Intelligence — Implemented in V1

The AI pipeline is implemented and has been exercised against the deployed V1 application. The deployed Go server is wired to Cloudflare Workers AI; Ollama provider implementations may remain in the repository for local experimentation, but they are not the configured provider used by the current Render deployment.

Implemented V1 flow:

```text
Incident
   ↓
AI Context Builder
   ├── Incident metadata and associated events
   └── EmbeddingProvider
          ↓
     Cloudflare BGE embeddings
          ↓
     PostgreSQL + pgvector retrieval
          ↓
     IncidentContext
          ↓
     Cloudflare Gemma 4
          ↓
     Structured response parsing
          ↓
     Analysis, evidence and action validation
          ↓
     AI analysis history + recommended actions
```

Verified prototype behavior includes a completed analysis for a deployed synthetic incident, with evidence references to the five associated events and three persisted investigation recommendations.

The following remain gaps rather than implemented capabilities:

- RAG retrieval-quality and answer-quality evaluation.
- Calibrated confidence and claim-level semantic verification.
- Knowledge document lifecycle management and richer chunking.
- AI usage budgets, rate limiting and operator-level usage auditing.
- Strong authentication and authorization for analysis and action mutations.

The model is decision support. Backend validation and human review remain necessary; a model-reported likely cause is not a confirmed root cause.

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

V1 now protects the action mutation endpoints (approve, reject and reconcile) with the shared `OPSPILOT_OPERATOR_SECRET` Bearer credential. The authenticated request context supplies an audit actor label from `OPSPILOT_OPERATOR_ID`.

This is a single-operator prototype gate, not user authentication or RBAC. The following remain open:

```text
multi-user identity and session management
role-based authorization
separation of requester, approver and reconciler roles
protection of all state-changing endpoints
protection and rate limiting of billable AI inference endpoints
service-to-service authentication
secret rotation and management
comprehensive audit logging and monitoring
```

The database dump has been removed from the current `main` tree and dump patterns are now ignored. The blob may still exist in earlier Git history. Inspect the local copy; if it contains application data, credentials or other non-public information, rewrite repository history and rotate any exposed credentials.

---

# 20. V1 Operator UI — Implemented

V1 includes a deployed React operator interface with:

```text
Overview dashboard
Incident list and incident details
AI analysis, evidence and recommendations
Action management and approval state
Integration management
System health
Help and architecture guidance
Responsive mobile layouts
```

The UI is usable as a portfolio demonstration. Future UX work includes richer event exploration, knowledge-base authoring, AI analysis comparisons, accessible interaction testing and deeper mobile-device verification.

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

## Implemented in V1

```text
Event ingestion API
PostgreSQL event persistence
Transactional outbox
Kafka publishing and consumption
Consumer retries and DLQ handling
Consumer idempotency
Redis sliding-window detection
Incident classification and severity calculation
Incident creation and lifecycle
Redis incident projection
AI context builder
Cloudflare BGE embeddings
PostgreSQL + pgvector retrieval
Cloudflare Gemma 4 incident analysis
Structured output parsing and validation
Evidence reference validation
AI analysis persistence and history API
Recommended action creation
Action approval/rejection/reconciliation workflow
React operator dashboard
Incident, action, integration and health views
Architecture documentation
Synthetic end-to-end deployment verification
```

## Known gaps / not yet production-hardened

```text
Authentication and role-based authorization
Authorization on all state-changing and billable endpoints
Per-user identity and audit attribution
API rate limiting and abuse protection
AI/RAG evaluation and calibrated confidence
Claim-level semantic grounding
Knowledge-base lifecycle and richer chunking
Automated recovery tests for all infrastructure failure modes
Production observability and alerting
Independent API/consumer scaling and high availability
Kubernetes deployment
```

# 26. Engineering Principle

OpsPilot V1 now includes an implemented and deployed AI-assisted incident-analysis pipeline. The accurate description is:

```text
OpsPilot is an incident-intelligence prototype that combines
event-driven incident detection with RAG-based AI analysis,
evidence-reference validation and controlled response recommendations.
```

The system must not describe a model-reported likely cause as a confirmed root cause. Evidence references are validated against supplied sources, but semantic claim accuracy and confidence calibration remain open evaluation problems.

---

# 27. Recruiter-Readiness Strategy

V1's main workflow is deployed and demonstrated. The next work should focus on hardening and verification rather than adding another large feature:

```text
1. Secure every state-changing and billable API endpoint
2. Inspect and purge sensitive artifacts from Git history if needed
3. Expand automated tests for failure and recovery paths
4. Add RAG/LLM quality evaluation and calibrated confidence
5. Improve observability, rate limiting and deployment resilience
6. Consider separate API/consumer scaling and high availability
```

This keeps the distinction clear between a working portfolio prototype and a production-grade incident response platform.
