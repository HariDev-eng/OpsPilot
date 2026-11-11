# OpsPilot — AI-Powered Incident Intelligence & Response

[![CI](https://github.com/HariDev-eng/OpsPilot/actions/workflows/ci.yml/badge.svg)](https://github.com/HariDev-eng/OpsPilot/actions/workflows/ci.yml)

OpsPilot is an incident-intelligence prototype that ingests operational events, detects repeated service failures, groups them into incidents, and uses retrieval-augmented generation (RAG) to produce evidence-referenced investigation summaries and response recommendations.

**Live application:** https://opspilot-7q8.pages.dev  
**Backend API:** https://opspilot-7o2e.onrender.com  
**Architecture documentation:** [docs/architecture](docs/architecture/readme.md)

> **Project status:** V1 portfolio prototype. It has been deployed and exercised with synthetic events. It is not presented as a production-ready incident-response service. Authentication/authorization, operational hardening, broader automated coverage and AI-quality evaluation remain important follow-up work.

## What it does

1. Accepts operational events through a Go HTTP API.
2. Persists events and outbox records transactionally in PostgreSQL.
3. Publishes outbox records asynchronously to Kafka.
4. Processes events with an idempotent Kafka consumer.
5. Uses Redis sliding-window state to detect repeated failures and create incidents.
6. Builds incident context from related events and retrieved knowledge.
7. Uses Cloudflare Workers AI (Gemma 4) for structured incident analysis and BGE embeddings for semantic retrieval.
8. Validates analysis evidence and action policies before persisting the result.
9. Tracks recommended actions, approval state and reconciliation workflow in the operator UI.

## V1 architecture

```text
External services / synthetic events
                 |
                 v
          Go Event API
                 |
         PostgreSQL transaction
           |             |
         Events        Outbox
                          |
                          v
                    Outbox Publisher
                          |
                          v
                        Kafka
                          |
                          v
                 Incident Consumer
                          |
                          v
               Redis failure window
                          |
                          v
                Incident Processor
                          |
                          v
             PostgreSQL incidents
                          |
                          v
                 AI Context Builder
                   |           |
                   v           v
             BGE Embeddings  Incident/events
                   |
                   v
             pgvector retrieval
                   |
                   v
             Cloudflare Gemma 4
                   |
                   v
        Evidence/action validation
                   |
                   v
           PostgreSQL AI analysis
                   |
                   v
             React operator UI
```

The deployed Go server starts the outbox publisher and incident consumer in the same Render service to avoid a separate paid worker for this prototype. That is a cost-conscious V1 deployment choice, not the recommended scaling boundary for a larger production system.

## Technology stack

| Area | Technology |
|---|---|
| Backend | Go 1.26, net/http |
| Frontend | React 19, TypeScript, Vite, Tailwind CSS |
| Primary database | PostgreSQL |
| Vector search | pgvector |
| Event streaming | Apache Kafka |
| Detection state | Redis |
| LLM inference | Cloudflare Workers AI — Gemma 4 |
| Embeddings | Cloudflare Workers AI — BGE |
| Deployment | Render, Cloudflare Pages, Neon |

## Verified prototype exercise

The deployed prototype was exercised with synthetic test events:

- 50 events were submitted across 10 simulated services.
- The outbox query showed all 50 messages as published.
- The consumer-processing query showed all 50 events processed.
- The incident API returned 10 HIGH-severity incidents for that batch.
- A separate incident-analysis run returned `COMPLETED`, with five event evidence references and three investigation recommendations; the saved record was retrieved again through the API.

These are controlled synthetic tests, not production customer traffic or a throughput benchmark.

## Run locally

### Prerequisites

- Go 1.26 or newer
- Node.js compatible with the frontend toolchain (Node 22 recommended)
- Docker Compose
- Cloudflare Workers AI account credentials for live AI inference

### Start infrastructure

From the repository root:

```bash
docker compose up -d postgres kafka redis
```

### Configure the API

Set the environment variables in your shell or local environment file. Do not commit real secrets.

| Variable | Purpose |
|---|---|
| `DATABASE_URL` | PostgreSQL connection string |
| `REDIS_URL` or `REDIS_ADDR` | Redis connection |
| `KAFKA_BROKERS` | Kafka broker list |
| `KAFKA_TOPIC` | Main event topic (default: `ops.events`) |
| `KAFKA_DLQ_TOPIC` | Dead-letter topic |
| `KAFKA_CONSUMER_GROUP` | Incident consumer group |
| `CLOUDFLARE_ACCOUNT_ID` | Cloudflare account ID |
| `CLOUDFLARE_API_TOKEN` | Cloudflare API token |
| `CLOUDFLARE_LLM_MODEL` | Optional LLM model override |
| `CLOUDFLARE_EMBEDDING_MODEL` | Optional embedding model override |
| `CORS_ALLOWED_ORIGINS` | Comma-separated allowed browser origins |
| `OPSPILOT_OPERATOR_SECRET` | Shared operator secret for protected management routes |

Apply the SQL migrations under `services/event-api/migration` to your database in order before starting the application.

### Start the Go API and in-process consumer

```bash
cd services/event-api
go test ./...
go run ./cmd/server
```

The server starts the HTTP API, outbox publisher and incident consumer in one process.

### Start the frontend

In another terminal:

```bash
cd frontend
npm ci
npm run lint
npm run build
npm run dev
```

For local-to-deployed API testing, set `VITE_API_BASE_URL` to the API base URL ending in `/api/v1` and allow the frontend origin in `CORS_ALLOWED_ORIGINS`.

## Useful API routes

| Method | Route | Purpose |
|---|---|---|
| GET | `/api/v1/health` | Dependency readiness |
| GET | `/api/v1/ai/status` | AI provider configuration |
| POST | `/api/v1/events` | Ingest an event |
| GET | `/api/v1/incidents` | List incidents |
| GET | `/api/v1/incidents/{id}/events` | List incident events |
| POST | `/api/v1/incidents/{id}/analyze` | Run AI analysis |
| GET | `/api/v1/incidents/{id}/analysis` | Retrieve latest analysis |
| GET | `/api/v1/incidents/{id}/analysis/history` | Retrieve analysis history |
| GET | `/api/v1/incidents/{id}/actions` | List incident actions |
| POST | `/api/v1/actions/{id}/approve` | Approve an action |
| POST | `/api/v1/actions/{id}/reject` | Reject an action |
| POST | `/api/v1/actions/{id}/reconcile` | Reconcile an uncertain execution outcome |

Review the current deployment configuration and security status before exposing mutation endpoints to real users.

## Architecture documentation

The [architecture documentation index](docs/architecture/readme.md) links the detailed design notes covering:

- System overview and PostgreSQL data model
- Transactional outbox and Kafka processing
- Consumer idempotency and failure handling
- Redis sliding-window incident detection
- Incident lifecycle and state projection
- AI context construction, embeddings, vector retrieval and LLM analysis
- Action policy, human approval and execution safety
- Testing, known gaps and future hardening

## Known limitations and next hardening steps

- Add and enforce real authentication/authorization for all state-changing and billable endpoints; a shared operator secret is only a single-operator prototype gate, not multi-user identity or RBAC.
- Review and remove database backups or other generated artifacts from the public repository and Git history; rotate any credentials if a dump contains them.
- Expand tests for event ingestion, outbox recovery, Kafka retry/DLQ, incident correlation, AI pipeline failures and HTTP authorization.
- Add AI/RAG quality evaluation, calibrated confidence and claim-level grounding.
- Improve observability, rate limiting, deployment resilience and recovery behavior.
- Separate API and consumer compute when the workload requires independent scaling and uptime.

## Repository layout

```text
services/event-api/       Go API, consumer, handlers, services and repositories
frontend/                 React + TypeScript operator interface
docs/architecture/        Architecture decisions, flows and limitations
docker-compose.yaml       Local PostgreSQL, Kafka and Redis
Dockerfile                Multi-stage API/consumer container build
```

## License

No license file is currently present. Contact the repository owner before reusing the code.
