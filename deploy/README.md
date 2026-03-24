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

本地链节点与后端服务现在已经解耦。推荐先单独启动三条本地链并部署或复用合约：

```bash
bash deploy/scripts/bootstrap-local-multichain.sh
```

这个脚本会：

- 启动宿主机上的 3 条本地链：`ethereum / arbitrum / base`
- 如果 `deploy/env/local-chains.env` 中已有地址且链上合约代码仍存在，则复用现有合约
- 仅在链被重置或合约地址失效时重新部署
- 写出 `deploy/env/local-chains.env` 与前端 `.env.local`
- 本地充值确认数要求默认写为：`ETH=12 / ARB=20 / BASE=20`

然后再启动后端和依赖：

```bash
docker compose up -d --build
```

这会启动：

- MySQL、Redis、RabbitMQ
- `migrator`、`api-server`、`indexer`、`market-data`、`order-executor-worker`、`risk-engine-worker`、`funding-worker`、`liquidator-worker`、`hedger-worker`

如果需要启动前端，再执行：

```bash
sh deploy/scripts/start-frontend-local.sh
```

前端默认运行在 `http://127.0.0.1:5173`。

补充说明：

- 后端服务共用一个镜像：`rgperp-backend:local`
- `docker compose up` 不会再拉起或重置三条链
- 后端容器直接只读挂载宿主机上的 `deploy/env/local-chains.env`
- `api-server` 暴露在 `http://127.0.0.1:8080`

如果只想拉基础依赖，也可以使用：

```bash
docker compose up -d mysql mysql-init redis rabbitmq
```

## 手动挖块

本地充值后如果需要手动推进确认块，可以使用：

```bash
bash deploy/scripts/mine-local-blocks.sh eth 6
bash deploy/scripts/mine-local-blocks.sh arb 6
bash deploy/scripts/mine-local-blocks.sh base 6
```

参数说明：

- 第一个参数：`eth | arb | base`
- 第二个参数：要挖出的块数
