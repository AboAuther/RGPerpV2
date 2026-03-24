# Indexer 模块设计说明

## 1. 目标

Indexer 的唯一职责是消费链上事实，并把这些事实安全地推进到链下资金链路：

- 记录充值链上事实；
- 按确认数推进充值状态；
- 通过 Wallet/Ledger 域完成最终入账；
- 跟踪提现链上执行结果；
- 处理 orphan broadcast 回补；
- 持久化链扫描游标；
- 通过 outbox 发布异步事件。

Indexer 不直接修改：

- `account_balance_snapshots`
- `ledger_entries`
- 用户可用余额

所有资金变化仍然必须通过 Wallet -> Ledger 域完成。

## 2. 不变式

必须始终满足：

1. 链上事实先持久化，后推进业务状态；
2. 幂等键稳定，重复扫描不会重复落账；
3. `chain_cursors` 只前进不回退；
4. 充值确认前不得进入用户可用余额；
5. 重组导致的未入账充值只能冲正待确认镜像，已入账充值不得自动扣回；
6. 提现链上成功但本地缺失时，必须可由 Indexer 回补；
7. 未知 router / token / vault 事件不得静默丢弃，必须进入异常 outbox。

## 3. 本次实现

本次已经实现：

- `backend/internal/domain/indexer`
  - `Service`：充值检测、确认推进、提现成功/失败处理、reorg 回补、异常事件发布
  - `Runner`：按链扫描、按游标推进、批量 reconcile pending deposits
- `backend/internal/infra/db`
  - `chain_cursors` 的 GORM model 与仓储适配
  - Indexer outbox publisher
- `backend/internal/domain/wallet`
  - `ReverseDeposit`，用于确认前重组冲正
- `backend/internal/infra/db/tx_manager.go`
  - 复用已存在事务，保证 Indexer 编排时可以把 Wallet/Ledger/Outbox 置于同一事务上下文

## 4. 关键链路

### 4.1 充值

`DepositForwarded` -> 校验 chain/token/vault/router -> `DetectDeposit` -> 发布 `wallet.deposit.detected`

达到确认数后：

`AdvanceDeposit(CREDIT_READY)` -> 发布 `wallet.deposit.credit_ready` -> `ConfirmDeposit` -> 发布 `wallet.deposit.credited`

### 4.2 重组

`removed log` / 非 canonical 事件 -> 若尚未 `CREDITED`，执行 `ReverseDeposit` -> 发布 anomaly

### 4.3 提现成功

`WithdrawExecuted` -> 若本地仍在 `APPROVED`，先 `MarkWithdrawBroadcasted` 回补广播账务 -> `CompleteWithdraw`

补充说明：

- `SIGNING + broadcast_nonce` 表示该提现已经在数据库中预留 nonce；
- 若发送结果不确定，Indexer 不应把这类提现直接回退到可重新分配 nonce 的状态，而应等待链上事件回补或异常对账。

### 4.4 提现失败

链上失败 / receipt failed -> `withdraw_requests.status = FAILED` -> 发布 `wallet.withdraw.failed` -> `RefundWithdraw`

## 5. 重要风险

当前合约事件里的 `WithdrawExecuted.withdrawId` 是 `bytes32`，而现有链下 `withdraw_requests.withdraw_id` 是字符串 ID。

在真正接 EVM RPC 监听器之前，必须统一这两个层面的提现 ID 编码契约，否则真实链上事件无法无损映射本地提现单。

建议方案：

- 广播前把链下 `withdraw_id` 规范化为可逆的 `bytes32`；
- 或者新增 `chain_withdraw_id` 字段，作为链上唯一映射键。
