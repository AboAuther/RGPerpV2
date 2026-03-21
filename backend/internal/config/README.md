# Config Package

本目录负责：

- 启动时静态配置加载
- 配置校验
- 动态配置快照拉取
- 配置访问接口

推荐入口：

- `LoadStaticConfig()`
- `LoadStaticConfigWithOptions()`
- `LoadRuntimeConfigSnapshot()`

静态配置加载顺序：

1. 代码默认值
2. `deploy/env/common.env`
3. `deploy/env/<APP_ENV>.env`
4. 进程级环境变量覆盖

运行时默认配置文件：

- `deploy/config/runtime/dev.yaml`
- `deploy/config/runtime/review.yaml`
- `deploy/config/runtime/staging.yaml`
- `deploy/config/runtime/prod.yaml.example`
