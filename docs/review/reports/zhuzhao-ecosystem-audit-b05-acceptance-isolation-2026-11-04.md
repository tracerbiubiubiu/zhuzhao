# zhuzhao 全面审计 B05：验收脚本环境隔离核查

> **报告口径（2026-11-08整理）**：本文保留注明时点的基线、原失败、发现编号和复验范围；代码路径是定位提示，历史内容以文内固定提交为准。后续变化见[正式报告索引](README.md)，当前能力、开放债与运行承接见[项目控制台](../11-project-control.md)。报告通过不等于main或生产已承接。
日期：2026-11-04。执行：Codex 当前会话。以下为原发现时点记录；F01/P1、F02/P2 已由末节 R02 固定提交复验关闭，非整体审计完成。

## 范围与基线

主仓受审提交 `5c03581d5999139a3e8e5ead5972c026868aa181`；此前两笔审计治理提交不改变本批脚本基线。本批核对 `Makefile` 的 acceptance 入口、四档脚本、`migration-roundtrip-check.sh`，六个文件与该基线逐字相同，见 源清单（本地 `source-manifest.json`）。

设计/操作依据：standards §12-10 多会话纪律、runbook §7 环境纪律，以及执行卡 §5 的隔离测试要求。触发为 R01 候选的正式门禁准备，不将其记作 Phase 1–5 设计阅读完成。

## 分级发现

### ECO-20261104-B05-F01 / P1：迁移往返的 SQL 与 migrate 目标可分裂

锚点：`scripts/migration-roundtrip-check.sh` 的 `PG_C` / `DSN` 赋值及临时库创建路径，[基线第 9–20 行入口](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/scripts/migration-roundtrip-check.sh#L9)；`Makefile` 的 [acceptance 入口](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/Makefile#L110)。

设置 `PG_CONTAINER=b05-isolated-postgres` 后，临时库创建/删除和内容快照会走指定容器，但 migrate 的 DSN 仍固定为 `postgres://zhuzhao:zhuzhao_dev@localhost:5432/$DB`。因此在独立端口/口令的 test 环境，`make acceptance` 链尾会连接错误实例或因目标库不存在/认证失败而失败；仅设置 PG_CONTAINER 不能使这个门禁隔离运行。

动态证据：命令桩轨迹（本地 `isolation-command-trace.json`）、复核回执（本地 `isolation-probe-result.json`）。Docker 与 MIG 均替换为仅记参数的命令桩：轨迹显示 Docker 指向指定容器、migrate 仍指向 localhost:5432；MIG 桩在首次迁移处按预期 exit 23，未连接任何实际数据库，未执行任何实际 Docker 操作，未走到共享快照写入。不能将该 exit 23 宣称为真实 migrate 的失败日志。

影响边界：默认 dev 拓扑可能正常；确定缺陷是自定义容器/端口下的门禁目标不一致。本轮未观察到 dev 数据被改写，不声称已经发生数据损坏。该问题独立于 R01 认证候选，受审基线已有。

### ECO-20261104-B05-F02 / P2：不同工作树/环境共用固定临时产物路径

锚点：迁移往返 `snapshot` 的 [固定输出/比对路径](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/scripts/migration-roundtrip-check.sh#L64)；Phase 1 [Refresh 并发夹具的固定文件](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/scripts/acceptance-phase1.sh#L325)；Phase 2c 的 [http_of/code_of](https://github.com/tracerbiubiubiu/zhuzhao/blob/5c03581d5999139a3e8e5ead5972c026868aa181/scripts/acceptance-phase2c.sh#L91)。

迁移往返固定使用 `/tmp/rt_before.txt`、`/tmp/rt_after.txt`；四档脚本也重复使用 `/tmp/p1.json`、`/tmp/p2a.json`、`/tmp/p2b.json`、`/tmp/p2c.json`、`/tmp/r1.json` 等。即使数据库、Redis、端口和工作树已经隔离，临时文件仍不隔离。不同会话可相互覆盖响应/快照，造成假红或证据失真；Refresh 响应文件还可能承载测试令牌，清理也不能按公共文件名盲删。

逐行静态证据（本地 `shared-tmp-anchors.json`） 登记六文件中的 96 处 `/tmp/` 引用，**不是 96 个独立缺陷**。本轮未动态复现多会话互相覆盖；并发影响是基于固定共享路径的推断。R01 四档运行实际沿用了原脚本的固定临时文件，此限制明确保留，不宣称仅隔离数据库即可隔离全部宿主产物。

## 修复建议与挂点

- 建议将 F01/F02 合为同根的门禁可移植性修复批：SQL 与 migrate 共用一组明确的连接参数，正确编码凭据；自定义容器而未给出可验证连接信息时在任何写入前拒绝执行。不要只把 dev DSN 改为另一套本地端口。
- 四档及迁移往返按每次调用创建唯一临时目录，显式传递给子脚本，trap 仅清理本次目录；补不同实例/不同临时目录的命令桩反例，断言错误配置未触发迁移。保留原 dev 默认行为的兼容验证。
- F01 直接阻塞 R01 的原样隔离 `make acceptance`，建议作为下一修复批优先处理；F02 同根处理，不扩大到业务功能改造。原报告保留，本轮不修改受审脚本。

## 验证及实际门禁边界

命令桩确认 F01；六文件源哈希核对及 F02 静态逐行核对完成。未用原始迁移往返脚本连接 dev，未运行未隔离的原样 `make acceptance`。

R01 的原四档链通过 `make acceptance-2c` 独立运行；迁移往返只在仓外副本改 DSN 和两个快照路径，SQL/快照/断言均保持不变，81 对零漂移。精确适配 diff 与运行证据在 R01 本地记录中；这不能冒充原样 `make acceptance` 通过，也不构成对本报告两条发现的修复。

本批证据清单：SHA-256 manifest（本地 `manifest.json`）。结论涉及门禁可移植性与并行验证边界，建议所有者承接控制台健康/遗留状态回填；审计员只添加报告导航，不直接改既有健康结论或验收计数。

本地证据目录：`.codex/audits/ECO-20261104/evidence/ECO-20261104-b05/`。命令桩、源快照、完整日志与 manifest 不随源码提交；本报告的代码锚点与验证摘要保留以供修复追溯。

## R02 修复与独立复验承接（2026-11-04）

修复提交：`4de9070ab444137689774f8b7e93dc69145b1157`（本地主仓，尚未推送）。F01 挂点为 [migration-roundtrip-check.sh 的 ADMIN_DSN/DSN 与 cleanup](../../../scripts/migration-roundtrip-check.sh#L52)：SQL 和 migrate 共用显式管理连接派生的随机临时库；拒绝 query 中的库名/连接覆盖，只有本次成功建库后才允许清理。`Makefile` 的 acceptance 在四档写入前调用只读 `--check`。F02 挂点为 [四档脚本的 TMP_DIR/cleanup（Phase 1 例）](../../../scripts/acceptance-phase1.sh#L5) 及往返 `TMP_DIR`：原 96 处固定路径全部改用各次私有目录，清理不触及父目录或其他调用。

从上述完整提交 `git archive` 到全新临时目录，重新构建原业务源码（业务基线 `5c03581`、正式依赖 utils `v0.4.2`、`GOWORK=off`），独立执行：

- 16 项脚本黑盒回归全部通过，含拒错、同库连接、失败/TERM 清理、并行调用及首个响应文件的动态隔离；外部命令均为桩。
- 实际七容器/宿主应用的一次性栈，原样 `make acceptance` 退出 0：四档 `124+68+26+35=253 PASS / 0 FAIL / 0 SKIP`，81 对迁移往返零漂移。计数随种子初态及可选能力分支变化，不替换控制台原历史数字。
- 提交前同一候选另用全新 PG15、自定义账号/端口与含保留字符的口令完成 81 对往返；两类定向变异各串行 8/8 在连接一致性/响应路径唯一性断言处失败。提交源与候选十文件 SHA-256 相符。
- 一次性进程、容器、卷和网络已清理；其他现存容器 ID 与启动时间完全一致。首轮缺宿主 psql、整链首次端口绑定失败均保留原始记录，未计为产品通过。

**F01、F02 在该修复提交范围内复验关闭。** 本批没有自动验证 API/PG/Redis 全栈归属，也未修复其他脚本的共享临时路径；操作前置及 SIGKILL 等残留边界见 [runbook §7.1](../../ops/runbook.md#71-验收连接与临时文件隔离2026-11-04)。这次验证未使用 R01 认证候选，不关闭 B02/P0，后者须正式 utils v0.4.3 依赖下单独跑完整集成及验收。

本地复验证据：`.codex/audits/ECO-20261104/evidence/ECO-20261104-r02/`，含完整日志、源/归档哈希、变异回执及清理记录；不随源码提交。
