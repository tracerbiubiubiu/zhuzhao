# Phase 6 开工前 Web 易用性复验

> 用途：记录两轮浏览器走查、跨助手复证和现有工作包的验收承接。范围：现有 Web 页面及主要用户旅程。状态：发现已登记，产品改造待交付。
> 审计基线：Core `42f83a1`、UI `232afbdd`、ActiveList `d2fd455`、Taskrunner `30b839a`。历史对照为 [11 月 13 日前端全站审计](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/232afbdd3ec40edbdb6ab0ab50e4a6fb4fc0a333/docs/review/reports/frontend-full-audit-2026-11-13.md)。

## 优先补进首批验收

1. **待办链接首次打开或刷新会丢失查询条件（U01、U03、U07）。** 普通处理人从首页「我的待办」进入 `#/tickets?assignee=me&statuses=assigned,in_progress`，或从顶栏进入 `#/tickets?assignee=me&exclude_statuses=closed,rejected`，在应用内均显示 3 条。整页刷新、已登录的新标签直接打开同一链接，两种地址都变成 `#/tickets`，列表请求返回 200，查询只剩分页参数，结果为 23 条可见工单。DeepSeek 首报，主审独立复现两条链接。验收同时检查站内进入、刷新、新标签的 URL、筛选控件、请求参数和结果集。源码锚点为 UI `src/permission.ts` 的动态路由注册后重放。
2. **从详情回不到原工作位置（U03、U07）。** 在预筛选待办列表加关键词，结果由 3 条收窄到 1 条。进详情后点「返回」，关键词清空、结果恢复为 3 条。另一条预填关键词的路径由 1 条恢复为 20 条。普通无预筛选的列表能保留手工条件和页码。新标签独立打开详情后点「返回」可到 `about:blank`；经登录重定向进入详情时，也可回到登录表单。验收分别覆盖重新点击待办入口、从详情恢复原条件、页码、滚动与行焦点，以及无来源详情的受权应用内落点。源码锚点为 UI `src/views/ticket/list/index.vue` 的 `applyRouteFilters`、`onActivated` 和 `src/views/ticket/detail/index.vue` 的返回按钮。
3. **关键入口缺少键盘等价操作（U02、U03、U06、U07）。** 首页最近工单整行和工单列表标题可用鼠标打开详情。实际 Tab 跳过标题，列表标题在可访问性树中呈现为单元格文本；消息行同样跳过。桌面头像菜单打开后，在「个人中心」菜单项按 Enter 只收起菜单，鼠标点击可进个人中心，窄屏头像触发器没有可访问名称。验收使用真实路由链接和有稳定名称的菜单项，覆盖 Tab、Enter、菜单项的 Space 及原生链接的复制与新标签打开。源码锚点为 UI `src/views/Home/index.vue` 的最近工单、`src/views/ticket/list/index.vue` 的标题插槽、`src/views/message/center/index.vue` 的消息行及 `src/components/UserInfo/index.vue` 的菜单项。
4. **覆盖层里的焦点和 Esc 需要按层处理（U02、U07）。** 1440px 表格聚焦后仍能 Tab 或 Shift+Tab 到被遮住的壳层。390px 收起侧栏时首个 Tab 可进入视口外 Logo；展开后 Tab 可进入遮罩下的顶栏，Esc 无法关闭侧栏。第二轮在聚焦表格里打开「新建用户」，按一次 Esc 会同时退出弹窗和底层表格聚焦。验收覆盖收起时退出 Tab 顺序、展开时移入焦点、背景控件不可键盘操作、Esc 逐层退出及焦点归还。源码锚点为 UI `src/layout/Layout.vue` 的侧栏遮罩与 `src/components/ProTable/index.vue` 的 `toggleFocus`。

## 视觉、框体与状态反馈

- **窄屏分页与动作框（U02、U06、U07）。** 在 390px 工单列表里，472px 的分页行把「共 23 条」完全裁出视口，每页条数只露下拉控件的边缘。在同宽度用户管理页，480px 的「新建用户」弹窗把保存和右上关闭按钮裁出首屏。768px 下两者均完整可见，桌面弹窗关闭后能归还焦点。建议为共用分页保留总数与翻页入口，按宽度收纳页大小与跳页；短操作窗限制到视口内。活动数据主从栏在 390px 挤压是既有发现，本轮用真实上游的合成数据复见。源码锚点为 UI `src/components/ProTable/index.vue` 的分页模板、`src/views/system/user/index.vue` 的新建弹窗。
- **亮暗主题读数（U02、U07）。** 首页亮色 24px 粗体计数有 5/6 低于大字的 3:1，最低 2.19:1。暗色 14px 卡片说明为 3.17:1，低于普通文字的 4.5:1；暗色大数字实测为 4.98–7.01:1。首页暗色最近工单标题的稳态悬停在独立复测中约为 10.22:1。建议保留紫色主操作，补亮暗语义色、持久可读的字段标签和统一状态词表。源码锚点为 UI `src/views/Home/index.vue` 的统计卡与 `src/components/FilterBar/index.vue` 的折叠控制。[WCAG 2.2 对比度口径](https://www.w3.org/TR/WCAG22/#contrast-minimum)
- **多工单辨识（U03）。** 同时打开两张不同工单，两个应用页签和两个浏览器标题均显示「工单详情」。同一票的优先级在列表为「中」，详情为「3」。若保留应用页签，建议与浏览器标题、详情及操作确认一起显示稳定单号，按当前可见性显示短标题，列表和详情使用同一优先级词表。源码锚点为 UI `src/hooks/useTitle.ts` 的 `useTitle` 与 `src/views/ticket/detail/index.vue` 的优先级展示。
- **头像裁剪（U06、U07，沿既有 F18–F20）。** 选 640×360 合成静态图片后，Tab 只落到关闭、取消、确认。逐一测试四个方向键与加减键，裁剪均无变化。滚轮让预览图宽从约 388 CSS px 变为 426 CSS px。Esc 能回到上传触发器。建议补可命名的缩放和移动控件，本轮在确认上传前退出。源码锚点为 UI `src/components/StorageAvatarUpload.vue` 的裁剪对话框。[既有头像事项](../../phase6/07-backlog-and-triggers.md)
- **已排事项的失败样本（U03、U06、U07）。** 建单刷新后标题和描述清空；待办 tab 初读时显示 0，真实加载后显示 3。持久化合成评论可正常读取，单独注入评论 GET 503 时，详情显示「暂无评论」且区块无重试，撤去注入后可重读。单独注入标读 POST 503 时，仍进入工单详情，页面无标读失败提示，返回后保留原未读数。已排草稿、空错态、消息状态和责任口径沿既有包验收。源码锚点为 UI `src/views/ticket/detail/index.vue` 的 `commentsQuery` 与 `src/views/message/center/index.vue` 的 `openItem` 和待办 tab。[页面族详设](../../phase6/frontend-page-family-detail.md)

## 工单重设计的计划归属

- **P6.3 U03「工单旅程与批量标签」** 覆盖日常工单体验。列表重排四视角、编号、组合筛选及批量操作；发起与编辑收口字段、预填与本地草稿；详情按主栏的说明、字段、证据和活动流，与侧栏的负责人、状态、时限等上下文组织，处理动作与管理动作分区。
- **P6.3 U04「表单与流程配置体验」** 覆盖管理端类型、字段、流程配置工作区及预览、冲突恢复、未保存保护、草稿与发布。U02 提供壳层、视觉变量及共用组件，U07 提供代表性浏览器验收，两者从 P6.1 样板起随批交付。
- 首条人工纵切的薄 UI 按已选办理、取证和复核流程交付。完整工单旅程和管理端重设计按 U03/U04 推进。本次报告为现行计划补浏览器回归场景，保留既有工作包、范围与估算。最终页面样板及交互取舍随对应实施窗口确认。[规划总账](../../planning/planning-backlog.md#p63-工单体验与受控业务写入) · [工单模块详设](../../phase6/ticket-module-detail.md#6-u01u07-页面与状态详细合同) · [UI 施工合同](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/232afbdd3ec40edbdb6ab0ab50e4a6fb4fc0a333/docs/design/frontend-implementation-contract.md)

## 证据与边界

两轮以同一版本组合分别重建隔离环境，通过 Chrome 158 走查 27 个清单路由入口。在 390、768、1440 CSS px 抽查真实登录、首页、列表、详情、发起、消息、系统管理和亮暗主题。使用合成账号及数据，活动数据连接隔离 ActiveList，任务中心连接隔离 Taskrunner。第二轮由主审直接委派 Codex、GLM 与 DeepSeek 三位前端检查者及一位产品体验设计者。检查者使用独立浏览器会话，关键路径由不同助手或主审复现。产品体验设计者复核计划归属和验收取舍。

评论与标读的失败状态来自浏览器响应注入。浏览器结果证明该基线下观察到的交互，实际触屏、原生读屏、完整写入矩阵、外部情报与对象存储接线仍需对应验收。报告保留复现所需的基线、角色、条件、操作和结果，原始截图、探针、测试状态和日志留本地。本次仅更新文档，产品、部署及准入状态保持原记录。
