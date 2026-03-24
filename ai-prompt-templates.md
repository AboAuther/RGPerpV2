# AI Prompt Templates

This document contains reusable prompt templates for AI coding exams and rapid product delivery. It is designed to work well with `$ai-engineering-guardrails`.

## Usage Notes

- Use the templates as starting points, not rigid scripts.
- Fill placeholders such as `{{exam_requirement}}` and `{{architecture_doc}}` with your actual materials.
- For complex or high-risk tasks, keep the structure.
- For simpler tasks, trim sections but keep the reasoning standard.

## Master Template

Use this when you first receive an exam requirement and want a full project breakdown before coding.

```text
Use $ai-engineering-guardrails.

You are acting as a top-tier technical lead, architect, and product manager.
I will give you an exam requirement. Do not start coding immediately. First produce a professional project breakdown and delivery plan.

Goals:
1. Translate the requirement into clear product and engineering language.
2. Identify core workflows, critical states, edge cases, and risks.
3. Propose a practical system design and module breakdown.
4. Define responsibilities across frontend, backend, contract, data, and testing roles.
5. Produce execution-ready inputs for downstream engineering work.

Please output:
1. Requirement summary
2. Product goals and non-goals
3. Core user workflows
4. Core business objects and state transitions
5. Core invariants
6. Trust boundaries
7. Failure and replay analysis
8. Security review
9. System architecture breakdown
10. Module split across API / DB / contracts / async jobs / frontend pages
11. Recommended role split
12. Delivery priority
13. Critical test cases
14. Open questions and assumptions

Inputs:
- Exam requirement:
{{exam_requirement}}

Optional materials:
- Architecture design doc: {{architecture_doc}}
- API design doc: {{api_doc}}
- DB schema: {{db_schema}}
- Contract design doc: {{contract_doc}}
```

## Product Manager Template

Use this to convert raw requirements into an engineering-ready requirement breakdown.

```text
Use $ai-engineering-guardrails.

You are acting as a top-tier product manager who is excellent at converting ambiguous requirements into execution-ready engineering documents.
Based on the exam requirement and available design materials, produce a professional requirement breakdown document that engineering can implement directly.

Please focus on:
- clarifying goals, scope, and non-goals
- mapping user roles, critical flows, and failure flows
- extracting key business rules
- identifying risks, ambiguities, and assumptions
- defining implementation priority

Please output:
1. Background
2. Product goals
3. Non-goals
4. User roles
5. Core feature list
6. Core user flows
7. Key business rules
8. Edge cases and exception scenarios
9. Data and state change requirements
10. Risks and questions to clarify
11. MVP scope recommendation
12. Delivery requirements for frontend / backend / contracts / data / testing

Inputs:
- Exam requirement: {{exam_requirement}}
- Architecture design doc: {{architecture_doc}}
- API design doc: {{api_doc}}
- DB schema: {{db_schema}}
- Contract design doc: {{contract_doc}}
```

## Architect Template

Use this to produce the high-level system design.

```text
Use $ai-engineering-guardrails.

You are acting as a top-tier system architect.
Based on the requirement and design materials, produce a system architecture plan suitable for an exam delivery. Avoid vague theory. Balance practicality, time, risk, and extensibility.

Please analyze:
- module boundaries
- source of truth
- critical state transitions
- sync and async boundaries
- concurrency and consistency
- retry, recovery, and replay
- security and permission boundaries
- observability and test strategy

Please output:
1. Architecture goals
2. Overall architecture explanation
3. Module breakdown and responsibilities
4. Core invariants
5. Trust boundaries
6. State transitions
7. Concurrency and idempotency strategy
8. Failure and replay analysis
9. Security review
10. Data flow and event flow
11. Technology recommendations
12. MVP vs future expansion
13. Critical test cases
14. Largest risks and mitigation

Inputs:
- Exam requirement: {{exam_requirement}}
- Architecture design doc: {{architecture_doc}}
- API design doc: {{api_doc}}
- DB schema: {{db_schema}}
- Contract design doc: {{contract_doc}}
```

## Frontend Engineer Template

Use this when implementing from architecture and API docs.

```text
Use $ai-engineering-guardrails.

You are acting as a top-tier frontend engineer.
Based on the architecture design, API documentation, and product requirement, design and implement the frontend solution. Focus not only on rendering pages, but on correctness, permissions, resilient UX, and failure handling.

Before coding, output:
1. Page and route breakdown
2. State management plan
3. API interaction model
4. Permission model and trust boundaries
5. Risk handling for submit, retry, duplicate click, refresh recovery
6. Loading / empty / error / retry states
7. Critical test cases

Implementation requirements:
- Do not rely on frontend as the final authority for authorization
- Prevent duplicate submission and stale state issues
- Correctly represent pending / success / failed / partial states
- Keep environment configuration explicit and avoid hardcoded endpoints
- Keep component boundaries maintainable

Inputs:
- Product requirement doc: {{prd_doc}}
- Architecture design doc: {{architecture_doc}}
- API design doc: {{api_doc}}
- UI spec: {{ui_spec}}
```

## Backend Engineer Template

Use this for services, APIs, jobs, and domain logic.

```text
Use $ai-engineering-guardrails.

You are acting as a top-tier backend engineer.
Based on the requirement, architecture, API design, and DB schema, design and implement the backend service. Focus on consistency, idempotency, authorization, recovery, and observability. Do not implement only the happy path.

Before coding, output:
1. Module breakdown
2. Core domain model
3. Core invariants
4. Trust boundaries
5. State transitions
6. Transaction, locking, and idempotency strategy
7. Async jobs and replay/recovery strategy
8. Security review
9. Critical test cases

Implementation requirements:
- Keep validation and mutation inside safe transaction boundaries when required
- Protect against retries, concurrency, duplicate callbacks, and duplicate consumption
- Persist critical workflow states
- Do not use cache as the source of truth
- Do not rely on request-body identity for authorization
- Define failure and compensation paths explicitly

Inputs:
- Product requirement doc: {{prd_doc}}
- Architecture design doc: {{architecture_doc}}
- API design doc: {{api_doc}}
- DB schema: {{db_schema}}
```

## Smart Contract Engineer Template

Use this for on-chain design and implementation.

```text
Use $ai-engineering-guardrails.

You are acting as a top-tier smart contract engineer.
Based on the requirement and system design, design and implement the contract solution. Focus on access control, fund safety, event design, precision, reentrancy, upgrade authority, and coordination with off-chain systems.

Before coding, output:
1. Contract responsibility boundaries
2. On-chain vs off-chain responsibility split
3. Core invariants
4. Trust boundaries
5. State transitions
6. Security review
7. Event design and indexer friendliness
8. Critical test cases

Implementation requirements:
- Make access control explicit
- Defend against reentrancy, precision, and non-standard token risks
- Emit events sufficient for off-chain reconstruction and recovery
- Define admin powers and limits clearly
- Make clear which guarantees are enforced on-chain vs off-chain

Inputs:
- Product requirement doc: {{prd_doc}}
- Architecture design doc: {{architecture_doc}}
- Contract design doc: {{contract_doc}}
- Off-chain integration doc: {{offchain_doc}}
```

## Test Engineer Template

Use this to generate a high-value test plan.

```text
Use $ai-engineering-guardrails.

You are acting as a top-tier test engineer.
Based on the requirement and design documents, produce a high-value test plan. Do not just list normal feature tests. Prioritize security, concurrency, consistency, retry, recovery, authorization, edge cases, and failure paths.

Please output:
1. Test scope
2. Risk grading
3. Core test strategy
4. Key test points by module
5. Critical test cases
6. Concurrency / retry / replay / crash-recovery tests
7. Security and authorization tests
8. Automation priority
9. The minimum tests that must not be skipped under time pressure

Inputs:
- Product requirement doc: {{prd_doc}}
- Architecture design doc: {{architecture_doc}}
- API design doc: {{api_doc}}
- DB schema: {{db_schema}}
- Contract design doc: {{contract_doc}}
```

## Data Expert Template

Use this for analytics, metrics, auditing, and data modeling.

```text
Use $ai-engineering-guardrails.

You are acting as a top-tier data expert and data architect.
Based on the requirement and system design, design the data model, key metrics, instrumentation, and reporting plan. Focus on source of truth, consistency, traceability, risk visibility, and future analysis capability.

Please output:
1. Data goals
2. Core business entities and relationships
3. Metric definitions
4. Event and instrumentation design
5. Data source-of-truth definitions
6. Sync vs async data paths and delay tolerance
7. Reconciliation, audit, and forensic suggestions
8. Key data risks
9. Critical test cases

Inputs:
- Product requirement doc: {{prd_doc}}
- Architecture design doc: {{architecture_doc}}
- API design doc: {{api_doc}}
- DB schema: {{db_schema}}
- Analytics requirement: {{analytics_doc}}
```

## Sequential Workflow Template

Use this when you want AI to guide the full exam process step by step.

```text
Step 1:
Use $ai-engineering-guardrails to convert the exam requirement into a professional requirement breakdown.

Step 2:
Based on step 1, use $ai-engineering-guardrails to produce a system design document from a top-tier architect perspective.

Step 3:
Based on the architecture design, API design, and DB schema, separately produce execution plans from the perspectives of:
- top-tier frontend engineer
- top-tier backend engineer
- top-tier smart contract engineer
- top-tier test engineer
- top-tier data expert

Requirements:
- all roles must share the same core invariants, trust boundaries, and critical state transitions
- conflicts across documents must be called out explicitly
- correctness, security, recovery, and delivery quality take priority over happy-path speed
- do not design only for the happy path
```

## Recommended Exam Flow

Use the prompts in this order when time is limited:

1. Master Template
2. Product Manager Template
3. Architect Template
4. The role template that matches your implementation task
5. Test Engineer Template for final back-check
