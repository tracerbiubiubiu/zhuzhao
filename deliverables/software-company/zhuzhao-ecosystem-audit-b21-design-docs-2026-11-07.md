# B21 / L-5：工单与架构主体文档校准

## 范围与基线

仅校准 `docs/modules/ticket.md` 与 `docs/design/architecture.md` 的主体；实现证据钉在Phase5 `996c167f4567e39362226937b25368b55b7f44bd`，兄弟职责使用正文各自的固定commit。原稿分别保留在Git历史及ignored取证副本，不将大幅删减本身作为正确性证据。本批为文档修复，不变更实现、账号、运行环境或main。后笔B19/B23的具体契约/资源修复由各自报告覆盖，不倒写996c167为其已修快照。

## B21-F01 / P2：旧设计主体混写成现行能力

旧架构仍含可直接复制的陈旧Compose/配置/Token设备结构、统一Runner、主仓Asynq worker及伪完整API表；旧工单文档含接口草图、轮询事件等旧设计。阶段稿和控制台已演进，但新读者会将旧主体当现行入口。原固定锚点：[architecture.md](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/docs/design/architecture.md)、[ticket.md](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/docs/modules/ticket.md)，挂点为[Phase5 L-5](../../docs/phase5/S6-S5-debt-ledger.md#l-文档架构整理2026-10-27-立项phase5-合回-main-后执行)。

## 修复与能力边界

- 工单：按实际L1入口、L2 OR可见集和L3动作梳理权限；校准field_meta/transition_meta、配置CAS、分派、附件/抄送/标签/超时、SLA0.5、真实Service接口及两类消息消费者。旧“转部门属主不豁免”与当前已验收OR规则的差异明确展示，不由文档差异自行宣布越权或改变既有策略。
- 架构：区分主仓IAM+工单与UI/AL/TR/utils职责；回填RT设备槽与PWE、当前Redis键/三类审计、实际App启动停机顺序、81迁移权威入口、真实部署拓扑及API分族。归档能力与备份/调度是否启用分开，不凭存在代码推断常驻环境已备份。
- 演进：保留Phase1→Phase2、2026-08暂停/09封版/10-09自研翻案、Phase3/4底座及Phase5深化的动机和裁定；旧Engine/Approval Port、多租户原型、选型比较保留并标历史或未实现。移除失效的可执行副本，现行配置/路由/迁移指向各自SSOT；不靠Git历史替代必要设计上下文。
- 初始seed账号 `admin123 + must_change_password=true` 与dev/acceptance/UI setup终态 `admin12345` 分开标注；没有访问或修改账号。

## 验证证据与限制

主体改稿以74条claim、45个主仓文件及84条五仓固定源码链接建证据矩阵。另一次独立只读复核抽核鉴权、seed、消息、存储、归档/Cron、启动停机及生态边界，指出两处级联残留并修正：checklist不再指示实现不存在的Runner；Registry只保证未知资源拒绝，动作裁决委派具体Resource，不扩大到管理员豁免之前的动作校验。

76条相对文件链接均存在，独立复核的58条入站/文内fragment均能定位；旧节号/锚点保留兼容，特别保留Token细节、组织/审计API和404语义入口。围栏/目录及 `git diff --check` 通过。文档批不另起业务门禁；正在运行的组合业务门禁不能替代本批逐项语义核对。

剩余仅历史跨文件语气/章节显示及P3措辞：phase3 README旧L1轮询叙述、phase1/proposal统一Runner旧目标、auth文档旧节号、architecture“多设备管理”概括，已在ignored交接列明；本批停止扩面，后续随文档归并承接。L-1至L-4整体搬树仍按合main后独立批执行，不能因L-5主体完成就视为这些搬迁已完成。固定原始claim/link/fragment/围栏检查与前后文证据只在ignored `.codex/audits/ECO-20261104/phase5-closeout-20261107/l5-docs/`。

## 固定提交与最终组合补证

本批主仓完整提交 `f27994bcbd832b7a4162846f181044431b925516`。最终运行源码354项逐字绑定主仓 `f87ab1aefcbf1843bd8714c5accf245033f99d8b`，原样253/81及29文件230单测、41/41 E2E均通过；[组合补证与保留限制](./zhuzhao-ecosystem-audit-b20-ui-feedback-2026-11-07.md#验证证据与已纠正的初版误判)保留初版失败、wrapper缺checker与未执行附加control，不扩大本批原覆盖。准确远端CI/常驻承接仍待。
