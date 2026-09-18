# zhuzhao

> **定位（2026-09-03，§23 重定位后）**：IAM 内核 + 统一网关 + 通用能力底座（事件/审计/组织/HR）。工单模块已封版（对接公司内部平台，重启条件见 [design-decisions §23](docs/design/design-decisions.md)）；taskrunner/activelist 为独立部署服务，经 zhuzhao 网关接入（权限架构定版见 §25）。
> 仓库名与 module path 的正式命名合并到内网迁移时执行（M-Mig，[13 号 §1](docs/phase3/13-implementation-plan.md)）——现只注定位不改库。

zhuzhao 是 zhuzhao 生态的主仓库：Go（Gin + PostgreSQL + Casbin + Redis + Wire）模块化单体，对外是生态唯一入口（统一网关），对内承载 IAM 与通用能力。

## 核心能力

| 域 | 能力 |
|---|---|
| IAM | 用户 / 角色 / 组织 / 菜单 / 认证（AT+RT 双令牌、登录失败锁定）/ 请求级与业务审计 |
| 三层鉴权 | L1 路由级（Casbin）→ L2 数据范围（内置策略库 + Resource 注册表）→ L3 动作矩阵 |
| 统一网关 | `/al/*` 反代 activelist（AK/SK 出站签名 + X-Operator 身份断言）、`/api/v1/tasks|jobs|runs` 任务管理代理（E-④） |
| 内网回调 | `POST /internal/jobs/callback`（AK/SK 验签——taskrunner 任务执行入口，幂等栅栏 + 动作注册表） |
| 可观测 | 结构化访问日志（operator / auth / caller 归因）、审计归档（超期导出 JSONL） |

## 生态

| 项目 | 角色 |
|---|---|
| [zhuzhao-utils](https://github.com/tracerbiubiubiu/zhuzhao-utils) | 公共库 10 包（crypto / errcode / jwt / logger / aksk / response / postgres / redis / jsonutil / validate），当前 v0.4.1 |
| [taskrunner](https://github.com/tracerbiubiubiu/taskrunner) | 事件/任务总线：调度、重试、死信、`job_runs` 记录（与 zhuzhao AK/SK 互信） |
| [activelist](https://github.com/tracerbiubiubiu/activelist) | 动态数据模型薄层：类型注册 / Schema 演进 / 数据 CRUD / 导入导出 |

生态工程公约（API 设计 / 目录结构 / 迁移 / 安全基线 / 测试）SSOT：[docs/standards.md](docs/standards.md)。

## 快速开始

```sh
# 演示栈（app + PG + Redis + pgbackup 侧车；演示账号 E000001 / Phase3Demo#2026，详见 runbook）
docker compose -f deployments/docker-compose.yaml up -d --build
curl http://localhost:33333/health/ready

# 本地开发栈（仅 PG/Redis 容器，应用进程 go run）
docker compose -f deployments/docker-compose.dev.yaml up -d
make dev
```

- 端点：`/api/v1/*`（业务 API，JWT）、`/al/*`（activelist 反代）、`/internal/jobs/callback`（内网回调，默认关）、`/health/live`、`/health/ready`；
- 运维两套栈的区分、env 注入与常见故障：[docs/ops/runbook.md](docs/ops/runbook.md)。

## 开发与门禁

```sh
make lint              # go vet + gofmt 漂移检查
make test-unit         # 单元测试（-race -count=1）
make test-integration  # 集成测试（testcontainers 真 PG，-tags integration，-p 1）
make test-cover        # 覆盖率报告
make acceptance        # 【门禁】四档验收链 phase1 → 2a → 2b → 2c（须标准环境，见 runbook §3）
make migrate-up        # 应用迁移（up/down 成对，编号全局唯一）
make swag              # 重新生成 Swagger（docs/docs.go）
make guard             # 架构守护测试（go/ast 分层/命名静态断言）
```

CI（GitHub Actions）跑最小质量网（vet + gofmt + build + 单测 race）；集成与四档 acceptance 按纪律在本地标准环境执行。

## 文档

[docs/README.md](docs/README.md) 为文档树索引（modules / phase1-3 / review / adr / design / ops）。重点：

- [docs/standards.md](docs/standards.md) —— 生态工程公约 SSOT（API 设计 / 工程结构 / 数据迁移 / 安全基线 / 测试约定）
- [docs/design/design-decisions.md](docs/design/design-decisions.md) —— 演进中的设计拍板（权限架构 §25 / 工单封版 §23）
- [docs/phase3/16-external-integration.md](docs/phase3/16-external-integration.md) —— 外部集成 SSOT（16 号：基线 §9 / taskrunner 对齐清单）
- [docs/review/11-project-control.md](docs/review/11-project-control.md) —— 能力总览（能力矩阵 / 健康状态 / 遗留问题）
- [docs/api/errcode.md](docs/api/errcode.md) —— 业务错误码登记表
- [docs/ops/runbook.md](docs/ops/runbook.md) —— 运维手册（两套栈速查 / 故障处置 / 故障演练复现）

## License

[MIT](LICENSE)
