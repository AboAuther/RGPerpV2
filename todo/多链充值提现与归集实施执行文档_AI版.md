# 多链充值提现与归集实施执行文档（AI 版）

## 1. 文档目的

本文档用于指导 AI 在当前仓库中，从“本地单链 Anvil 联调完成”演进到“支持多链充值、提现、资金归集”的完整实施。

目标不是只补几段代码，而是完成以下一整套能力：

- 多链合约部署与配置管理
- 多链充值地址分配
- 多链 Indexer 监听、幂等、补偿、重组处理
- 多链提现执行与链上确认跟踪
- 多链资金归集与 gas 补给
- 多链对账、异常告警与验收用例

本文档是执行规范，不是讨论稿。AI 应按本文档顺序实施，除非遇到明确阻塞，否则不要中途改范围。

## 2. 执行前必须阅读的文档

实施前必须完整阅读并遵守以下文档：

- [CFD永续合约交易所_扩充版系统需求与技术设计.md](/Users/xiaobao/RGPerp/CFD永续合约交易所_扩充版系统需求与技术设计.md)
- [Vault合约设计说明.md](/Users/xiaobao/RGPerp/docs/Vault合约设计说明.md)
- [充值提现链路与Indexer规范.md](/Users/xiaobao/RGPerp/docs/充值提现链路与Indexer规范.md)
- [账本分录模板与资金状态机规范.md](/Users/xiaobao/RGPerp/docs/账本分录模板与资金状态机规范.md)
- [组件数据表归属与读写边界.md](/Users/xiaobao/RGPerp/docs/组件数据表归属与读写边界.md)
- [组件API边界与契约规范.md](/Users/xiaobao/RGPerp/docs/组件API边界与契约规范.md)
- [服务间事务、幂等与补偿规范.md](/Users/xiaobao/RGPerp/docs/服务间事务、幂等与补偿规范.md)
- [环境配置与配置加载规范.md](/Users/xiaobao/RGPerp/docs/环境配置与配置加载规范.md)

若实现与文档冲突，以这些规范文档为准。

## 3. 当前仓库的已知基础能力

当前仓库已经具备以下基础，不要重复造轮子：

- 多链静态配置骨架：
  - [backend/internal/config/config.go](/Users/xiaobao/RGPerp/backend/internal/config/config.go)
  - [backend/internal/config/chains.go](/Users/xiaobao/RGPerp/backend/internal/config/chains.go)
- 可运行的 Indexer worker：
  - [backend/cmd/indexer/main.go](/Users/xiaobao/RGPerp/backend/cmd/indexer/main.go)
- 多链 EVM 事件源：
  - [backend/internal/infra/chain/evm_event_source.go](/Users/xiaobao/RGPerp/backend/internal/infra/chain/evm_event_source.go)
- 按链创建充值 Router 的地址分配器：
  - [backend/internal/infra/chain/router_deposit_address_allocator.go](/Users/xiaobao/RGPerp/backend/internal/infra/chain/router_deposit_address_allocator.go)
- 按链执行 Vault 提现的执行器：
  - [backend/internal/infra/chain/vault_withdraw_executor.go](/Users/xiaobao/RGPerp/backend/internal/infra/chain/vault_withdraw_executor.go)
- 本地单链部署脚本：
  - [deploy/scripts/deploy-local-contracts.sh](/Users/xiaobao/RGPerp/deploy/scripts/deploy-local-contracts.sh)

## 4. 本次实施的最终目标

实施完成后，系统必须满足：

1. 支持至少三条链：
   - Ethereum
   - Arbitrum
   - Base
2. 每个用户在每条链拥有唯一充值地址。
3. 每条链都有独立的：
   - RPC 配置
   - 确认数
   - USDC 地址
   - Vault 地址
   - Factory 地址
   - 提现 signer
   - 归集钱包配置
4. Indexer 可对多链并行工作，服务重启后不重不漏。
5. 提现执行链路支持多链广播、确认、失败退款、orphan broadcast 补录。
6. 增加资金归集能力，至少支持：
   - 热钱包
   - 温钱包
   - 冷钱包
   - Sweep 账务
7. 增加链上与账本的日对账能力。
8. 保留本地 Anvil 联调模式。
9. 所有资金变动仍严格遵守账本模型，不允许绕过 Ledger 直接改余额。

## 5. 明确不在本次范围内的事项

以下内容不属于本次实施范围，除非单独要求：

- 新增非 USDC 资产支持
- 非 EVM 链支持
- 前端大范围重设计
- 冷钱包真实硬件集成
- 第三方托管或 MPC 商业服务接入

可以为未来预留接口，但不要在本次实施中引入过重的外部依赖。

## 6. 关键设计约束

AI 实施时必须遵守以下约束：

1. Indexer 不直接改 `account_balance_snapshots`。
2. Indexer 不直接写 `ledger_entries`，只能通过 Wallet/Ledger 域服务推进。
3. 充值去重键必须保持：
   - `chain_id + tx_hash + log_index`
4. 提现广播唯一键必须保持：
   - `withdraw_id`
5. 链上跟踪键必须保持：
   - `chain_id + tx_hash`
6. 业务状态、账本分录、快照、outbox 必须同事务提交。
7. 任何异常不得静默丢弃，必须进入 anomaly/outbox 或可追踪错误链路。
8. 不允许为了省事跳过重组处理、幂等处理、补偿处理。
9. 不允许把私钥明文硬编码到代码仓库。
10. 生产模式必须 fail closed。

## 7. AI 的执行总原则

AI 必须按以下原则工作：

1. 先补基础设施，再补业务 worker，再补测试与脚本。
2. 优先复用现有模块，不要重写已存在的 Indexer、Wallet、Vault 执行器。
3. 每完成一个阶段，都要运行相关测试与 smoke test。
4. 新增文件、脚本、env 模板、runbook 时，必须同步补文档。
5. 所有新增配置都必须经过 `StaticConfig.Validate()` 的 fail-closed 校验。
6. 所有多链逻辑必须显式按 `chain_id` 隔离，不允许隐式共用状态。

## 8. 必须实现的阶段与交付物

### 阶段 A：多链配置模型升级

目标：让系统既支持本地多链联调，也支持测试网和生产网多链。

必须完成：

1. 重构链配置模型，避免当前 `dev/review` 下把 `base` 直接映射为单个本地链的硬编码限制。
2. 支持显式配置“启用哪些链”，不要只靠 `RPCURL` 是否为空来隐式判断。
3. 支持每条链配置以下字段：
   - `chain_id`
   - `key`
   - `display_name`
   - `rpc_primary_url`
   - `rpc_secondary_url`
   - `confirmations`
   - `usdc_address`
   - `vault_address`
   - `factory_address`
   - `withdraw_signer_ref`
   - `hot_wallet_address`
   - `warm_wallet_address`
   - `cold_wallet_address`
   - `gas_topup_enabled`
4. 为 `dev`、`review`、`staging`、`prod` 提供清晰的 env 模板和说明。
5. 启动失败规则中加入：
   - 某条启用链缺失主 RPC
   - 启用提现但缺失 Vault 或 signer
   - 启用充值但缺失 Factory 或 token
   - 启用归集但缺失 warm/cold wallet

需要重点修改的文件：

- [backend/internal/config/config.go](/Users/xiaobao/RGPerp/backend/internal/config/config.go)
- [backend/internal/config/chains.go](/Users/xiaobao/RGPerp/backend/internal/config/chains.go)
- [deploy/env/common.env.example](/Users/xiaobao/RGPerp/deploy/env/common.env.example)
- [deploy/env/dev.env](/Users/xiaobao/RGPerp/deploy/env/dev.env)
- [deploy/env/review.env](/Users/xiaobao/RGPerp/deploy/env/review.env)
- [deploy/env/prod.env.checklist](/Users/xiaobao/RGPerp/deploy/env/prod.env.checklist)

验收标准：

- `api-server`、`indexer`、`withdraw executor` 都能识别多条启用链。
- 配置缺失时进程启动失败。
- 本地环境仍可只启用本地链。

### 阶段 B：多链本地部署能力

目标：把现在的单条本地 Anvil 脚本，升级成“可在本地模拟多链”的部署体系。

必须完成：

1. 提供三条本地链的启动脚本，建议端口独立。
2. 为每条本地链部署：
   - `MockUSDC`
   - `Vault`
   - `DepositRouterFactory`
3. 生成统一的 deployment manifest，而不是只写几个 env 变量。
4. deployment manifest 至少包含：
   - `env`
   - `chain_id`
   - `chain_key`
   - `rpc_url`
   - `token_address`
   - `vault_address`
   - `factory_address`
   - `deploy_block`
   - `deploy_tx_hash`
   - `admin_address`
   - `roles`
   - `created_at`
5. 本地脚本应支持重复执行时复用已部署合约。

建议新增：

- `deploy/scripts/start-local-multichain.sh`
- `deploy/scripts/deploy-multichain-contracts.sh`
- `deploy/manifest/dev/*.json`

验收标准：

- 本地一键拉起三条链。
- 每条链都有独立合约地址。
- 系统可以读取 manifest 或由其生成 env。

### 阶段 C：充值地址分配的多链正式化

目标：确保“用户 x 链”唯一充值地址在多链下可持续工作。

必须完成：

1. 审查 `deposit_addresses` 表与相关仓储，确认其唯一键是：
   - `user_id + chain_id + asset`
2. 地址分配流程支持按链创建 Router。
3. 地址分配失败时保留错误与重试能力。
4. 增加“按链校验 Router 地址归属”的回查能力。
5. 对 Factory 已有 Router 但本地未落库的情况，支持修复同步。

重点文件：

- [backend/internal/infra/chain/router_deposit_address_allocator.go](/Users/xiaobao/RGPerp/backend/internal/infra/chain/router_deposit_address_allocator.go)
- `wallet` 相关 usecase/repository
- 需要时新增修复脚本与管理命令

验收标准：

- 同一个用户在三条链拿到三个不同 Router 地址。
- 重复申请地址不重复创建。
- Factory 已创建但本地缺记录时，可自动或手工修复。

### 阶段 D：Indexer 多链生产化

目标：把当前“能多链轮询”的 Indexer 升级为“多链可运维的 Indexer”。

必须完成：

1. 保留现有多链扫描主干，不要推翻：
   - [backend/cmd/indexer/main.go](/Users/xiaobao/RGPerp/backend/cmd/indexer/main.go)
   - [backend/internal/domain/indexer/service.go](/Users/xiaobao/RGPerp/backend/internal/domain/indexer/service.go)
   - [backend/internal/infra/chain/evm_event_source.go](/Users/xiaobao/RGPerp/backend/internal/infra/chain/evm_event_source.go)
2. 每条链使用独立 cursor，不允许共用。
3. 为每条链增加主备 RPC 支持和切换逻辑。
4. 节点切换时不允许回退已成功处理的唯一键。
5. 支持以下事件的多链监听：
   - `RouterCreated`
   - `DepositForwarded`
   - `WithdrawExecuted`
6. 保持以下补偿能力：
   - deposit confirm 前 reorg reversal
   - orphan broadcast 补录
   - withdraw failed refund
7. 增加每链运行指标：
   - latest block
   - cursor block
   - scan lag
   - rpc error count
   - reorg count
   - anomaly count
8. 增加链身份校验，避免本地链重建后旧游标污染。

验收标准：

- 同时监听三条链，充值与提现互不串链。
- 服务重启后可继续从正确 cursor 位置恢复。
- 节点切换、receipt not found、短暂分叉不会导致静默漏单。

### 阶段 E：提现执行多链化

目标：把提现从“能调用链上合约”升级为“多链可控执行链路”。

必须完成：

1. 将提现 signer 从“单全局私钥”升级为“每链独立 signer 配置”。
2. 为每条链提供：
   - signer ref
   - signer address
   - nonce 维护
   - gas 策略
3. 保证 `withdraw_id` 到链上 `bytes32 withdrawId` 的编码是可逆的、稳定的。
4. 为广播失败、链上失败、orphan broadcast 保留完整补偿链路。
5. 增加链级 pause / 熔断能力。
6. 提现执行前检查：
   - chain health
   - vault balance
   - signer 可用性
   - address 合法性

重点文件：

- [backend/internal/infra/chain/vault_withdraw_executor.go](/Users/xiaobao/RGPerp/backend/internal/infra/chain/vault_withdraw_executor.go)
- 提现 worker / admin 审核相关服务

验收标准：

- 不同链提现由不同 signer 执行。
- 广播成功后本地失败时可被 Indexer 补录。
- `withdraw_id` 在链上链下可一一映射。

### 阶段 F：资金归集与 gas 补给

目标：新增多链 Treasury 能力，不再只停留在充值入账和提现完成。

必须完成：

1. 新增 `treasury` 或 `sweep` worker。
2. 至少支持以下流程：
   - Vault 资金从 hot 归集到 warm
   - warm 补给 hot
   - warm 向 cold 归档
3. 账务必须符合 [账本分录模板与资金状态机规范.md](/Users/xiaobao/RGPerp/docs/账本分录模板与资金状态机规范.md) 中的：
   - `CUSTODY_HOT`
   - `CUSTODY_WARM`
   - `CUSTODY_COLD`
   - `SWEEP_IN_TRANSIT`
4. 每条链配置：
   - hot 阈值
   - warm 阈值
   - cold 归档策略
   - gas top-up 阈值
5. gas 补给必须可审计。
6. 归集任务失败必须进入异常队列。

建议新增：

- `backend/cmd/treasury-worker/main.go`
- `backend/internal/domain/treasury/...`
- `backend/internal/infra/chain/...` 中的 sweep executor

验收标准：

- 充值入账后可执行 sweep。
- Ledger 中可查询归集分录。
- 失败重试不会重复记账。

### 阶段 G：多链对账与异常治理

目标：补齐生产必须的运营闭环。

必须完成：

1. 新增或补齐以下对账任务：
   - Router -> Vault
   - Vault balance -> custody mirror account
   - withdraw broadcast -> chain receipt
   - credited deposit -> chain event
2. 每条链生成独立对账结果。
3. 对账异常必须落库、发 outbox、可追踪。
4. 增加链级 anomaly 类型，例如：
   - unknown_router
   - unknown_token
   - unknown_vault
   - deposit_reorg_after_credit
   - withdraw_stuck_signing
   - rpc_failover_triggered
5. 提供最小化的管理/修复入口或脚本。

验收标准：

- 可按链查看对账异常。
- 异常不会吞掉。
- 可对单笔链上事件进行重放或修复。

### 阶段 H：测试、脚本与运行手册

目标：保证新增多链能力可持续回归。

必须完成：

1. 补单元测试。
2. 补集成测试。
3. 补本地 smoke test。
4. 补至少以下场景用例：
   - 三条链分别充值
   - 同一用户三链都有充值地址
   - 达确认数前不可用
   - 重启后继续扫块
   - 重复事件不重复落账
   - receipt 丢失与恢复
   - orphan broadcast 回补
   - withdraw fail -> refund
   - 本地链重建后游标重置
   - sweep 重试不重复记账
5. 更新运行文档和联调文档。

建议补充：

- `deploy/scripts/smoke-test-multichain.sh`
- `deploy/scripts/reconcile-multichain.sh`
- `docs/多链联调测试手册.md`

验收标准：

- `go test ./...` 通过。
- 合约测试通过。
- 本地多链 smoke test 一键通过。

## 9. 数据与表结构要求

AI 在实施时必须检查并补齐以下数据模型能力：

1. `deposit_addresses`
   - 唯一键必须覆盖 `user_id + chain_id + asset`
2. `deposit_chain_txs`
   - 唯一键必须覆盖 `chain_id + tx_hash + log_index`
3. `withdraw_requests`
   - 必须明确记录 `chain_id`、`broadcast_tx_hash`、链路状态
4. `chain_cursors`
   - 至少按 `chain_id + cursor_type` 唯一
5. 若新增归集能力，需要新增：
   - `sweep_requests`
   - `sweep_chain_txs`
   - 或等效表结构
6. 若新增对账结果存储，需要新增：
   - `reconciliation_runs`
   - `reconciliation_items`
   - 或等效表结构

所有表变更必须通过 migration 落地，不要直接手工改库。

## 10. 推荐实施顺序

AI 必须按以下顺序执行：

1. 阶段 A：配置模型升级
2. 阶段 B：多链本地部署能力
3. 阶段 C：充值地址分配正式化
4. 阶段 D：Indexer 多链生产化
5. 阶段 E：提现执行多链化
6. 阶段 F：资金归集与 gas 补给
7. 阶段 G：对账与异常治理
8. 阶段 H：测试、脚本、文档

禁止跳过 A 和 B 直接做 D。因为没有多链配置和多链本地部署，后续实现无法可靠验收。

## 11. AI 执行时的具体要求

AI 在实际编码时，必须遵守：

1. 优先使用现有目录结构，不要随意发明新的分层。
2. 新增 worker 时，遵守现有 `cmd/*/main.go` 风格。
3. 新增 infra 适配器时，放在 `backend/internal/infra/chain` 或 `backend/internal/infra/db`。
4. 新增领域逻辑时，放在 `backend/internal/domain/*`。
5. 新增配置必须补：
   - 配置加载
   - 校验
   - env 模板
   - 运行文档
6. 新增脚本必须可重复执行。
7. 所有涉及资金状态迁移的逻辑必须补单元测试。
8. 所有链上交互必须考虑：
   - timeout
   - retry
   - idempotency
   - failure compensation

## 12. 关键风险提示

AI 实施时必须特别注意以下风险：

1. 当前链上 `WithdrawExecuted.withdrawId` 是 `bytes32`，链下 `withdraw_id` 是字符串。必须保证编码可逆。
2. 当前仓库的多链支持骨架存在，但运维级高可用还不完整，不要误判为“已经生产可用”。
3. 本地 Anvil 多链和真实测试网多链不能混用同一套链标识语义，需提前规范。
4. 归集不是简单转账，必须同时写账本并可对账。
5. 节点故障和链重组不能靠“重启试试”处理，必须有明确补偿逻辑。

## 13. 完工定义

只有同时满足以下条件，才算本次实施完成：

1. 三条链都能完成充值地址分配。
2. 三条链都能完成充值检测、确认入账、提现广播、提现确认。
3. 三条链都能完成至少一条归集路径。
4. 服务重启后不会丢失已处理链上事实。
5. 异常事件可查、可补、不可静默丢失。
6. 本地多链 smoke test 通过。
7. `go test ./...` 和合约测试通过。
8. env 模板、部署脚本、运行文档齐全。

## 14. 建议 AI 的最终输出格式

AI 完成实施后，应输出：

1. 变更摘要
2. 新增/修改的核心文件列表
3. 多链启动步骤
4. 本地多链联调步骤
5. 风险与剩余待办
6. 测试结果

不要只给“代码已完成”的笼统回答。

## 15. 可直接给 AI 的执行指令

下面这段文字可以直接作为后续执行提示词的一部分交给 AI：

```text
请严格按照《多链充值提现与归集实施执行文档（AI 版）》执行，不要重新定义范围。先阅读文档中列出的所有规范文档，再检查当前仓库已有实现。按阶段 A 到 H 顺序完成多链配置、多链本地部署、多链充值地址分配、多链 Indexer、多链提现执行、资金归集、对账治理、测试与运行文档。实施过程中必须遵守账本模型、幂等、补偿、重组处理和组件边界，禁止绕过账本直接修改余额。完成后给出变更摘要、运行步骤、测试结果和剩余风险。
```
