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
- `risk-engine-worker -> hedger-worker -> Hyperliquid Testnet` 的真实对冲执行链路
- 管理后台页面、用户侧 Explorer 页面、docker compose 本地联调环境、E2E 基础用例

当前对冲账务边界：

- 平台外部对冲账户当前作为独立风险域管理；
- 外部 Venue 的仓位、保证金占用、未实现盈亏、已实现盈亏暂不进入核心统一账本；
- 核心统一账本仍只承载用户资金、平台内部账务、链上充值提现、手续费、保险基金等已定义口径；
- Admin 的 `外部仓位 / 外部漂移` 目前用于观测与对账，不参与新对冲单目标计算。

## 快速启动

本地联调推荐按下面的顺序启动。链节点与后端服务已经解耦，先起三条链并部署合约，再起后端，最后起前端。

1. 启动本地三链并部署或复用合约

```bash
bash deploy/scripts/bootstrap-local-multichain.sh
```

这个脚本会：

- 启动宿主机上的 `Ethereum / Arbitrum / Base` 三条本地链
- 部署或复用本地联调合约
- 写出 `deploy/env/local-chains.env`
- 写出前端本地环境文件 `frontend/.env.local`

2. 启动后端依赖和服务

```bash
docker compose up -d --build
```

启动后可访问：

- API: `http://127.0.0.1:8080`
- RabbitMQ: `http://127.0.0.1:15672`
- MySQL: `127.0.0.1:3306`
- Redis: `127.0.0.1:6379`

3. 启动前端

```bash
sh deploy/scripts/start-frontend-local.sh
```

前端默认运行在：

- Frontend: `http://127.0.0.1:5173`

4. 本地手动挖块

本地 Anvil 不会像真实链那样持续自动推进确认块。充值后如果需要手动增加确认数，可以执行：

```bash
bash deploy/scripts/mine-local-blocks.sh eth 6
bash deploy/scripts/mine-local-blocks.sh arb 6
bash deploy/scripts/mine-local-blocks.sh base 6
```

参数说明：

- 第一个参数：`eth | arb | base`
- 第二个参数：要手动挖出的区块数量

例如，给 ETH 链补 12 个确认块：

```bash
bash deploy/scripts/mine-local-blocks.sh eth 12
```

当前本地充值确认数要求默认是：

- Ethereum: `12`
- Arbitrum: `20`
- Base: `20`

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

## 当前阶段

当前阶段以里程碑 5 和里程碑 6 的收口为主，重点包括：

- 会话与权限治理继续补强
- Explorer、后台和审计能力继续完善
- 对账、对冲账务镜像、可观测性、故障恢复与交付文档补齐
