# Phase 2 文档体系架构审查报告

> 审查视角：资深后端开发 / 系统架构设计 / 微服务架构师
> 审查对象：`docs/` 全量文档（VISION、roadmap、README、phase1、phase2×6 PRD、phase3、api、design、modules、ops、proposal、adr）
> 审查日期：2026-08-27
> 结论总览：**文档体系成熟度高于同类个人/中小项目，SSOT 意识强、阶段边界清晰、安全设计对齐业界最佳实践；主要风险集中在少量文档内部不一致、工程化配置细节、以及 2a 已修复项在文档/代码的收口同步。** 整体风险等级：低—中。

---

## 0. 审查方法与范围

逐份精读纲领性文档（VISION / roadmap / README）+ 核心实现计划（00-implementation-plan）+ 关键 PRD（02-authz-resource / 09-ticket / 04-org-delegation 抽样）+ 契约（api/errcode / api/response）+ 工程化（Makefile / deployments/）+ 演进（phase3）。代码侧以 `internal/service/ticket/*`、`internal/repository/ticket_repo.go`、`internal/handler/ticket_handler.go` 为对照，验证"文档↔代码"一致性。

> 注：phase1 文档已在前序审查（见 `docs/review/00~05`）充分覆盖，本报告聚焦 phase2 及其与上、下游的衔接。

---

## 1. 项目整体目标与能力边界

**结论：定义完整、清晰，且具备明确的"不做"边界。质量高。**

| 维度 | 评价 | 证据 |
|------|------|------|
| 总目标 | 单体 IAM + 工单业务能力，先把基础打牢 | `VISION.md`、roadmap 主视图 |
| 能力边界 | 显式声明"不做什么"，防止范围蔓延 | `README §1.5 不做什么整个 Phase 2`、`09-ticket §0 2a 不做` |
| 设计取向 | "宽松优先、基础为先"总则贯穿全程 | `00 §1 P2-D6 V0 总则` |
| 演进纪律 | 需求触发式暂缓（Phase 3 整体暂缓、按需启动） | `phase3/README §0 暂缓声明 + 启动条件表` |

**发现的问题：**
- **[低风险] 目标层级略显分散**：目标同时出现在 VISION / roadmap / 各 phase README / 各 PRD §0，虽各有侧重（纲领 vs 执行），但新成员需读 4+ 文件才能拼出全貌。建议在 `README.md` 顶部增加一张"目标→阶段→关键决策"一页索引表。

**风险等级：低。改进建议：** 补充一页式导航索引（非内容重复，仅指针聚合）。

---

## 2. 模块划分、使用场景合理性与完整性

**结论：模块划分符合业界通用实践，分层合理。**

架构主线：`L1 Casbin 路由级 RBAC` → `L2 GetFilter 行级可见性` → `L3 canOperate 属主/角色`，外加 `ResourceRegistry` 统一委托。这与业界主流做法一致：
- 路由级 RBAC（Casbin/OpenFGA 风格策略）负责"能不能调接口"；
- 行级谓词（SQL WHERE 拼接）负责"列表该看哪些行"——Casbin 的 KeyMatch 表达不了 `created_by=$user OR assigned_to=$user`，文档在 `02-authz §2.1` 对此有清晰论证，正是 Google Zanzibar / ACL 行级过滤思路的工程化落地；
- 资源级属主判断（resource-level permission）负责"能不能碰这条数据"。

模块清单：`authz-resource`（资源抽象）/ `org-enhance`（虚拟组+scope）/ `ticket`（工单 MVP）/ `storage`（附件）/ `org-delegation`（组内委托）/ `auth-enhance`（设备+密码）/ `hr-sync`（目录同步）—职责正交，耦合面小。

**发现的问题：**
- **[低风险] `scope_resolver.go` 文档/代码定位漂移**：`02-authz §2.2` 仍画出完整 `ScopeResolver` 接口（`ReadAnchorPaths`/`NearestEntityOrg`/`ScopePathsForMembership`），但 2a 代码实现后该接口已被简化为仅 `HasRole` 辅助（见 `00 §9 BK-8`：文件名与内容不符）。文档未同步 2a Step 2 的重构结果，可能误导实现者以为接口已落地。
- **[低风险] 模块交互图缺一张总览图**：各 PRD 内部有交互描述，但缺少一张"Phase 2 模块依赖有向图"（谁依赖谁的迁移/服务）。`00 §4` 的批次图接近，但偏排期而非依赖。

**风险等级：低。改进建议：** 在 `00 §4` 基础上补一张模块依赖图（org-enhance → ticket scope；org-delegation → ticket Authorize）；同步 `02-authz §2.2` 接口示例为 2a 实际形态。

---

## 3. 安全性与可靠性设计

**结论：对齐 OWASP / 身份认证最佳实践，成熟度高于同类项目。亮点突出。**

安全设计亮点：
1. **信息隐藏语义正确**：不可见资源统一 404（非 403），防枚举/越权探测。见 `api/errcode §3`、`09-ticket §5.4`。与"不暴露资源存在性"的 OWASP ASVS V4 一致。
2. **凭证防探测**：密码错误与账号不存在同文案（401/20001）；AK 验签失败统一 20009（不区分 AK 不存在与 SK 错误）。见 `api/errcode §5`。符合防用户枚举最佳实践。
3. **fail-close**：Redis 不可用时鉴权链路返回 503（10008），不降级放行。见 `api/errcode §3`。
4. **三层鉴权失败语义区分**：404（不可见）/ 403（可见无权）/ 400（非法状态）/ 409（已关闭）——`09-ticket §5.4` 对照表清晰。
5. **org_path 快照 + 工单跟人走**：`P2-D6 V7/V8` 对齐 Jira/ServiceNow "工单跟 reporter/assignee + 组织路径快照，重构不联动"范式，边界场景拍板清晰。

可靠性设计：
- 乐观锁（`10006 ErrConcurrentModification`）预留；
- move 事务内级联改写 `tickets.org_path`（P2-D1 方案 A）——数据一致性保障；
- 迁移幂等 + down 可逆检查单（`00 §5.3`）。

**发现的问题（真实风险）：**
- **[中风险] `is_internal` 内部备注不过滤**：`09-ticket §3` 的 `POST /tickets/notes` 在 2a 口径为"创建人或处理人可见/可写"，但 `ListComments` 实际返回全部 comments（含 `is_internal=true`）而不按调用者角色裁剪。文档已意识到此问题并登记为 `00 §9 BK-1`（"2b 策略 B 扩大读者后必须过滤"），但**2a 阶段读者集合（creator/assignee/admin）⊆ 备注可见集合，当前无泄露**——属"已识别、已排期、2b 前置"的可接受状态。风险在 2b 忘记做则升级为真实漏洞。
- **[中风险] Casbin 无 Watcher（多实例策略不一致）**：已显式声明为 Phase 3 触发条件（`02-authz §2.6`、phase3 `02-multi-instance` 待编写），单实例 Compose 拓扑下可接受。属"已知 tradeoff，已记录触发信号"，非缺陷。
- **[低风险] `update` 不写 `ticket_events` + close 读后写 TOCTOU**：`00 §9 BK-3` 已登记（审计断档、并发 close 后仍可 patch）。2a 审计非硬要求，建议 2b 补 `UPDATE ... WHERE status<>'closed'` 防 TOCTOU。

**风险等级：中（is_internal 为最高优先修复项，但已排期）。改进建议：** 将 BK-1 提升为 2b Step 4 的"验收门禁"（非仅 backlog）；BK-3 的 TOCTOU 用条件更新在 2b 顺手解决。

---

## 4. 模块间交互与业务流程

**结论：流程合理，覆盖正常使用场景；异常路径以用例固化。**

- 创建工单：`Casbin → 校验 type/org → 读 org.path 写 org_path → INSERT + ticket_events`（`09-ticket §6`）——与代码 `service.go` 一致。
- 可见性演进链：`assigned(2a)` → `策略 B 实体透明读 + scope 并集(2b)` → `org admin/owner + ancestor owner(2c)`，三层叠加而非替换，逻辑自洽（`09-ticket §5.2`）。
- 读改写分离（L2 读扩大、L3 写收紧）：策略 B 下"兄弟虚拟组可读不可改"——符合企业工单"部门可见、仅属主可改"的通用协作模型（如 Jira project 可见性 + 角色权限）。
- 委托防提权矩阵：`04-org-delegation §3`（文档抽样，引用完整）覆盖 org admin/owner / ancestor owner / vg admin 跨组隔离。

**发现的问题：**
- **[低风险] `CreateRelation` 双端 update 鉴权的边界**：`09-ticket §API` 注"关联视为修改操作，复用 update 权限码，对 source/target 双端均要求 update"。这在 2b 把 update 收窄为"仅创建人"后（`RK-11`），意味着**只有工单创建人能建立关联**，处理人不能。对"协作关联"场景可能偏严，但属有意为之的防御性设计，建议在 2c 文档明确"关联权限归属创建人"以避免误解。
- **[低风险] 状态推进端点缺失**：`00 §9 BK-10` 已登记——种子状态机含 `in_progress`/`pending_verify`，但 2a API 无"开始处理/待验证"端点，assigned 工单须先取消分派回 open 才能关闭（T6 固化）。属产品决策待拍板，非缺陷。

**风险等级：低。改进建议：** 在 04/09 文档补全"关联权限归属"与"状态推进端点"两项产品决策的落点（已部分登记，建议显式 SSOT）。

---

## 5. 开发阶段目标与边界

**结论：阶段目标明确、边界清晰，是本套文档最成熟的部分之一。**

- Step 0–10 + 三轨拆分（2b-core / 2b-org / 2b-ext）：`00 §1 P2-D7` 把原 2b 的"核心可见性"与"增强（虚拟组/附件/HR/密码）"解耦，关键路径最短化为 `2a → 2b-core → 2c`，符合"先交付能跑的工单模块"的增量交付原则。
- 每 Step 标注"做什么/不做/迁移号/验收用例/落点 PRD"，可执行性强。
- 里程碑门禁（M2a-0 ~ M2c-3）只列"该里程碑新增可测用例"，不重编号，防交叉引用漂移。

**发现的问题：**
- **[低风险] Step 与子阶段编号两套管**：文档同时用"Step 1-10"和"2a/2b-core/2b-org/2c"两套命名，新人需在 `00 §2` 表格中来回映射。建议在每份 PRD §0 直接标注"本 PRD 覆盖 Step X–Y（里程碑 Mx）"，减少跳转。

**风险等级：低。改进建议：** PRD §0 统一标注 Step+里程碑双编号。

---

## 6. 文档一致性与演进平滑性

**结论：一致性整体强（SSOT + 变更记录 + 交叉引用），存在少量单点不一致待修。**

**一致性亮点：**
- 每份 PRD 顶部声明"本文为 XX 实现 SSOT"，交叉引用表完整（`09-ticket §11`、`02-authz §7`）。
- `00 §8 SSOT 映射总表` 把 Step↔里程碑↔PRD↔用例↔迁移 五维对齐，极强。
- 变更记录（`00 §10`、各 PRD 修订行）保留决策演进脉络，可审计。

**发现的不一致（按严重度）：**
| # | 不一致点 | 位置 | 严重度 | 建议 |
|---|---------|------|--------|------|
| C1 | 2c 阶段编号：`09-ticket §0 L3` 写 "Step 9（2c Authorize）"，而 `00 §8 L329` 与 `00 §9 L336` 已明确 2c = **Step 8–10** | `09-ticket.md:3` vs `00-implementation-plan.md:329,336` | 低 | 改为 "Step 8–10（2c Authorize）" |
| C2 | `02-authz §2.5` 代码示例 `registry.Register(NewResource(s.repo, s.scope))` 传 `scope` 参数，但 2a Step 2 重构后代码为 `NewResource(s.ticketRepo)`（已去掉 scope 参数） | `02-authz-resource.md:216-220` vs `internal/service/ticket/resource.go` | 中 | 同步示例为 `NewResource(s.ticketRepo)` |
| C3 | `docs/ops/` 目录仅有 `README.md`，无 `deployment.md`（若 ops/README 引用了该文件则为断链） | `docs/ops/` | 低 | 核实 ops/README 引用；补 deployment.md 或修正引用 |
| C4 | `02-authz §2.2` 画出完整 `ScopeResolver` 接口，与 2a 实际仅 `HasRole` 辅助不符（见 §2） | `02-authz-resource.md:90-115` | 低 | 接口示例标注"（2b 待实现）"或同步 2a 形态 |

**演进平滑性（第 6 项要求）：** 优秀。
- Phase 1（认证鉴权）→ Phase 2（工单业务能力）→ Phase 3（生产加固+工单闭环）递进自然；
- Phase 3 整体暂缓但"设计就绪"（`phase3/README` 10-ticket-business / SLA / 审批流已设计），且明确"工单主链路已在 2a/2b 实现，SLA/通知/审批流属 Phase 3"——无重复建设、无断层；
- 触发信号量化表（`phase3/README §0`）把"暂缓"变为"有触发条件的等待"，避免无限期搁置或盲目启动，符合精益/演进式架构。

**风险等级：低（C2 为中，但仅文档示例误导）。改进建议：** 在 2a 收口 PR 中一并修 C1–C4（同 PR 修文档，符合 `00 §5.4` 文档纪律）。

---

## 7. 测试用例完整性与场景覆盖

**结论：测试策略完整，正常/异常场景覆盖充分；存在少量未自动化登记的边界。**

- **已 PASS（2a 收口）**：T1–T7（`09-ticket §8`）、R1–R8（`02-authz §4`）、state_machine 9 例、Phase 1 27 例回归；`00 §10 变更记录` 记载 **153 用例全绿（M2a-3 达成）**。
- **异常场景覆盖好**：404（不可见）/ 403（可见无权）/ 400（非法状态转换 90002）/ 409（已关闭 90004）/ 密码同文案防枚举——均有专门用例。
- **2b/2c 用例**：R9–R12 / T8–T14 / D1–D12 用 `⏳` 明确标记延期 Step，不误判失败，符合"里程碑只列新增可测"纪律。

**发现的问题：**
- **[中风险] 已识别但未自动化的边界（BK 系列）**：`00 §9` 登记 BK-1~BK-10（is_internal 过滤、反向关联判重、TOCTOU close、字面量常量化、move 级联后代分支无 Go 测试等）。这些**已在代码/文档审查中识别并排期**，但尚未转化为自动化用例。若 2b 实现时遗漏，回归无法拦截。
- **[低风险] `setupTicket2a` 死代码回退分支**（BK-9）：`if orgID==0` 不可达且 ON CONFLICT 冲突时 Scan 报错——测试辅助代码隐患，建议清。

**风险等级：中（BK 需转化为验收门禁）。改进建议：** 把 BK-1/3/5/6 直接写入 2b Step 4/5 的验收脚本（而非仅 backlog），确保"识别即拦截"。

---

## 8. 部署方案与 Makefile 工程化

**结论：Makefile 可用、合理；部署目录真实存在；存在少量工程化打磨点。**

- **Makefile 验证**：`.PHONY` 完整，`build/run/dev/wire/migrate/docker-*/swag/lint/test*` 目标齐全。`deployments/` 目录真实存在（`docker-compose.dev.yaml` / `docker-compose.yaml` / `Dockerfile` / `redis/`），Makefile 的 `docker-dev-*` 引用有效。✅
- **测试分层清晰**：`test-unit`（`-race`）/ `test-integration`（`-tags=integration`，testcontainers PG+MinIO）/ `acceptance` 脚本分段——对齐 `00 §5.2`。
- **gofmt 门禁**：`lint` 目标含 `gofmt -l` 漂移检测并 exit 1，质量纪律好。

**发现的问题：**
- **[中风险] 迁移连接串硬编码**：`Makefile:26,29` 写死 `postgres://zhuzhao:zhuzhao_dev@localhost:5432/zhuzhao`（明文密码 + 固定 host）。多环境（staging/prod）或不同开发者需改 Makefile，违反 12-factor（配置外置）。建议抽取为 `MIGRATE_DSN ?= ...` 变量或读 `.env`。
- **[低风险] integration 测试前置依赖未文档化**：`test-integration` 依赖 Docker（testcontainers），但 README/Makefile 注释未说明"需本地 Docker daemon"。新人易 `make test-integration` 失败。
- **[低风险] `docker-dev-reset` 粗暴清数据卷**：`down -v` 删所有本地 dev 数据——个人项目可接受，但建议在 README 标注"不可逆"。
- **[低风险] `docs/ops/deployment.md` 引用断链风险**：`docs/ops/` 仅有 README，若其引用 deployment.md 则断链（C3）。实际部署以 `deployments/` 为准，与 docs 解耦，建议 ops/README 明确指向 `deployments/`。

**风险等级：中（硬编码 DSN 为最实际改进项）。改进建议：** DSN 外置为变量/`.env`；README 补 integration 测试需 Docker 的说明。

---

## 9. 代码实现 ↔ 文档一致性

**结论：高度一致。前序审查（2a Step 0–3）发现的问题已被修复并文档化收口。**

- `00 §9` 明确记载：P0×1 / P1×6 已全部修复（迁移合并、authorize 404/403/500 三路语义、swagger 重生成、gofmt 门禁、模板预填、软删例外回标），**153 用例全绿**。即前几轮对话提出的 is_internal / 404-403 语义 / 模板覆盖 / Assign 状态机 / SoftDelete 命名等问题，均已在代码落地并在文档登记。
- 路由风格（`POST /tickets/update` 动作路径而非 RESTful PATCH）：文档 `09-ticket §1.1` 解释为"menu_apis L1 鉴权依赖操作类 POST 路径不含 :id"，代码一致——**有意为之，合理**，非偏差。

**仍存的偏差（含已识别待办）：**
| # | 偏差 | 现状 | 判断哪方更合理 |
|---|------|------|----------------|
| D1 | `scope_resolver.go` 仅 `HasRole`，但 `02-authz §2.2` 画了完整接口 | 2b 待办，文档已标 2b | 代码合理（2a 只需 HasRole）；文档示例应标注 2b |
| D2 | `02-authz §2.5` `NewResource(repo, scope)` vs 代码 `NewResource(ticketRepo)` | 代码已重构去 scope | **代码更合理**（scope 逻辑内联进 resource）；文档 C2 需同步 |
| D3 | `is_internal` 备注不过滤 | 代码当前返回全部；BK-1 已登记 2b 前置 | 文档更合理（应过滤）；属已排期修复 |
| D4 | `09-ticket §0` "Step 9" vs 实际 2c=Step 8-10 | 00 已正确 | 以 00 为准；09 §0 需改（C1） |

**业界对照：** 工单可见性"assigned + 组织快照"对齐 Jira/ServiceNow；三层鉴权对齐 ACL 行级过滤 + RBAC；事件机制 L1→L2（Outbox+Asynq）对齐微服务可靠事件分发（如 Uber Cadence / Temporal 思路的轻量落地）；Phase 3 暂缓微服务化、先领域目录隔离——对齐 Martin Fowler "Monolith First" 与 "演进式架构"（不提前分布式）。**整体取向稳健，未过度设计。**

---

## 10. 综合风险矩阵

| 风险 | 等级 | 类型 | 触发条件 | 建议处置 |
|------|------|------|----------|----------|
| is_internal 备注不过滤（BK-1） | 中 | 安全 | 2b 策略 B 扩大读者后忘做 | 提为 2b Step 4 验收门禁 |
| Makefile 迁移 DSN 硬编码 | 中 | 工程化 | 多环境/协作 | 外置为变量/`.env` |
| 文档示例未同步重构（C2/D2） | 中 | 一致性 | 新成员按文档实现 | 2a 收口 PR 同修 |
| BK 系列未转自动化用例 | 中 | 测试 | 2b 实现遗漏 | 写入 2b 验收脚本 |
| Casbin 无 Watcher | 中 | 安全/可靠 | 多实例部署 | Phase 3 启动（已记录） |
| update 审计断档 + TOCTOU（BK-3） | 低-中 | 可靠/审计 | 并发 close+patch | 2b 条件更新 |
| 09 §0 / 02 §2.2 / ops 引用 不一致（C1/C3/C4） | 低 | 一致性 | 误导 | 文档同步 |
| 目标分散多文件 | 低 | 可用性 | 新成员上手 | 补一页索引 |
| 状态推进端点缺失（BK-10） | 低 | 产品 | 待拍板 | 2b 拍板 |

---

## 11. 改进路线图（按优先级）

**P0（建议本迭代完成，低成本）：**
1. 修 C2：`02-authz §2.5` 示例改 `NewResource(s.ticketRepo)`。
2. 修 C1：`09-ticket §0` 2c 改 "Step 8–10"。
3. Makefile DSN 外置为 `MIGRATE_DSN ?= ...`（含 `.env` 读取说明）。

**P1（2b Step 4 收口时一并）：**
4. 将 BK-1（is_internal 过滤）作为 2b Step 4 **验收门禁**（非仅 backlog）。
5. 将 BK-3（TOCTOU 条件更新）、BK-5（反向关联判重）、BK-6（move 级联后代测试）写入 2b 验收脚本。
6. 修 C4：`02-authz §2.2` 接口示例标注"（2b 待实现）"。

**P2（文档打磨，可并行）：**
7. README 顶部补"目标→阶段→关键决策"一页索引。
8. `00 §4` 补模块依赖有向图。
9. 核实 `docs/ops/README` 引用，补 `deployment.md` 或修正指向 `deployments/`。
10. PRD §0 统一标注 Step+里程碑双编号。

---

## 12. 总体结论

文档体系在**目标边界、模块划分、安全设计、阶段拆分、一致性纪律、测试策略、演进平滑性**七个维度均达到甚至超过同类项目水准，尤其是：
- 三层鉴权（L1/L2/L3）分层论证专业，对齐 Zanzibar/ACL 行级过滤；
- P2-D6 "工单跟人走 + 组织快照"对齐 Jira/ServiceNow 范式并固化为决策；
- SSOT + 变更记录 + 五维映射表使大规模文档可维护；
- Phase 3 触发信号量化表把"暂缓"变为可管理的演进状态。

**主要短板不在设计，而在"收口同步"**：少量文档示例未随 2a 重构更新（C2/D2）、个别安全/测试边界已识别但未固化为门禁（BK-1/3/5/6）、工程化配置（DSN 硬编码）待外置。均为低风险到中风险的"打磨项"，不影响架构正确性，建议按第 11 节路线图在 2b 收口前完成。

> 审查产出：本报告 + 前序 `docs/review/00~05`（phase1）。代码侧验证基于对 `internal/service/ticket/*`、`ticket_repo.go`、`ticket_handler.go` 的精读，与 `00 §9` 登记的 2a 修复结论一致。
