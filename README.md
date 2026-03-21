# RGPerp

基于以下技术栈实现的链上托管、链下交易、链下风控、链下清算、外部对冲永续合约系统：

- Frontend: React + Vite + TypeScript + Ant Design
- Backend: Go + Gin + GORM
- Data: MySQL + Redis + RabbitMQ
- Contracts: Solidity + Foundry
- Hedge Venue: Hyperliquid Testnet

当前仓库处于里程碑 1，已完成：

- 需求拆解与实施路线
- Architecture Document
- 实施级规格文档
- API / Event / DDL 初稿
- 仓库目录骨架

## 目录结构

- `frontend/`: 前端应用
- `backend/`: Go 后端与 worker 进程
- `contracts/`: Solidity 合约与 Foundry 测试
- `deploy/`: 环境变量、Compose、初始化 SQL
- `docs/`: 需求、架构、实施级设计文档
- `spec/`: OpenAPI、事件 Schema、DDL、任务清单

## 当前基线文档

- [需求拆解与实施路线](/Users/xiaobao/RGPerp/docs/需求拆解与实施路线.md)
- [Architecture Document](/Users/xiaobao/RGPerp/docs/Architecture%20Document.md)
- [Architecture Appendix](/Users/xiaobao/RGPerp/docs/Architecture%20Appendix.md)
- [任务清单](/Users/xiaobao/RGPerp/spec/TASKS.md)

## 当前阶段

当前阶段目标是完成 Milestone 1：

- 固化仓库布局
- 固化配置与环境边界
- 为 Milestone 2 的合约、账本、钱包、鉴权编码建立稳定基础
