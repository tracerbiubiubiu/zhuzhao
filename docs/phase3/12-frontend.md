# 12 - 前端工程方案（frontend）

> **⚠ 状态变更（2026-09-02，design-decisions §23）**：工单自研暂缓（内部引擎优先，自研兜底）——本文档转**参考规格**（Phase 2 侧 BK-18 管理页/动态表单仍有效，Phase 3 审批/报表前端暂缓）。

> **状态**：已编写（2026-08-31，范式参考 Duke1616/ecmdb-web 调研）｜ **Wave 归属**：W2（审批/报表页）；管理页与动态表单属 BK-18（IW3 独立窗口，Phase 2 侧）
> **覆盖范围（2026-08-31 明确）**：本文档 = 前端**总规格**——含 **BK-18 前端**（类型/字段/模板管理页 + 动态表单渲染器，IW3 批次）与 **Phase 3 审批前端**（审批人配置/审批操作页，W2/7c）；BK-18 前端实施以本文档 §2/§3.1–3.2 为规格，无需另立文档。
> **后端契约**：以 zhuzhao 代码为准（已实现：`GET /ticket-types`、`GET /ticket-types/:code/fields`、`GET /ticket-templates`；规划：类型/字段/模板 CRUD，见 phase2/00 §9 BK-18）。

---

## 1. 技术栈（2026-08-31 拍板）

Vue 3 + TypeScript + Vite ｜ Element Plus ｜ Pinia + Vue Router ｜ UnoCSS（可选）

**schema 表单两段路径**：
- **阶段 1（随 BK-18）**：手写 `DynamicForm.vue` 渲染器——按字段 schema（`field_key/label/type/options/required/sort_order`，消费既有 `GET /ticket-types/:code/fields`）switch 映射 Element 组件 + `defineExpose(validate)`。字段类型 7 种：input / textarea / number / date / select / multi_select / tips。
- **阶段 2（远期可选）**：form-create 设计器（`<fc-designer>` 拖拽 → `rules/options` JSON），后端存储不变（仍是字段 schema），仅前端换渲染与配置方式。

## 2. 能力地图（页面 ↔ 后端契约）

| 页面 | 后端契约 | Wave |
|------|---------|------|
| 登录/菜单/权限 | 既有 auth + user/menus + user/permissions | 已有 |
| 工单发起（分类侧栏 + 模板卡片 + 动态表单） | 既有读 API + `POST /tickets` | BK-18（IW3） |
| **类型/字段/模板管理页** | BK-18 CRUD API（规划：`POST/PUT/POST .../disable`，发布/停用两态；有工单的类型禁删） | BK-18（IW3） |
| 工单列表/详情/处理 | 既有工单 API | 已有 |
| 审批人配置页 | `PUT /workflows/:code`（7-0 设计产出，权限码 `workflow:manage`） | W2（7c） |
| 审批操作页 | `workflow-tasks` API（通过/驳回/转签，WhatCanIDo 驱动按钮） | W2（7c） |
| SLA/通知/报表 | 7a/7b/7e API（`report:read`） | W2 |

## 3. 关键范式（ecmdb-web 参照，落到 zhuzhao）

### 3.1 动态表单渲染器（阶段 1 核心）

```ts
interface DynamicFormField {
  key: string; label: string;
  type: 'input'|'textarea'|'number'|'date'|'select'|'multi_select'|'tips';
  required: boolean; options?: {label:string; value:string}[];
  props?: Record<string, unknown>;      // placeholder 等透传
}
```
- `v-for` + 按类型映射组件；el-form rules 由 `required` 动态生成；提交值收进 `custom_data`（`{[key]: value}`）。
- **不做**字段联动/条件显隐（eflow 亦仅有 hidden/readonly，够用）。

### 3.2 管理页三件套（类型/字段/模板）

列表 Table + **三步向导**（基本信息 → 字段设计 → 设置/绑定）+ 字段编辑**右侧抽屉**（35%）+ 字段类型卡片选择器 + 拖拽排序。状态机编辑（types.transitions JSONB）以"源码模式 JSON 编辑器 + 常用模板按钮"呈现（可视化状态图编辑器为远期）。

### 3.3 审批人配置页（W2/7c）

统一数据模型 `Assignee { rule, values }`，策略注册表：指定人 / 发起人 / 模板字段 / 部门领导 / 分管领导 / 团队 / 部门（7 种，min_level 由 Assignee 模型替代——2026-08-31 拍板）。UI = 策略列表卡片 + 多 Tab 选择弹窗 + 会签开关。后端只存 rule+values，解析在运行期（对接组织表 / 2c 委托）。

### 3.4 审批操作页

单页承载：只读工单表单（动态表单 disabled 态）+ 当前节点动态表单 + 评论；同意/驳回/转签三按钮，ElMessageBox 二次确认，按钮显隐由 `WhatCanIDo` 接口驱动；草稿按流程实例 ID 缓存（内存 Map）。

### 3.5 权限前端表达（capability 三件套）

- `capability` 强类型字典（`"ticket:create"`、`"workflow:manage"`、`"report:read"`…），来源 = 后端 `user/permissions` 下发；
- `usePermission(ANY/ALL)` composable + `<AuthButton :capability>` / `v-permission` 指令（无权限销毁或置灰）；
- 路由/菜单级沿用后端菜单下发（既有机制），按钮级用三件套——与 ecmdb-web 同构，且后端零新增。

## 4. 工程结构（建议）

```
web/
  src/api/<域>/<资源>/{index.ts, types/}
  src/pages/<域>/<功能>/{components, composables}
  src/common/components/{DynamicForm, Workflow?}   # Workflow 组件远期画布才引入
  src/common/auth/{capability.ts}
  src/directives/permission/
```

## 5. 验收标准

| # | 用例 | 通过标准 |
|---|------|---------|
| FE1 | 动态表单 | 7 种字段类型渲染正确；required 校验拦截提交；值落 `custom_data` 并在详情回显 |
| FE2 | 管理页 | 前端完成类型+字段+模板建配全流程，全程无 SQL |
| FE3 | 权限 | viewer 角色看不到管理入口（路由+按钮双级） |
| FE4 | 审批页 | WhatCanIDo 驱动按钮显隐；通过/驳回/转签全链路（7c 联调） |

## 6. 开放问题

- 画布（LogicFlow）：远期，`flow_data` 存 GraphConfigData JSON（eflow 已验证同构），上画布时后端仅加全量保存 API。
- 移动端/响应式：未规划。
