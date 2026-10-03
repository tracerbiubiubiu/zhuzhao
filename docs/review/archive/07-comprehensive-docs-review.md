# docs 目录全面审查报告

> 审查视角：资深后端开发 / 系统架构设计 / 微服务架构师
> 审查范围：`docs/` 全量 66 份 Markdown 文档 + 代码实现对照 + 工程化配置
> 审查日期：2026-08-27
> 与前序审查关系：本报告为 `06-phase2-docs-architecture-review.md` 的补充与深化，聚焦前序未覆盖项、已识别问题的当前状态验证、以及新增发现。

---

## 审查结论总览

| 维度 | 评级 | 说明 |
|------|------|------|
| 1. 项目目标与能力边界 | ✅ 优秀 | VISION SSOT + 三层真相源 + "不做什么"显式声明 |
| 2. 模块划分与使用场景 | ✅ 良好 | 三层鉴权对齐业界实践；模块正交、耦合面小 |
| 3. 安全性与可靠性 | ⚠️ 有待修 | 鉴权设计优秀，但 CORS AllowAll + Phase 1 服务层测试盲区为高风险项 |
| 4. 模块交互与业务流程 | ✅ 良好 | 覆盖正常与异常路径；用例固化验收 |
| 5. 开发阶段目标与边界 | ✅ 优秀 | 2a/2b-core/2b-org/2b-ext/2c 拆分清晰，门禁驱动 |
| 6. 文档一致性与演进 | ⚠️ 有待修 | 4 项已知不一致未修复（C1–C4）+ 4 项新发现（含 auth-design.md 偏差） |
| 7. 测试用例完整性 | ⚠️ 有待修 | 2a 用例全绿，但 Phase 1 service 层约 940 行核心写路径无单元测试守护 |
| 8. 部署与工程化 | ⚠️ 有待修 | DSN 硬编码 + ops 文档空壳 + CORS 全开 |
| 9. 代码↔文档一致性 | ⚠️ 有待修 | 3 项文档滞后于代码重构 + auth-design.md 提案偏差 |

**整体风险等级：中。** 设计层面无重大缺陷；但存在 2 项高风险项（Phase 1 服务层测试盲区、CORS AllowAll）和多收口同步问题需在 2b 前修复。

---

## 1. 项目整体目标与能力边界

**结论：定义完整、清晰，具备明确的"不做"边界。**

### 亮点

- `VISION.md` 作为范围判定唯一锚点，设立"MVP 边界铁律"（§2）和"修改纪律"（§4），从制度上防止文档互相打架。
- 三层真相源体系（roadmap = 执行视图 → design-decisions = 决策日志 → 代码/migrations = 真实落地）清晰可审计。
- 各阶段均有显式"不做什么"声明：`phase2/README §1.5`、`09-ticket §0`、`phase3/README §0`。
- Phase 3 触发信号量化表把"暂缓"变为"有触发条件的等待"，避免无限期搁置。

### 发现的问题

| # | 问题 | 风险 | 改进建议 |
|---|------|------|----------|
| N1 | `VISION.md §4 L44` 写"磁盘 `migrations/` 现状至 000009；000010+ 为规划编号"，但实际磁盘已有 000010/000015/000016 | 低 | 更新为"现状至 000010、000015、000016（2a 已落地）" |
| N2 | `roadmap.md L98` 引用 `doc/soar/activelist.md`，该路径在仓库中不存在 | 低 | 改为正确路径或标注为外部文档链接（前序 05 号审查 G-2 已识别但未修复） |

---

## 2. 模块划分与使用场景

**结论：模块划分符合业界通用实践，分层合理，职责正交。**

### 架构主线对齐业界

三层鉴权（L1 Casbin 路由级 RBAC → L2 GetFilter 行级可见性 → L3 canOperate 属主/角色）对应：
- 路由级 RBAC（Casbin/OpenFGA 风格策略）→ "能不能调接口"
- 行级谓词（SQL WHERE 拼接）→ "列表该看哪些行"（Google Zanzibar / ACL 行级过滤工程化落地）
- 资源级属主判断 → "能不能碰这条数据"

模块清单（authz-resource / org-enhance / ticket / storage / org-delegation / auth-enhance / hr-sync）职责正交、耦合面小。

### 发现的问题

| # | 问题 | 风险 | 改进建议 |
|---|------|------|----------|
| N3 | 缺少"Phase 2 模块依赖有向图"——各 PRD 内部有交互描述，但无总览图 | 低 | 在 `00-implementation-plan §4` 补一张模块依赖 DAG（org-enhance → ticket scope；org-delegation → ticket Authorize） |

---

## 3. 安全性与可靠性设计

**结论：对齐 OWASP / 身份认证最佳实践，成熟度高于同类项目。**

### 安全设计亮点

1. **信息隐藏语义正确**：不可见资源统一 404（非 403），防枚举/越权探测（OWASP ASVS V4）
2. **凭证防探测**：密码错误与账号不存在同文案（401/20001）
3. **fail-close**：Redis 不可用时鉴权链路返回 503（10008），不降级放行
4. **三层鉴权失败语义区分**：404（不可见）/ 403（可见无权）/ 400（非法状态）/ 409（已关闭）
5. **org_path 快照 + 工单跟人走**：对齐 Jira/ServiceNow 范式

### 可靠性设计

- 乐观锁（ErrConcurrentModification 10006）预留
- move 事务内级联改写 tickets.org_path（P2-D1 方案 A）
- 迁移幂等 + down 可逆检查单

### 发现的问题

| # | 问题 | 风险 | 改进建议 |
|---|------|------|----------|
| S1 | `is_internal` 内部备注不过滤——2a 阶段读者集合 ⊆ 备注可见集合，当前无泄露；但 2b 策略 B 扩大读者后若忘做则升级为真实漏洞 | 中 | 将 BK-1 提升为 2b Step 4 验收门禁（非仅 backlog） |
| S2 | Casbin 无 Watcher——单实例下可接受，多实例策略不一致 | 中 | 已声明为 Phase 3 触发条件，可接受 |
| S3 | `update` 不写 `ticket_events` + close 读后写 TOCTOU | 低-中 | 2b 补 `UPDATE ... WHERE status<>'closed'` 条件更新 |
| S4 | **CORS AllowAllOrigins**——`internal/middleware/cors.go L12` 设 `config.AllowAllOrigins = true`，注释写"上线前再收紧"但未设定具体收紧时机和方式 | 高 | 在 2b 或部署前改为白名单模式（读取 config `server.cors_origins`），至少限制为前端域名 |
| S5 | **Phase 1 服务层测试盲区**——`audit_service.go`（134 行）无单元测试、`user_service.go`（450 行）仅集成测试无分支级单测、`auth_service.go`（354 行）关键分支（锁定 429 / status=0 / disabled / RT 篡改）未覆盖。合计约 940 行核心写路径无守护测试 | 高 | 进入 2b Step 7（改造 auth_service RT 结构）前补齐；至少在 2a 收口时为 audit/user service 补单测 |
| S6 | **DB SSL 默认关闭**——`config.yaml L20` `sslmode: disable`，注释写"生产改为 require"但无强制机制（如 release 模式校验） | 中 | 在 config 校验中增加 release 模式下 `sslmode` 必须 `require` 的断言 |

---

## 4. 模块间交互与业务流程

**结论：流程合理，覆盖正常使用场景；异常路径以用例固化。**

### 正常流程覆盖

- 创建工单：Casbin → 校验 type/org → 读 org.path 写 org_path → INSERT + ticket_events（文档与代码一致）
- 可见性演进链：assigned(2a) → 策略 B 实体透明读 + scope 并集(2b) → org admin/owner + ancestor owner(2c)，三层叠加而非替换
- 读改写分离（L2 读扩大、L3 写收紧）："兄弟虚拟组可读不可改"——符合企业工单协作模型
- 委托防提权矩阵覆盖 org admin/owner / ancestor owner / vg admin 跨组隔离

### 发现的问题

| # | 问题 | 风险 | 改进建议 |
|---|------|------|----------|
| B1 | `CreateRelation` 双端 update 鉴权——2b 把 update 收窄为"仅创建人"后，处理人不能建立关联，对协作场景偏严 | 低 | 在 04/09 文档明确"关联权限归属创建人"的产品决策 |
| B2 | 状态推进端点缺失——种子状态机含 `in_progress`/`pending_verify`，但 2a API 无对应端点 | 低 | 产品决策待拍板（BK-10 已登记） |

---

## 5. 开发阶段目标与边界

**结论：阶段目标明确、边界清晰，是本套文档最成熟的部分之一。**

### 亮点

- Step 0–10 + 三轨拆分（2b-core / 2b-org / 2b-ext）将原 2b 的"核心可见性"与"增强"解耦，关键路径最短化为 `2a → 2b-core → 2c`
- 每步标注"做什么/不做/迁移号/验收用例/落点 PRD"，可执行性强
- 里程碑门禁（M2a-0 ~ M2c-3）只列"该里程碑新增可测用例"，防交叉引用漂移

### 发现的问题

| # | 问题 | 风险 | 改进建议 |
|---|------|------|----------|
| P1 | Step 与子阶段两套编号——文档同时用"Step 1-10"和"2a/2b-core/2b-org/2c"，新人需在映射表中来回跳转 | 低 | 在每份 PRD §0 标注"本 PRD 覆盖 Step X–Y（里程碑 Mx）" |

---

## 6. 文档一致性与演进平滑性

**结论：一致性整体强（SSOT + 变更记录 + 交叉引用），但存在 6 项不一致待修。**

### 演进平滑性：优秀

- Phase 1（认证鉴权）→ Phase 2（工单业务）→ Phase 3（生产加固+工单闭环）递进自然
- Phase 3 整体暂缓但"设计就绪"，无重复建设、无断层
- 触发信号量化表符合精益/演进式架构原则

### 仍未修复的不一致（前序 06 号审查已识别）

| # | 不一致点 | 位置 | 风险 | 当前状态 |
|---|---------|------|------|----------|
| C1 | 2c 阶段编号：`09-ticket §0 L3` 写 "Step 9（2c Authorize）"，而 `00 §8` 已明确 2c = Step 8–10 | `09-ticket.md:3` | 低 | **未修复** |
| C2 | `02-authz §2.5 L218` 代码示例 `NewTicketResource(s.repo, s.scope)` 传 scope 参数，但 2a 代码已重构为 `NewResource(s.ticketRepo)` | `02-authz-resource.md:218` vs `resource.go:31` | 中 | **未修复** |
| C3 | `docs/ops/` 仅有 `README.md`，无 `deployment.md` | `docs/ops/` | 低 | **未修复** |
| C4 | `02-authz §2.2` 画出完整 `ScopeResolver` 接口，与 2a 实际仅 `HasRole` 辅助不符 | `02-authz-resource.md:88-115` | 低 | **未修复** |

### 新发现的不一致

| # | 不一致点 | 位置 | 风险 | 改进建议 |
|---|---------|------|------|----------|
| N4 | `README §2.4` 迁移表 000013 仅列 `ticket_attachments`，但 `10-storage.md` 在该迁移中定义了 `file_objects` + `ticket_attachments` 两张表 | `phase2/README.md:242` vs `10-storage.md:66,78` | 低 | README §2.4 000013 改为"附件：`file_objects` / `ticket_attachments`" |
| N5 | `VISION.md §4 L44` 写 migrations 现状至 000009，但磁盘已有 000010/000015/000016 | `VISION.md:44` | 低 | 更新为实际状态 |
| N6 | **`auth-design.md` 提案与实际实现偏差**——§1.3 写 `devices:{userId} SADD` 但 Phase 1 实际无 devices 集合（2b Step 7 才实现）；§2.4 列过滤用 `creator_id`（snake_case）但实际 DDL 用 `created_by` | `docs/proposal/auth-design.md:168,§1.3` | 中 | 在各偏差点增加内联标注"实际实现见 phase1/02-auth.md / phase2/09-ticket.md SSOT" |
| N7 | `03-org-enhance.md` 缺少 §1 前置条件章节——其他 PRD（01/02/04/09/10）均有显式前置清单，唯 03 直接从"预期功能"跳到"核心设计" | `docs/phase2/03-org-enhance.md` | 中 | 补 §1 前置条件（Phase 1 org + user_orgs + ltree / 2a ticket org_id/org_path / hr-directory-sync DDL / BFS RoleFetcher） |

---

## 7. 测试用例完整性

**结论：测试策略完整，正常/异常场景覆盖充分；BK 系列需转化为自动化门禁。**

### 已验证

- 41 个测试文件（15 个集成测试），153 用例全绿（M2a-3 达成）
- 异常场景覆盖好：404（不可见）/ 403（可见无权）/ 400（非法状态转换）/ 409（已关闭）/ 密码同文案防枚举
- 2b/2c 用例用 `⏳` 标记延期，不误判失败
- 验收脚本分层：`acceptance-phase1.sh`（27 例）+ `acceptance-phase2a.sh`（含 Phase 1 回归段 + 2a 新增段）

### 发现的问题

| # | 问题 | 风险 | 改进建议 |
|---|------|------|----------|
| T1 | BK-1~BK-10 已识别但未自动化——若 2b 实现遗漏，回归无法拦截 | 中 | 把 BK-1/3/5/6 写入 2b Step 4/5 验收脚本 |
| T2 | `setupTicket2a` 死代码回退分支（BK-9）：`if orgID==0` 不可达 | 低 | 清理测试辅助代码 |
| T3 | **Phase 1 service 层测试盲区**——`audit_service.go`（134 行）零单元测试、`user_service.go`（450 行）仅集成测试无分支单测、`auth_service.go`（354 行）关键分支（锁定 429 / status=0 / disabled / RT 篡改 / dummy bcrypt 时延）未覆盖。2a 工单鉴权链路依赖这些模块，2b 还要改 `auth_service.go`（RT value 结构升级 D2-49），无守护测试直接改风险高 | 高 | 2b Step 7 改造前必须补齐 T-新-1~3 测试守护（见 `14` 号文档 Part 1 类别 2） |
| T4 | **鉴权 fail-closed 测试缺失**——`11-authz-architecture-review.md §3 Q3` 指出 Authorize 抛 DB 错误时 Service 必须按拒绝处理（503），但 R1-R8 测试用例无 DB 错误注入测试 | 中 | 在 `02-authz §4` 增加 R-DB-1 用例：Mock Repository 返回 DB 错误 → Authorize 返回 503 |
| T5 | **工单状态机并发场景测试不足**——T6 仅测单线程非法转换，未覆盖并发 close 同一工单（TOCTOU 窗口） | 中 | 增加 T-concurrent-1 用例：并发 close → 仅一次成功 |

---

## 8. 部署方案与工程化配置

**结论：Makefile 可用、部署目录真实；存在工程化打磨点。**

### 已验证

- Makefile `.PHONY` 完整，`build/run/dev/wire/migrate/docker-*/swag/lint/test*` 目标齐全
- `deployments/` 目录真实存在（docker-compose.dev.yaml / docker-compose.yaml / Dockerfile / redis/）
- Dockerfile 多阶段构建（CGO 禁用 + 非_root 用户 + 最小镜像）
- docker-compose dev/prod 隔离（项目名/容器名/卷名分离）
- gofmt 门禁（lint 目标含 `gofmt -l` 漂移检测）
- 健康检查（postgres pg_isready + redis ping）
- 配置外置（config.yaml + 环境变量覆盖，JWT_SECRET release 模式强制）

### 发现的问题

| # | 问题 | 风险 | 改进建议 |
|---|------|------|----------|
| E1 | `Makefile:26,29` 迁移 DSN 硬编码 `postgres://zhuzhao:zhuzhao_dev@localhost:5432/zhuzhao`（明文密码 + 固定 host），违反 12-factor | 中 | 抽取为 `MIGRATE_DSN ?= ...` 变量或读 `.env` |
| E2 | `test-integration` 依赖 Docker（testcontainers），README/Makefile 未说明 | 低 | 补注释"需本地 Docker daemon" |
| E3 | `docker-dev-reset` 用 `down -v` 删所有本地 dev 数据——不可逆但未标注 | 低 | Makefile 注释标"不可逆" |
| E4 | `docs/ops/` 仅有 README 空壳，`deployment.md` 和 `runbook.md` 均待创建 | 低 | 至少补 deployment.md 指向 `deployments/` 目录 |
| E5 | **CORS AllowAllOrigins 未设定收紧时机**——`cors.go L9` 注释"上线前再收紧"但无 issue/里程碑绑定 | 高 | 在 2b 或部署前改为白名单模式，读取 config `server.cors_origins` |
| E6 | **验收纪律依赖手动**——无 CI 平台，Phase 1 27 例回归 + Phase 2 验收全靠开发者自觉跑脚本 | 中 | 增加简单 pre-commit hook 自动运行 `make test -race`（不含验收脚本）；PR 模板加验收确认 checkbox |

---

## 9. 代码实现 ↔ 文档一致性

**结论：高度一致。前序审查发现的问题已修复并文档化收口。3 项文档滞后于代码重构待修。**

### 已验证一致

- 路由风格（`POST /tickets/update` 动作路径而非 RESTful PATCH）：文档解释为"menu_apis L1 鉴权依赖操作类 POST 路径不含 :id"，代码一致——有意为之
- `service.go` 事务修复（Close/Assign 使用 Tx 版本）：代码已修复
- `resource.go` Authorize 三路语义（404/403/500）：代码已修复
- `ticket_repo.go` SoftDelete → Delete 重命名：代码已修复
- `Assign` 状态机（取消分派 → open 回退）：代码已修复
- `List` admin bypass L2：代码已修复
- 迁移 `000016_ticket_relations` FK ON DELETE CASCADE：已修复

### 仍存的偏差

| # | 偏差 | 现状 | 判断 | 改进 |
|---|------|------|------|------|
| D1 | `scope_resolver.go` 仅 `HasRole`，但 `02-authz §2.2` 画了完整 `ScopeResolver` 接口 | 2b 待办 | 代码合理（2a 只需 HasRole） | 文档示例标注"（2b 待实现）" |
| D2 | `02-authz §2.5 L218` `NewTicketResource(s.repo, s.scope)` vs 代码 `NewResource(s.ticketRepo)` | 代码已重构去 scope | **代码更合理** | 文档 C2 需同步 |
| D3 | `09-ticket §0 L3` "Step 9" vs `00 §8` "2c = Step 8–10" | 00 正确 | 以 00 为准 | 09 §0 需改（C1） |
| D4 | `auth-design.md §1.3` 写 `devices:{userId} SADD`，但 Phase 1 实际无 devices 集合（仅 `SET refresh:{uid}:{devId}`） | 提案超前于实现 | 代码合理（2b 才实现） | auth-design.md 标注"2b Step 7 实现" |
| D5 | `auth-design.md §2.4` 列过滤用 `creator_id`，但实际 DDL/code 用 `created_by` | 提案笔误 | 代码正确 | auth-design.md 改为 `created_by` |

### 业界对照

| 设计决策 | 业界对照 | 评价 |
|----------|----------|------|
| 工单可见性 assigned + 组织快照 | Jira/ServiceNow reporter/assignee + 组织路径快照 | ✅ 对齐 |
| 三层鉴权 L1+L2+L3 | ACL 行级过滤 + RBAC | ✅ 对齐 |
| 事件机制 L1→L2 (Outbox+Asynq) | 微服务可靠事件分发（Uber Cadence/Temporal 轻量落地） | ✅ 合理 |
| Phase 3 暂缓微服务化 | Martin Fowler "Monolith First" + "演进式架构" | ✅ 稳健，未过度设计 |
| 迁移治理（预分配编号 + up/down 可逆 + 软删部分索引） | Django/Rails 大型项目迁移治理规范 | ✅ 典范 |

---

## 10. 综合风险矩阵

| 风险 | 等级 | 类型 | 触发条件 | 建议处置 | 时机 |
|------|------|------|----------|----------|------|
| Phase 1 service 层测试盲区（S5/T3） | **高** | 测试/可靠 | 2b 改 auth_service RT 结构时回归无法拦截 | 2b Step 7 前补齐 T-新-1~3 单测 | 2b 前置 |
| CORS AllowAllOrigins（S4/E5） | **高** | 安全 | 任何部署上线 | 改白名单模式，读 config | 部署前 |
| is_internal 备注不过滤（BK-1） | 中 | 安全 | 2b 策略 B 扩大读者后忘做 | 提为 2b Step 4 验收门禁 | 2b |
| Makefile 迁移 DSN 硬编码 | 中 | 工程化 | 多环境/协作 | 外置为变量/`.env` | P0 |
| 文档示例未同步重构（C2/D2） | 中 | 一致性 | 新成员按文档实现 | 2a 收口 PR 同修 | P0 |
| auth-design.md 提案偏差（N6/D4/D5） | 中 | 一致性 | 新成员按提案实现 | 各偏差点加内联标注 | P1 |
| 03-org-enhance 缺前置条件（N7） | 中 | 一致性 | 实现者遗漏依赖 | 补 §1 前置条件 | P1 |
| BK 系列未转自动化用例 | 中 | 测试 | 2b 实现遗漏 | 写入 2b 验收脚本 | 2b |
| fail-closed 测试缺失（T4） | 中 | 测试 | DB 故障时 Authorize 行为不可预测 | 补 R-DB-1 用例 | 2a 收口 |
| 验收纪律手动（E6） | 中 | 工程化 | 改动破坏 Phase 1 无自动拦截 | pre-commit hook + PR checkbox | P1 |
| DB SSL 默认关闭（S6） | 中 | 安全 | 生产部署时忘改 | release 模式强制 require | 部署前 |
| Casbin 无 Watcher | 中 | 安全/可靠 | 多实例部署 | Phase 3 启动（已记录） | Phase 3 |
| update 审计断档 + TOCTOU（BK-3） | 低-中 | 可靠/审计 | 并发 close+patch | 2b 条件更新 | 2b |
| C1/C3/C4/N4/N5 文档不一致 | 低 | 一致性 | 误导 | 文档同步 | P1 |
| 状态推进端点缺失（BK-10） | 低 | 产品 | 待拍板 | 2b 拍板 | 2b |

---

## 11. 改进路线图（按优先级）

### P0 — 建议本迭代完成（低成本，高频引用）

1. **修 C2**：`02-authz §2.5 L218` 示例改 `NewResource(s.ticketRepo)`
2. **修 C1**：`09-ticket §0 L3` 2c 改 "Step 8–10"
3. **修 N4**：`README §2.4` 000013 改为"附件：`file_objects` / `ticket_attachments`"
4. **修 N5**：`VISION §4 L44` 更新 migrations 现状
5. **修 N2**：`roadmap.md L98` 修正 `doc/soar/activelist.md` 路径
6. **Makefile DSN 外置**为 `MIGRATE_DSN ?= ...`（含 `.env` 读取说明）
7. **CORS 收紧**：`cors.go` 改为读 config `server.cors_origins` 白名单，默认空=拒绝所有跨域

### P1 — 2b 收口前必须完成

8. **补齐 Phase 1 service 层单测**（S5/T3）：audit_service / user_service / auth_service 关键分支
9. 将 BK-1（is_internal 过滤）作为 2b Step 4 **验收门禁**
10. 将 BK-3（TOCTOU 条件更新）、BK-5（反向关联判重）、BK-6（move 级联测试）写入 2b 验收脚本
11. **修 C4**：`02-authz §2.2` 接口示例标注"（2b 待实现）"
12. **修 D1**：同步 scope_resolver 文档描述
13. **修 N6/D4/D5**：`auth-design.md` 各偏差点加内联标注
14. **修 N7**：`03-org-enhance.md` 补 §1 前置条件
15. **补 T4**：`02-authz §4` 补 R-DB-1 fail-closed 测试用例
16. **补 E6**：pre-commit hook 自动 `make test -race`

### P2 — 文档打磨（可并行）

17. README 顶部补"目标→阶段→关键决策"一页索引
18. `00 §4` 补模块依赖有向图（N3）
19. 核实 `docs/ops/README` 引用，补 `deployment.md` 或修正指向 `deployments/`（C3/E4）
20. PRD §0 统一标注 Step+里程碑双编号（P1）
21. Makefile `test-integration` 补 Docker 依赖说明（E2）
22. `docker-dev-reset` 标注"不可逆"（E3）
23. `ticket.md`（831 行）考虑拆分——workflow 引擎设计独立成文

---

## 12. 总体结论

文档体系在**目标边界、模块划分、安全设计、阶段拆分、一致性纪律、测试策略、演进平滑性**七个维度均达到甚至超过同类项目水准。核心亮点：

- **三层鉴权（L1/L2/L3）** 分层论证专业，对齐 Zanzibar/ACL 行级过滤
- **P2-D6 "工单跟人走 + 组织快照"** 对齐 Jira/ServiceNow 范式并固化为决策
- **SSOT + 变更记录 + 五维映射表** 使大规模文档可维护
- **Phase 3 触发信号量化表** 把"暂缓"变为可管理的演进状态
- **迁移治理三规范** 对齐 Django/Rails 大型项目规范

**主要短板不在设计，而在"收口同步"与"测试守护"**：6 项文档不一致仍未修复（C1–C4 + N4–N5）、`auth-design.md` 提案与实现存在偏差（N6/D4/D5）、Phase 1 服务层约 940 行核心写路径无单元测试守护（S5/T3）、CORS 全开放未设定收紧时机（S4/E5）、工程化配置（DSN 硬编码）待外置。其中 S5（测试盲区）和 S4（CORS）为高风险项，需在 2b 编码前和部署前分别修复。建议按第 11 节路线图在 2b 收口前完成 P0 项，2b 前置完成 P1 项。
