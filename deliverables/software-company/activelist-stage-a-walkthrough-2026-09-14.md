# activelist 阶段 A 收尾走查记录（2026-09-14）

> 联调五阶段计划的阶段 A 实机验证留痕（00-startup-checklist §5）。全部经 zhuzhao 网关 `/al` 反代 E2E（login → JWT → `/al/api/v1/*`），演示栈 = zhuzhao-app + activelist apiserver×2 + postgres + pgbackup。

## A1 pgbackup 恢复演练 ✅

按 `deploy/backup/README.md` 全步骤实跑：

1. 毁库前 `pg_restore -l` 预检备份归档（al-20260914.dump，58 entries，可读）
2. 前态基线：6 表行数（data_types=3 / schema_history=3 / 动态表全 0 / schema_migrations=1）
3. `docker compose stop apiserver`（双副本停写）→ DROP/CREATE DATABASE
4. `pg_restore --no-owner --role=activelist` ← 管道退出码显式检查 = **0，零报错**
5. `docker compose start apiserver` → 双副本 healthy
6. **恢复后 6 表行数逐一与基线一致**；网关 E2E code=0、types=2 与恢复前一致

## A2 多副本验证 ✅

| 项 | 结果 |
|---|---|
| 停 apiserver-1 后网关 GET×10 | 10/10 成功（Docker DNS 别名自动摘除停副本） |
| 单副本 POST 写路径 | 正常到达上游（无效体 400=上游业务校验，非网关层失败） |
| `--scale apiserver=3` → E2E | 新副本 healthy、code=0；缩回 2 正常 |

## A3 CRUD 矩阵走查 ✅ 30/30 PASS

| 面 | 项（全 PASS） |
|---|---|
| 类型管理 | 注册 / 重复注册 409 / 列表 / 单查 / 单查不存在 404 / rules 静态规则 |
| 数据写入 | 插入 id 递增 / 缺必填 422(100000) / 不存在类型 404 |
| 查询分页 | keyset 首页+cursor / 翻页至终止 null / 游标残缺 400 / 单查 |
| 乐观锁 | 更新携 version 成功→version=2 / 陈旧 version 409(100008) |
| 软删生命周期 | 软删 deleted / 列表排除软删 / 单查软删可见 / 恢复 active |
| 导入导出 | 导出裸数组 3 行 / 幂等重导 / 重导后行数不变 |
| Schema 演进 | 全量定义+version CAS→2 / 陈旧 409 / 演进后缺新必填 422 / 历史 2 条 |
| 废弃语义 | 废弃 / 废弃后插入 409 / 废弃后更新 409 / 软删·恢复仍放行（存量清理） |

备注：cursor 时间戳含时区 `+`，经网关传参须 URL 编码（走查脚本首跑踩坑，urlencode 后全过）。走查产物 zz_walk 类型以 deprecated 态留演示库（平台生命周期语义的活样例）。

## A4 限流真机 ✅

前置：演示栈启用限流（zhuzhao configs/config.yaml，默认 20 rps / burst 40，演示用法无感；镜像重建生效）。

- 50 并发锤 `/al/api/v1/admin/types`：**40×200 + 10×429**——恰好耗尽 burst=40 后拒绝对溢出部分
- 429 响应：`Retry-After: 1` 头 + 信封 `{code:10007, message:请求过于频繁, request_id}`
- 1.5s 后同路由恢复 200（20 rps 补桶节拍）
- 同时刻 zhuzhao 自有路由不受影响（跨路由桶隔离，批次 5a C1 的真机面）

## 附：pgbackup 事故修复（同日，activelist 54cec15）

三天定时备份失败（09-12~14）根因 = postgres 容器无 restart 策略停机不自启 × 旧版脚本失败静默（`done 0`）。修复：postgres 补 `restart: unless-stopped`；backup.sh 0 字节残留删除重做 + 空产物并入失败判定；RETAIN_DAYS→RETAIN_COUNT 统一。容器内三路径实测（新备份/跳过/0 字节重做）全过。

## 遗留

- activelist 54cec15 与 zhuzhao 配置/文档提交均未 push（push 单独指示）
- zz_walk 残留：deprecated 类型 + 动态表 3 行（演示库既有同类残留 demo_kind/preflight_check/verify_e2e，口径一致）
