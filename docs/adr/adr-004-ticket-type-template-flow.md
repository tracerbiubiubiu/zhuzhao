# ADR-004: 工单类型 / 模板 / 流程三者关系 —— 维持单向（类型为根，模板为预填预设）

## 日期
2026-10-27

## 状态
已采纳（[phase6/00 号](../phase6/00-flow-engine-v2.md) §5 拍板点 2，所有者 2026-10-27 拍 A）

## 背景
- phase6 启动段 P6-R0（流程引擎 v2）要交付出边条件构建器（B0-5）与定义快照版本化（B0-3），两者都先要回答「条件左值从哪一份字段 schema 取」「快照以什么为单位、钉住什么」——答案取决于类型 / 模板 / 流程三者的归属关系。
- 现状（代码级）：`ticket_types` 持 states / transitions / transition_meta（流程定义），`ticket_type_fields` 持字段 schema（含 `field_meta`：formula / visible_when / fetch / 布局）；`ticket_templates` 有 `type_code` 外键与 default_fields / default_priority / default_sla_minutes、org 隔离——**模板不持表单 schema、不持流程**，是类型下的发单预填预设。`CreateTicketInput.template_code` 后端已实现、前端发起页未接（03 号 §2 缺口①）。
- 三方对标（2026-10-27 代码级盘点）：eflow（ecmdb 体系）用 form-create 模板 ↔ workflow **双向引用**（B 形态）；ficus-oa 流程持 formId + 节点级表单（A 形态）。

## 决策
**A：维持单向。**
- 类型 = 字段 schema + 流程定义的**根**；模板 = 类型下的预填预设（org 隔离），**不升为独立资源**；流程**不脱离类型**成为跨类型复用的工件。
- 条件构建器（B0-5）左值 = 同类型字段 schema 下拉；定义快照（B0-3）以类型为单位、**含字段**（00 号拍板点 5）。
- 模板选择的前端接入随 [06 号](../phase6/06-ticket-completeness.md) B2 波（既有登记缺口①，零契约变更）。

## 理由
- **与服务台 / OA 主流同形**：ServiceNow（表为根、字段即列、`sys_template` 预填、Flow 挂表）、钉钉 / 飞书审批（审批模板 = 表单 + 流程 + 可见范围一体）、ficus-oa。
- **Jira 的反向先例**：公司管理项目的 scheme 体系（Workflow Scheme / Screen Scheme / Field Configuration Scheme）是 B 的完整形态，也是管理员投诉首位；Atlassian 后推出团队管理项目（每项目捆绑配置）即回到 A。B 的价值只在数百项目复用同一套流程 / 表单的规模下兑现。
- **eflow 的双向引用有前提**：form-create 表单天然是独立产品；zhuzhao 没有这个前提，字段本就归类型。
- **零迁移**：A 不动任何表；B 需模板升独立资源、流程独立成表、全部 transition_meta / 字段引用改绑。
- **B 阵营真正有价值且与 A 兼容的两件已另行吸收**：Jira Transition Screen（转换时弹字段）= 06 号件 1「转换时字段采集」；ServiceNow UI Policy（按状态控字段可见 / 必填）= P2「状态级字段权限矩阵」。

## 后果
- 正面：B0-3 / B0-4 / B0-5 的引用设计落定；快照以类型为单位含字段（在途单按自身版本渲染与校验）；模板接入为小批。
- 负面 / 风险：同一流程要在多个类型间复用时需各自配置——由 06 号件 7（类型定义包导入导出）与 P2 类型克隆缓解。
- **翻案条件**：出现「同一流程被多个表单复用」或「同一表单服务多个类型」的真实诉求时重评 B；届时迁移路径 = 模板升独立资源 + 流程定义独立成表 + 引用改绑，并需为在途单快照补「模板版本」维度。
- 跟进：00 号 §5 拍板点 2 标 ✅（已回写）；06 号 B2 波接入 `template_code` 前端。
