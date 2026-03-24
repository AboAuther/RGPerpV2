# Deploy

该目录用于：

- `compose/`: docker compose 配置
- `env/`: 静态启动配置文件
- `config/runtime/`: 运行时默认配置快照模板
- `sql/`: 初始化 SQL 与迁移辅助文件

当前统一采用两层配置：

- `deploy/env/common.env` + `deploy/env/<APP_ENV>.env`
- `deploy/config/runtime/<APP_ENV>.yaml`

## Docker Compose

本地如需一键拉起完整环境，现在可以直接执行：

```bash
docker compose up -d --build
```

这会同时启动：

- 3 条本地链：`anvil-ethereum`、`anvil-arbitrum`、`anvil-base`
- 链上合约 bootstrap：`chain-bootstrap`
- MySQL、Redis、RabbitMQ
- `migrator`、`api-server`、`indexer`、`market-data`、`order-executor-worker`、`risk-engine-worker`、`funding-worker`、`liquidator-worker`

补充说明：

- 后端服务共用一个镜像：`rgperp-backend:local`
- 链节点与合约部署共用一个镜像：`rgperp-foundry:local`
- `chain-bootstrap` 会把运行时链配置写到 compose volume 中的 `/shared/local-chains.env`，后端容器统一从这里读取
- 宿主机仍可通过 `http://127.0.0.1:8545/8546/8547` 访问三条本地链
- `api-server` 暴露在 `http://127.0.0.1:8080`

如果你还是想走“宿主机进程 + 本地脚本”模式，再使用：

```bash
bash deploy/scripts/bootstrap-local-multichain.sh
docker compose up -d mysql mysql-init redis rabbitmq
```
