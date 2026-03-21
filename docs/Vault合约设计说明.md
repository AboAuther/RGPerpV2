# Vault 合约设计说明

## 1. 文档目标

本文档定义链上托管合约的最小实现边界、事件模型、权限与安全要求。

链上合约的唯一职责是：

1. 托管用户资产；
2. 记录可审计的链上存取款事件；
3. 为链下系统提供可信的资产边界。

链上合约不负责：

- 订单；
- 仓位；
- 清算；
- 资金费率；
- 风险计算；
- 对冲执行。

## 2. P0 推荐合约组成

P0 推荐由以下合约组成：

1. `Vault`
2. `DepositRouter`
3. `DepositRouterFactory`

## 2.1 设计理由

题目要求每个用户分配独立充值地址。  
为同时满足：

- 独立充值地址；
- 统一托管；
- 可链上审计；
- 合约边界清晰；

推荐采用：

- 每个用户每条链一个独立 `DepositRouter`
- `DepositRouter` 收到 USDC 后转发至 `Vault`
- `Vault` 作为最终托管地址

这样既满足“独立地址”，又维持统一托管。

## 3. 合约职责

### 3.1 Vault

#### 职责

- 持有 USDC
- 仅接受白名单 Router 或运营账户的入金
- 执行角色控制下的提现
- 发出存取款事件
- 支持 pause

### 3.2 DepositRouter

#### 职责

- 作为用户独立充值地址
- 只接受白名单 USDC
- 收到资金后立即转发给 Vault
- 发出 `DepositForwarded` 事件

### 3.3 DepositRouterFactory

#### 职责

- 使用 CREATE2 为用户生成稳定地址
- 确保 `user_id + chain_id + salt` 唯一映射
- 记录 Router 创建事件

## 4. 资产与角色边界

### 4.1 支持资产

P0 仅支持 USDC。

每条链必须配置：

- `usdc_token`
- `vault_address`
- `router_factory_address`

### 4.2 角色

`Vault` 推荐角色：

- `DEFAULT_ADMIN_ROLE`
- `PAUSER_ROLE`
- `UNPAUSER_ROLE`
- `WITHDRAW_EXECUTOR_ROLE`
- `TREASURY_ROLE`
- `ROUTER_MANAGER_ROLE`

### 4.3 权限要求

- 提现执行仅允许 `WITHDRAW_EXECUTOR_ROLE`
- pause 仅允许专门角色
- Router 白名单管理仅允许 `ROUTER_MANAGER_ROLE`
- 管理员不得通过通用方法转出任意用户资产而不留事件

## 5. 合约接口

## 5.1 Vault 最小接口

```solidity
function withdraw(
    address token,
    address to,
    uint256 amount,
    bytes32 withdrawId
) external;

function pause() external;
function unpause() external;
function setRouterAllowed(address router, bool allowed) external;
function setTokenAllowed(address token, bool allowed) external;
function rescueToken(address token, address to, uint256 amount, bytes32 rescueId) external;
```

## 5.2 DepositRouter 最小接口

```solidity
function forward() external;
function forwardToken(address token) external;
function sweepNative(address to) external;
```

说明：

- Router 默认只处理 USDC；
- 如收到其他 token，不自动入账，只允许运营清理；
- `sweepNative` 仅用于回收误转 gas token。

## 5.3 Factory 最小接口

```solidity
function createRouter(uint256 userId, bytes32 salt) external returns (address router);
function predictRouter(uint256 userId, bytes32 salt) external view returns (address router);
```

## 6. 事件定义

## 6.1 Vault 事件

```solidity
event WithdrawExecuted(
    bytes32 indexed withdrawId,
    address indexed token,
    address indexed to,
    uint256 amount,
    address operator
);

event RouterAllowedUpdated(address indexed router, bool allowed, address operator);
event TokenAllowedUpdated(address indexed token, bool allowed, address operator);
event Paused(address operator);
event Unpaused(address operator);
event RescueExecuted(bytes32 indexed rescueId, address indexed token, address indexed to, uint256 amount, address operator);
```

## 6.2 DepositRouter 事件

```solidity
event DepositForwarded(
    uint256 indexed userId,
    address indexed token,
    uint256 amount,
    address indexed from,
    address vault
);
```

## 6.3 Factory 事件

```solidity
event RouterCreated(uint256 indexed userId, address indexed router, bytes32 indexed salt);
```

## 7. 关键安全要求

### 7.1 Reentrancy

- `Vault.withdraw` 必须 `nonReentrant`
- Router 的 `forwardToken` 也应 `nonReentrant`

### 7.2 Token 白名单

- 只允许白名单 token
- 禁止任意 token 直接作为存款资产

### 7.3 Router 白名单

- Vault 仅接受白名单 Router 路径的存款事件认定
- 未白名单 Router 的转账不得自动认定为用户充值

### 7.4 Pause

以下场景必须可 pause：

- 提现异常
- 权限泄漏
- 代币合约异常
- 发现索引与链上不一致扩散

### 7.5 Rescue

`rescueToken` 仅用于：

- 误转非支持 token
- 运维恢复

并要求：

- 不得用于白名单 USDC 正常用户资金迁移
- 必须发出审计事件
- 链下必须同步人工审批和审计记录

## 8. 存款路径规范

## 8.1 标准路径

1. 后端为用户分配 Router 地址
2. 用户将 USDC 转入 Router
3. Router 立即把 USDC 转入 Vault
4. Router 发出 `DepositForwarded`
5. Indexer 监听 Router 事件和 token transfer
6. 达到确认数后链下入账

## 8.2 为什么不直接由 Vault 识别用户

因为题目要求每用户独立充值地址。  
若直接往 Vault 存款，则链上无法天然区分用户来源，除非附加 memo，而 ERC20 transfer 不支持 memo。  
因此使用 Router 是更稳妥的实现。

## 9. 提现路径规范

1. 用户在链下提交提现申请
2. 风控审核通过
3. 链下生成 `withdrawId`
4. 受控执行器调用 `Vault.withdraw`
5. Vault 发出 `WithdrawExecuted`
6. Indexer 监听事件并更新提现链路状态

## 10. 多链部署规范

每条链独立部署：

- `Vault`
- `DepositRouterFactory`
- Router 实例按需创建

链配置项：

- chain_id
- rpc_url
- finality_confirmations
- usdc_token
- vault_address
- factory_address

## 11. 升级策略

P0 推荐：

- 合约不做可升级代理
- 采用不可升级、最小接口、最小权限模型

原因：

- 降低代理升级带来的权限和存储布局风险
- 提高审计清晰度
- 适合评审环境与短周期交付

若必须升级，应通过新合约迁移与后台配置切换，不建议首期引入代理升级。

## 12. Foundry 测试清单

必须覆盖：

- Router 创建地址可预测
- Router 仅转发白名单 token
- Router 存款成功转发到 Vault
- Vault 仅允许有权限账户提现
- Vault pause 后提现失败
- Vault 非白名单 token 操作失败
- Reentrancy 防护
- 事件字段正确
- rescue 行为受限

## 13. 链下索引要求

链下必须同时记录：

- Router 创建事件
- DepositForwarded 事件
- WithdrawExecuted 事件
- 相关 token Transfer 日志

以便：

- 审计 Router -> Vault 的资金链
- 对账链上资产
- 补录孤儿事件

## 14. 禁止事项

- 禁止链上维护订单和仓位状态
- 禁止合约内引入复杂业务逻辑
- 禁止将管理员提现与用户提现共用不可审计入口
- 禁止没有链下审批与链上事件映射的资金迁移
