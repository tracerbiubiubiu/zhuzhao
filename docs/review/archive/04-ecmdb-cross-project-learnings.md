# 跨项目借鉴复核：Duke1616/ecmdb（2026-08-26）

> 目的：全面复核 `github.com/Duke1616/ecmdb`（CMDB+工单一体化，Go+MongoDB，RBAC/IAM/工作流/定时/告警均来自外部库 eiam/easy-workflow/etask/ealert），对比 zhuzhao 当前模块，找出**值得借鉴**的点。
> 工作流引擎部分单独见 `phase3/10-ticket-business.md §4.8`。
> 结论先行：**ecmdb 在 IAM/组织/审计/中间件 四块弱于 zhuzhao（它把 RBAC 全委托给 eiam、用物理删除、无脱敏审计）；zhuzhao 自研的 Casbin+ResourceRegistry+ltree+软删+脱敏审计 更成熟。可借鉴点集中在 zhuzhao 尚未建或待增强的领域。**

---

## 1. ecmdb 模块全貌 vs zhuzhao

| ecmdb 能力 | 实现位置 | zhuzhao 现状 | 谁更成熟 |
|-----------|---------|-------------|---------|
| 模型驱动资源（动态字段/EAV） | 自研 `domain`+Mongo Map inline | 无（工单字段固定 + workflow JSONB） | ecmdb 有此能力，zhuzhao 暂不需要 CMDB |
| 拓扑关系图（递归遍历） | Mongo `$graphLookup` | 无拓扑概念（非 CMDB） | 不适用 |
| RBAC / IAM / 租户 | **外部库 eiam**（本仓仅 `TenantID` 字段） | 自研 Casbin + ResourceRegistry + 角色/菜单/组织角色 | **zhuzhao 更成熟** |
| 组织层级树 | 无（仅扁平 ModelGroup；租户在 eiam） | 自研 ltree 组织树 + 虚拟组 + HR 同步 | **zhuzhao 更成熟** |
| 审计日志 | 仅 `utime` + Kafka 加密变更事件，**无谁改了什么** | 自研 `audit_logs` + 脱敏 + action 注册 + 分级 | **zhuzhao 更成熟** |
| 软删除 | **物理删除**（无 `deleted_at`） | 全表 `deleted_at` 软删 | **zhuzhao 更成熟** |
| 统一响应/错误处理 | `ginx.Result`+`ErrorCoder` | 自研 `pkg/response` 信封 | 持平 |
| 依赖注入 | Google Wire | - | 见 §3 |
| 批量导入导出 | Excel 三元表头模板 | 暂无 | 见 §2.3 |
| 配置即代码（bootstrap） | YAML 幂等加载 + 版本化 init + dry-run | SQL 迁移 `ON CONFLICT DO NOTHING` 种子 | 见 §2.1 |
| 事件补偿（增量+去抖） | `ListBeforeUtime` + per-key worker | Asynq 重试（无增量补偿模式） | 见 §2.2 |

---

## 2. 值得借鉴的点（落到 zhuzhao 的具体建议）

### 2.1 业务配置"配置即代码 + 版本化幂等 init"（高价值，直接相关）
- ecmdb：`bootstrap/loader.go` 读 YAML（模型/字段/关系）→ 幂等创建；`cmd/initial` 支持版本化增量迁移 + dry-run。
- zhuzhao 当前：workflow_definitions / SLA 规则 / 角色菜单 用 SQL `ON CONFLICT DO NOTHING` 种子（可用但非声明式、不可 dry-run、版本语义弱）。
- **建议**：把"工单流程定义 / SLA 规则 / 系统角色菜单"建模为**版本化、幂等、可 dry-run 的配置即代码**（YAML/JSON 载入器），与 §4.7 的 `PUT /workflows/:code` 管理编辑同源。避免手改 SQL 种子导致环境漂移。Phase 3 工单落地时优先采用。

### 2.2 事件驱动的"增量 + 按 key 去抖"补偿模式（高价值，契合 Asynq）
- ecmdb：字段加密变更 → 发事件 → 消费者按 `utime` 增量扫描历史 + **per-key worker 协程（空闲退出）** 合并去抖，避免全量重算。
- zhuzhao：Asynq 已有失败重试，但**重活（SLA 全量重算、批量通知、组织树级联）缺"增量+去抖"模式**，易触发全量重扫。
- **建议**：SLA 重算 / 批量通知类 Asynq worker 借鉴此模式——按 `updated_at` 游标分批 + 按业务 key（如 org_id / ticket_type）合并同批次重复触发。与 ADR-002 的 Asynq 定位一致。

### 2.3 Excel 三元表头导入导出模板（中价值，按需）
- ecmdb：`dataio/buildExcel` 用 3 行表头（约束/UID/名称）+ 列排序 + 下拉数据验证 + 空模板导出。
- zhuzhao：暂无批量导入。若后续开放用户/组织/工单**批量导入**，直接套用此模板约定，降低前端对接成本。

### 2.4 动态字段（EAV）模式（中低价值，仅当工单需自定义字段）
- ecmdb：`Resource.Data Map` inline 实现无 schema 资源。
- zhuzhao：若未来工单需要**用户自定义字段**，用 `jsonb` + GIN 索引（非 Mongo Map），模式等价。当前工单字段固定，暂不急需；列为扩展点。

### 2.5 插件式拦截（AutoID/Tenant 自动注入）（低价值）
- ecmdb：Mongo 插件 `BeforeInsert` 自动注入 tenant_id / AutoID。
- zhuzhao：已有 `tenant_id`/`created_by` 审计列 + 审计中间件；DAO 层若想免重复注入可借鉴此拦截器思路，但非刚需。

---

## 3. 不借鉴 / zhuzhao 已更好的点（避免误抄）
- **RBAC/组织/审计**：ecmdb 委托外部库且用物理删除、无脱敏审计——**不要抄**，zhuzhao 自研方案更完整。
- **拓扑关系图 DSL（`pkg/plugin`）**：ecmdb 核心是 CMDB 资产拓扑，zhuzhao 无此领域，**不适用**（除非未来引入 CI/资产关联）。
- **Mongo `$graphLookup` 递归**：zhuzhao 用 Postgres ltree/递归 CTE，等价能力已具备，无需引入 Mongo。
- **统一响应/错误码**：zhuzhao 已有 `pkg/response`，模式一致，无需改。

---

## 4. 一句话总结
> ecmdb 的精华是"模型驱动 + 配置即代码 + 事件增量补偿 + Excel 模板"这套**可扩展内核工程手法**；其 RBAC/组织/审计反而是短板。对 zhuzhao，最该吸收的是 **2.1 配置即代码（落到 workflow/SLA 种子）** 与 **2.2 增量去抖补偿（落到 Asynq 重活）**，其余按需。
