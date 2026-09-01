# 02 - 多实例部署（multi-instance）

> **状态**：已编写（2026-08-31，基于并发/多实例审计 + eiam 参考实现）｜ **Wave 归属**：W1 可运维基座（检查单 B9）｜ 对应 Step 2
> **前置**：W0 清零（检查单 A 档）。**本模块是 W2 的硬前置**（提案已确认，README §2.1.0）——SLA 扫描/通知的多实例防重与本模块的 Casbin 策略同步是两条独立机制，缺一不可。

---

## 0. 问题清单（多实例会坏什么）

2026-08-31 审计结论（见检查单 §4）：**zhuzhao 工单/IAM 模块无进程内可变状态**，共享层全部在进程外——这是多实例部署的先天优势。逐项盘点：

| 维度 | 现状 | 多实例下 |
|------|------|---------|
| 会话 / RT 轮换 / Token 黑名单 / 登录限流 | Redis（Lua 脚本原子） | ✅ 天然共享 |
| 工单状态并发 | DB 乐观锁（`WHERE status=$from`）+ closed 守卫 | ✅ DB 级，跨实例有效 |
| 组织 Move 串行化 | `pg_advisory_xact_lock('org:move')` | ✅ DB 级 |
| Create×Move 快照 | 事务内 `FOR SHARE`（BK-11） | ✅ DB 级 |
| 时间戳 | 全部 DB 生成（`NOW()`），无应用时钟依赖 | ✅ 免时钟偏移 |
| Scope 解析 / 鉴权判定 | 每请求实时 SQL，无缓存 | ✅ 无状态 |
| **Casbin 策略** | `SyncedEnforcer` 内存策略；`reloadPolicy` 只刷新处理管理请求的**本实例** | ❌ **唯一鉴权正确性缺口**：其他实例策略陈旧到下一次自己 reload（授权回收延迟放行） |
| **L1 事件消费** | 单消费者轮询（Phase 3 启用时） | ❌ 多实例需抢锁防重复消费 |

**本模块只解决加粗两行。** 其余维度零改动。

## 1. 决策一：Casbin Watcher（策略跨实例同步）

采用 `github.com/casbin/redis-watcher/v2`，方案移植自 eiam `ioc/casbin.go`（已验证的完整形态）：**Watcher 推送（即时）+ StartAutoLoadPolicy 定时兜底（1min）** 双保险——推送丢失时 1 分钟内必然收敛。

```go
// internal/casbin/enforcer.go 增量（示意）
w, _ := rediswatcher.NewWatcher(redisAddr, rediswatcher.WatcherOptions{
    Options:   rediswatcher.Options{Channel: cfg.Watcher.Channel}, // 默认 "casbin"
    OptionalSC: rediswatcher.OptionalSC{PrevEnable: true},
})
enforcer.SetWatcher(w)
_ = w.SetUpdateCallback(func(msg string) { _ = enforcer.LoadPolicy() })
enforcer.StartAutoLoadPolicy(cfg.Watcher.Interval) // 默认 1m，兜底收敛
// cleanup 增加：enforcer.StopAutoLoadPolicy(); w.Close()
```

- **单实例部署**：`casbin.watcher.enabled=false`，零开销（现有行为不变）。
- **既有多实例下变更路径**：管理端写 `casbin_rule` → 本实例 `reloadPolicy`（既有，B3-5）→ Watcher 通知其他实例 LoadPolicy。**含义：授权回收在其他实例 ≤ 秒级（推送）或 ≤1min（兜底）生效**，替代原来的"无限期陈旧"。
- 配置段：`casbin.watcher: { enabled: bool, channel: string, interval: duration }`。

## 2. 决策二：L1 事件消费互斥锁

L1 为**单消费者模型**（P2-4，ADR-001）。多实例下轮询消费者按实例去重：

```sql
-- 消费循环每轮开始（事务内）：
SELECT pg_try_advisory_xact_lock(hashtext('ticket:l1:consume')) AS got;
-- got=false → 本轮跳过（其他实例正在消费）；true → 消费至事务提交
```

- 与 `org:move` 的 advisory lock 同款先例；**DB 级**跨实例互斥，实例崩溃自动释放（会话结束）。
- 不用 Redis 锁的理由：消费者本来就要开 DB 事务处理事件，DB 锁与事务同生命周期，天然无租约续期问题。

## 3. 部署形态

- Docker Compose：`deploy.replicas` 或多容器 + Nginx upstream；`/health/live`、`/health/ready` 供 LB 摘除。
- 优雅关闭：复用 Phase 1 信号处理；关闭前完成当前消费事务（advisory lock 随连接释放）。
- **不做**：K8s、服务网格、消息中间件（Kafka 触发条件 = 真实微服务拆分，ADR-001 已裁决）。

## 4. 验收标准（Wave W1 退出标准）

| # | 用例 | 通过标准 |
|---|------|---------|
| MI1 | 双实例策略同步 | 实例 A 改角色菜单 → 实例 B 上被回收 API 在 ≤5s 内返回 403（推送路径） |
| MI2 | Watcher 推送丢失兜底 | 断开 B 的订阅 → 1min 内 autoload 收敛（权限变更生效） |
| MI3 | L1 消费防重 | 双实例并发消费：同一 signal 事件只被处理一次（`processed` 标记 + 互斥锁） |
| MI4 | 登录限流跨实例 | 交替打两个实例登录，5 次失败后全局锁定（Redis 计数） |
| MI5 | 单实例回归 | `watcher.enabled=false` 时行为与现状完全一致（自动化测试 + acceptance 全链） |

MI3 依赖 L1 机制实现（Step 7 前置），先以集成测试桩验证锁语义。

## 5. 涉及文件

```
internal/casbin/enforcer.go        # watcher 接线（上示）
internal/config/config.go          # casbin.watcher 段
internal/service/ticket/           # L1 消费循环加 try_advisory_lock（Step 7 时）
configs/config.yaml                # 配置样例
docs/phase3/02-multi-instance.md   # 本文档
```
