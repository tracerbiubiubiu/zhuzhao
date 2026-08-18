# Phase 1 编码前决策对照表

> 开写正式实现前，对 **文档已定、骨架代码未对齐** 的两处契约做拍板对照。  
> SSOT：`architecture.md` §16/§17、`api/errcode.md` §5、`proposal/data-init.md` menu_apis。  
> 创建日期：2026-08-14

---

## 决策 1：POST 写操作 — id 放 body 还是 path？

### 背景

| 来源 | 约定 |
|------|------|
| **文档（已定）** | GET 详情可用 `GET /resource/:id`；**POST 更新/删除/状态变更** 用动词子路径，**id 放 body** |
| **种子数据** | `menu_apis` 已按 body 版路径写入（如 `/api/v1/users/update`） |
| **当前骨架** | ~~`router.go` 使用 `POST /:id/update`~~ → **已对齐** `POST /users/update` 等（id 在 body） |

### 方案对比

| 维度 | **A. id 放 body（推荐，与文档一致）** | B. id 放 path（当前骨架） |
|------|--------------------------------------|---------------------------|
| 路由示例 | `POST /users/update` body `{ "id": "1", ... }` | `POST /users/1/update` |
| 与 menu_apis / Casbin | ✅ 种子与 architecture §17 已对齐 | ❌ 策略路径不匹配，需改种子或改 Casbin 规则 |
| 与 GET 详情 | ✅ GET 仍用 `GET /users/:id`（GET 无 body） | ✅ 同样可用 |
| 前端对接 | 写操作统一 POST + JSON body | 写操作需拼 path 参数 |
| Gin 路由 | 动词路径更稳定，Casbin key 不含动态 id | path 含 id，Casbin 需 `keyMatch2` 匹配 |
| 变更面 | 改 `router.go` + handler 取 id 方式 | 改 `data-init.md` menu_apis + architecture §17 + 全部 phase1 文档 |

### 推荐决策

**采用方案 A：POST 写操作 id 放 body，GET 详情保留 path 参数。**

理由：文档、种子、验收路径已按 A 编写；改骨架比改全库文档和 Casbin 种子成本低且风险小。

### 采纳 A 后的路由对照（骨架已对齐 ✅）

| 模块 | 文档 / 种子 | skeleton | 状态 |
|------|-------------|----------|------|
| 用户 | `POST /users/update` 等 | `POST /users/update` 等 | ✅ |
| 角色 / 菜单 / 组织 | 同上 body id | 同上 | ✅ |
| 组织 move | `POST /orgs/move` body `{ id, parent_id }` | `POST /orgs/move` | ✅ |

**保持不变（GET + path）**：

- `GET /users/:id`、`GET /roles/:id`、`GET /menus/:id`、`GET /orgs/:id`
- `GET /users/:id/orgs`、`GET /roles/:id/menus`、`GET /roles/:id/permissions`

### 请求体示例（方案 A）

```json
// POST /api/v1/users/update
{ "id": "1", "real_name": "张三", "email": "a@b.com" }

// POST /api/v1/users/delete
{ "id": "1" }

// POST /api/v1/users/roles
{ "id": "1", "role_ids": ["2", "3"] }

// POST /api/v1/orgs/move
{ "id": "2", "parent_id": "3" }
```

> id 字段 JSON 使用 string（`,string` tag），与 BIGINT 精度约定一致。

### 拍板

| 选项 | 说明 |
|------|------|
| ☑ **A（已采纳）** | POST 写操作 id 放 body；按上表改 router |
| ☐ **B** | 维持 path id；需回改 data-init menu_apis + architecture §17 + phase1 各模块路由表 |

---

## 决策 2：Casbin 无角色 — 返回 70001 还是 70003？

### 背景

| 来源 | 约定 |
|------|------|
| **errcode.md §5** | 验收路径 #15：**无角色**访问鉴权路由 → **403 + 70003**（`ErrNoRoles`） |
| **errcode 语义** | `70003` = 未分配角色；`70001` = 已分配角色但 **无该路由权限** |
| **当前 skeleton** | ~~70001~~ → **已对齐** `ErrNoRoles`（70003） |

### 方案对比

| 场景 | **推荐（区分码）** | 统一 70001 |
|------|-------------------|------------|
| 用户 **零角色**，访问 `GET /users` | **403 + 70003**「未分配角色」 | 403 + 70001「无权限」 |
| 用户有 `viewer`，访问 `POST /users` | **403 + 70001**「无权限」 | 同左 |
| 前端分支 | 70003 → 引导联系管理员分配角色；70001 → 权限不足 | 无法区分 |
| Phase 1 验收 #15 | ✅ 通过 | ❌ 不通过 |

### 推荐决策

**区分使用：**

```text
len(roles) == 0     → 403 + 70003 (ErrNoRoles)
Casbin 全部 deny    → 403 + 70001 (ErrNoPermission)
```

### 实现要点（Casbin 中间件）

```go
if len(roles) == 0 {
    response.ForbiddenWithCode(c, errcode.ErrNoRoles) // 70003
    return
}
// ... enforce ...
if !allowed {
    response.ForbiddenWithCode(c, errcode.ErrNoPermission) // 70001
    return
}
```

`errcode.go` 需补充：

```go
ErrNoRoles = New(70003, "未分配角色")
ErrServiceUnavailable = New(10008, "服务暂时不可用") // fail-close 另项
ErrCannotRemoveLastSuperadmin = New(30006, "不能移除最后一个超级管理员")
```

> `response` 包需支持返回 `{ code, message }` 而不仅是 message 字符串（若尚未实现）。

### 拍板

| 选项 | 说明 |
|------|------|
| ☑ **区分 70003 / 70001（已采纳）** | 与 errcode.md 验收一致 |
| ☐ 统一 70001 | 需改 errcode.md §5 验收表及 phase1/README §1.3 #15 |

---

## 决策 3（附带）：与上两项同批对齐的骨架项

编码 Step 1 建议与决策 1、2 **同一 PR / 同一迭代** 完成，避免半套契约：

| 项 | 文档 | 骨架现状 | 状态 |
|----|------|----------|------|
| 审计 log 主键 | BIGINT | BIGINT | ✅ |
| UserRepo.FindByID | `int64` | `int64` | ✅ |
| POST 路由 id 位置 | body | body | ✅ |
| 70003 / 70001 | 区分 | 已实现 | ✅ |
| Redis fail-close | 503 + 10008 | JWT/Casbin 已实现 | ✅ |
| `user:disabled` | 403 + 30003 | JWT 已实现 | ✅ |
| Casbin adapter | PG | 内存 TODO（Step 5 切换） | 📋 待实现 |
| `migrations/` | 000001–000002 | 目录可能为空 | 📋 Step 1 |
| 组织成员 / users/orgs 路由 | architecture §17 | router 可能缺 4 条 | 📋 Step 1/9 |
| `roles.priority` / `deleted_at` | 05-role DDL | model.Role 可能缺字段 | 📋 Step 1 |
| `user_orgs` 无 role_id | 04-user DDL | model.UserOrg 可能有 role_id | 📋 Step 1 |
| `ErrMultipleAuthMethods` 20008 | Phase 1 验收 #22 | errcode.go 可能未定义 | 📋 Step 4 |
| `internal/pkg/resource/` | Step 5 空 Registry | 目录可能不存在 | 📋 Step 5 |
| PG 连接池 | pgxpool + Ping + Close | 部分（lifetime 写死） | 📋 Step 1 补可配置项 |
| Redis 连接池 | PoolSize/超时/Ping/Close | 仅 Addr/Password | 📋 Step 1 补全 |
| `/health/ready` | 不暴露 DB/Redis 原文 | 只返回 component | ✅ |

---

## 推荐阅读顺序（开写前）

1. [00-pre-coding-decisions.md](./00-pre-coding-decisions.md) — 决策 1–4 均已关闭  
2. [README.md](./README.md) §1.3 + **§2.3 里程碑** — 按 M1→M7 推进，勿死跟旧线性 6→7→8→9  
3. [../api/errcode.md](../api/errcode.md) — code 清单  
4. [../proposal/data-init.md](../proposal/data-init.md) — menu_apis、种子  
5. 按批次打开 `01`~`10` 对应文档；**10-concurrency** 在 Step 5+ 写 AssignMenus 时必读  

---

## 决策 4：自服务路由 / HTTP 200 / 种子 role_menus（2026-08-17）

| 项 | 决策 | SSOT |
|----|------|------|
| 自服务路由 Casbin | **中间件白名单**（业界主流 A 类） | [authz §2.2.1](../modules/authz.md#221-自服务路由业界做法--本项目决策)、[09-middleware §Casbin](./09-middleware.md#casbin-中间件-g-表消除) |
| Create 成功 HTTP | **200** | [api/response.md](../api/response.md)、[04-user §Handler 测试](./04-user.md#handler-层httptest) |
| 种子 role_menus | `operator`/`viewer` **空**；`superadmin`/`admin` 全 IAM 菜单（含用户/角色/组织） | [data-init §4.2](../proposal/data-init.md#42-种子数据内容)、[01-infra §种子](./01-infra.md#种子数据内容) |

---

## 变更记录

| 日期 | 说明 |
|------|------|
| 2026-08-14 | 初版：路由 id 位置 + 70003/70001 对照 |
| 2026-08-14 | 采纳 A + 70003/70001；skeleton 已对齐（router、errcode、middleware、audit BIGINT） |
| 2026-08-17 | 补充无需拍板项：migrations DDL、组织路由、model 对齐、20008、ResourceRegistry 目录 |
| 2026-08-17 | 决策 4：自服务白名单 + Create HTTP 200 + operator/viewer 空菜单 |
| 2026-08-17 | README §2 里程碑 M1–M7、Step 5+ 并行、验收 #14b–#15b/#25–#27 |
