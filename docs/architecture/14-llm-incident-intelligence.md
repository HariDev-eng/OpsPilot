# 14 — LLM Incident Intelligence

> **V1 runtime status (2026-11-11):** The deployed API is configured with **Cloudflare Workers AI**: `@cf/google/gemma-4-26b-a4b-it` for incident analysis and `@cf/baai/bge-base-en-v1.5` for 768-dimensional embeddings. PostgreSQL + pgvector stores and retrieves knowledge chunks. Ollama adapters remain optional/local implementations in the codebase; they are not wired by the deployed `cmd/server` runtime. A completed production analysis and persisted history record have been verified using synthetic incident data.

## 1. Objective

Introduce an AI-powered incident intelligence layer that analyzes detected incidents using structured incident information, historical events, and retrieved technical knowledge.

The purpose of this layer is to help operators understand:

- What is happening?
- Which service is affected?
- What could be causing the incident?
- Which events support the diagnosis?
- Which technical documentation is relevant?
- What should the operator investigate next?
- What risks should be considered before taking action?

The system combines Retrieval-Augmented Generation (RAG) with provider-backed model inference. In the current deployment, embeddings and generation are sent to Cloudflare Workers AI; the Go interfaces keep provider-specific HTTP details separate from context construction and analysis validation.

Instead of sending an incident description directly to an LLM and accepting its response, OpsPilot constructs a context containing relevant information and validates the generated response before making it available to downstream components.

The LLM is an analytical component, not an execution authority.

---

## 2. High-Level Architecture

```text
                  Incident
                     |
                     v
                ContextBuilder
                     |
          +----------+----------+
          |          |          |
          v          v          v
      Incident    Related    Knowledge
      Details     Events     Retrieval
          |          |          |
          |          |          v
          |          |      Embeddings
          |          |          |
          |          |          v
          |          |      pgvector
          |          |          |
          +----------+----------+
                     |
                     v
              IncidentContext
                     |
                     v
              Prompt Builder
                     |
                     v
          Cloudflare Workers AI
               Gemma 4
                     |
                     v
             Structured JSON
                     |
                     v
            JSON Deserialization
                     |
                     v
            Analysis Validation
                     |
          +----------+----------+
          |          |          |
          v          v          v
       Analysis   Evidence    Action
       Validation Validation Validation
          |          |          |
          +----------+----------+
                     |
                     v
            IncidentAnalysis
                     |
                     v
          Action Recommendation Layer
```

### Architectural principle

The LLM does not directly query arbitrary production systems or execute operations.

The backend controls:

- Which incident information is supplied.
- Which knowledge documents are retrieved.
- Which model is called.
- Which response fields are accepted.
- Which evidence references are valid.
- Which recommended actions can proceed to the action-management layer.

---

## 3. Main Components

### 3.1 ContextBuilder

Location:

`internal/ai/`

Responsibility:

Construct a complete, structured context for incident analysis.

The context includes:

1. Incident information.
2. Related events.
3. Retrieved knowledge-base documents.

The ContextBuilder separates data collection and retrieval from LLM inference.

This is important because the LLM should analyze information supplied by the application rather than independently determine which database records it should access.

Conceptual interface:

```go
type ContextBuilder interface {
    Build(
        ctx context.Context,
        incidentID int64,
    ) (*IncidentContext, error)
}
```

This represents the architectural responsibility; the actual implementation should be checked against the repository interface.

### 3.2 EmbeddingProvider

Responsible for converting text into numerical vector representations.

The current abstraction is:

```go
type EmbeddingProvider interface {
    Embed(
        ctx context.Context,
        text string,
    ) ([]float32, error)
}
```

The abstraction separates embedding generation from the provider implementation.

This allows the application to change embedding providers without coupling the retrieval pipeline to one particular model.

### 3.3 Deployed Cloudflare Embedding Provider

The deployed runtime uses Cloudflare Workers AI with:

`@cf/baai/bge-base-en-v1.5`

| Property | Value |
|---|---|
| Provider | Cloudflare Workers AI |
| Model | `@cf/baai/bge-base-en-v1.5` |
| Embedding dimensions | 768 |
| API | Cloudflare Workers AI REST API |
| Runtime | Hosted inference |

The model returns 768-dimensional vectors, matching the current `knowledge_chunks.embedding` schema.

Conceptually:

```text
Input text
    |
    v
Cloudflare Workers AI
    |
    v
BGE base embeddings
    |
    v
768-dimensional vector
    |
    v
PostgreSQL + pgvector
```

An Ollama embedding adapter is also present for local development/provider experimentation, but it is not selected by the deployed server wiring.
 
### 3.4 LLMProvider

The LLM provider abstracts incident analysis from a particular model implementation.

The current implementation uses a locally hosted Ollama model.

The abstraction allows the application to separate:

- Incident analysis logic.
- Prompt construction.
- Model-specific HTTP communication.
- Response parsing.
- Validation.

### 3.5 Deployed Cloudflare Gemma LLM

The current server wiring selects Cloudflare Workers AI:

| Property | Value |
|---|---|
| Provider | Cloudflare Workers AI |
| Model | `@cf/google/gemma-4-26b-a4b-it` |
| API | Cloudflare Workers AI REST API |
| Output | Structured incident-analysis JSON |
| Streaming | Disabled |
| Request timeout | 120 seconds |
| Runtime | Hosted inference |

The adapter supports the Cloudflare response formats used by the model, extracts a JSON object from the model content, and deserializes it into the application-defined `IncidentAnalysis` structure. Backend validation checks required fields, evidence source references and action policies before the analysis is accepted.

Gemma 4 may wrap JSON in Markdown fences or explanatory text; the adapter extracts the first complete JSON object before decoding. Prompt instructions are not treated as a substitute for server-side validation.

The existing Ollama LLM adapter remains an optional local implementation and is not used by the deployed `cmd/server` initialization.
 
## 4. Knowledge Base Architecture

The knowledge base provides technical information that can help the LLM reason about an incident.

It currently contains four technical documents:

1. PostgreSQL Connection Pool Exhaustion
2. Redis Timeout Troubleshooting
3. Kafka Consumer Lag
4. Payment Service Deployment Failure

Each document currently has one stored chunk.

### Knowledge ingestion pipeline

```text
Knowledge Document
       |
       v
Text Extraction
       |
       v
Chunking
       |
       v
EmbeddingProvider
       |
       v
Ollama
       |
       v
Vector Representation
       |
       v
PostgreSQL + pgvector
```

The knowledge ingestion process converts documents into vector representations so that semantically relevant content can be retrieved later.

### Current implementation scope

- Four knowledge documents.
- One chunk per document.
- 768-dimensional embeddings.
- Local Cloudflare BGE embedding generation.
- PostgreSQL vector storage.

The current knowledge base is intentionally small.

Production document processing will require more advanced chunking, metadata, document versioning, and ingestion management.

---

## 5. Vector Search and Retrieval

### 5.1 Why vector search?

Traditional SQL search primarily matches explicit values or text patterns.

However, incident descriptions may use different terminology while referring to the same underlying problem.

Example:

Knowledge document:

"PostgreSQL connection pool exhaustion."

Incident:

"Payment requests are timing out because database connections are unavailable."

The terminology differs, but the underlying concepts are related.

Embeddings allow the system to compare semantic representations instead of relying exclusively on exact keyword matching.

### 5.2 Retrieval pipeline

```text
Current Incident
       |
       v
Construct Retrieval Query
       |
       v
Generate Query Embedding
       |
       v
768-dimensional Vector
       |
       v
pgvector Similarity Search
       |
       v
Rank Relevant Chunks
       |
       v
Top 3 Knowledge Chunks
       |
       v
IncidentContext
```

### 5.3 Cosine distance

The current implementation uses pgvector's cosine-distance operator:

```sql
<=>
```

Conceptual query:

```sql
SELECT
    id,
    content,
    embedding <=> $1 AS distance
FROM knowledge_chunks
ORDER BY embedding <=> $1
LIMIT 3;
```

The exact selected columns and filters should follow the repository's actual query implementation.

Important:

- Lower cosine distance indicates greater similarity.
- Distance is not a percentage.
- Similarity does not establish factual correctness.
- Retrieved content is supporting context, not proof of a root cause.

### 5.4 Top-K retrieval

The current ContextBuilder retrieves the top three knowledge chunks.

This keeps the context relatively small and limits unrelated knowledge from entering the LLM prompt.

However, top-K retrieval is a configuration choice, not a guarantee of retrieval quality.

A production system should evaluate whether the retrieved documents are actually relevant to the incident.

---

## 6. IncidentContext

The IncidentContext is the structured information supplied to the LLM.

It combines information from different sources into one analysis input.

Conceptually:

```go
type IncidentContext struct {
    Incident  IncidentContextData
    Events    []EventContext
    Knowledge []KnowledgeContext
}
```

The exact model fields should follow the current implementation.

### Context sources

| Source | Information |
|---|---|
| Incident | Incident identity, service, severity and status |
| Events | Related failures and supporting event data |
| Knowledge | Retrieved troubleshooting documents and runbooks |

### Why structured context?

Sending arbitrary database output directly to an LLM makes the system difficult to control and debug.

Structured context provides:

- Predictable input.
- Explicit source identification.
- Evidence traceability.
- Better prompt construction.
- Easier validation.
- Separation between data retrieval and model reasoning.

### Context limitation

The current incident detection implementation has a known completeness gap.

When a failure threshold is crossed, the incident processor currently attaches only the triggering event rather than every event in the detection window.

For example:

```text
Failure Event 1
Failure Event 2
Failure Event 3
Failure Event 4
Failure Event 5
       |
       v
Threshold Crossed
       |
       v
Incident Created
       |
       v
Only Triggering Event Attached
```

This means the LLM may receive incomplete incident history.

The ContextBuilder cannot retrieve related events that were never attached to the incident.

This must be corrected in the incident processing layer.

---

## 7. Prompt Engineering

The prompt is responsible for explaining the task to the LLM and defining the expected output.

The model receives:

1. Its role as an incident-analysis assistant.
2. The incident context.
3. Related event information.
4. Retrieved knowledge.
5. Instructions for evidence-based analysis.
6. The required JSON structure.

### Prompt objectives

The model should:

- Analyze the supplied incident.
- Identify possible causes.
- Distinguish confirmed causes from unconfirmed hypotheses.
- Reference supporting evidence.
- Recommend investigation or mitigation actions.
- Identify operational risks.
- Avoid inventing event IDs or knowledge references.

### Grounding principle

The model must distinguish:

**Observed facts**

Information directly present in the supplied incident context.

**Hypotheses**

Potential explanations inferred from the available evidence.

**Recommendations**

Actions that an operator may consider taking.

A plausible explanation must not automatically be treated as a confirmed root cause.

### Prompt limitation

Prompt instructions alone cannot guarantee that an LLM follows every requirement.

Backend validation remains mandatory.

---

## 8. Structured Incident Analysis

The LLM returns structured JSON rather than unrestricted conversational text.

The application deserializes the response into an `IncidentAnalysis` model.

### IncidentAnalysis fields

| Field | Purpose |
|---|---|
| Summary | Short explanation of the incident |
| LikelyCause | Potential explanation |
| CauseStatus | Confirmation state of the suspected cause |
| Confidence | Model-reported confidence |
| Evidence | References supporting the analysis |
| RecommendedActions | Suggested investigation or mitigation actions |
| Risks | Potential operational risks |

### CauseStatus

Supported values:

| Value | Meaning |
|---|---|
| CONFIRMED | Cause is established by available evidence |
| UNCONFIRMED | A plausible cause exists but needs further verification |
| UNKNOWN | Available information is insufficient |

A model-generated cause should not be treated as independently confirmed merely because the model uses the word CONFIRMED.

The current validation checks that the status is an allowed value. It does not independently establish root-cause truth.

### Confidence

Confidence is represented as a floating-point value between zero and one.

The backend validates:

- The value is within the allowed range.
- The value is not NaN.
- The value is not positive or negative infinity.

However, confidence is currently model-reported and is not statistically calibrated.

A confidence value of 0.9 does not establish that the diagnosis has a 90% probability of being correct.

---

## 9. Evidence Reference Architecture

Evidence references connect AI-generated claims to actual source information.

This is one of the most important reliability mechanisms in OpsPilot.

Without evidence references, an LLM could generate a plausible explanation that cannot be traced back to the incident data.

### Supported evidence types

| Source type | Meaning |
|---|---|
| EVENT | A related incident event |
| KNOWLEDGE | A retrieved knowledge-base chunk |

Conceptual evidence structure:

```json
{
  "source_type": "EVENT",
  "source_id": "payment-rag-final-5",
  "claim": "Payment failures increased during the incident window."
}
```

The exact field names should match the implemented `EvidenceReference` model.

### Evidence validation pipeline

```text
LLM Response
     |
     v
Evidence References
     |
     v
Validate Source Type
     |
     v
Validate Source ID
     |
     v
Check Supplied Context
     |
     v
Validate Claim
     |
     v
Detect Duplicate References
     |
     v
Accept / Reject Analysis
```

### Current validation rules

The backend verifies:

1. The source type is supported.
2. The source ID exists in the supplied context.
3. The claim is not empty.
4. Duplicate references are rejected.

Unknown source types are rejected.

Missing event or knowledge references are rejected.

Duplicate evidence references are rejected.

### Important limitation

Evidence validation verifies reference integrity, not semantic truth.

For example:

An event may exist and be correctly referenced, but the LLM may describe that event incorrectly.

The backend currently does not prove that the natural-language claim logically follows from the referenced event.

This distinction is critical:

```text
Reference Validation
        !=
Semantic Validation
```

Future improvements may include claim-level verification, deterministic evidence extraction, contradiction detection, and human review.

---

## 10. Recommended Action Classification

The LLM can recommend operational actions as part of its analysis.

Each action includes a classification and an approval requirement.

| Action Type | Purpose | Approval |
|---|---|---|
| INVESTIGATION | Inspect or collect information | Not required |
| MITIGATION | Change system state to reduce impact | Required |
| DESTRUCTIVE | Potentially high-impact operation | Required |

### Validation responsibilities

The backend validates:

- Supported action types.
- Approval requirement consistency.
- Required action fields.

For example:

```text
INVESTIGATION
RequiresApproval = false

MITIGATION
RequiresApproval = true

DESTRUCTIVE
RequiresApproval = true
```

### Security boundary

Action classification is advisory.

The LLM is not authorized to execute operations.

A recommendation does not automatically become an executable operation.

The backend operation registry independently determines which operations are allowed.

The subsequent action-management and execution workflow is documented in:

`15-action-management-and-execution-safety.md`

---

## 11. LLM Response Validation Pipeline

The LLM response must pass multiple validation stages.

```text
Ollama Response
      |
      v
JSON Parsing
      |
      v
IncidentAnalysis
      |
      v
Analysis Validation
      |
      +---- Required fields
      +---- Valid cause status
      +---- Valid confidence
      +---- Evidence presence
      |
      v
Evidence Validation
      |
      +---- Supported source types
      +---- Valid source IDs
      +---- Non-empty claims
      +---- No duplicate references
      |
      v
Action Validation
      |
      +---- Valid action types
      +---- Correct approval policy
      |
      v
Validated Analysis
```

Validation is performed in the backend rather than trusting the model's response.

### Validation boundaries

| Validation | What it guarantees |
|---|---|
| JSON parsing | Response can be deserialized |
| Required-field validation | Required information is present |
| Cause-status validation | Only supported statuses are accepted |
| Confidence validation | Confidence is a valid finite value |
| Evidence validation | References exist in supplied context |
| Action validation | Supported action classifications and approval policies are used |

None of these checks independently proves that the model's reasoning is factually correct.

---

## 12. End-to-End Incident Analysis Example

Consider a payment service experiencing repeated failures.

### Incident information

```text
Service: payment-service
Severity: HIGH
Status: OPEN
```

### Related signals

```text
payment.failed
payment.failed
payment.error
database.timeout
http.5xx
```

### Retrieved knowledge

The RAG pipeline retrieves:

- PostgreSQL Connection Pool Exhaustion
- Payment Service Deployment Failure
- Redis Timeout Troubleshooting

The retrieved documents are supplied as context, not as confirmed explanations.

### LLM analysis

The model analyzes the incident and generates a structured response.

Illustrative output:

```json
{
  "summary": "The payment service is experiencing repeated failures.",
  "likely_cause": "Possible database connection pool exhaustion.",
  "cause_status": "UNCONFIRMED",
  "confidence": 0.6,
  "evidence": [
    {
      "source_type": "EVENT",
      "source_id": "payment-rag-final-5",
      "claim": "A payment failure was recorded."
    }
  ],
  "recommended_actions": [
    {
      "description": "Inspect payment service database connection usage.",
      "action_type": "INVESTIGATION",
      "requires_approval": false
    }
  ],
  "risks": [
    "Changing database connection settings without checking capacity could increase database load."
  ]
}
```

This is an illustrative example of the output structure, not a claim that every live inference returns this exact response.

### Backend validation

The backend checks:

- JSON structure.
- Allowed cause status.
- Confidence range.
- Evidence reference existence.
- Action classification.
- Approval policy consistency.

Only validated analysis proceeds to downstream action recommendation handling.

---

## 13. Failure Handling and Operational Considerations

The AI layer depends on external model-serving and embedding infrastructure, even when those services run locally.

Potential failures include:

| Failure | Impact | Required behavior |
|---|---|---|
| Ollama unavailable | No LLM analysis | Return an inference error |
| Embedding service unavailable | Retrieval query cannot be embedded | Return a retrieval error |
| Malformed JSON | Analysis cannot be parsed | Reject response |
| Missing evidence ID | Unsupported reference | Reject analysis |
| Invalid confidence | Invalid model output | Reject analysis |
| No relevant knowledge | Reduced contextual support | Analyze only with available context or report insufficient context |
| Inference timeout | Delayed analysis | Apply timeout and controlled retry policy |
| Incomplete incident events | Missing evidence | Identify context limitation |

### Timeout

The current Ollama LLM request uses a 120-second timeout.

The system should not allow an unbounded model request to block incident processing indefinitely.

### Model availability

Ollama runs locally, so the model-serving environment must be available before inference can succeed.

The current implementation is suitable for local development and controlled demonstrations.

Production deployment requires resource planning, model health monitoring, capacity management, and failure isolation.

---

## 14. Testing and Verification

### Embedding tests

Verified:

- Local embedding generation.
- Ollama communication.
- Vector generation.
- Expected embedding dimensionality.

### Knowledge-base tests

Verified:

- Knowledge document ingestion.
- Embedding storage.
- Knowledge retrieval.
- Vector similarity search.

### Context-building tests

Verified:

- Incident retrieval.
- Related event loading.
- Knowledge retrieval.
- Context construction.

### LLM tests

Verified:

- Ollama inference.
- Structured JSON generation.
- Response deserialization.
- Incident analysis validation.

### Evidence tests

Verified:

- Valid EVENT references.
- Valid KNOWLEDGE references.
- Unknown source rejection.
- Missing reference rejection.
- Empty claim rejection.
- Duplicate reference rejection.

### Action validation tests

Verified:

- Supported action types.
- Approval policy consistency.
- Invalid classification rejection.

### Build verification

```bash
go test ./...
go build ./...
```

The current AI implementation and its validation tests pass.

Live Cloudflare Workers AI inference has been successfully tested against a synthetic production incident.

---

## 15. Implementation Summary

| Component | Implementation | Status |
|---|---|---|
| EmbeddingProvider | Go interface | Implemented |
| Cloudflare embeddings | BGE base en v1.5 | Implemented in deployed runtime |
| Vector storage | PostgreSQL + pgvector | Implemented |
| Knowledge ingestion | Document embedding and storage | Implemented |
| Vector retrieval | Cosine-distance search | Implemented |
| ContextBuilder | Incident, events and knowledge | Implemented |
| LLMProvider | Provider abstraction | Implemented |
| CloudflareLLM | Gemma 4 26B A4B IT | Implemented in deployed runtime |
| Structured output | JSON response | Implemented |
| Analysis validation | Backend validation | Implemented |
| Evidence validation | Source reference validation | Implemented |
| Action validation | Type and approval validation | Implemented |
| Persistent analysis history | PostgreSQL AI analysis records | Implemented |
| Incident analysis API | Analyze, latest and history endpoints | Implemented |
| Semantic claim verification | Claim-level grounding | Not implemented |
| RAG evaluation framework | Retrieval and answer-quality evaluation | Not implemented |

---

## 16. Known Limitations

1. Evidence source IDs are validated against the supplied incident context, but semantic accuracy of every natural-language claim is not guaranteed.
2. Confidence is model-reported and is not statistically calibrated.
3. The V1 knowledge corpus and chunking strategy are small; richer document ingestion, chunk metadata and versioning remain future work.
4. Retrieval currently uses top-three vector results; there is no production-grade reranking or automated retrieval evaluation.
5. Prompt instructions do not guarantee schema compliance; parsing and backend validation are required.
6. Action type and approval policy validation does not by itself prove that a recommended operation is safe. The backend operation allowlist remains the execution boundary.
7. The action mutation routes use a shared Bearer secret and configured audit actor label in V1. Production-grade multi-user identity, role separation, AI inference authorization and usage auditing remain future work.
8. Live inference depends on Cloudflare availability, account limits and configured credentials.
9. The current single-process Render deployment runs the API, outbox publisher and incident consumer together for cost reasons; this is not an independently scalable high-availability topology.
10. The deployed prototype was exercised with synthetic event data. It has not been validated against real production incidents or a formal AI quality benchmark.

---

## 17. Future Improvements

### Retrieval improvements

- More granular document chunking.
- Chunk overlap and metadata preservation.
- Hybrid keyword and vector retrieval.
- Reranking.
- Retrieval relevance evaluation.
- Knowledge document versioning.

### Grounding improvements

- Claim-level evidence verification.
- Contradiction detection.
- Evidence coverage scoring.
- Explicit uncertainty handling.
- Better separation between observed facts and inferred causes.

### LLM improvements

- Persistent analysis history.
- Prompt versioning.
- Model configuration management.
- Structured output schema versioning.
- Inference latency and failure metrics.
- Model evaluation datasets.
- Fallback strategies.

### Production AI architecture

- Dedicated AI analysis API.
- Asynchronous analysis jobs.
- Analysis status tracking.
- Retry and timeout management.
- AI audit history.
- Operator feedback and evaluation.

---

## 18. Relationship With Other Architecture Components

The LLM Incident Intelligence layer depends on several other OpsPilot components.

| Component | Relationship |
|---|---|
| Event ingestion | Supplies original event information |
| Kafka consumer | Processes events and triggers incident handling |
| Incident engine | Creates and maintains incident records |
| Redis | Supports fast incident-state operations |
| PostgreSQL | Stores incidents, events and knowledge |
| pgvector | Supports semantic knowledge retrieval |
| Ollama embeddings | Converts text into vectors |
| Ollama LLM | Generates incident analysis |
| Action management | Receives validated recommendations |

The architecture documentation is divided into separate documents to keep responsibilities clear.

The next document covers controlled action execution, operator approval, execution recovery, and reconciliation.

---

## 19. Conclusion

OpsPilot's LLM Incident Intelligence layer combines structured incident context, semantic knowledge retrieval, local LLM inference, and backend validation.

Its central design principle is:

**The LLM provides analysis and recommendations; the backend controls validation, authorization, and execution.**

This separation reduces dependence on unverified model output and establishes a foundation for a more reliable AI-assisted incident response platform.

The current implementation demonstrates a working local RAG and LLM analysis pipeline. Further work is required to establish semantic grounding, retrieval quality, persistent analysis history, production API exposure, and operational reliability.
