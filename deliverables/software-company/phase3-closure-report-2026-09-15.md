# Phase 3 收官报告（2026-09-15，口径 A）

> **结论**：Phase 3 **项目内工程收口**。主链 M-E / M-A 全收官，M-HR 按所有者拍板=预留接口即可；M-SSO / M-Mig 🚦 挂外部前置（进内网/迁移窗口），非工程项。终验四档 acceptance FAIL=0、四仓全门禁绿。CORS 收紧（B7）经所有者拍板**暂不动**（随上线做）。

## 一、主链终态

| 主链件 | 终态 | 关键证据 |
|---|---|---|
| **M-E 事件/任务总线（taskrunner）** | ✅ 全收官 | zhuzhao 侧 E-①–E-⑦ 全实施；taskrunner 独立仓交付（PG 形态/容器化/AK-SK 验签/cronloop）；回调链全环实机贯通（提交→验签→执行→幂等栅栏→audit_archive→双库 succeeded）；audit_archive 每日 02:30 周期跑通（AL5 真数据闭环）；故障演练四项全过（队列续跑/PG 自愈/dead 重驱/SIGTERM 排空） |
| **M-A activelist** | ✅ 全收官 | 独立仓交付（动态数据模型平台，M-A1–A6）；网关化批次 B（反代/X-Operator/限流/Restrict/menu_apis/BK-22 对账）；compose 双网络隔离、双副本、pgbackup；`/al` 签名透传 E2E + CRUD 矩阵 30/30 + 限流真机 40×200+10×429 |
| **M-HR HR 同步** | ✅ 按拍板口径完成 | 预留接口版 `HRFetcher` 在位；真实对接时启动（三项启动拍板：离职在途工单/部门撤销级联/跨部门权限分配，登记于 hr-directory-sync §3.2/§4.3） |
| **M-SSO** | 🚦 外部前置 | 设计定稿（design-decisions §24，OAuth2 授权码预留接口版）；进内网拿到公司接入信息后 2–3 人日落地 |
| **M-Mig 迁移准备** | 🚦 外部前置 | 网络对接/凭据/命名与 module path 合并变更/工单平台表达力评估，随迁移窗口 |

## 二、终验（2026-09-15）

- **标准环境**：docker-dev-reset + make dev（干净库+迁移到 29）
- **四档 acceptance**：Phase 1 回归 27 + 2a 66 + 2b 26 + 2c 34——**FAIL=0 全程**
- **四仓全门禁**：
  - zhuzhao：lint + 13 包单测 + 13 包集成 `-race` 全绿
  - taskrunner：lint + 5 包单测 `-race` + PG 集成（scratch 库）+ build 全绿
  - activelist：lint + 6 包单测 `-race` + 集成 `-race`（testcontainers）全绿
  - zhuzhao-utils：lint + 5 包（count=1）+ build 全绿

## 三、交付物清单

- **zhuzhao**：网关反代 + API 限流 + 任务管理代理 + 内网回调端点 + 判定日志 + 审计归档 + BK-22 对账 + 迁移 000020–000029 + 双网络部署态 compose + pgbackup 侧车
- **taskrunner**：Asynq 任务引擎（PG 存储/HTTP API/AK-SK/cronloop/CLI）+ deploy/compose.yaml（三服务双网络零宿主端口）
- **activelist**：动态数据模型平台（类型/演进/CRUD/导入导出/AK-SK/双副本）+ pgbackup
- **zhuzhao-utils**：共享库 v0.3.0（aksk/crypto/errcode/jwt/logger/postgres 等 9 包）
- **演示栈**：本机三栈全容器化（zhuzhao:33333 / activelist `/al` / taskrunner 共享网），13 容器 healthy

## 四、2026-09-14 联调五阶段（证据索引）

| 阶段 | 结果 | 记录 |
|---|---|---|
| A activelist 收尾 | ✅ | 恢复演练/多副本/CRUD 30 项/限流真机——00 §5 + deliverables 走查记录 |
| B+C PG 回调链回环 | ✅ | 全链 succeeded 三侧核验 + dead 复验（抓出演示库漂 4 版、镜像目录缺陷并修） |
| D 三服务 compose 拓扑 | ✅ | taskrunner 容器化新交付物、零宿主端口、R5 探针负向实测 |
| E 周期归档+故障演练 | ✅ | cron 每日 02:30 + AL5 真数据 + 演练四项 |
| 运维补强批（追加） | ✅ | zhuzhao pgbackup 侧车、迁移 `--profile migrate` 固化、zhuzhao 恢复演练 21 表零差异、**PITR 实测**（并勘误 activelist 蓝本：pg_dump 逻辑备不可回放 WAL，须 pg_basebackup） |

## 五、遗留与触发条件（收口后状态）

| 项 | 触发条件 |
|---|---|
| M-SSO 实施 | 进内网/拿到公司 SSO 接入信息（2–3 人日） |
| M-Mig 迁移准备 | 迁移窗口确认 |
| M-HR 真实对接 | 公司 HR 对接决策（届时拍三项） |
| CORS 收紧 B7 | 上线前（所有者 2026-09-15 拍板暂不动） |
| Casbin Watcher B9 / 多实例 | 多实例部署时（02 号方案在位） |
| K8s vs Compose / HA 水位 / 部署分离时机 | 随真实部署拍板（00 §3） |
| taskrunner PG 备份口径 | M4（job_runs 保留期一并拍） |
| BK-21 护栏泛化 / 在线用户管理面 / RT 宽限窗口 / 前端 zhuzhao-ui 等 | 既有触发驱动登记（11 §8 / 随手项）不变 |

## 六、结论

Phase 3 自 2026-09-02 重定位定版主链，至 2026-09-15 项目内工程全部交付并实机验证：**可交付（四档验收+四仓门禁）、可演示（三栈容器化 E2E）、可运维（备份/恢复/PITR/演练闭环）**。后续仅随外部前置（进内网/迁移窗口）重启对接实施。
