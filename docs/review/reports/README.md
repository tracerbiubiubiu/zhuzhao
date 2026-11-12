# 正式审查与验证报告

本目录保留需要长期追溯的正式时点报告。报告中的“当前”、分支、运行产物、测试数与失败结果只对应其注明的基线；后续修复不反写原记录。当前能力、债和main/生产承接以[项目控制台](../11-project-control.md)为准，文档维护方式见[维护约定](../../documentation.md)。

## 阅读顺序

先看当前控制台；追溯11月修复先读11-08复验与B27，再按发现编号进入分批报告。11-06总报告保留首次全量审计覆盖与原始基线。较早审计用于追根因、决策与误报裁定，不作为今天的开放问题列表。

## 最新收口与总报告

- [真实前端首组基线（2026-11-12）](frontend-first-baseline-2026-11-12.md)：24图、实际色对/Logo404和规则TDZ，保留错误与未覆盖；全站仍部分。

- [新旧对照、近期补研究与执行排期（2026-11-12）](near-term-research-and-schedule-2026-11-12.md)：四组补研究、119条触发理由与13个发布切片；其余研究和真实验证仍按门推进。

- [全会话关注点研究深度核验（2026-11-12）](concern-depth-verification-2026-11-12.md)：32条真实输入、65核对点，明确研究充分、部分深入与合理后置；不以材料交付代替全部深研。

- [原始需求到现行计划的闭环审计（2026-11-11）](pre-phase6-requirements-audit-2026-11-11.md)

- [Phase6前详设与新增要求复核（2026-11-11）](pre-phase6-design-deepening-2026-11-11.md)
- [Phase5后首轮全项目重规划（2026-11-11）](pre-phase6-replanning-2026-11-11.md)

- [五仓文档原始要求二次复核（2026-11-08）](documentation-recheck-2026-11-08.md)
- [五仓文档归位与内容校准（2026-11-08，首次整理验证）](documentation-reorganization-2026-11-08.md)

- [报告复核后分批修复 — 2026-11-08](zhuzhao-report-followup-2026-11-08.md)
- [B27：Phase5五仓后续修复与环境收口](zhuzhao-ecosystem-audit-b27-phase5-closeout-2026-11-07.md)
- [zhuzhao 生态首次全面审计总报告（2026-11-06）](zhuzhao-ecosystem-full-audit-2026-11-06.md)

## 2026-11生态分批报告

批号不重排。B06的两个文件连续覆盖F01–F20；B17补扫/依赖修复并入B16文件，B22工具/入口补证并入B18文件。因此本目录23份生态分批文件的**文件名包含22个不同批号**，不是“每个数字一份报告”；正文中的执行批与文件名数量分别计。B01/B03/B04分别为基线、环境与修复治理准备批，不另造正式发现报告；详见11-08复验的编号归档说明。

- [生态全面审计 B02：认证撤权初核（2026-11-04）](zhuzhao-ecosystem-audit-b02-auth-revocation-2026-11-04.md)
- [zhuzhao 全面审计 B05：验收脚本环境隔离核查](zhuzhao-ecosystem-audit-b05-acceptance-isolation-2026-11-04.md)
- [生态审计 B06 续批：流程快照、提醒与调度（2026-11-06）](zhuzhao-ecosystem-audit-b06-flow-reminders-2026-11-06.md)
- [生态审计 B06：存储并发与事务冲突（2026-11-05）](zhuzhao-ecosystem-audit-b06-storage-transactions-2026-11-05.md)
- [生态审计 B07：兄弟服务与公共库（2026-11-06，阶段报告）](zhuzhao-ecosystem-audit-b07-sibling-services-2026-11-06.md)
- [生态全面审计 B08：前端功能、交互、视觉与实践（阶段报告，2026-11-06）](zhuzhao-ecosystem-audit-b08-frontend-experience-2026-11-06.md)
- [生态审计 B09：临时成员过期与组织委托（2026-11-05）](zhuzhao-ecosystem-audit-b09-membership-expiry-2026-11-05.md)
- [生态审计 B10：现行设计与能力说明校准（2026-11-06）](zhuzhao-ecosystem-audit-b10-document-alignment-2026-11-06.md)
- [B11 · 当前 L1 全角色与全路由矩阵审计（2026-11-06）](zhuzhao-ecosystem-audit-b11-permission-matrix-2026-11-06.md)
- [生态全面审计 B12：错误码登记完整性与守卫（2026-11-06）](zhuzhao-ecosystem-audit-b12-error-contract-2026-11-06.md)
- [生态全面审计 B13：环境恢复、启动编排与真实Chrome（2026-11-06）](zhuzhao-ecosystem-audit-b13-environment-chrome-2026-11-06.md)
- [生态全面审计 B14：资料写入覆盖账号安全状态（2026-11-06）](zhuzhao-ecosystem-audit-b14-user-security-state-2026-11-06.md)
- [生态全面审计 B15：接口、工具与迁移边界（2026-11-06）](zhuzhao-ecosystem-audit-b15-contract-migration-boundaries-2026-11-06.md)
- [生态全面审计 B16：Phase5安全验证（2026-11-07）](zhuzhao-ecosystem-audit-b16-security-validation-2026-11-07.md)
- [五仓审计 B18：生成工具与模板回滚预检](zhuzhao-ecosystem-audit-b18-tooling-rollback-2026-11-07.md)
- [B19：存储输出ID字符串契约](zhuzhao-ecosystem-audit-b19-storage-id-2026-11-07.md)
- [B20 / G-12：前端即时反馈与失败恢复](zhuzhao-ecosystem-audit-b20-ui-feedback-2026-11-07.md)
- [B21 / L-5：工单与架构主体文档校准](zhuzhao-ecosystem-audit-b21-design-docs-2026-11-07.md)
- [B23：表达式字典查询复用当前事务与请求上下文](zhuzhao-ecosystem-audit-b23-expression-context-2026-11-07.md)
- [B24：三仓 PostgreSQL 冷启动就绪判定](zhuzhao-ecosystem-audit-b24-postgres-readiness-2026-11-07.md)
- [B25：审计JSONL持久化与独立恢复](zhuzhao-ecosystem-audit-b25-archive-persistence-2026-11-07.md)
- [B26 / G-14：字段选项变更软提示边界](zhuzhao-ecosystem-audit-b26-field-warnings-2026-11-07.md)

## 较早正式审计与验证

以下保留独立基线、原发现、修法复验及权限时点快照。12/14两份权限矩阵的端点口径不同，当前L1验证看B11；不能把历史端点数相加或当今天的覆盖证明。

- [12 · B13 权限覆盖矩阵（point-in-time 审计）](12-b13-permission-matrix-2026-09-26.md)
- [14 · B13 权限覆盖矩阵复审（2026-10-26 point-in-time）](14-b13-permission-matrix-2026-10-26.md)
- [activelist 阶段 A 收尾走查记录（2026-09-14）](activelist-stage-a-walkthrough-2026-09-14.md)
- [zhuzhao 全项目代码核查报告（2026-09-26）](code-audit-2026-09-26.md)
- [27 批修复验证报告（2026-09-26）](fix-verification-2026-09-26.md)
- [JSON 契约一致性审计（zhuzhao / activelist / taskrunner / zhuzhao-utils）](json-contract-audit-2026-10-14.md)
- [p4-w0 提交审查（2026-09-22）](p4-w0-commit-review-2026-09-22.md)
- [Phase 3 收官报告（2026-09-15，口径 A）](phase3-closure-report-2026-09-15.md)
- [Phase 5 提交与规划全景审计（2026-10-27）](phase5-postaudit-2026-10-27.md)
- [zhuzhao 生态 · 实现↔文档一致性 + 设计合理性 + 业界对标 审查报告](zhuzhao-audit-2026-09-11.md)
- [五批代码审计报告（2026-10-21，双线合并版）](zhuzhao-batch-audit-2026-10-21.md)
- [zhuzhao 全项目「未考虑问题」研究（2026-09-22）](zhuzhao-blind-spots-2026-09-22.md)
- [zhuzhao 能力差距核查 · 合并终报（2026-10-09）](zhuzhao-capability-gap-audit-2026-10-09.md)
- [zhuzhao 全面代码检查报告](zhuzhao-code-review-2026-09-11.md)
- [zhuzhao 代码检查 · 发现二次确认报告](zhuzhao-code-review-verification-2026-09-11.md)

## 证据与后续报告

正式报告进入本目录，原始执行卡、日志、截图、快照和临时探针放ignored `.codex/audits/<轮次>/`；共享导航只链接纳管报告。报告需写明证据可恢复边界，不能把同盘备份当异地灾备，也不能用NOT_RUN、命令桩或概率性负控代替实际通过。

后续正式报告使用`主题-YYYY-MM-DD.md`，保留既有批号/发现编号。历史计划、导读及合并前资料的出处见[archive索引](../archive/README.md)，已拍设计与当前方法留在对应权威文档，不机械随日期归档。

- [Phase6前全项目重规划审查（2026-11-11）](pre-phase6-replanning-2026-11-11.md)：合并后五仓、供给边界、AI双轨、工程/前端与全部遗留去向；规划和实现验收分开。
