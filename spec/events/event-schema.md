# Event Schema

## 1. 目标

本文档定义 RabbitMQ 事件 envelope、routing key、payload 字段、幂等规则、重试和死信策略。

## 2. Envelope

所有事件必须使用统一 envelope：

```json
{
  "event_id": "evt_01...",
  "event_type": "wallet.deposit.credited",
  "aggregate_type": "deposit",
  "aggregate_id": "dep_01...",
  "trace_id": "trc_01...",
  "producer": "indexer",
  "version": 1,
  "occurred_at": "2026-03-21T12:00:00Z",
  "payload": {}
}
```

字段定义：

- `event_id`：全局唯一
- `event_type`：事件名称
- `aggregate_type`：聚合类型
- `aggregate_id`：聚合 ID
- `trace_id`：链路追踪 ID
- `producer`：发布者服务
- `version`：Schema 版本
- `occurred_at`：事件生成时间
- `payload`：业务体

## 3. Exchange / Queue / Routing Key

### 3.1 Exchanges

- `domain.events`
- `ops.events`

### 3.2 Routing Keys

- `wallet.deposit.detected`
- `wallet.deposit.credit_ready`
- `wallet.deposit.credited`
- `wallet.withdraw.requested`
- `wallet.withdraw.broadcasted`
- `wallet.withdraw.completed`
- `wallet.withdraw.failed`
- `trade.order.accepted`
- `trade.order.canceled`
- `trade.fill.created`
- `trade.position.updated`
- `risk.snapshot.updated`
- `risk.liquidation.triggered`
- `risk.liquidation.executed`
- `risk.funding.batch.applied`
- `hedge.requested`
- `hedge.updated`
- `config.changed`
- `audit.logged`

### 3.3 Recommended Queues

- `explorer.indexer.q`
- `notification.q`
- `risk.liquidation.q`
- `funding.apply.q`
- `hedge.execute.q`
- `wallet.reconcile.q`

每个关键队列应绑定死信队列。

## 4. 幂等规则

- producer 幂等：同一个业务对象同一个状态变化只发同一个 `event_id`
- consumer 幂等：以 `consumer_name + event_id` 去重
- 重放事件时保持 `event_id` 不变

## 5. 重试与死信

- 最大重试次数：5
- backoff：0s / 5s / 30s / 120s / 600s
- 超过次数进入 DLQ
- 不可重试错误直接进入 DLQ

不可重试错误：

- schema mismatch
- 配置缺失
- 唯一键冲突且请求体不一致
- 权限错误

## 6. Payload Schemas

## 6.1 `wallet.deposit.detected`

```json
{
  "deposit_id": "dep_01",
  "chain_id": 8453,
  "tx_hash": "0x...",
  "log_index": 12,
  "block_number": 123,
  "user_id": 1001,
  "router_address": "0x...",
  "vault_address": "0x...",
  "token_address": "0x...",
  "asset": "USDC",
  "amount": "100.000000",
  "confirmations": 3,
  "status": "DETECTED"
}
```

## 6.2 `wallet.deposit.credited`

```json
{
  "deposit_id": "dep_01",
  "user_id": 1001,
  "chain_id": 8453,
  "tx_hash": "0x...",
  "asset": "USDC",
  "amount": "100.000000",
  "ledger_tx_id": "ldg_01",
  "status": "CREDITED"
}
```

## 6.3 `wallet.withdraw.broadcasted`

```json
{
  "withdraw_id": "wd_01",
  "user_id": 1001,
  "chain_id": 42161,
  "tx_hash": "0x...",
  "to_address": "0x...",
  "asset": "USDC",
  "gross_amount": "100.000000",
  "net_amount": "99.000000",
  "fee_amount": "1.000000",
  "ledger_tx_id": "ldg_02",
  "status": "BROADCASTED"
}
```

## 6.4 `trade.order.accepted`

```json
{
  "order_id": "ord_01",
  "client_order_id": "cli_01",
  "user_id": 1001,
  "symbol": "BTC-USD-PERP",
  "side": "BUY",
  "type": "LIMIT",
  "qty": "0.100000000000000000",
  "price": "65000",
  "frozen_margin": "1000",
  "status": "ACCEPTED"
}
```

## 6.5 `trade.fill.created`

```json
{
  "fill_id": "fill_01",
  "order_id": "ord_01",
  "user_id": 1001,
  "symbol": "BTC-USD-PERP",
  "side": "BUY",
  "qty": "0.1",
  "price": "65100",
  "fee_amount": "0.5",
  "position_id": "pos_01",
  "ledger_tx_id": "ldg_03"
}
```

## 6.6 `trade.position.updated`

```json
{
  "position_id": "pos_01",
  "user_id": 1001,
  "symbol": "BTC-USD-PERP",
  "side": "LONG",
  "qty": "0.1",
  "avg_entry_price": "65100",
  "mark_price": "65120",
  "initial_margin": "1000",
  "maintenance_margin": "500",
  "unrealized_pnl": "2",
  "status": "OPEN"
}
```

## 6.7 `risk.liquidation.triggered`

```json
{
  "liquidation_id": "liq_01",
  "user_id": 1001,
  "margin_ratio": "0.98",
  "equity": "450",
  "maintenance_margin": "460",
  "trigger_price_ts": "2026-03-21T12:00:00Z",
  "status": "TRIGGERED"
}
```

## 6.8 `risk.liquidation.executed`

```json
{
  "liquidation_id": "liq_01",
  "user_id": 1001,
  "symbol": "BTC-USD-PERP",
  "liquidated_qty": "0.1",
  "execution_price": "64000",
  "penalty_amount": "10",
  "insurance_fund_used": "0",
  "ledger_tx_id": "ldg_04",
  "status": "EXECUTED"
}
```

## 6.9 `risk.funding.batch.applied`

```json
{
  "funding_batch_id": "fb_01",
  "symbol": "BTC-USD-PERP",
  "time_window_start": "2026-03-21T11:00:00Z",
  "time_window_end": "2026-03-21T12:00:00Z",
  "normalized_rate": "0.0001",
  "status": "APPLIED",
  "applied_count": 1024
}
```

## 6.10 `hedge.requested`

```json
{
  "hedge_intent_id": "hint_01",
  "symbol": "BTC",
  "side": "SELL",
  "target_qty": "10",
  "current_net_exposure": "12",
  "soft_threshold_ratio": "0.2",
  "hard_threshold_ratio": "0.4",
  "status": "PENDING"
}
```

## 6.11 `hedge.updated`

```json
{
  "hedge_order_id": "hord_01",
  "hedge_intent_id": "hint_01",
  "venue": "hyperliquid_testnet",
  "venue_order_id": "123456",
  "symbol": "BTC",
  "side": "SELL",
  "qty": "10",
  "filled_qty": "10",
  "avg_fill_price": "64950",
  "status": "FILLED"
}
```

## 6.12 `config.changed`

```json
{
  "config_key": "risk.mark_price_stale_sec",
  "scope_type": "global",
  "scope_value": "global",
  "version": 2,
  "old_value": "3",
  "new_value": "2",
  "created_by": "ops_01",
  "approved_by": "risk_admin_01",
  "effective_at": "2026-03-21T12:00:00Z"
}
```

## 7. 审计要求

以下事件必须同时写审计日志并投递：

- `wallet.withdraw.*`
- `risk.liquidation.*`
- `config.changed`
- `audit.logged`

## 8. 版本演进

- schema 不兼容变更时增加 `version`
- 同一 `event_type` 不允许静默改字段语义
- consumer 必须显式声明支持的 `version`
