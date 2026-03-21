# Config Package

本目录负责：

- 启动时静态配置加载
- 配置校验
- 动态配置快照拉取
- 配置访问接口

推荐入口：

- `LoadStaticConfig()`
- `ValidateStaticConfig()`
- `LoadRuntimeConfigSnapshot()`
