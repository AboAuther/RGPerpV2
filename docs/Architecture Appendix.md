# 架构附录

## 1. 模块级 WBS 与开发分解

### 里程碑 1：架构基线及仓库骨架

- [ ] 建立 `frontend`、`backend`、`contracts`、`deploy`、`spec` 的仓库布局
- [ ] 定义领域模块：auth、ledger、wallet、market、order、position、risk、liquidation、funding、hedge、explorer、admin
- [ ] 创建 `dev`、`review`、`staging`、`prod` 环境配置
- [ ] 定义共享 ID 策略、小数精度策略及 Outbox 模式
- [ ] 定义 Ethereum、Arbitrum、Base、Hyperliquid 测试网 的链配置
- [ ] 确定 deposit、withdraw、order、position、funding、hedge 的状态机

### 里程碑 2：托管、认证及账本基础

- [ ] 实现带 deposit/withdraw 事件模型的 Vault 合约
- [ ] 添加 Foundry 测试：暂停、角色控制、提现授权、事件发出
- [ ] 在 Go 中实现钱包签名登录及会话流程
- [ ] 实现账本交易模型和余额快照模型
- [ ] 实现充值索引、确认处理及入账流程
- [ ] 实现提现请求、锁定、审批、广播、确认及退还流程
- [ ] 实现内部划转流程

### 里程碑 3：交易核心 P0

- [ ] 构建市场数据摄取及价格推导服务
- [ ] 实现交易对元数据及分级配置
- [ ] 实现订单 API、订单状态及取消流程
- [ ] 实现市价单和限价单的交易引擎
- [ ] 实现仓位更新及已实现/未实现 PnL
- [ ] 实现账户、订单、仓位与行情的轮询刷新机制

### 里程碑 4：风险、强平及资金费率

- [ ] 实现交易前风险检查
- [ ] 实现运行时权益及维持保证金重算
- [ ] 实现强平触发生成
- [ ] 实现强平执行器及惩罚金核算
- [ ] 实现保险基金核算
- [ ] 实现资金费率批次生成及应用 Worker
- [ ] 实现风险仪表盘及关键告警

### 里程碑 5：对冲、Explorer 及对账

- [ ] 实现平台净敞口计算
- [ ] 实现 Hyperliquid 测试网 对冲连接器
- [ ] 实现对冲订单及对冲仓位持久化
- [ ] 实现 Explorer 查询模型及事件页
- [ ] 实现钱包、账本、资金费率及仓位对账任务
- [ ] 实现审计日志及管理员操作历史

### 里程碑 6：交付硬化及评审环境

- [ ] 添加所有 P0 服务的 docker compose
- [ ] 添加种子评审数据及水龙头路径
- [ ] 添加可观测性：结构化日志、指标、追踪、死信队列
- [ ] 添加 deposit、withdraw、trade、liquidation、funding 的 E2E 测试套件
- [ ] 添加重放、重复交付、重启、重组等故障恢复测试
- [ ] 定稿 README、架构文档、AI 使用报告及运维手册

## 2. 数据库实体与表设计

### 2.1 身份与访问

#### `users`

- `id` bigint pk
- `evm_address` varchar(64) unique
- `status` varchar(32)
- `created_at` datetime
- `updated_at` datetime

#### `login_nonces`

- `id` bigint pk
- `evm_address` varchar(64)
- `nonce` varchar(128) unique
- `chain_id` bigint
- `domain` varchar(255)
- `expires_at` datetime
- `used_at` datetime null
- `created_at` datetime

#### `sessions`

- `id` bigint pk
- `user_id` bigint
- `access_jti` varchar(128) unique
- `refresh_jti` varchar(128) unique
- `device_fingerprint` varchar(255)
- `ip` varchar(64)
- `user_agent` varchar(512)
- `expires_at` datetime
- `revoked_at` datetime null
- `created_at` datetime

#### `admin_users`

- `id` bigint pk
- `username` varchar(128) unique
- `role` varchar(64)
- `status` varchar(32)
- `created_at` datetime
- `updated_at` datetime

### 2.2 账户与账本

#### `accounts`

- `id` bigint pk
- `user_id` bigint null
- `account_code` varchar(64)
- `account_type` varchar(64)
- `asset` varchar(32)
- `status` varchar(32)
- `created_at` datetime
- `updated_at` datetime

唯一键：

- `(user_id, account_code, asset)`

#### `ledger_tx`

- `id` bigint pk
- `ledger_tx_id` varchar(64) unique
- `event_id` varchar(64) unique
- `biz_type` varchar(64)
- `biz_ref_id` varchar(64)
- `asset` varchar(32)
- `idempotency_key` varchar(128)
- `operator_type` varchar(32)
- `operator_id` varchar(64)
- `trace_id` varchar(64)
- `status` varchar(32)
- `created_at` datetime

#### `ledger_entries`

- `id` bigint pk
- `ledger_tx_id` varchar(64)
- `account_id` bigint
- `user_id` bigint null
- `asset` varchar(32)
- `amount` decimal(38,18)
- `entry_type` varchar(64)
- `created_at` datetime

索引：

- `(ledger_tx_id)`
- `(account_id, created_at)`
- `(user_id, created_at)`

#### `account_balance_snapshots`

- `id` bigint pk
- `account_id` bigint
- `asset` varchar(32)
- `balance` decimal(38,18)
- `version` bigint
- `updated_at` datetime

唯一键：

- `(account_id, asset)`

#### `outbox_events`

- `id` bigint pk
- `event_id` varchar(64) unique
- `aggregate_type` varchar(64)
- `aggregate_id` varchar(64)
- `event_type` varchar(128)
- `payload_json` json
- `status` varchar(32)
- `published_at` datetime null
- `created_at` datetime

### 2.3 钱包与链

#### `vaults`

- `id` bigint pk
- `chain_id` bigint
- `contract_address` varchar(64)
- `asset` varchar(32)
- `status` varchar(32)
- `created_at` datetime

#### `deposit_addresses`

- `id` bigint pk
- `user_id` bigint
- `chain_id` bigint
- `address` varchar(64)
- `asset` varchar(32)
- `status` varchar(32)
- `created_at` datetime

唯一键：

- `(user_id, chain_id, asset)`

#### `deposit_chain_txs`

- `id` bigint pk
- `chain_id` bigint
- `tx_hash` varchar(128)
- `log_index` bigint
- `from_address` varchar(64)
- `to_address` varchar(64)
- `token_address` varchar(64)
- `amount` decimal(38,18)
- `block_number` bigint
- `confirmations` int
- `status` varchar(32)
- `credited_ledger_tx_id` varchar(64) null
- `created_at` datetime
- `updated_at` datetime

唯一键：

- `(chain_id, tx_hash, log_index)`

#### `withdraw_requests`

- `id` bigint pk
- `withdraw_id` varchar(64) unique
- `user_id` bigint
- `chain_id` bigint
- `asset` varchar(32)
- `amount` decimal(38,18)
- `fee_amount` decimal(38,18)
- `to_address` varchar(64)
- `status` varchar(32)
- `risk_flag` varchar(64) null
- `hold_ledger_tx_id` varchar(64)
- `broadcast_tx_hash` varchar(128) null
- `broadcast_nonce` bigint null
- `completed_at` datetime null
- `created_at` datetime
- `updated_at` datetime

#### `signer_nonce_states`

- `id` bigint pk
- `chain_id` bigint
- `signer_address` varchar(64)
- `next_nonce` bigint
- `created_at` datetime
- `updated_at` datetime

唯一键：

- `(chain_id, signer_address)`

#### `chain_cursors`

- `id` bigint pk
- `chain_id` bigint
- `cursor_type` varchar(64)
- `cursor_value` varchar(128)
- `updated_at` datetime

唯一键：

- `(chain_id, cursor_type)`

### 2.4 市场与合约

#### `symbols`

- `id` bigint pk
- `symbol` varchar(64) unique
- `asset_class` varchar(32)
- `base_asset` varchar(32)
- `quote_asset` varchar(32)
- `contract_multiplier` decimal(38,18)
- `tick_size` decimal(38,18)
- `step_size` decimal(38,18)
- `min_notional` decimal(38,18)
- `status` varchar(32)
- `session_policy` varchar(32)
- `created_at` datetime
- `updated_at` datetime

#### `symbol_mappings`

- `id` bigint pk
- `symbol_id` bigint
- `source_name` varchar(64)
- `source_symbol` varchar(64)
- `price_scale` decimal(38,18)
- `qty_scale` decimal(38,18)
- `status` varchar(32)

#### `risk_tiers`

- `id` bigint pk
- `symbol_id` bigint
- `tier_level` int
- `max_notional` decimal(38,18)
- `max_leverage` decimal(38,18)
- `imr` decimal(38,18)
- `mmr` decimal(38,18)
- `liquidation_fee_rate` decimal(38,18)
- `created_at` datetime

#### `market_price_snapshots`

- `id` bigint pk
- `symbol_id` bigint
- `source_name` varchar(64)
- `bid` decimal(38,18)
- `ask` decimal(38,18)
- `last` decimal(38,18)
- `mid` decimal(38,18)
- `source_ts` datetime
- `received_ts` datetime
- `created_at` datetime

#### `mark_price_snapshots`

- `id` bigint pk
- `symbol_id` bigint
- `index_price` decimal(38,18)
- `mark_price` decimal(38,18)
- `calc_version` bigint
- `created_at` datetime

### 2.5 订单、成交与仓位

#### `orders`

- `id` bigint pk
- `order_id` varchar(64) unique
- `client_order_id` varchar(128)
- `user_id` bigint
- `symbol_id` bigint
- `side` varchar(16)
- `position_effect` varchar(16)
- `type` varchar(32)
- `time_in_force` varchar(16)
- `price` decimal(38,18) null
- `trigger_price` decimal(38,18) null
- `qty` decimal(38,18)
- `filled_qty` decimal(38,18)
- `avg_fill_price` decimal(38,18)
- `reduce_only` tinyint
- `max_slippage_bps` int
- `status` varchar(32)
- `reject_reason` varchar(255) null
- `frozen_margin` decimal(38,18)
- `created_at` datetime
- `updated_at` datetime

唯一键：

- `(user_id, client_order_id)`

#### `fills`

- `id` bigint pk
- `fill_id` varchar(64) unique
- `order_id` varchar(64)
- `user_id` bigint
- `symbol_id` bigint
- `side` varchar(16)
- `qty` decimal(38,18)
- `price` decimal(38,18)
- `fee_amount` decimal(38,18)
- `execution_price_snapshot_id` bigint null
- `ledger_tx_id` varchar(64)
- `created_at` datetime

#### `positions`

- `id` bigint pk
- `position_id` varchar(64) unique
- `user_id` bigint
- `symbol_id` bigint
- `side` varchar(16)
- `qty` decimal(38,18)
- `avg_entry_price` decimal(38,18)
- `mark_price` decimal(38,18)
- `notional` decimal(38,18)
- `initial_margin` decimal(38,18)
- `maintenance_margin` decimal(38,18)
- `realized_pnl` decimal(38,18)
- `unrealized_pnl` decimal(38,18)
- `funding_accrual` decimal(38,18)
- `liquidation_price` decimal(38,18)
- `bankruptcy_price` decimal(38,18)
- `status` varchar(32)
- `updated_at` datetime
- `created_at` datetime

唯一键：

- `(user_id, symbol_id, side)`

### 2.6 风险、强平与资金费率

#### `risk_snapshots`

- `id` bigint pk
- `user_id` bigint
- `equity` decimal(38,18)
- `available_balance` decimal(38,18)
- `maintenance_margin` decimal(38,18)
- `margin_ratio` decimal(38,18)
- `risk_level` varchar(32)
- `triggered_by` varchar(64)
- `created_at` datetime

#### `liquidations`

- `id` bigint pk
- `liquidation_id` varchar(64) unique
- `user_id` bigint
- `symbol_id` bigint null
- `mode` varchar(32)
- `status` varchar(32)
- `trigger_risk_snapshot_id` bigint
- `penalty_amount` decimal(38,18)
- `insurance_fund_used` decimal(38,18)
- `bankrupt_amount` decimal(38,18)
- `created_at` datetime
- `updated_at` datetime

#### `liquidation_items`

- `id` bigint pk
- `liquidation_id` varchar(64)
- `position_id` varchar(64)
- `liquidated_qty` decimal(38,18)
- `execution_price` decimal(38,18)
- `ledger_tx_id` varchar(64)
- `created_at` datetime

#### `funding_batches`

- `id` bigint pk
- `funding_batch_id` varchar(64) unique
- `symbol_id` bigint
- `time_window_start` datetime
- `time_window_end` datetime
- `normalized_rate` decimal(38,18)
- `settlement_price` decimal(38,18)
- `status` varchar(32)
- `created_at` datetime
- `updated_at` datetime

#### `funding_batch_items`

- `id` bigint pk
- `funding_batch_id` varchar(64)
- `position_id` varchar(64)
- `user_id` bigint
- `funding_fee` decimal(38,18)
- `ledger_tx_id` varchar(64) null
- `status` varchar(32)
- `created_at` datetime

### 2.7 对冲

#### `hedge_intents`

- `id` bigint pk
- `hedge_intent_id` varchar(64) unique
- `symbol_id` bigint
- `side` varchar(16)
- `target_qty` decimal(38,18)
- `current_net_exposure` decimal(38,18)
- `status` varchar(32)
- `created_at` datetime
- `updated_at` datetime

#### `hedge_orders`

- `id` bigint pk
- `hedge_order_id` varchar(64) unique
- `hedge_intent_id` varchar(64)
- `venue` varchar(32)
- `venue_order_id` varchar(128) null
- `symbol` varchar(64)
- `side` varchar(16)
- `qty` decimal(38,18)
- `price` decimal(38,18) null
- `status` varchar(32)
- `error_code` varchar(64) null
- `created_at` datetime
- `updated_at` datetime

#### `hedge_fills`

- `id` bigint pk
- `hedge_fill_id` varchar(64) unique
- `hedge_order_id` varchar(64)
- `venue_fill_id` varchar(128)
- `qty` decimal(38,18)
- `price` decimal(38,18)
- `fee` decimal(38,18)
- `created_at` datetime

#### `hedge_positions`

- `id` bigint pk
- `symbol` varchar(64)
- `side` varchar(16)
- `qty` decimal(38,18)
- `avg_entry_price` decimal(38,18)
- `realized_pnl` decimal(38,18)
- `unrealized_pnl` decimal(38,18)
- `updated_at` datetime

### 2.8 Explorer、审计与配置

#### `audit_logs`

- `id` bigint pk
- `audit_id` varchar(64) unique
- `actor_type` varchar(32)
- `actor_id` varchar(64)
- `action` varchar(128)
- `resource_type` varchar(64)
- `resource_id` varchar(64)
- `before_json` json null
- `after_json` json null
- `trace_id` varchar(64)
- `created_at` datetime

#### `config_items`

- `id` bigint pk
- `config_key` varchar(128)
- `scope_type` varchar(64)
- `scope_value` varchar(128)
- `version` bigint
- `value_json` json
- `effective_at` datetime
- `status` varchar(32)
- `created_by` varchar(64)
- `approved_by` varchar(64) null
- `reason` varchar(255)
- `created_at` datetime

唯一键：

- `(config_key, scope_type, scope_value, version)`

#### `explorer_events`

- `id` bigint pk
- `internal_seq` bigint unique
- `event_id` varchar(64) unique
- `event_type` varchar(128)
- `user_id` bigint null
- `address` varchar(64) null
- `symbol` varchar(64) null
- `order_id` varchar(64) null
- `fill_id` varchar(64) null
- `position_id` varchar(64) null
- `ledger_tx_id` varchar(64) null
- `chain_tx_hash` varchar(128) null
- `block_number` bigint null
- `payload_json` json
- `created_at` datetime

## 3. API 列表

### 3.1 认证 API

- `POST /api/v1/auth/nonce`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`

### 3.2 账户 API

- `GET /api/v1/account/summary`
- `GET /api/v1/account/balances`
- `GET /api/v1/account/ledger`
- `GET /api/v1/account/risk`
- `POST /api/v1/account/transfer`

### 3.3 钱包 API

- `GET /api/v1/wallet/deposit-addresses`
- `GET /api/v1/wallet/deposits`
- `POST /api/v1/wallet/withdrawals`
- `GET /api/v1/wallet/withdrawals`
- `GET /api/v1/wallet/withdrawals/:withdrawId`
- `POST /api/v1/review/faucet`

### 3.4 市场 API

- `GET /api/v1/markets/symbols`
- `GET /api/v1/markets/tickers`
- `GET /api/v1/markets/orderbook-synthetic`
- `GET /api/v1/markets/funding`
- `GET /api/v1/markets/mark-price-history`

### 3.5 订单与仓位 API

- `POST /api/v1/orders`
- `POST /api/v1/orders/:orderId/cancel`
- `GET /api/v1/orders`
- `GET /api/v1/orders/:orderId`
- `GET /api/v1/fills`
- `GET /api/v1/positions`
- `POST /api/v1/positions/:positionId/close`

### 3.6 Explorer API

- `GET /api/v1/explorer/events`
- `GET /api/v1/explorer/events/:eventId`
- `GET /api/v1/explorer/ledger/:ledgerTxId`
- `GET /api/v1/explorer/tx/:chainTxHash`
- `GET /api/v1/explorer/address/:address`

### 3.7 管理 API

- `GET /api/v1/admin/users`
- `POST /api/v1/admin/users/:userId/status`
- `GET /api/v1/admin/configs`
- `POST /api/v1/admin/configs`
- `POST /api/v1/admin/withdrawals/:withdrawId/approve`
- `POST /api/v1/admin/withdrawals/:withdrawId/reject`
- `POST /api/v1/admin/reconciliation/run`
- `GET /api/v1/admin/liquidations`
- `GET /api/v1/admin/funding-batches`
- `GET /api/v1/admin/hedges`

### 3.8 刷新机制

- 交易页通过 HTTP 轮询获取 `tickers`、订单、仓位、账户概览
- 关键写操作成功后主动重载账户、订单、仓位数据
- 断线、接口失败、页面回到前台时主动刷新关键读模型

## 4. 事件模型

### 4.1 领域事件分类

- 认证事件
- 钱包事件
- 交易事件
- 风险事件
- 资金费率事件
- 对冲事件
- 配置事件
- 审计事件

### 4.2 核心事件类型

#### 钱包

- `deposit.detected`
- `deposit.confirming`
- `deposit.credited`
- `deposit.reorg_reversed`
- `withdraw.requested`
- `withdraw.hold_created`
- `withdraw.approved`
- `withdraw.broadcasted`
- `withdraw.completed`
- `withdraw.failed`
- `withdraw.refunded`

#### 交易

- `order.accepted`
- `order.rejected`
- `order.canceled`
- `order.triggered`
- `fill.created`
- `position.updated`
- `position.closed`
- `ledger.committed`

#### 风险与强平

- `risk.snapshot.updated`
- `liquidation.triggered`
- `liquidation.executed`
- `insurance_fund.used`

#### 资金费率

- `funding.batch.created`
- `funding.batch.applied`
- `funding.batch.failed`
- `funding.batch.reversed`

#### 对冲

- `hedge.requested`
- `hedge.order.sent`
- `hedge.order.updated`
- `hedge.failed`
- `hedge.position.updated`

#### 运维

- `config.changed`
- `reconciliation.failed`
- `audit.logged`

### 4.3 事件信封

每个事件应包含：

- `event_id`
- `event_type`
- `aggregate_type`
- `aggregate_id`
- `trace_id`
- `occurred_at`
- `producer`
- `payload`
- `version`

## 5. P0 验收清单

### 5.1 架构与平台

- [ ] 单仓库可用 docker compose 启动
- [ ] 所有核心服务可连接 MySQL、Redis、RabbitMQ
- [ ] Vault 合约可本地部署并发出所需事件

### 5.2 托管与账本

- [ ] 充值经确认后仅对用户钱包入账一次
- [ ] 提现锁定、广播、确认及退还流程完整
- [ ] 内部划转写入平衡的账本分录
- [ ] 无任何路径在不经账本分录的情况下直接修改余额

### 5.3 交易

- [ ] 用户可下市价单
- [ ] 用户可下限价单
- [ ] 用户可取消挂单
- [ ] 成交原子性地更新仓位和账本
- [ ] 对冲模式独立支持多空仓位

### 5.4 风险与强平

- [ ] 交易前风险拒绝保证金不足的订单
- [ ] 运行时风险随标记价格更新重算
- [ ] 低于阈值时自动触发强平
- [ ] 强平产生账本、仓位及审计记录

### 5.5 资金费率与对冲

- [ ] 资金费率批次可幂等生成并应用
- [ ] 可按交易对计算平台净敞口
- [ ] 对冲意图可发送至 Hyperliquid 测试网
- [ ] 对冲失败会触发运维告警

### 5.6 可观测性与审计

- [ ] Explorer 可按地址、订单、账本交易、链上交易查询
- [ ] 管理操作写入审计日志
- [ ] 对账任务可触发并产出报告

## 6. P0 测试用例矩阵

| 领域 | 测试用例 | 类型 | 预期结果 |
| --- | --- | --- | --- |
| 认证 | 重放 nonce | 安全 | 第二次登录被拒绝 |
| 认证 | 过期签名登录 | 安全 | 登录被拒绝 |
| 充值 | 重复链日志交付 | 集成 | 充值仅入账一次 |
| 充值 | 检测后最终确认前的重组 | 集成 | 充值回滚且不入账 |
| 提现 | 余额不足的提现请求 | 集成 | 请求被拒且无账本变更 |
| 提现 | 预留 nonce 后广播结果不确定 | 集成 | 保持 `SIGNING + broadcast_nonce`，只能复用同一 nonce 重试或由 Indexer/人工对账收敛 |
| 账本 | 平衡账本不变性检查 | 单元 | 每笔 ledger tx 金额之和为零 |
| 账本 | 并发划转请求 | 并发 | 无超额支出且最终余额一致 |
| 订单 | 市价单正常路径 | E2E | 订单接受、成交、仓位更新 |
| 订单 | 限价单触发路径 | E2E | 挂单后于可接受价格成交 |
| 订单 | 重复 `client_order_id` | 集成 | 第二次请求幂等拒绝/返回 |
| 仓位 | 同一交易对多空并存 | 集成 | 对冲模式存储两个独立仓位 |
| 风险 | 开仓保证金不足 | 单元/集成 | 订单被拒 |
| 风险 | 标记价格陈旧 | 集成 | 新开仓订单被拒 |
| 强平 | 风险突破触发强平 | E2E | 订单取消、保证金释放、强平记录 |
| 强平 | 重复强平触发交付 | 并发 | 强平仅执行一次 |
| 资金费率 | 资金费率批次重跑 | 集成 | 无重复扣费 |
| 对冲 | 敞口跨越阈值 | 集成 | 创建对冲意图 |
| 对冲 | Hyperliquid 失败 | 集成 | 对冲请求标记失败并发出告警 |
| Explorer | 按链交易哈希查询 | E2E | 返回匹配的充值/提现事件 |
| 配置 | 风险参数变更审计 | 集成 | 审计日志包含变更前后 |
| 恢复 | outbox 写入后发布前重启 | 韧性 | 中继恢复后事件被发布 |
| 恢复 | 从旧游标重放索引器 | 韧性 | 状态收敛且无重复入账 |
