# OpsPilot — Architecture Documentation

This directory documents the architecture, implementation decisions, workflows, reliability mechanisms and limitations of OpsPilot.

## Architecture

| Document | Subject |
|---|---|
| 01 | System Overview |
| 02 | PostgreSQL Architecture |
| 03 | Transactional Outbox |
| 04 | Kafka Architecture |
| 05 | Consumer Idempotency |
| 06 | Incident Detection |
| 07 | Redis Architecture |
| 08 | Incident State Projection |
| 09 | Incident Lifecycle |
| 10 | Failure Modes and Recovery |
| 11 | Bugs and Fixes |
| 12 | Testing and Verification |
| 13 | Known Gaps and Future Hardening |
| 14 | LLM Incident Intelligence |
| 15 | Action Management, Human Approval and Execution Safety |

## Documentation Goals

Each document should explain:

- The problem being solved.
- The architectural design.
- Component responsibilities.
- Data flow and state transitions.
- Important implementation decisions.
- Failure scenarios and recovery behavior.
- Testing and verification.
- Known limitations and future improvements.

## Project Status

OpsPilot currently implements event ingestion, transactional outbox publishing, Kafka-based event processing, incident detection, Redis projections, RAG-based context retrieval, LLM incident analysis, and controlled action execution with human approval and reconciliation.

Production hardening, deployment and the operator interface remain planned work.
