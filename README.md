# zhuzhao

Go 模块化单体，提供 IAM、三层鉴权、工单和统一网关；UI、taskrunner、activelist 独立维护与部署，utils 为公共库。当前能力与后续方案请从 [文档入口](docs/README.md) 和 [项目控制台](docs/review/11-project-control.md) 区分。

## 开始使用

先按 [开发指南](docs/guides/development.md) 准备 SDK、确认环境归属，再执行：

```sh
make hooks
make docker-dev-up
make migrate-up
make dev
```

以上只启动主仓依赖和主应用。UI / 上游服务、生产与测试 Compose、密钥和显式迁移顺序见 [部署指南](docs/ops/deployment.md)；测试账号唯一台账与故障处理见 [运行手册](docs/ops/runbook.md)。

## 开发与交付

修改遵循 [工程公约](docs/standards.md) 与根 [协作协议](AGENTS.md)，同步模块文档和控制台。常用检查为 `make lint`、`make test-unit`、`make test-integration`、`make test-cover`、`make acceptance`；生成用 `make wire` / `make swag`。完整前置、环境隔离和失败口径见开发指南。

五仓文档各放本仓 `docs/`，结构与维护约定见 [文档规则](docs/documentation.md)。正式报告在 `docs/review/reports/`，本地参考、执行卡和原始证据不进入 Git / Docker 上下文。
