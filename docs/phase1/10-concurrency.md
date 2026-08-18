# 10 - 并发与事务性（concurrency）

> **横切关注点**，贯穿 Step 5+ 所有写路径。AssignMenus / 用户创建 / 组织移动等 **必读**；里程碑与顺序见 [README §2](./README.md#2-实施顺序)。

---

## Phase 1 必须实现

Phase 1 是单实例部署，以下是在单实例环境下也必须正确处理的并发和事务问题：

### DB 事务（必须实现）

以下操作涉及同一 PostgreSQL 内的多表写入，必须在同一事务内完成：

| 操作 | 涉及表 | 事务保证 | 所在模块 |
|------|--------|----------|---------|
| 角色分配菜单 → Casbin 策略同步 | `role_menus` + `casbin_rule` | 同一事务，全成功或全回滚 | role |
| 用户分配角色 | `user_roles` | 单表批量操作，事务保证 | user |
| 创建用户 | `users` + `user_roles`（如有初始角色） | 同一事务 | user |
| 组织移动（path 递归更新） | `organizations` | 同一事务，含子节点 path 更新 | organization |
| 菜单变更 | `menus` + `role_menus`（如有） | 同一事务 | menu |

以"角色分配菜单"为例，这是最关键的事务场景：

```go
func (s *roleService) AssignMenus(ctx context.Context, roleID int64, menuIDs []int64) error {
    tx, err := s.pool.Begin(ctx)
    defer tx.Rollback(ctx)  // commit 前自动回滚

    // 1. 删除旧关联
    tx.Exec(ctx, "DELETE FROM role_menus WHERE role_id = $1", roleID)

    // 2. 插入新关联
    for _, menuID := range menuIDs {
        tx.Exec(ctx, "INSERT INTO role_menus (role_id, menu_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", roleID, menuID)
    }

    // 3. 查 role code（Casbin v0 用 role::{code}）
    role, err := s.roleRepo.GetByID(ctx, roleID)
    if err != nil {
        return err
    }

    // 4. 删除该角色的旧 Casbin 策略
    tx.Exec(ctx, "DELETE FROM casbin_rule WHERE v0 = $1", fmt.Sprintf("role::%s", role.Code))

    // 5. 根据新菜单的 menu_apis 生成新策略
    menus := s.menuRepo.GetByIDs(ctx, menuIDs)
    for _, m := range menus {
        for _, apiPath := range m.APIPaths {
            tx.Exec(ctx, "INSERT INTO casbin_rule (ptype, v0, v1, v2) VALUES ('p', $1, $2, 'GET')", ...)
            tx.Exec(ctx, "INSERT INTO casbin_rule (ptype, v0, v1, v2) VALUES ('p', $1, $2, 'POST')", ...)
        }
    }

    // 6. 提交事务
    if err := tx.Commit(ctx); err != nil {
        return err
    }

    // 7. 事务提交后，ReloadPolicy（从 PG adapter 重载）
    s.enforcer.ReloadPolicy()
    return nil
}
```

**关键原则**：`ReloadPolicy()` 必须在事务提交之后调用，不能在事务内——否则 Casbin 可能加载到未提交的数据。

### SyncedEnforcer（必须实现）

Phase 1 使用 `casbin.SyncedEnforcer` 而非普通 `casbin.Enforcer`，保证并发读安全：

```go
// internal/casbin/enforcer.go
func New(...) (*casbin.SyncedEnforcer, func(), error) {
    enforcer, _ := casbin.NewSyncedEnforcer(m, adapter)  // 不是 NewEnforcer
    enforcer.LoadPolicy()
    return enforcer, func() { enforcer.StopAutoLoadPolicy() }, nil
}
```

### Redis 原子操作（必须实现）

Phase 1 涉及的 Redis 操作需要保证原子性：

| 操作 | 原子性要求 | 实现方式 |
|------|-----------|---------|
| RT 刷新 | 旧 RT 删除 + 新 RT 写入必须原子 | Lua 脚本或 `GETDEL` + `SET` 分离（GETDEL 成功后再 SET，失败说明已被刷新） |
| 登出 | AT 加黑名单 + RT 删除 | 两个独立操作，部分失败可接受（AT 黑名单是关键，RT 删除失败最多残留到 TTL 过期） |
| 登录限流 | Lua LoginLocker（INCR+首次 EXPIRE 原子） | 达阈值 429；Redis 故障 fail-close |
| 黑名单 / `user:disabled` 查询 | 读 Redis 决定放行 | **fail-close**：Redis 故障返回 503，禁止查失败仍放行 |

Phase 1 RT 刷新的简化方案（不引入 Lua 脚本）：

```go
func (s *authService) Refresh(ctx context.Context, rt string) (*TokenPair, error) {
    // 1. 解析 RT
    claims, err := s.jwt.ParseRefreshToken(rt)
    key := fmt.Sprintf("refresh:%d:%s", claims.UserID, claims.DeviceID)

    // 2. 原子删除旧 RT（GETDEL：返回旧值并删除）
    //    如果返回空，说明 RT 已被其他请求刷新 → 401
    val, err := s.rdb.GetDel(ctx, key).Result()
    if err != nil || val == "" {
        return nil, errcode.ErrRefreshTokenInvalid
    }

    // 3. 签发新 AT + 新 RT（保留 deviceID）
    newAT := s.jwt.GenerateAccessToken(claims.UserID, claims.Username, claims.MustChangePassword)
    newRT, newJTI := s.jwt.GenerateRefreshToken(claims.UserID, claims.DeviceID)

    // 4. 存新 RT（key 与 deviceID 绑定）
    newRTKey := fmt.Sprintf("refresh:%d:%s", claims.UserID, claims.DeviceID)
    s.rdb.Set(ctx, newRTKey, newJTI, s.jwt.RefreshTTL())

    return &TokenPair{AT: newAT, RT: newRT}, nil
}
```

`GetDel` 是 Redis 6.2+ 原子命令，保证"检查 + 删除"不会被打断。Phase 1 用这个足够，不需要 Lua 脚本。

### 乐观锁（建议实现）

`roles`、`menus`、`organizations`、`users` 等在 Phase 1 建表时已含 `version INT DEFAULT 1`（见各 `phase1/0x-*.md` 与 `migrations/000001_init.up.sql`），**无需**再 `ALTER TABLE ... DEFAULT 0`。

```go
func (r *roleRepo) Update(ctx context.Context, role *Role) error {
    tag, err := r.pool.Exec(ctx,
        "UPDATE roles SET name=$1, description=$2, version=version+1, updated_at=NOW() WHERE id=$3 AND version=$4",
        role.Name, role.Description, role.ID, role.Version,
    )
    if tag.RowsAffected() == 0 {
        return errcode.ErrConcurrentModification // 10006
    }
    return nil
}
```

新插入行 `version` 从 **1** 起；更新时 `WHERE version=$4` 匹配当前值，成功后 `version+1`。

---

## Phase 1 不实现（仅设计预留）

以下问题在多实例环境下才出现，Phase 1 单实例不需要实现，但在设计中预留接口和扩展点：

### 分布式锁（不实现）

| 场景 | 预留方式 | 启用阶段 |
|------|---------|---------|
| 组织树结构变更 | `OrgService` 的 `Move` 方法预留 `Lock` 接口参数，Phase 1 传 nil | Phase 3 |
| Casbin 策略批量重载 | `ReloadPolicy()` 调用点预留加锁位置 | Phase 3 |
| 缓存重建 | Phase 1 无缓存，不需要 | Phase 2-3 |

Phase 1 组织移动只需 DB 事务 + `SELECT ... FOR UPDATE` 行锁即可：

```go
func (r *OrgRepo) Move(ctx context.Context, orgID, newParentID int64) error {
    tx, _ := r.pool.Begin(ctx)
    // 行锁锁定当前组织及其所有子节点
    tx.Exec(ctx, "SELECT id FROM organizations WHERE path <@ (SELECT path FROM organizations WHERE id=$1) FOR UPDATE", orgID)
    // 执行 path 更新
    // ...
    return tx.Commit(ctx)
}
```

### Casbin Watcher（不实现）

Phase 1 单实例，`SyncedEnforcer` 的内存策略通过 `ReloadPolicy()` 直接刷新。多实例时才需要 Watcher 通过 Redis Pub/Sub 通知其他实例重载。

预留方式：`enforcer.go` 中注释标记 Watcher 接入点。

### 跨实例事件广播（不实现）

Phase 1 无需 Redis Pub/Sub 广播。权限变更后直接 `ReloadPolicy()` 即可。

### 缓存一致性（不实现）

Phase 1 无权限缓存，每次请求走 Casbin **SyncedEnforcer + PG adapter**（策略持久化在 `casbin_rule`）。Phase 2 引入 Redis 权限缓存后才需要考虑缓存失效。

---

## 总结

| 关注点 | Phase 1 | Phase 2-3 |
|--------|---------|-----------|
| DB 事务（同库多表） | ✅ 必须实现 | — |
| SyncedEnforcer | ✅ 必须实现 | — |
| Redis 原子操作（GetDel） | ✅ 必须实现 | — |
| 乐观锁（roles/menus） | ✅ 建议实现 | — |
| DB 行锁（SELECT FOR UPDATE） | ✅ 组织移动用 | — |
| 分布式锁 | ❌ 不实现，预留接口 | Phase 3 |
| Casbin Watcher | ❌ 不实现，预留接入点 | Phase 3 |
| 跨实例事件广播 | ❌ 不实现 | Phase 3 |
| 缓存一致性 | ❌ 无缓存 | Phase 2-3 |
| singleflight | ❌ 无缓存 | Phase 2-3 |

**核心原则**：Phase 1 用 DB 事务 + 行锁 + SyncedEnforcer + Redis 原子命令，覆盖单实例下的所有并发场景。多实例相关的能力只预留接口，不引入额外复杂度。

> **与部署解耦**：上文「Phase 1 单实例」指 **验收与默认运行形态为 1 个 App 副本**，不是禁止 PG/Redis HA，也不是第二套代码。换 Cluster/Sentinel/多副本时 **只改配置与编排**；启用 Watcher 等见 [design-decisions §18](../design/design-decisions.md#18-部署与代码解耦一套代码多种部署)。
