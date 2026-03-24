# 充值提现链路与 Indexer 规范

## 1. 文档目标

本文档定义链上事件索引、充值确认、提现跟踪、链重组处理、广播补录与对账要求。

## 2. Indexer 职责边界

Indexer 只负责：

- 监听链上事件；
- 按确认数推进状态；
- 记录链上事实；
- 触发异步事件。

Indexer 不负责：

- 直接修改用户可用余额；
- 直接做业务审批；
- 直接计算风控。

## 3. 充值链路

### 3.1 检测

监听范围：

- `DepositForwarded` 事件；
- 关联 `Transfer` 事件；
- Router 创建事件。

### 3.2 状态机

```text
DETECTED -> CONFIRMING -> CREDIT_READY -> CREDITED -> SWEPT
DETECTED -> REORG_REVERSED
ANY -> FAILED
```

### 3.3 游标推进

- 以 `chain_id + block_number + log_index` 为自然序；
- Indexer 只能在“已持久化事件记录成功”后推进游标；
- 游标推进前必须保证去重键持久化成功。

### 3.4 去重键

```text
deposit_unique = chain_id + tx_hash + log_index
```

### 3.5 确认数

每条链单独配置确认数：

- Ethereum
- Arbitrum
- Base

达到确认数前不可转为用户可用余额。

### 3.6 重组处理

- 若已记录事件在确认前从 canonical chain 消失，标记 `REORG_REVERSED`
- 若已入账后发现异常，必须进入人工对账，不允许自动扣回用户可用余额

## 4. 提现链路

### 4.1 状态机

```text
REQUESTED -> HOLD -> RISK_REVIEW -> APPROVED -> SIGNING -> BROADCASTED -> CONFIRMING -> COMPLETED
HOLD -> CANCELED
RISK_REVIEW -> REJECTED
BROADCASTED/CONFIRMING -> FAILED -> REFUNDED
```

### 4.2 广播幂等

提现广播唯一键：

```text
withdraw_id
```

nonce 分配唯一键：

```text
chain_id + signer_address
```

链上交易跟踪键：

```text
chain_id + tx_hash
```

### 4.3 orphan broadcast

若出现“链上已广播，但本地未更新状态”：

- Indexer 必须可通过 `WithdrawExecuted` 事件回补；
- 回补流程必须关联 `withdraw_id`；
- 回补时不得重复落账。
- `SIGNING` 状态必须视为“已预留 nonce 待收敛”，发送结果不确定时不得改回 `APPROVED` 重新分配新 nonce。

## 5. MQ 事件

Index 侧至少发布：

- `deposit.detected`
- `deposit.confirming`
- `deposit.credit_ready`
- `withdraw.chain_confirmed`
- `withdraw.chain_failed`

## 6. 对账

必须至少有：

1. Router -> Vault 资金链对账
2. Vault 余额 -> 托管镜像账户对账
3. 提现广播记录 -> 链上确认结果对账
4. deposit credited -> chain event 对账

## 7. 安全要求

- Indexer RPC 节点至少双源
- 节点切换不允许回退已成功处理的唯一键
- 对未知 router、未知 token、未知 vault 的事件一律进入异常队列
- 异常事件不得静默丢弃
