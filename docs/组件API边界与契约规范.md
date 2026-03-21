# 组件 API 边界与契约规范

## 1. 文档目标

本文档补充外部 OpenAPI 之外的组件级 API 边界，明确：

1. 各组件对外暴露什么接口；
2. 同步与异步调用如何划分；
3. 哪些接口可以直接调用，哪些只能通过事件触发；
4. 哪些字段是组件边界上的最小必需字段。

本文档用于约束：

- Frontend
- Backend API
- Vault
- Indexer
- Trade Engine
- Risk Engine
- Liquidator
- Hedger
- Funding Worker

## 2. 外部接口与内部接口的划分

### 2.1 外部接口

外部接口定义在：

- [openapi.yaml](/Users/xiaobao/RGPerp/spec/api/openapi.yaml)

适用对象：

- 前端
- 管理后台
- review 工具

### 2.2 内部接口

内部接口不通过公网暴露，分两类：

1. 进程内 usecase / service interface
2. 进程间 RabbitMQ 事件契约

RabbitMQ 事件契约定义在：

- [event-schema.md](/Users/xiaobao/RGPerp/spec/events/event-schema.md)

## 3. Frontend 边界

### 3.1 Frontend -> Backend API

允许：

- 调用 OpenAPI 定义的 HTTP 接口
- 建立 WebSocket 订阅

禁止：

- 直接连接 MySQL / Redis / RabbitMQ
- 直接调用链上索引逻辑
- 直接访问 Hyperliquid

### 3.2 Frontend 依赖的核心读接口

- Auth
- Account Summary
- Balances
- Deposit Addresses
- Withdraw History
- Market Tickers
- Orders / Fills / Positions
- Explorer Queries
- Admin Config / Withdraw Review / Risk Views

### 3.3 Frontend 依赖的核心推送接口

- `account.updates`
- `order.updates`
- `position.updates`
- `market.tickers`
- `market.markPrices`
- `notification.events`

## 4. Backend API 边界

### 4.1 对 Frontend 暴露

- 登录、会话、用户查询
- 充值地址、提现请求、内部转账
- 下单、撤单、订单/成交/仓位查询
- Explorer 查询
- 管理后台操作

### 4.2 对内部领域层调用

Backend API 只能依赖 usecase 接口，不直接依赖底层仓储实现。

推荐 usecase 边界：

- `AuthUseCase`
- `AccountUseCase`
- `WalletUseCase`
- `OrderUseCase`
- `ExplorerQueryUseCase`
- `AdminUseCase`

### 4.3 禁止事项

- API handler 不直接写 GORM model
- API handler 不直接访问 RabbitMQ
- API handler 不直接访问链上节点

## 5. Vault 边界

### 5.1 Vault 对链下的可见接口

链下只通过以下事实消费 Vault：

- `DepositForwarded`
- `WithdrawExecuted`
- router/factory 相关事件

### 5.2 Vault 不暴露给 Frontend 的内部能力

- Router 白名单管理
- token 白名单管理
- rescue 流程
- pause / unpause

这些只允许管理员或运维通过链下审批后由受控执行器调用。

## 6. Indexer 边界

### 6.1 输入

- EVM RPC
- Vault / Router / Token 事件
- 当前 chain cursor

### 6.2 输出

同步落库：

- `deposit_chain_txs`
- `withdraw_requests` 的链路状态推进
- `chain_cursors`

异步输出：

- `wallet.deposit.detected`
- `wallet.deposit.credit_ready`
- `wallet.withdraw.completed`
- `wallet.withdraw.failed`

### 6.3 Indexer 禁止事项

- 不直接写 `account_balance_snapshots`
- 不直接写 `ledger_entries`
- 不直接决定提现审核通过

## 7. Trade Engine 边界

### 7.1 输入

- 标准化行情
- symbol 元数据
- 已通过预校验的订单
- 用户资金与仓位快照

### 7.2 输出

同步写链路：

- `orders`
- `fills`
- `positions`
- `ledger_tx`
- `ledger_entries`
- `account_balance_snapshots`
- `outbox_events`

异步输出：

- `trade.order.accepted`
- `trade.fill.created`
- `trade.position.updated`

### 7.3 进程内接口建议

- `SubmitOrder(ctx, cmd)`
- `CancelOrder(ctx, cmd)`
- `ExecuteMarketableOrder(ctx, orderID)`
- `TriggerConditionalOrders(ctx, symbol, markPrice)`

## 8. Risk Engine 边界

### 8.1 输入

- mark price updates
- fills / positions changes
- funding application results
- account balance snapshot

### 8.2 输出

同步能力：

- `CheckPreTrade(ctx, input)`
- `RecalculateAccountRisk(ctx, userID)`

异步输出：

- `risk.snapshot.updated`
- `risk.liquidation.triggered`
- `hedge.requested`

### 8.3 禁止事项

- Risk Engine 不直接广播提现
- Risk Engine 不直接修改链上资金

## 9. Liquidator 边界

### 9.1 输入

- `risk.liquidation.triggered`

### 9.2 输出

同步写链路：

- `liquidations`
- `liquidation_items`
- `orders`
- `positions`
- `ledger_tx`
- `ledger_entries`
- `account_balance_snapshots`

异步输出：

- `risk.liquidation.executed`
- `trade.position.updated`

### 9.3 必须遵守

- 清算前先撤单
- 清算与账本同事务提交
- 清算结果必须有快照和审计

## 10. Hedger 边界

### 10.1 输入

- `hedge.requested`
- Hyperliquid 状态回查

### 10.2 输出

同步落库：

- `hedge_intents`
- `hedge_orders`
- `hedge_fills`
- `hedge_positions`

异步输出：

- `hedge.updated`
- `hedge.failed`

### 10.3 边界约束

- 对冲状态不直接回写用户仓位
- 对冲镜像账务只能影响平台账户

## 11. Funding Worker 边界

### 11.1 输入

- funding source data
- active positions
- funding interval schedule

### 11.2 输出

同步写链路：

- `funding_batches`
- `funding_batch_items`
- `positions.funding_accrual`
- `ledger_tx`
- `ledger_entries`
- `account_balance_snapshots`

异步输出：

- `risk.funding.batch.applied`

### 11.3 边界约束

- 资金费率批次只能由 Funding Worker 驱动
- 不允许其他组件绕过 Funding Worker 写 `funding_batches`

## 12. 组件通信矩阵

| From | To | Mode | Purpose |
| --- | --- | --- | --- |
| Frontend | Backend API | Sync HTTP | 用户与后台操作 |
| Frontend | WS Gateway | WebSocket | 行情与账户更新 |
| Backend API | Domain UseCases | In-process sync | 核心业务 |
| Indexer | Ledger/Wallet flow | Async MQ | 充值提现链路推进 |
| Trade Engine | Explorer/Notification | Async MQ | 成交、仓位、订单广播 |
| Risk Engine | Liquidator | Async MQ | 强平触发 |
| Risk Engine | Hedger | Async MQ | 对冲触发 |
| Funding Worker | Explorer/Risk | Async MQ | 资金费率结果传播 |

## 13. 结论

外部接口规范已经由 OpenAPI 覆盖。  
本文件补充了组件之间的最小 API 契约和调用边界，确保后续实现时：

1. 不会把组件职责做穿；
2. 不会把内部依赖硬耦合到 HTTP 或数据库；
3. 不会让强一致写链路被异步逻辑污染。
