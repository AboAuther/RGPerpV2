# Deploy

该目录用于：

- `compose/`: docker compose 配置
- `env/`: 静态启动配置文件
- `config/runtime/`: 运行时默认配置快照模板
- `sql/`: 初始化 SQL 与迁移辅助文件

当前统一采用两层配置：

- `deploy/env/common.env` + `deploy/env/<APP_ENV>.env`
- `deploy/config/runtime/<APP_ENV>.yaml`
