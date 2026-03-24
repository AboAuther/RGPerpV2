# RGPerp

基于以下技术栈实现的链上托管、链下交易、链下风控、链下清算、外部对冲永续合约系统：

- Frontend: React + Vite + TypeScript + Ant Design
- Backend: Go + Gin + GORM
- Data: MySQL + Redis + RabbitMQ
- Contracts: Solidity + Foundry
- Hedge Venue: Hyperliquid Testnet

当前仓库的实现基线已经完成里程碑 2，基本打通里程碑 3 和里程碑 4 的核心链路，并进入里程碑 5 的局部增强阶段。

当前已经落地的主能力包括：

- 链上托管、充值、提现、内部转账、统一账本、Explorer 基础事件链路
- EVM 登录、access/refresh token、refresh/logout session management
- 交易对种子、行情聚合、下单、成交、仓位、PnL、风险率、强平、资金费率
- 管理后台页面、用户侧 Explorer 页面、docker compose 本地联调环境、E2E 基础用例

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

当前阶段以里程碑 5 和里程碑 6 的收口为主，重点包括：

- 会话与权限治理继续补强
- Explorer、后台和审计能力继续完善
- 对账、对冲、可观测性、故障恢复与交付文档补齐
