# Frontend

前端技术栈：

- React
- Vite
- TypeScript
- Ant Design
- pnpm

## 目录约定

- `src/app`: app shell、router、providers
- `src/pages`: 页面级入口
- `src/features`: 业务模块
- `src/shared/api`: HTTP client 与 DTO
- `src/shared/components`: 通用组件
- `src/shared/types`: 全局类型

前端实施必须遵守：

- [前端页面信息架构与交互状态规范](/Users/xiaobao/RGPerp/docs/前端页面信息架构与交互状态规范.md)

## 本阶段实现约束

- 当前优先交付 `spec/TASKS.md` 里里程碑 2 的前端范围：登录、会话保持、账户概览、充值、提现
- 真实接口未覆盖部分，前端使用 `mock` / `auto` provider 兜底，但所有状态名称遵循文档规范
- access token 仅保存在内存；仅 `mock` 会话快照允许保存到 `sessionStorage`

## 命令

```bash
pnpm install
pnpm --dir frontend dev
pnpm --dir frontend build
```
