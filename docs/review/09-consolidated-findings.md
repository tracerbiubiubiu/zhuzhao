# 09 · 审查问题合集（最终版）

> **定位**：对 `05/06/07/08` 四份审查报告 + 两轮去重验证（原 `09-findings-consolidated-and-verified.md` 与本文 v1.0）的**最终合并问题合集**，为唯一执行依据。前序五份报告保留为档案，不再修改。
> **验证基线**：HEAD `ebb5635`（2026-08-28 复核）。每条均经磁盘/代码实测取证。
> **来源对照**：05=`05-docs-systematic-review`，06=`06-phase2-docs-architecture-review`，07=`07-comprehensive-docs-review`，08=`08-final-recheck`，C=`09-consolidated-findings v1.0`（已并入本文后删除）。

**统计**：去重后开放问题 **31 条**（P0×9 / P1×11 / P2×11）；已修复关闭 4 条；误报/勘误 5 条；信息性 4 条。

---

## 一、P0 —— 2b 开工前必须（9 条）

| ID | 级别 | 问题 | 验证锚点（实测） | 来源 |
|----|------|------|-----------------|------|
| F-01 | 🔴高 | VISION 把 2a/2b 交付项写成「Phase 1 已完成」（资源级鉴权/虚拟组/HR 同步） | `VISION.md:10` | 08-V-1=C-H1 |
| F-02 | 🔴高 | VISION「migrations 现状至 000009」过期（实际至 000016） | `VISION.md:44` | 05-G-1=07-N5=C-D1 |
| F-09 | 🟠中 | 02-authz §2.5 示例三重失真（`NewTicketResource(s.repo, s.scope)` vs 实际 `NewResource(repo)`） | `02-authz-resource.md:218` vs `resource.go:31` | 06/07-C2=C-D2 |
| F-10 | 🟠中 | architecture 同文件 L2/L3 顺序两说（:558 旧短路序 vs :637 路径 A） | `architecture.md:558,637` | 08-A-1=C-H3 |
| F-18 | 🟠中偏高 | Redis requirepass「Phase 2 加」承诺到期未兑现，dev compose 暴露 6379 | `redis.conf:7-8`、`docker-compose.dev.yaml:31` | 08-S-1=C-H2 |
| **F-24** | 🟠P1 | **`make acceptance` 目标不存在**，而 `00 §5.5` 明文承诺该入口——本地回归纪律开箱即坏 | Makefile 无该 target | 05-E-1（**C-v1 路线图遗漏，恢复**） |
| **F-25** | 🟠P1 | **验收脚本默认容器名 `zhuzhao-postgres`** 与 dev compose 实际 `zhuzhao-dev-postgres` 不匹配，按 README §1.6 工作流首组断言即崩 | `acceptance-phase1.sh:7` 等 | 05-E-2（**C-v1 路线图遗漏，恢复**） |
| F-19 | 🟠中 | JWT Secret 双通道注入命名不一（compose `APP_JWT_SECRET` vs BindEnv `JWT_SECRET`），靠 AutomaticEnv 推导兜底 | `docker-compose.yaml:77`、`config.go:161` | 08-E-2=C-M2 |
| F-03 | 🟡低 | roadmap:98 引用 `doc/soar/activelist.md` 死链 | `roadmap.md:98` | 05-G-2=C-L1 |

## 二、P1 —— Phase 2 内偿还（11 条）

| ID | 级别 | 问题 | 验证锚点 | 来源 |
|----|------|------|---------|------|
| F-31① | 🟠中 | is_internal 备注过滤未列为 2b Step 4 验收门禁（2a 无泄露，2b 扩大读者后成真漏洞） | `service.go` ListComments 无过滤；BK-1 已登记 | 四报共识=C-M1 |
| F-31② | 🟠中 | **90004 死码**：close 后再 close/assign 应 409，全仓零用例触发 | grep 测试+脚本零命中 | 05-T-2（**C-v1 排除理由不实，恢复**） |
| F-31③ | 🟠中 | 并发 close 同一工单（TOCTOU 窗口）无测试 | 集成测试无并发用例 | 07-T5=C-T3 |
| F-31④ | 🟡低 | CreateRelation 越权负向用例缺失 | 集成测试无 Relation 用例 | 05-T-4 |
| **F-35** | 🟠中 | **鉴权链 DB 错误注入测试缺失**：Authorize 遇 DB 故障应上抛 500/503（而非静默拒绝），无 fake-repo 错误注入用例覆盖该分支 | `authz_resource_integration_test.go` 无注入用例 | 07-T4=C-T2（**合并新增**） |
| F-31⑤(BK-6) | 🟠中 | move 级联后代分支（subpath ELSE）零 Go 测试 | repository 测试仅角色级联 | 05-T-1=07-T1=C-T1 |
| F-31⑥(BK-3/5) | 🟠中 | Update 不写 ticket_events + closed TOCTOU；反向关联判重 | BK-3/5 已登记 | 05/06/07=C-M5 |
| F-32 | 🟡中 | audit/user service 分支级单测缺失（集成已兜底核心分支，07-S5 由高降中） | service 包无 audit/user 独立单测文件 | 07-S5=C-§2.4 |
| F-20 | 🟠中 | DB sslmode disable 无 release 强制断言 | `config.yaml:20`、validate.go 无校验 | 07-S6=C-M4 |
| F-21 | 🟠中 | CORS AllowAll 属既定决策但收紧时机无管控绑定 | `cors.go:12`；不在 phase3 触发表/检查单 | 07-S4（08 改判中）=C-M3 |
| F-22 | 🟠中 | 备份恢复策略缺失（无 pg_dump 文档/Makefile 目标，VISION 未定职责） | ops 空壳、Makefile 无 backup | 05-S-2/G-3=C-L-ops |
| F-23 | 🟠中 | audit/ticket_events 防篡改零设计（无触发器/RLS） | migrations 无 TRIGGER | 05-S-1=C-M6 |
| F-27 | 🟠中 | Makefile migrate DSN 明文硬编码 | `Makefile:26,29` | 06/07-E1=C-E1 |
| F-26 | 🟠中 | compose app 服务缺 healthcheck/资源限制/日志策略 | `docker-compose.yaml` app 段 | 08-E-1=C-E2 |
| F-16 | 🟠中 | auth-design 提案偏差（`devices:SADD` 2b 才实现；`creator_id` 应为 `created_by`） | `auth-design.md:47,168` | 07-N6/D4/D5=C-D6 |
| F-17 | 🟠中 | 03-org-enhance 缺 §1 前置条件（六 PRD 唯一） | `03-org-enhance.md` 无该章节 | 07-N7=C-D7 |

## 三、P2 —— 打磨（可并行，11 条）

| ID | 问题 | 验证锚点 | 来源 |
|----|------|---------|------|
| F-04 | README:242 000013 仅列 `ticket_attachments`（实为两表） | `phase2/README.md:242` | 07-N4=C-D8 |
| F-05 | 09-ticket **三处**「90001 待定义」过期注记（:274/:381/:461，:381 措辞不同易漏检） | 实测三处均在 | 05-C-1=C-L2 |
| F-06 | 01-infra:276 `DATABASE_PASSWORD` 笔误（有效名 `DB_PASSWORD`） | `01-infra.md:276` | 05-C-2=C-L3 |
| F-07 | 已完成任务的待办时态未回标（wire.go:33 注释、00 Step 0 checkbox、02:53,280 G-2 时态、README:148） | 实测均在 | 05-C-3=08-E-7=C-L4 |
| F-08 | 09-ticket 残留 `000010_ticket_menu` 引用（文件已并入 000010_ticket） | 09-ticket.md 命中 | 05-C-4（**C-v1 遗漏**） |
| F-11 | roadmap/docs-README 未跟进 2b 三轨细分 | `roadmap.md:59-61` | 08-R-1=C-D4 |
| F-12 | hr-sync 头部未回标延后 2b-ext | `hr-directory-sync.md:5` | 08-H-1=C-D5 |
| F-13 | deployment-evolution 未回标「Phase 3 整体暂缓」 | `deployment-evolution.md:18-19` | 08-D-1=C-L7 |
| F-14 | proposal/overview 拓扑图仍画 IAM 拆分 + 定位句窄于 VISION | `overview.md:16,82-90` | 08-O-1=C-L9（**C-v1 漏拓扑图半条**） |
| F-15 | modules/ticket:625,729 引用不存在的 phase3/02-multi-instance | 实测两处 | 08-M-1=C-L8 |
| F-28/29/30 | reset sleep5+compose v1 / 无 CI·pre-commit / ops deployment.md 待创建 | `Makefile:33-44` 等 | 05/06/07=C-U6/E3/L5 |
| F-33/34 | 导航三件套（一页索引/依赖 DAG/PRD 双编号）+ 153 口径措辞 + 第三方登录设计占位 | 00 §4、roadmap 预留表 | 05-M-1/T-3、06/07-N3=C-U1/2/3 |
| U-4/U-5 | swag CLI 全局依赖 + .gitignore `doc/` 错位；config 路径硬编码无 -c | `Makefile:53`、`main.go:17` | 08-E-3/E-4=C-U4/U5 |

## 四、已修复关闭（4 条，不再执行）

| 原观点 | 关闭依据 |
|--------|---------|
| 05-P-1：00 计划四处 2c 旧编号 9–11 | ✅ ebb5635 已统一 8–10 |
| 05-F-3：模板 DefaultFields 只落 priority | ✅ 已失效：1804325 实现完整预填（service.go:109-134 + 5 测试） |
| 06/07-C4、C-v1-L6：02 §2.2 接口图未标注 2a 形态 | ✅ ebb5635 已标注「ScopeResolver（2a 桩）」+ 方法级 2b 注记，诉求已满足 |
| 05-C-3 主引用：scope_resolver 路径漂移 | ✅ 主引用已清零（残余归 F-07 时态回标） |

## 五、误报 / 勘误（5 条，裁决记录）

| 观点 | 裁决 |
|------|------|
| **06-C1/07-C1、C-v1-D3**：「09 §0 L3/L14 写 Step 9 与 00 的 2c=8–10 矛盾，应改为 8–10」 | **不成立**。实测 L3 与 L14：09 只覆盖 ticket 相关 Step，2c 三步（8 委托/9 Authorize/10 验收）中本 PRD 对应的恰是 Step 9，引用正确。**按其建议改为 8-10 反而会改错文档**——该条从路线图移除 |
| 05-S-3：黑名单 TTL 未量化 | 不成立：`modules/auth.md:46` 明确「TTL: AT 剩余有效期」 |
| 06/07 §7：「409（90004）有专门用例」 | 不实：grep 零命中，90004 为死码（恢复为 F-31②；C-v1 沿用其排除并附「脚本含 close 后再操作断言」的理由，实测脚本无此断言，理由不成立） |
| C-v1-D1 锚点细节：「migrations 目录有 000011–000014 空文件」 | 勘误：`ls migrations` 无这些文件；F-02 问题本身属实，锚点细节更正 |
| 07-S5「高」：940 行服务层无守护 | 降级为中：集成 + redis Lua 测试已覆盖禁用/吊销/防提权/限流核心分支，缺的是分支级单测（F-32） |

## 六、信息性 / 决策类（4 条，非缺陷）

1. **Casbin 无 Watcher**：已声明 Phase 3 触发条件，单实例下正确 tradeoff（06/07 共识）。
2. **路由风格 `POST /tickets/update`**：menu_apis L1 鉴权依赖动作型 POST 的有意设计，文档已解释。
3. **失步方向全部为「代码已完成、文档待回标」**，无「文档超前误导实现」的反向危险（05 判断经 F-16 复核仍成立，auth-design 为最接近案例已单列）。
4. **CORS/Redis 两案例的流程教训**：「Phase 2 加 X / 上线前改 Y」类带时间承诺必须落入检查单/触发表（已体现于 F-18/F-21 处置方式）。

---

## 七、修复路线图（最终版，取代所有前序路线图）

### 批次一：2b 开工前（≈2h，P0 九条）
1. F-01+F-02 VISION 两处同批修；2. F-10 architecture §4.1 收口路径 A；3. F-09 示例改真实签名；4. F-18 Redis requirepass 全链路；5. F-24 `acceptance:` target + F-25 脚本容器名探测（两处开箱即坏）；6. F-19 JWT 命名统一（建议改 compose 传 `JWT_SECRET` 短名，同步 .env.example）；7. F-03 死链改指 ADR-003。

### 批次二：随 2b-core/org 收口（P1）
8. F-31①⑤②③④⑥ 逐项写入 2b 验收脚本与集成测试（BK-1 为 Step 4 门禁；补 90004/并发 close/关联越权/DB 注入/级联后代用例）；9. F-32 audit/user 单测（Step 7 前置）；10. F-20 sslmode 断言 + F-23 防篡改触发器；11. F-22 备份最简版；12. F-27 DSN 外置 + F-26 compose app healthcheck；13. F-16/F-17 提案对齐两件；14. F-11~F-15 叙事回标。

### 批次三：打磨（可并行，P2）
15. F-04/05/06/07/08 五处文档单点修正；16. F-28 compose v2 化 + F-29 pre-commit + F-30 ops 骨架；17. F-33/34 导航与占位；18. U-4/U-5 工程化小项；19. F-21 CORS 转轨登记（phase3 触发表 + 上线检查单）。

### 进入 2b 判定
架构主线经 153 断言端到端实证与代码一致，无代码级 blocker；**批次一 9 条完成后即可启动 2b-core/2b-org 编码**，批次二与 2b 功能并行，批次三不阻塞。
