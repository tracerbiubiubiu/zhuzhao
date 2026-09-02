# Phase 3 · 运维工具（ops）

> **定位**：把「可重复、可验证、可回滚」的运维动作脚本化/CI 化：Swagger 同步检查、DB 迁移检查、集成测试自动化、一键部署。
> **Wave 归属**：W3（Step 6）｜ 配套文档：[ops/deployment.md](../ops/deployment.md)（B10，已补齐）。
> **状态**：**已编写（2026-09-02，规划/设计就绪）**。
> **标记约定**：`🚦` = 触发条件驱动；`⚠️` = 待拍板。

---

## 1. 能力总览

| 能力 | 说明 | 优先级 |
|---|---|---|
| Swagger CI | `make swag` + diff 检查，防止文档漂移 | 高 |
| 迁移 CI | 编号冲突 / up-down 成对 / 迁移可应用 / acceptance 联动 | 高 |
| 集成测试自动化 | `make test-integration`（-p 1）+ `make acceptance` 四档链式 | 高 |
| 一键部署 | Docker Compose 单命令拉起 + 迁移脚本化 | 高 |
| 部署文档 | [ops/deployment.md](../ops/deployment.md)（B10） | 已完成 |

---

## 2. Swagger CI

- 现状：`make swag` 重新生成 `docs/docs.go` / `swagger.json` / `swagger.yaml`（[swag](../phase1/README.md) 已接入）。
- CI 检查：`make swag && git diff --exit-code docs/` → 有漂移即失败（提示提交前重跑）。
- ⚠️ 生成器与 Go 版本绑定，CI 固定 Go 版本。

---

## 3. 迁移 CI

| 检查 | 方式 |
|---|---|
| up/down 成对 | 每个迁移必须有 `.up.sql` / `.down.sql`（脚本断言） |
| 编号全局唯一 + 无冲突 | 启动时按 A2 规则核对占用表（phase2/README §2.4） |
| 可应用 / 可回滚 | 空库 `make migrate-up` → `migrate-down` 全量演练 |
| 语法 | 迁移文件 SQL 语法检查（pg 解析或试跑） |
| 联动 | `make migrate-up` + `make acceptance` 作为门禁 |

---

## 4. 集成测试自动化

- `make test-integration`：13 包集成测试，`-p 1` 跨包串行（2026-09-01 已定，防 TRUNCATE 踩踏）。
- `make acceptance`：四档链式（phase1 → 2a → 2b → 2c），Phase 3 启动后追加 **3a/3b 脚本**（SLA/通知/审批流/分派/报表验收用例，见 [10-ticket-business §8](./10-ticket-business.md)）。
- 覆盖率：`make test-cover` 跟踪趋势。

---

## 5. 一键部署

```bash
# 开发/内网（Phase 3-min）
docker compose up -d            # app + postgres + redis
make migrate-up                 # 应用迁移（脚本化）
# 观测栈（可选，W1 后）
docker compose --profile observability up -d
```

- 配置：环境变量注入（非明文入库）；`.env.example` 模板。
- 健康检查：`/health/live` `/health/ready`（LB/编排依赖）。
- 多副本（W1 后）：`deploy.replicas` 或 compose scale + Nginx（见 [06-ha](./06-ha.md)）。

---

## 6. 涉及文件（规划）

```
Makefile                        # 扩展 swag-check / migrate-check / acceptance-3 目标
scripts/ci/                     # CI 脚本（swag diff / 迁移断言）
docker-compose.yml              # 一键部署
deploy/nginx/                   # 反代（可选）
docs/ops/deployment.md          # 部署文档（已补齐）
```

---

## 7. 验收用例

| # | 用例 | 通过标准 |
|---|---|---|
| OP1 | Swagger CI | 未重新生成时 CI 失败；生成后通过 |
| OP2 | 迁移 CI | 缺 down 文件 / 编号冲突 → CI 失败 |
| OP3 | 一键部署 | 空环境单命令拉起 + 迁移应用 + 健康检查通过 |
| OP4 | 集成回归 | `make acceptance` 四档全绿（Phase 3 追加后含 3 档） |

---

## 8. 待决策点（⚠️）

| # | 事项 | 建议 | 状态 |
|---|---|---|---|
| D1 | CI 平台 | GitHub Actions / 自建，按仓库托管选 | 待拍板 |
| D2 | acceptance 3 档脚本形式 | 沿用 2c 脚本模式追加 | 待拍板 |
| D3 | 一键部署是否含观测栈 | 默认不含（profile 可选） | 建议沿用 |

---

## 9. 变更记录

| 日期 | 说明 |
|---|---|
| 2026-09-02 | 已编写：Swagger CI / 迁移 CI / 集成自动化 / 一键部署 + 验收 + 待决策点 |
