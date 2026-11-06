# review/ — 审查记录

本目录三层结构；长期审计方法与时点报告分开维护：

- [audit-playbook.md](./audit-playbook.md) — **可复用全面审计指南**：设计→实现→关键实践，跨仓、前端功能/交互/视觉、七阶段、证据与报告模板；每次审计另建执行卡，不在指南保存当次基线或结论。
- 正式发现：[B02 认证撤权/P0](../../deliverables/software-company/zhuzhao-ecosystem-audit-b02-auth-revocation-2026-11-04.md)、[B05 验收脚本隔离/P1·P2](../../deliverables/software-company/zhuzhao-ecosystem-audit-b05-acceptance-isolation-2026-11-04.md)，保留基线、代码锚点与必要验证结论；B02 由 `10f3f94`、B05 由 `4de9070` 固定快照独立复验在代码层关闭（完整 hash 与失败/成功证据见报告末节），常驻 app 部署状态另计。
- [B09 临时成员过期/P0](../../deliverables/software-company/zhuzhao-ecosystem-audit-b09-membership-expiry-2026-11-05.md)：原业务快照连续三轮确认过期委托缺口，已由 `d2ab771` 固定提交独立复验在代码层关闭；常驻 app 尚未升级，未合 main。
- [B06 存储并发与事务冲突](../../deliverables/software-company/zhuzhao-ecosystem-audit-b06-storage-transactions-2026-11-05.md)：实物误删P0两项、额度P1、既有BK11残留P1；`d07f560` / `475b7ac` 固定提交代码复验关闭，主线/部署待承接，保留失败轮与边界。
- [B06 流程/提醒/调度续批](../../deliverables/software-company/zhuzhao-ecosystem-audit-b06-flow-reminders-2026-11-06.md)：P0快照、提醒/调度/通知身份/情报单飞等F05–F10已固定联合12401aa全量门禁关闭；编辑/公开回复漏消息F11与连续数组审计漏脱敏F12也已在7389固定完整门禁关闭，早前RustFS初始化失败轮保留。
- [B07 兄弟服务与公共库](../../deliverables/software-company/zhuzhao-ecosystem-audit-b07-sibling-services-2026-11-06.md)：十一项已证实发现，含两项既有panic债去重、网关413与生产探针；AL/TR固定复验及test升级分列，utils读错Close已正式pin到主仓bd05/AL60e候选、联合门禁待验，AL静默覆盖按P0。
- [B08 前端功能、交互与视觉](../../deliverables/software-company/zhuzhao-ecosystem-audit-b08-frontend-experience-2026-11-06.md)：十四项功能/交互/视觉发现，11项原范围修复复验完成、3项P2已修/专项通过；9f已test承接、c9最终schema与镜像待新组合完整41；f104旧组合41/41保留，视觉与全量认证边界分列。
- [B10 现行文档对账](../../deliverables/software-company/zhuzhao-ecosystem-audit-b10-document-alignment-2026-11-06.md)：五组设计/导航事实校准，含菜单及工单L1速查P1失真；历史留存，L-5主体未销账。
- [B11 当前L1全角色/全路由矩阵](../../deliverables/software-company/zhuzhao-ecosystem-audit-b11-permission-matrix-2026-11-06.md)：19主体八轮共25840矩阵请求/5440交叉，差异0；权限行数与端点数、L1到达与业务成功分别计，不替代L2/L3。
- [B12 错误码登记与守卫](../../deliverables/software-company/zhuzhao-ecosystem-audit-b12-error-contract-2026-11-06.md)：77当前定义补全、双向测试与四类稳定变异，联合门禁待验。
- [B13 环境恢复与真实Chrome](../../deliverables/software-company/zhuzhao-ecosystem-audit-b13-environment-chrome-2026-11-06.md)：dev保卷恢复、测试准确镜像/端口及专属浏览器走查，启动修复与未覆盖边界分列。
- [B14 资料写覆盖安全状态/P0](../../deliverables/software-company/zhuzhao-ecosystem-audit-b14-user-security-state-2026-11-06.md)：原两类各八轮、两资料入口修复32场/稳定变异及原生TestMain复验；最终联合全门禁待验，未合main/部署。
- [B15 接口/工具/迁移边界](../../deliverables/software-company/zhuzhao-ecosystem-audit-b15-contract-migration-boundaries-2026-11-06.md)：存储ID与Wire入口P2记账、定制引用回滚前置；撤销模板及MIG-S04运行P1候选，不把静态线索计作当前越权。
- 本地执行卡、过程报告、日志和快照默认存 `.codex/audits/<轮次>/`，不受 Git 跟踪或进入 Docker 上下文；当前轮为 `ECO-20261104`。共享索引不链接未纳管文件。

- **顶层（活文档+近期快照）**：
  - [11-project-control.md](./11-project-control.md) — 项目控制台**活文档**：能力矩阵 / 健康状态（遗留问题 SSOT，§6）/ Phase 3 前置随行分类（§8）/ 迁移地图。**每次改动后回填**（AGENTS.md 铁律），引用惯例 = 直接按路径 `docs/review/11-project-control.md`。
  - [12-b13-permission-matrix-2026-09-26.md](./12-b13-permission-matrix-2026-09-26.md) — B13 权限覆盖矩阵 point-in-time 快照（79 端点/8 盲区定性/IAM 四平面边界）——W1 词表终态基线，词表再动时新生一份而非改旧。
  - [14-b13-permission-matrix-2026-10-26.md](./14-b13-permission-matrix-2026-10-26.md) — B13 权限覆盖矩阵复审（phase5 终态 point-in-time：123 menu_apis 绑定+45 豁免全量对账——通过；B-19 销账件）。
  - [13-bk11-pool-deadlock-2026-10-26.md](./13-bk11-pool-deadlock-2026-10-26.md) — BK11 连接池死锁复盘（三层根因/已交付修复 `039aca6`/业界三档对照/彻底方案 dbtx 连接解析待拍板）——事务窗口嵌套池取类缺陷的 SSOT。

- **[archive/](./archive/)**：Phase 1–2 历史快照（00–10 共 13 篇）——各阶段系统性审查/验收/文档体系审查报告，发现与处置已全部闭环（闭环证据见 [11 §6](./11-project-control.md)）。编号即时间序，04 有两篇系同期各自成文的历史现状（维持不重排）。

- **后续新报告**：带日期戳的定期审计（`NN-topic-YYYY-MM-DD.md`）直接放顶层；闭环后移入 archive/。

引用本文目录：[docs/README.md](../README.md) §review。
