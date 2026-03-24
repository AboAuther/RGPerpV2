# Subagents

This directory contains 10 project-specific subagent system prompts for `RGPerp`.

These prompts are based on the CFD perpetual exchange roles previously defined, but are grounded in the current repository baseline:

- Frontend: React + Vite + TypeScript + Ant Design
- Backend: Go + Gin + GORM
- Data: MySQL + Redis + RabbitMQ
- Contracts: Solidity + Foundry
- Hedge venue: Hyperliquid Testnet

Important usage note:

- Some original role prompts referenced Kafka, PostgreSQL, or TimescaleDB as possible target-state technologies.
- The current repo baseline uses MySQL and RabbitMQ.
- Each subagent is expected to surface any gap between current implementation and target architecture explicitly instead of silently assuming the target stack already exists.

Available subagents:

1. `01-chief-architect.md`
2. `02-high-performance-backend.md`
3. `03-ledger-financial-logic.md`
4. `04-blockchain-engineer.md`
5. `05-frontend-engineer.md`
6. `06-database-expert.md`
7. `07-devops-infrastructure.md`
8. `08-security-auditor.md`
9. `09-code-reviewer.md`
10. `10-quant-risk.md`

Recommended usage pattern:

1. Load the relevant prompt as the system prompt for the subagent.
2. Provide task-specific context plus the referenced project documents.
3. Require the subagent to state assumptions, identify architecture mismatches, and produce implementation-grade outputs.

