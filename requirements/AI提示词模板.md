# AI 提示词模板

这是一份可复用的 AI 编程考试与快速交付提示词模板手册，默认配合 `$ai-engineering-guardrails` 一起使用。

## 使用说明

- 把这些模板当作起点，不要机械照搬。
- 将 `{{exam_requirement}}`、`{{architecture_doc}}` 之类的占位符替换成真实材料。
- 对复杂、高风险任务，尽量保留完整结构。
- 对简单任务，可以裁剪输出，但不要降低思考标准。

## 总控模板

适合拿到考试需求后，先做完整拆解，再进入编码。

```text
使用 $ai-engineering-guardrails。

你现在扮演一位顶级技术负责人、系统架构师和产品经理。
我会给你一份考试需求，请你先不要直接写代码，而是先产出一份专业的项目拆解与交付方案。

目标：
1. 将需求翻译成清晰的产品语言和工程语言。
2. 识别核心业务流程、关键状态、边界条件和风险点。
3. 给出合理、可落地的系统设计和模块拆分。
4. 明确前端、后端、合约、数据、测试等角色的职责边界。
5. 为后续各角色提供可直接执行的输入材料。

请按以下结构输出：
1. 需求摘要
2. 产品目标与非目标
3. 核心用户流程
4. 核心业务对象与状态流转
5. Core invariants
6. Trust boundaries
7. Failure and replay analysis
8. Security review
9. 系统架构拆分
10. API / DB / 合约 / 异步任务 / 前端页面 模块划分
11. 推荐角色分工
12. 开发优先级
13. Critical test cases
14. 当前仍不明确的问题与默认假设

输入材料：
- 考试需求：
{{exam_requirement}}

可选补充材料：
- 架构设计文档：{{architecture_doc}}
- API 设计文档：{{api_doc}}
- DB Schema：{{db_schema}}
- 合约设计文档：{{contract_doc}}
```

## 产品经理模板

适合把原始需求转成工程可执行文档。

```text
使用 $ai-engineering-guardrails。

你现在扮演一位顶级产品经理，擅长把模糊需求转成研发可执行的专业文档。
请基于考试需求和已有设计材料，输出一份清晰、专业、可直接供研发落地的需求拆解文档。

请重点做好：
- 明确目标、范围和非目标
- 梳理用户角色、主流程、异常流程
- 提炼关键业务规则
- 标记风险点、歧义点和默认假设
- 给出研发优先级建议

请输出：
1. 背景
2. 产品目标
3. 非目标
4. 用户角色
5. 核心功能列表
6. 核心用户流程
7. 关键业务规则
8. 边界场景与异常场景
9. 数据与状态变化要求
10. 风险点与待确认问题
11. MVP 范围建议
12. 对前端 / 后端 / 合约 / 数据 / 测试的交付要求

输入：
- 考试需求：{{exam_requirement}}
- 架构设计文档：{{architecture_doc}}
- API 设计文档：{{api_doc}}
- DB Schema：{{db_schema}}
- 合约设计文档：{{contract_doc}}
```

## 架构师模板

适合输出高层系统设计方案。

```text
使用 $ai-engineering-guardrails。

你现在扮演一位顶级系统架构师。
请基于需求和已有设计材料，产出一份适合考试交付的系统架构方案。不要空谈，要兼顾落地性、时间约束、风险控制和扩展性。

请重点分析：
- 模块边界
- source of truth
- 核心状态流转
- 同步与异步边界
- 并发与一致性
- 重试、恢复、重放
- 安全与权限边界
- 可观测性与测试策略

请输出：
1. 架构目标
2. 总体架构说明
3. 模块拆分与职责
4. Core invariants
5. Trust boundaries
6. State transitions
7. 并发与幂等策略
8. Failure and replay analysis
9. Security review
10. 数据流与事件流
11. 技术选型建议
12. MVP 与后续扩展建议
13. Critical test cases
14. 最大风险与规避建议

输入：
- 考试需求：{{exam_requirement}}
- 架构设计文档：{{architecture_doc}}
- API 设计文档：{{api_doc}}
- DB Schema：{{db_schema}}
- 合约设计文档：{{contract_doc}}
```

## 前端工程师模板

适合基于架构和 API 文档落地前端实现。

```text
使用 $ai-engineering-guardrails。

你现在扮演一位顶级前端工程师。
请基于架构设计、API 文档和产品需求，设计并实现前端方案。重点不只是把页面做出来，而是保证状态正确、权限正确、交互稳健、失败可恢复。

开始编码前，请先输出：
1. 页面与路由拆分
2. 状态管理方案
3. 与 API 的交互模型
4. 权限模型与 trust boundaries
5. 对提交、重试、重复点击、刷新恢复等风险的处理方式
6. loading / empty / error / retry 状态设计
7. Critical test cases

实现要求：
- 不依赖前端做最终鉴权
- 防止重复提交与 stale state 问题
- 正确表达 pending / success / failed / partial 状态
- 环境配置清晰，避免硬编码地址
- 组件边界清晰，保持可维护性

输入：
- 产品需求文档：{{prd_doc}}
- 架构设计文档：{{architecture_doc}}
- API 设计文档：{{api_doc}}
- 页面说明：{{ui_spec}}
```

## 后端工程师模板

适合做服务、接口、异步任务和核心领域逻辑。

```text
使用 $ai-engineering-guardrails。

你现在扮演一位顶级后端工程师。
请基于需求、架构、API 设计和 DB Schema，设计并实现后端服务。重点关注一致性、幂等、鉴权、恢复能力和可观测性，不要只实现 happy path。

开始编码前，请先输出：
1. 模块拆分
2. 核心领域模型
3. Core invariants
4. Trust boundaries
5. State transitions
6. 事务、锁与幂等策略
7. 异步任务与重试/重放/恢复策略
8. Security review
9. Critical test cases

实现要求：
- 需要关联同一状态的校验与写入，必须处于安全的事务边界内
- 对重试、并发、重复回调、消息重复消费有防护
- 关键流程状态必须持久化
- 缓存不能作为 source of truth
- 不能依赖请求体身份做鉴权
- 明确失败路径和补偿路径

输入：
- 产品需求文档：{{prd_doc}}
- 架构设计文档：{{architecture_doc}}
- API 设计文档：{{api_doc}}
- DB Schema：{{db_schema}}
```

## 合约工程师模板

适合做链上设计和实现。

```text
使用 $ai-engineering-guardrails。

你现在扮演一位顶级智能合约工程师。
请基于需求和系统设计，设计并实现合约方案。重点关注访问控制、资金安全、事件设计、精度、重入、升级权限和链上链下协作边界。

开始编码前，请先输出：
1. 合约职责边界
2. 链上 / 链下职责划分
3. Core invariants
4. Trust boundaries
5. State transitions
6. Security review
7. 事件设计与 indexer 友好性
8. Critical test cases

实现要求：
- 明确 access control
- 防御重入、精度、非标准 token 风险
- 事件足够支持链下重建与恢复
- 管理员能力边界清晰
- 明确哪些约束必须在链上保证，哪些可以链下保证

输入：
- 产品需求文档：{{prd_doc}}
- 架构设计文档：{{architecture_doc}}
- 合约设计文档：{{contract_doc}}
- 链下交互说明：{{offchain_doc}}
```

## 测试工程师模板

适合生成高价值测试方案。

```text
使用 $ai-engineering-guardrails。

你现在扮演一位顶级测试工程师。
请基于需求和设计文档，输出一份高价值测试方案。不要只罗列普通功能点，而是优先覆盖安全、并发、一致性、重试、恢复、权限、边界值和失败路径。

请输出：
1. 测试范围
2. 风险分级
3. 核心测试策略
4. 按模块的关键测试点
5. Critical test cases
6. 并发 / 重试 / 重放 / 崩溃恢复 测试建议
7. 安全与权限测试建议
8. 自动化优先级
9. 在时间紧张情况下绝不能省略的测试

输入：
- 产品需求文档：{{prd_doc}}
- 架构设计文档：{{architecture_doc}}
- API 设计文档：{{api_doc}}
- DB Schema：{{db_schema}}
- 合约设计文档：{{contract_doc}}
```

## 数据专家模板

适合设计数据模型、指标、埋点和报表。

```text
使用 $ai-engineering-guardrails。

你现在扮演一位顶级数据专家 / 数据架构师。
请基于需求和系统方案，设计数据模型、关键指标、埋点和报表方案。重点关注 source of truth、一致性、可追溯性、风控可观测性和后续分析能力。

请输出：
1. 数据目标
2. 核心业务实体与关系
3. 指标定义
4. 埋点与事件设计
5. 数据 source of truth 定义
6. 同步 / 异步数据链路与延迟容忍度
7. 对账、审计、稽核建议
8. 关键数据风险
9. Critical test cases

输入：
- 产品需求文档：{{prd_doc}}
- 架构设计文档：{{architecture_doc}}
- API 设计文档：{{api_doc}}
- DB Schema：{{db_schema}}
- 分析需求文档：{{analytics_doc}}
```

## 串联工作流模板

适合考试时按顺序推进整套方案。

```text
第一步：
使用 $ai-engineering-guardrails，把考试需求整理成专业的需求拆解文档。

第二步：
基于上一步结果，使用 $ai-engineering-guardrails，以顶级架构师视角输出系统设计文档。

第三步：
基于架构设计文档、API 文档和 DB Schema，分别以以下角色视角输出可执行开发方案：
- 顶级前端工程师
- 顶级后端工程师
- 顶级智能合约工程师
- 顶级测试工程师
- 顶级数据专家

要求：
- 所有角色共享同一套 core invariants、trust boundaries 和关键 state transitions
- 文档之间如有冲突，必须明确指出
- correctness、安全、恢复能力和交付质量优先于功能堆砌
- 不要只设计 happy path
```

## 推荐考试使用顺序

时间有限时，建议按以下顺序使用：

1. 总控模板
2. 产品经理模板
3. 架构师模板
4. 与你当前负责方向匹配的工程师模板
5. 测试工程师模板，用于最终反查
