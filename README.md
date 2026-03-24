# RGPerp

链上托管、链下交易、链下风控、链下清算、外部对冲的永续合约系统。

当前代码基线已经完成里程碑 2，基本打通里程碑 3 和里程碑 4 的核心链路，并进入里程碑 5 的局部增强阶段。

## 项目快速启动方式

本地联调按下面顺序启动：

1. 启动本地三链并部署或复用合约

```bash
bash deploy/scripts/bootstrap-local-multichain.sh
```

这一步会生成：

- `deploy/env/local-chains.env`
- `frontend/.env.local`

2. 如果要启用真实 Hyperliquid Testnet 对冲，在 `deploy/env/local-chains.env` 里额外补上：

```bash
export HL_API_URL=https://api.hyperliquid-testnet.xyz
export HL_ACCOUNT_ADDRESS=0x...
export HL_PRIVATE_KEY=0x...
```

不配置这组参数时，`hedger-worker` 会退回 `simulated` 模式。

3. 启动后端依赖和服务

```bash
docker compose up -d --build
```

4. 启动前端

```bash
sh deploy/scripts/start-frontend-local.sh
```

5. 本地充值后如需补确认块，可手动挖块

```bash
bash deploy/scripts/mine-local-blocks.sh eth 6
bash deploy/scripts/mine-local-blocks.sh arb 6
bash deploy/scripts/mine-local-blocks.sh base 6
```

常用入口：

- Frontend: `http://127.0.0.1:5173`
- API: `http://127.0.0.1:8080`
- MySQL: `127.0.0.1:3306`
- Redis: `127.0.0.1:6379`

## 系统架构说明

当前系统采用“模块化单体 + 多进程 worker”的结构：

- `frontend`
  - React + Vite + TypeScript + Ant Design
- `api-server`
  - 提供认证、账户、钱包、订单、Explorer、Admin API
- `indexer`
  - 扫描 Ethereum / Arbitrum / Base 链上事件，推进充值提现状态机
- `market-data-worker`
  - 抓取市场数据，生成指数价、标记价和行情快照
- `order-executor-worker`
  - 执行挂单、触发单，推进成交、仓位和账本更新
- `risk-engine-worker`
  - 重算账户风险、净敞口、强平触发、对冲意图
- `liquidator-worker`
  - 执行清算
- `funding-worker`
  - 采集资金费率、生成批次并应用结算
- `hedger-worker`
  - 消费 `hedge.requested`，执行真实或模拟对冲，并刷新外部仓位快照
- `mysql`
  - 资金、订单、仓位、风险、对冲、审计的权威存储
- `redis`
  - 行情热缓存，不承载资金真相

当前已经落地的主能力：

- 链上托管、充值、提现、内部转账、统一账本、Explorer 基础事件链路
- EVM 登录、access/refresh token、refresh/logout session management
- 下单、成交、仓位、PnL、风险率、强平、资金费率
- `risk-engine-worker -> hedger-worker -> Hyperliquid Testnet` 的真实对冲执行链路
- Admin 后台、Explorer、基础 E2E 联调环境

## 关键设计决策

- 统一账本作为核心资金真相。
  所有用户资金、平台内部账务、充值提现、手续费、清算等主资金链路，都以核心账本为准，而不是以前端展示状态、缓存结果或业务表余额为准。这个取舍的优点是资金口径集中、审计和回放更清楚、补账和冲正有统一入口；代价是核心交易与钱包路径必须围绕账本事务设计，开发复杂度更高。
- MySQL 作为权威状态中心，Redis 只承担热点缓存。
  订单、仓位、风险快照、账本、对冲状态都落在 MySQL，Redis 只加速行情读路径，不承担财务或状态真相。这个设计优点是恢复简单、重启后不依赖缓存重建关键业务状态；代价是系统当前更偏 DB-centric，对数据库事务设计和索引质量要求更高。
- 模块化单体加多进程 worker，而不是微服务优先。
  当前把认证、钱包、订单、风控、资金费率、清算、对冲等领域保留在一套代码库里，再拆成多个 worker 进程运行。这样做的优点是事务边界清晰、本地联调成本低、功能闭环推进更快；代价是后续要进一步拆分时，需要更严格地守住领域边界和表归属。
- Outbox 轮询优先于复杂消息总线。
  关键异步链路以 MySQL `outbox_events` 和 worker 轮询消费为主。优点是可以把“业务写入”和“异步发布”放在同一事务里，降低消息丢失和双写不一致风险；代价是实时性和吞吐上限不如成熟消息系统，后续需要按容量和订阅复杂度评估是否引入专门消息设施。
- 风险优先于交易，交易优先于体验。
  下单、成交、清算、资金费率、配置变更等链路都优先保证资金安全、风险收敛和状态一致，而不是优先做激进的前端实时体验。这让系统更适合做资金型业务闭环，但也意味着当前前端更多依赖轮询和写后刷新，而不是重度 optimistic UI。
- 对冲目标只看内部净敞口和系统已管理仓位。
  新对冲单的目标不是由外部真实仓位直接反推，而是由内部净敞口和系统已确认的已管理仓位决定；外部仓位只用于观测与漂移展示。这样做的优点是可以避免人工在外部 venue 上残留仓位、查询延迟或第三方状态抖动直接污染内部决策；代价是外部漂移必须通过快照和对账单独监控，而不是混进目标计算逻辑。
- 平台外部对冲账户当前独立于核心账本管理。
  当前真实对冲已经接入 Hyperliquid Testnet，但外部账户的保证金、已实现盈亏、未实现盈亏、手续费还没有正式镜像进核心统一账本。这个阶段性取舍的优点是先把“执行正确”和“风险监控”做稳，避免在账务口径尚未冻结前把外部资金流错误并入总账；代价是平台层面的完整资金报表需要同时看核心账本和外部对冲视图。
- 运行时配置采用多进程轮询生效，而不是重启生效。
  管理后台修改关键参数后，由多个进程轮询加载最新快照并生效。优点是运维操作更贴近真实交易系统，能快速调整风控、funding、hedge 等参数；代价是必须持续检查“配置已写入”与“业务代码真实消费”是否一致，不能只看后台表单已保存。

## 已知限制

- 对冲当前采用“先执行闭环、后账务闭环”的阶段性方案。
  现在系统已经具备真实 HL 下单、执行状态回写、外部仓位快照和漂移观测能力，但外部 Venue 的保证金、手续费、已实现盈亏、未实现盈亏仍作为平台外部风险账户单独管理，尚未正式并入核心统一账本。这样做的优点是先把对冲执行正确性、风险观测和运营可见性做稳，避免在账务口径尚未完全冻结前把外部资金流错误并入总账；当前平台层面的完整资金视图因此仍需要结合核心账本与外部对冲视图一起理解，后续再补齐对冲账务镜像、对冲对账和更完整的失败治理。
- 异步基础设施仍以数据库轮询为主。
  当前关键 worker 链路依赖 `outbox_events` 轮询消费，这对一致性有利，但对高吞吐、低延迟和复杂订阅模型并不是最终形态。随着交易量和异步复杂度增长，后续需要评估更清晰的消息总线和死信治理。
- 前端实时性仍是保守实现。
  当前管理页和用户页大量依赖 HTTP 轮询、窗口聚焦刷新和写后刷新，WebSocket / SSE 尚未成为正式主链路。这种实现优点是简单稳健、调试容易；限制是对“毫秒级实时反馈”的支持不足，尤其在后台监控和高频状态变化场景下更明显。
- 对账与恢复能力还不完整。
  当前 Explorer、账本审计、对冲快照和事件查询已经具备基础能力，但正式的链上/账本/仓位/对冲对账任务体系还未完整落地。系统在“可观测”和“可解释”上已有基础，但在“自动发现差异”和“标准化修复流程”上还需要继续补。
- 权限治理仍需要继续加强。
  当前管理后台已经有鉴权、白名单管理员和会话管理，但完整 RBAC、MFA、细粒度高危操作治理还没有彻底成型。也就是说，系统已经具备后台运维能力，但离严格的生产级权限治理仍有差距。

## 当前基线文档

- [需求拆解与实施路线](/Users/xiaobao/RGPerp/docs/需求拆解与实施路线.md)
- [Architecture Document](/Users/xiaobao/RGPerp/docs/Architecture%20Document.md)
- [Architecture Appendix](/Users/xiaobao/RGPerp/docs/Architecture%20Appendix.md)
