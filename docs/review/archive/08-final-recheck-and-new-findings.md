# 08 · 文档审查终审：收口复验与新发现

> **定位**：`06-phase2-docs-architecture-review.md` 与 `07-comprehensive-docs-review.md` 已完成九维全量审查；本文不重复其结论与评级，仅做两件事：
> **(A)** 对两份报告的全部发现项，基于当前 HEAD `ebb5635` 逐项复核当前状态（含本日 `1804325` 模板预填 / `75e7522` 验收脚本修复 / `ebb5635` 文档回标三个提交带来的变化）；
> **(B)** 登记前序报告未覆盖的**新发现**（来源：docs/proposal、adr、phase3、工程化配置的深度扫描，及本日 88+65 用例端到端验收实证），并给出合并后的修复路线图。
>
> 审查日期：2026-08-27　|　方法：两路并行深扫（文档全景一致性 / 工程化配置）+ 会话内代码实证交叉验证
> 审查视角：资深后端开发 / 系统架构设计 / 微服务架构师（与前序报告一致）

---

## A. 前序发现项复核矩阵（基于 HEAD ebb5635）

### A.1 七号报告 P0 批次（六项）

| # | 问题 | 原级别 | 当前状态 | 说明 |
|---|------|--------|---------|------|
| C1 | `09-ticket §0 L3` 2c Step 编号 | P0 | ✅ **已修**（ebb5635） | 门禁表/任务标题/SSOT 映射/P2-D4 四处统一为 8–10 |
| C2 | `02-authz §2.5 L218` 自注册示例旧签名 | P0 | ❌ **未修，且偏差加深** | 示例仍写 `NewTicketResource(s.repo, s.scope)`；实现现为 `NewResource(repo)`（包内私有构造 + 无 scope 参数，scope_resolver 空壳已删）。**示例中类名、函数名、参数三处均失真** |
| N4 | `README §2.4 L242` 000013 表述 | P0 | ❌ 未修 | 迁移实建 `file_objects` + `ticket_attachments` 两表（10-storage §2 DDL 为证），文档仅列 `ticket_attachments` |
| N5 | `VISION L44`「migrations 现状至 000009」 | P0 | ❌ 未修 | 且叠加更严重的 V-1（见 B.1），建议同批处理 |
| N2 | `roadmap L98` 引用 `doc/soar/activelist.md` | P0 | ❌ 未修 | 路径不存在（`.gitignore` 忽略 `doc/` 且仓库无此文件），确认真断链；ADR-003 引用正常可保留 |
| — | Makefile migrate DSN 硬编码 | P0 | ❌ 未修 | `Makefile:26,29` 明文 `zhuzhao:zhuzhao_dev`；建议 `MIGRATE_DSN ?= ...` 外置 |

### A.2 七号报告高风险项复核

| 项 | 原判级 | 复核结论 |
|----|--------|---------|
| CORS AllowAllOrigins | 高 | **改判为中 + 转轨管理方式**。`middleware/cors.go:9-14` 属 Phase 1 README §4.2 **明确拍板的决策**（DefaultConfig + AllowAllOrigins，「生产上线前再改域名白名单」），且未开 credentials，非疏漏而是既定边界。问题在于该承诺**游离在管控之外**：不在 phase3 启动条件触发表、不在 ops 上线检查单中。建议登记为触发条件而非现在整改 |
| Phase 1 服务层测试盲区 | 高 | **实质性缓解，降级**。现状：service 包 14 个测试文件（stubRoleFetcher 单测 + d2/b4/rbac/ticket 集成套件），`go test -race -tags=integration ./internal/...` 12 包全绿（本日实测）；service 行为由 testcontainers 真表覆盖强于纯 mock 单测。剩余缺口已收敛进 BK 清单（BK-1/3/5/6 自动化） |

### A.3 其余发现项状态速览

| 来源 | 发现项 | 状态 |
|------|--------|------|
| 06 号 | scope_resolver 描述/路径不一致 | ✅ 已修（M-10/M-5，ebb5635 删空壳 + 路径回标） |
| 07 号 | BK-1~BK-10 「只在对话记忆」风险 | ✅ 已固化登记（00-implementation-plan §9） |
| 05 号 G-2（authz_service 残留） | 计划文档仍以「待删除」时态描述 | ⚠️ 部分未勾选：00 Step 0 checkbox 未回勾、README:148 措辞滞后（G-1/G-2 实际已完成于 39dd355） |
| 07 号 T2（setupTicket2a 死代码） | BK-9 已登记 | ✅ 有归属 |
| 14 号悬空引用（两个 13-、review/05/07 源文件） | 已删文件的历史描述 | ℹ️ 该文档自我声明保留（档案性质），无需处理 |

---

## B. 新发现（前序报告未覆盖）

### B.1 项目目标与能力边界

| # | 发现 | 位置 | 级别 | 说明与建议 |
|---|------|------|------|-----------|
| **V-1** | **VISION 把未交付能力写入「Phase 1，已完成」**：「Casbin RBAC + 资源级鉴权（ResourceRegistry）」（实际 ResourceRegistry 为 2a 交付）、「ltree 组织树（**含虚拟组 / HR 同步**）」（虚拟组 = 2b-org、HR 同步 = 2b-ext 延后） | `docs/VISION.md:10` | 🔴 **高** | VISION 是自我声明的「范围判定唯一锚点」（L3），锚点自身失真会沿修改纪律四步传导。07 号只抓到 L44 的版本号过期，未发现 L10 的阶段归属错误。**建议**：改为「认证（双 Token/RT 轮换）、路由级 Casbin RBAC、ltree 组织树（实体树）、操作审计…」并把资源级鉴权/虚拟组/HR 同步移入工单行的前置引用 |

### B.2 文档一致性与演进叙事

| # | 发现 | 位置 | 级别 | 建议 |
|---|------|------|------|------|
| **A-1** | **architecture.md 同文件内 L2/L3 顺序两说**：§4.1 执行顺序仍写「第三层属主（短路）→ 第二层 ltree」，而 §4.4（L637）已加注路径 A「属主判断（L3）不短路：先过 L2 可见性」。`design-decisions.md:188` 与 `11-authz §2` 拍板的是后者 | `architecture.md:558 vs :637` | 🟠 中-P1 | 路径 A 是 2026-08-26 用户拍板的安全语义（合取公式 Q1），同一 SSOT 内两说比跨文档不一致更危险——实现者读 §4.1 会写出先 canOperate 后可见性的越权宽容顺序。按 11 号待办 #2 收口即可 |
| R-1 | roadmap 与 docs/README 未跟进 P2-D7 三轨细分：仍写「2a → 2b → 2c」三分法且 HR/附件/密码策略整包挂在 2b 名下（未标 2b-ext 延后） | `roadmap.md:55-60`、`docs/README.md:148` | 🟠 中 | 执行视图层滞后于计划拍板（2026-08-26）。补三行子阶段 + 延后标注 |
| H-1 | hr-directory-sync 头部仍写「Schema 与虚拟组同属 Phase 2b」，未见降级 2b-ext 标注 | `proposal/hr-directory-sync.md:5,206-213,390` | 🟠 中 | 与 phase2 计划侧（README:17、00:81）单向记录，原文档无回标；遵循「偏差必须同 PR 修文档」纪律补一行 |
| D-1 | deployment-evolution 全篇以「Phase 3 = IAM 独立部署 + gRPC/PDP」为叙事，未回标 2026-08-25「微服务拆分推迟、Phase 3 整体暂缓」总决策 | `proposal/deployment-evolution.md:87-91,164-229` | 🟡 低-中 | 仅 §4 一句旧的自纠（拆服务归 Phase 3）；phase3/README:43 反向解释了暂缓但双向未闭合。头部加一段「2026-08-25 起 Phase 3 整体暂缓，本文为演进预案非排期承诺」 |
| O-1 | proposal/overview 架构全景仍画 IAM 独立部署三层拓扑、定位句窄于 VISION（「通用办公管理后台的认证鉴权底座」vs「IAM+工单统一平台」） | `overview.md:16,82-90` | 🟡 低 | 头部已有「冲突以 phase 为准」兜底；补 VISION 对齐即可 |
| M-1 | modules/ticket.md 两处引用**不存在的** `phase3/02-multi-instance.md` 当可用方案（分布式锁防重复消费） | `modules/ticket.md:625,729` | 🟡 低 | 改为「Phase 3 事件机制启动时落锁方案（见 ADR-001 L1 稳态）」 |

### B.3 安全性（新发现）

| # | 发现 | 位置 | 级别 | 说明 |
|---|------|------|------|------|
| S-1 | **Redis 无密码承诺到期未兑现**：redis.conf 注释自述「Phase 2 加密码」，现处 Phase 2；dev compose 又将 6379 暴露宿主机。应用侧影响有限（fail-close 键族由 noeviction 保护），但登录限流计数器可被外部 FLUSHALL 绕过 | `deployments/redis/redis.conf:7-8`、docker-compose.dev.yaml:31-33 | 🟠 **中偏高（整改急迫）** | 与 CORS 不同，这是一句**带时间的承诺**而非开放决策。启用 requirepass + `REDIS_PASSWORD` env 注入（config.yaml:32 已留好绑定位）约 15 分钟工作量 |
| E-2 | **JWT Secret 双通道注入命名不一致**：compose 注 `APP_JWT_SECRET`，显式 BindEnv 名是 `JWT_SECRET`，当前靠 viper AutomaticEnv 的键推导兜底生效 | docker-compose.yaml:77、config.go:161 | 🟠 中 | D2-02 曾因同类前缀错配出过真实事故（compose 头注释有案底）；显式绑定一个名字并在 compose/.env.example 对齐，消除隐式依赖 |

### B.4 工程化配置（新发现）

| # | 发现 | 位置 | 级别 |
|---|------|------|------|
| E-1 | 完整部署 compose 的 app 服务缺自身 healthcheck（`/health/live|ready` 已实现但未被消费）、无资源限制、无日志驱动策略 | docker-compose.yaml:59-86 | 🟠 中 |
| E-3 | swag target 依赖全局 PATH 安装 CLI（wire 用 `go run` 不依赖安装，风格不一）；swag 输出到 `docs/` 而 .gitignore 忽略的是 `doc/` | Makefile:53、.gitignore:40 | 🟡 低 |
| E-4 | 配置加载路径硬编码相对 cwd（`configs/config.yaml`），无 `-c/--config` 或 CONFIG_PATH | cmd/server/main.go:17 | 🟡 低 |
| E-5 | docker-dev-reset 固定 `sleep 5` 即跑迁移，非 healthcheck 轮询；PG 冷启动慢于 5s 时 migrate-up 失败 | Makefile:42-44 | 🟡 低 |
| E-6 | Dockerfile 固化国内源（aliyun apk + goproxy.cn）+ 浮动 minor tag（golang:1.26-alpine/alpine:3.20 非 digest 锁定）；容器日志写容器层未挂卷 | Dockerfile:8,10,27 | 🟡 低（海外 CI 需构建参数化） |
| E-7 | wire.go:32-34 注释称 Registry「当前注入链路无消费者」，实际 ticketService 已消费（G-1 完成）；计划文档多处 G-1/G-2 待办时态未回勾 | wire.go、phase2/00 Step 0、README:148 | 🟡 低 |
| E-8 | architecture.md:1614 errcode 分段概览止于 80000，缺 90000 工单/91000 存储两段 | architecture.md:1614 | 🟡 低 |

> 明确的**良好实践清单**（免误读为全面欠账）：多阶段 Dockerfile + 非 root（uid 33333）+ 静态二进制；release 模式 JWT secret 黑名单校验拒绝弱密钥/已知默认值；http.Server 四项超时齐备（slowloris 防护）；Redis noeviction fail-closed 保护吊销键族；DB DSN 特殊字符转义；双 compose 项目名/卷/容器名完全隔离（D2-29）；testcontainers 集成测试零外部依赖。工程化基线在同类项目中属上游水平。

---

## C. 合并修复路线图（取代 07 §11）

### 批次一：2b 开工前必须（≈1.5h）

1. **V-1** VISION L10 阶段归属纠正 + N5 L44 migrations 现状更新（同文件同批）
2. **A-1** architecture.md §4.1 L558 按 11 号待办 #2 改路径 A
3. **C2** 02-authz §2.5 示例改为 `registry.Register(NewResource(s.ticketRepo))` 并随 M-5 回标
4. **S-1** Redis 启用 requirepass（conf + dev/full compose + REDIS_PASSWORD 绑定链）
5. **E-2** JWT secret 注入命名统一（保留 `APP_JWT_SECRET` 显式 BindEnv 或改 compose 传 `JWT_SECRET`，二选一对齐 .env.example）
6. **N2** roadmap L98 断链改指 docs/adr/ADR-003

### 批次二：随 2b-core/org 收口（承接 00 §9 BK 清单）

7. BK-1 is_internal 过滤列为 **Step 4 验收门禁**；BK-3/5/6 写入 2b 验收脚本（延续 07 判断）
8. R-1 / H-1 / D-1 / O-1 四处叙事回标（各一行～半页，纯文档）
9. C1-N4（000013 表述）+ E-7（G-1/G-2 checkbox 回勾）+ E-8（errcode 概览补段）
10. M-1 modules/ticket 两处失效引用改指 ADR-001

### 批次三：打磨与上线检查单（可与任意 Step 并行）

11. Makefile：DSN 外置 `MIGRATE_DSN ?=`、`make swag` 改 tools 管理、reset 改轮询；.gitignore `doc/`→`docs/`
12. compose：app 服务 healthcheck + mem_limit + logging（json-file cap）
13. main.go 支持 `-c/--config` 覆盖配置路径（12-Factor env 化已有 Viper 底座，成本极低）
14. **CORS 决策转轨**：写入 phase3 触发表 + ops/deployment.md 上线检查单（替代「赶在前」）
15. lint 增强：引入 golangci-lint（errcheck/gosec 最低集），挂进现有 `make lint`

---

## D. 本轮新增验证证据（审查依据可追溯性）

- `go build/vet/lint` 通过；`gofmt -l` 全空；单元 12 包 + 集成（testcontainers PG）12 包全绿（-race）。
- `make docker-dev-reset`（全新库 golang-migrate CLI）：12 个迁移 up 至 version 16 非 dirty；down 3 → re-up 种子完整恢复（11 菜单/16 menu_apis/2 类型/22 role_menus）。
- `scripts/acceptance-phase2a.sh` 退出码 0：Section A（Phase 1 回归）88 PASS + Section B（2a）65 PASS，**0 FAIL**——此前两轮脚本从未端到端通过（本轮修复 `,string` 字段引号、operator 菜单绑定前置、move 字段名、None 解析等三类系统性 bug，详见提交 75e7522）。
- 三方一致性抽验：16 条 ticket 路由 = menu_apis 种子 = handler @Router 注解逐一吻合；90001–90004 在 errcode.go / api/errcode.md / handler 映射表三处一致；Resource 接口签名与 proposal/resource-model 逐字段相同（文件名 registry.go vs resource.go 一处低危失真已录 E 系列）。

## 总体判断

九维结构与评级**沿用 06/07 结论不变**（目标边界、模块划分、安全可靠性、流程交互、阶段边界均为优良；短板集中于收口同步）。本文新增的关键增量：**V-1（VISION 锚点阶段归属错误）为本轮最高优先**——唯一锚点的失真危害大于任何单一模块文档误差；S-1/E-2/E-1 说明「安全承诺需要 expiry 管理」；其余新发现均可落入批次一/二的低成本修补。架构主线（三层鉴权合取语义、工单↔组织弱单向耦合、事件机制 L1 稳态、单体优先演进）经代码实证**设计与实现一致**，具备进入 2b 的条件。
