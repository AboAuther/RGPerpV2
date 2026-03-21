# Backend

后端技术栈：

- Go
- Gin
- GORM
- MySQL
- Redis
- RabbitMQ

## 目录约定

- `cmd/`: 各可执行进程入口
- `internal/domain`: 领域模块
- `internal/transport`: HTTP / WS / MQ 适配层
- `internal/infra`: 数据库、缓存、链上、外部连接器
- `internal/config`: 配置加载
- `internal/pkg`: 通用基础库

核心实现必须遵守：

- [账本分录模板与资金状态机规范](/Users/xiaobao/RGPerp/docs/账本分录模板与资金状态机规范.md)
- [风险计算与清算公式规范](/Users/xiaobao/RGPerp/docs/风险计算与清算公式规范.md)
- [服务间事务、幂等与补偿规范](/Users/xiaobao/RGPerp/docs/服务间事务、幂等与补偿规范.md)
