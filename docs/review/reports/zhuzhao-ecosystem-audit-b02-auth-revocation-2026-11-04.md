# 生态全面审计 B02：认证撤权初核（2026-11-04）

> **报告口径（2026-11-08整理）**：本文保留注明时点的基线、原失败、发现编号和复验范围；代码路径是定位提示，历史内容以文内固定提交为准。后续变化见[正式报告索引](README.md)，当前能力、开放债与运行承接见[项目控制台](../11-project-control.md)。报告通过不等于main或生产已承接。

> 最新状态（2026-11-05）：应用修复 `10f3f9485f1bbe5722192e2e367e735f162653ac` 已完成固定提交独立重建与正常门禁，**B02-F01 在代码层复验关闭**，证据与兼容/部署边界见末节。常驻 app 尚未升级，不能将分支修复当作运行环境已修复。下方保留发现时点事实。

## 范围与基线

批号 `ECO-20261104-B02`。沿 auth/authz 设计阅读定向核对 Login、Refresh、UpdatePassword、ResetPassword、会话吊销与 JWT 检查；不代表 IAM 全面审查完成。五仓初盘与执行过程记录保存在本地工作区；本报告的主仓与 utils 固定基线见下文。

全部主仓代码锚点属于 `5c03581d5999139a3e8e5ead5972c026868aa181`；实际消费 utils 为 `v0.4.2`，tag commit `21155a98e87a944b20600833f940698d2759c2e5`。原有实现未修改；探针只加入固定提交的临时副本，变异在另一个实验副本进行。

## 发现：ECO-20261104-B02-F01 / P0 / 确认、待修、未复验关闭

**AT 撤权不单调，旧令牌在撤权后仍可使用。** 按本任务“安全/撤权门禁失效”口径分为 P0。利用前提是持有受影响用户尚未过期的旧 AT；残留受原 AT TTL 限制，当前配置基线为 30 分钟。两个触发路径合并为一项撤权发现，避免重复统计。

### 设计依据

[phase1/02-auth.md「管理员重置密码—安全要点」](../../phase1/02-auth.md)要求重置后吊销目标全部会话，旧 AT 由 disabled 拦截；强制改密期间其他接口应返回 403/20007。历史 P1-D 修复记录见 [11 号 2026-11-01 批次](../11-project-control.md)，声称自服务改密后其他设备旧 AT 即时失效。本轮对这些安全承诺独立复验。

### 路径 A：管理员重置后，新密码登录使旧 AT 复活

证据等级：真实 PG 服务调用与进程内 HTTP 集成复现。

1. 合成目标用户已绑定 viewer，先登录得到正常旧 AT；访问个人资料返回 200。
2. 合成 admin 调用真实 `UserService.ResetPassword`，其检查管理权限、更新密码与 mcp，并调用吊销逻辑。旧 AT 此时返回 403。
3. 目标用新密码调用真实 `AuthService.Login`。新 AT 带 mcp，访问个人资料为 403/20007；同一个重置前旧 AT 却重新返回 200。

原始观测：`before=200 after_reset=403 after_new_login=200; fresh_AT=403/20007; floor_keys=0`。

根因链：[`UserService.ResetPassword`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/service/user_service.go#L353) → [`revokeUserSessions`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/service/session_revoke.go#L35) 设置临时 disabled、递增 RT 纪元并删除 RT；[`AuthService.Login`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/service/auth_service.go#L141) 随后清除 disabled。AT 不携带/比较该纪元，重置路径也未写 AT 地板，原 AT 的 mcp=false 仍被信任。实验经过真实 JWT、SelfService、CasbinAuth（真实角色 SQL）和真实 `GetProfile` handler；不是完整 app 启动或浏览器 E2E。

原始 日志（本地 `B02-admin-reset-profile-probe.log`）、命令及退出码（本地 `B02-admin-reset-profile-probe-command.json`）、精确探针源（本地 `audit_reset_probe_integration_test.go.txt`）。安全断言预期拒绝而实际成功，所以测试 exit 1 是缺陷复现证据，不是“测试通过”。

### 路径 B：同秒旧 AT 不低于秒级地板

证据等级：原样 JWT 中间件构件复现，加生产写入链静态确认。

[`AuthService.UpdatePassword`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/service/auth_service.go#L321) 用 `time.Now().Unix()` 写地板；实际消费的 utils JWT 使用 NumericDate 的秒级时间精度。[`middleware.JWT`](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/internal/middleware/jwt.go#L134)只拒绝 `iat < floor`，相等时继续放行。同一秒内在撤权前签发的旧 AT 与撤权后新 AT 无法据此区分。

临时探针先签发合法旧 AT，再将地板写为该 iat（模拟同秒撤权状态），原样中间件 8/8 放行；将地板设为 iat+1 的对照 8/8 拒绝。该探针没有伪造签名，也没有修改实现；但它不是通过真实 UpdatePassword 恰好命中同秒的端到端实跑，生产触发条件由上述写入/比较链确认。

原始 日志（本地 `B02-at-floor-probe.log`）、命令及退出码（本地 `B02-at-floor-probe-command.json`）、探针源（本地 `audit_at_floor_probe_test.go.txt`）。8 次安全断言均失败，不能表述为产品通过。

### 现有测试有效性与盲区

本轮主仓原样单元测试已全绿，见本轮本地 B01 基线记录。`TestJWT_ATMinIATFloor` 原样串行 8/8 绿；在独立副本仅禁用地板比较后，该测试 8/8 红，失败对应旧 AT 未被拒绝。说明守卫能检测已有检查被移除，但覆盖的是早于/晚于地板及缺键场景，未覆盖相等边界；service 写地板集成用例也未断言另一旧 AT 真的失效。

原样日志（本地 `B02-at-floor-guard-original.log`）、变异日志（本地 `B02-at-floor-guard-without-check.log`）、精确变异 diff（本地 `B02-at-floor-mutation.diff`）、8 轮结果（本地 `B02-at-floor-mutation-results.json`）。未实现候选修复，所以没有修复后的通过或变异结论。

## 修复建议与挂点

优先形成统一的、不可因重新登录而回退的 AT 会话代际判定。候选为复用现有密码/会话纪元，随 AT 签发并在 JWT 中比较；挂点包含 utils `AccessClaims`/签发 API、主仓 `issueTokenPair`、JWT 及全量吊销入口。须明确旧令牌兼容策略、Redis 故障 fail-closed 与版本发布/三仓消费关系，不直接把 utils HEAD 替换进消费者。

不能仅将 `<` 改为 `<=`：那会拒绝同秒签发的合法新 AT。回归应同时包含同秒旧拒绝/新放行、重置后新登录旧 AT 仍拒绝、禁用再启用、并发 Refresh 与吊销交错；文件 cookie 复用 AT 的传播面需补验证，PAT 按既有独立凭证策略单列。以上为建议核验范围，不声称本轮均已测试。

建议所有者在 11 号健康状态及修复承接处登记本发现，保留原 P1-D 历史修复事实并补边界未闭合说明；审计员未擅自修改业务健康结论或实施修复。

## 验证环境、证据与限制

使用固定提交临时副本、新建 testcontainers PostgreSQL 15 和独立 miniredis；账号 E800901/E800902 为临时库合成 actor/target，PG 随测试退出自动终止。未访问共享 dev DB/Redis，未修改 E000001，未运行 acceptance。所有原有快照文件未变，新增测试仅存在于临时副本。

证据 manifest（本地 `manifest.json`）含 SHA-256。`.go.txt` 是精确实验源，不参与仓库编译；复现时复制到相同固定提交的临时副本并去掉 `.txt`，按命令回执运行。没有把原始令牌或真实用户数据写入证据。

状态仍为待修。后续新环境重建不等于漏洞已修复；任何修复须另钉提交窗口并做复验。本报告保留发现时点事实，当前承接状态见项目控制台；原始日志、探针与过程记录不随仓提交。

本地证据目录：`.codex/audits/ECO-20261104/evidence/ECO-20261104-b02/`；其中 manifest 记录 SHA-256。完整过程报告/执行卡在同轮本地目录。仅这份正式发现结论纳入 Git，后续关闭以主仓修复及独立复验为准。

## R01 修复与固定提交复验（2026-11-05）

应用提交：`10f3f9485f1bbe5722192e2e367e735f162653ac`，分支 `codex/fix-auth-revocation`；库版本：已发布 utils `v0.4.3`（tag commit `9296ae170b71b06c623c54b6010a107406903346`）。主仓正式 pin；固定提交归档内 `GOWORK=off go list -m -json` 实际解析为 v0.4.3，无 Replace。没有把旧 go.work 候选或库 HEAD 当作正式依赖证据。

修复链：`AuthService.issueTokenPair` 给 AT/RT 同时绑定签发纪元；`Login` 先读纪元、再重读并验证凭据，且仅在纪元仍匹配时清拒绝标记；Refresh 保留消费快照；`UpdatePassword` 使用本次撤销返回的纪元并重读密码/状态。JWT 对纪元逐命令检查，缺键仅兼容 0，损坏/负值/故障拒绝服务；同秒不再由秒级地板单独承担安全判定。

二次验证额外发现并在同笔修复：当前 AT 黑名单单条 SET 失败时，原候选先返回 503 而跳过全设备撤销，新密码重登可使其他设备旧 AT 幸存。现先完成全局撤销，再写单条黑名单；确定性故障回归覆盖该顺序。此项作为同一撤销不变量的变体，不重复统计为另一条发现。

固定提交经全新 `git archive` 提取、重新构建后：

- `make lint`、`make test-unit`、`make test-integration`、`make test-cover` 全部通过；集成门禁最终成功轮 507.74 秒。覆盖率按默认单元口径为 19.9%，未重新计算相同口径的原始基线差值，不声称覆盖率提升。
- 原样 `make acceptance`：四档 **253 PASS / 0 FAIL / 0 SKIP**（124+68+26+35），**81 对迁移往返零漂移**；依赖为隔离 PG/Redis/RustFS、冻结基线的两个兄弟应用镜像，不改验收断言。
- 固定提交的管理重置→新登录、跨设备改密、禁用解禁/删除、在途旧密码登录、刷新消费交错、单命令黑名单故障专项连续八轮通过；同秒地板相等、纪元缺键/损坏、后续 WRONGTYPE 被首个 redis.Nil 掩盖、文件 cookie 路径由中间件回归覆盖。
- 真实 app/API 13 步链验证旧 AT `200→403→401`、旧文件 cookie 401、新 AT 保留强改密限制、改密后合法 AT 200/旧 RT 401；专属 E800904/E800905，仅记录状态/业务码，不保存令牌。
- 三个独立实验副本分别移除纪元比较、登录凭据重读、全局撤销优先顺序，目标回归各 **8/8 稳定失败**；失败均对应安全断言，不是编译或环境初始化错误。

失败与重跑亦保留：固定提交全量集成首跑在最后一个 ticket 包的 testcontainers 清理器启动阶段超时，该包尚未执行测试；第一次重跑被会话中断、没有退出码，不能记为通过。确认句柄与 OS 进程均不存在后，新一轮原样完整集成通过，未修改源码或跳过用例。固定提交验收的“全体容器集合相等”探针因并行测试临时容器变动返回 false，不能用它证明所有容器都未变化；本批候选隔离轮有前后集合相等证据，固定轮的自身原生 app/专属 Compose 资源均已清理，不把临时资源集合变化混同业务门禁失败。

原始日志、归档哈希、命令回执与变异结果仅本地 `.codex/audits/ECO-20261104/evidence/ECO-20261104-r01b/` 保存；`postcommit/verification-results.json`、`resumed-results.json`、`retry-interrupted.json` 与 `full-acceptance-result.json` 分别保留各轮事实，不覆盖失败记录。

结论：原报告两条触发路径及上述同类窗口已在该固定代码提交复验关闭。常驻服务仍须按联合验证批升级；所有鉴权实例同时具备新实现后方能宣称部署修复生效，旧实例混跑或回滚会重新暴露问题。旧 AT 的纪元 0 兼容、Redis 会话状态持久化/灾难丢失及 DB 提交后 Redis 整体故障的补偿边界见 `docs/phase1/02-auth.md`；本修复没有提供跨 DB/Redis 原子事务，PAT 独立凭证语义保持既有设计。
