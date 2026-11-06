# 生态全面审计 B08：前端功能、交互与视觉（阶段报告，2026-11-06）

## 范围与基线

受审 UI 固定基线 `b20594cc744a61e2bcb41535b0ea668a711aa4df`；原仓 `/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao-ui` 的他方 `.gitignore` 不在本批。修复位于主仓本地 `.codex/audits/ECO-20261104/worktrees/ui-r03`、分支 `codex/fix-ui-audit`，不合 main。原 API 快照为主仓 `5c03581d5999139a3e8e5ead5972c026868aa181`，兄弟应用冻结原镜像；联合曾运行主仓 `d38f10f9c60a8457417c2630afba0926b103797d`，当前已换主仓 `f69b7cbefebff943972d24c647776375fb8d1f90`、taskrunner `a1309cc0fd28a2c488a5ef7fdeb47dc68002d8bd`，独立 RustFS59000。运行名为b205的UI镜像其静态文件被本批替换，**镜像名不能代替前端产物来源**；备份和逐文件SHA在证据目录。

设计源：主仓 phase4/01、03，phase5 S1/S2/S3/S4/S5/S6/S9/S10 对应前端边界，UI `frontend-standard`/`testing-standard`；参照 phase6/08 和 phase7–9 总纲。未来风险/工作台合并/脚本中心/DAG 未实施不算本阶段缺陷。设计文件全文与相关章节阅读分列，本报告不宣称全 docs 全文精读。

状态：**阶段审计与修复复验仍进行中，未到全面审计收口**。以下为当前可长期追溯的发现与证据，未通过固定提交完整六件门禁的项不提前关闭。

## 发现与修复状态

### B08-F01 / P1：草稿跨账号/类型恢复，丢弃和提交后清理不完整

原快照锚点（完整hash见基线）：[src/components/RichtextField.vue:74](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/components/RichtextField.vue#L74) 的 `watch / onMounted / clearDraft`；[src/views/ticket/create/index.vue:1](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/views/ticket/create/index.vue#L1) 的 `draft-key / handleSubmit`。

原基线 `src/components/RichtextField.vue` 的 watch/onMounted/clearDraft，`src/views/ticket/create/index.vue` 的 draft-key/handleSubmit 与 userStore.resetState：key仅含字段，缺少用户/类型，丢弃只隐藏提示，成功建单未清缓存。

真实旧栈浏览器：合成账号A在类型A编辑草稿，正常退出后账号B选类型B同名字段恢复A全文；丢弃后内容仍在。证据 `draft-A-edit.png`、`draft-B-other-account-restored.png`、`draft-discard-retains-content.png`，范围限同一浏览器origin的本地内容，不声称后端越权。

修复 `fc1068b66ea4f33f2f1a02e9a9ef557b488c7560`：账号/类型/字段完整作用域，旧无属主草稿安全删除，统一model同步、timer取消、丢弃与提交后复原。S8d真实栈+API回读通过，固定产物 lint/type/unit/build 证据已有；整体六件门禁待联合收口。状态：待最终复验。

### B08-F02 / P1：旧会话异步结果覆盖或清理新登录

原快照锚点（完整hash见基线）：[src/common/request/src/index.ts:63](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/common/request/src/index.ts#L63) 的 `refreshAccessToken`；[src/store/modules/user.ts:1](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/store/modules/user.ts#L1) 的 `loadSession / resetState`。

原基线 `src/common/request/src/index.ts` 的 refreshAccessToken/401分支、`src/store/modules/user.ts` 的 loadSession/resetState。三个deferred实验：旧A刷新成功覆盖B token；A刷新终态失败清B会话；A会话加载晚到覆盖B profile/permissions/menus。原19条请求单测绿，三目标红；未突破后端鉴权。

修复 `604b96c2fb6133fa75b2800f7121d5badd0c966c`：generation隔离签入/拆除、普通成功/失败响应、刷新单飞与store/守卫写入。三目标移除修复8轮均红；S16换账号/S8d2例真实栈通过；固定产物基础门禁通过。状态：待最终六件复验。最初跨目录mock解析错误的实验无效，以isolated日志为准。

### B08-F03 / P1：首页/角标点击与列表筛选口径不一致

原快照锚点（完整hash见基线）：[src/views/Home/index.vue:103](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/views/Home/index.vue#L103) 的 `goTickets`；[src/components/NavbarBadge.vue:36](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/components/NavbarBadge.vue#L36) 的 `goTodo`；[src/views/ticket/list/index.vue:1](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/views/ticket/list/index.vue#L1) 的 `URL筛选初始化`。

原基线 `src/views/ticket/list/index.vue` 的URL初始化、`src/views/Home/index.vue` 的goTickets、`src/components/NavbarBadge.vue` 的goTodo。AppView按fullPath缓存；先点待处理卡、手动改为已关闭、回首页再点同一待处理卡，URL=open但筛选仍closed。不同query首次进入的对照正常，不能把缺口误说成所有query变化都失效。

修复首笔 `ad5874590dd3ccee9f878541e629cc1a3ff075df`：显式深链每次激活恢复，普通无query页签仍保筛选；Home待办统计原为assigned+in_progress，跳转却仅assignee=me，改服务端statuses集合并联动导出/状态回显，配主仓cd79374。缓存+Enter候选8/8绿，联合S2已验证集合包含本轮assigned、排除closed。

同类角标仍在途：BadgeService.todoCount为NOT IN(closed,rejected)，与Home两状态语义不同；仅跳assignee=me同样混闭单。主仓已补9e2fef6，UI配套 `a1f8476c1102328a0c39dbe132f34befdb0506b9` 同步NavbarBadge/list/export/codegen，排除集合回显、单态改选/重置退出，S2新增真实custom图状态、角标count=list.total与双入口导出/缓存回归。真实exclude运行验证待固定UI新产物；不能客户端分页后裁剪或把角标计数改成两状态。状态：F03待联合复验。

### B08-F04 / P1：选催办上级恒400，抄送大ID丢精度

原快照锚点（完整hash见基线）：[src/api/ticket/index.ts:239](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/api/ticket/index.ts#L239) 的 `AssignTicketInput`；[src/views/ticket/detail/index.vue:215](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/views/ticket/detail/index.vue#L215) 的 `addCC / addSelfCC`。

原基线 `src/api/ticket/index.ts` 的assignTicketApi/CreateTicketInput、建单handleSubmit、详情doAddCC/addCC/addSelfCC。supervisor经Number出站但后端标量json,string；CC的Number把9007199254740993变9007199254740992。

真实旧app合成验证：supervisor数字1→400/10001，同值字符串→200；实际大ID用户的JS舍入CC→400“抄送用户不存在”，精确numeric control→200，string[]旧DTO→400。只证明选人失败，**未声称已错误抄送给另一用户**。账号/消息夹具已清；首轮清理遇notifications FK未完成，随后仅清自有recipient后成功，原失败不抹去。

修复 `2d4afd06ff703cf26609af7eca7cd5f0ae6ed32c`：两路supervisor、CC两入口保持string；两处ID排序用BigInt；主仓91d7b90接收Int64Slice兼容旧数字，codegen同步string[]。原supervisor请求单测实红；新传输/排序4条与全224条通过。S17真实浏览器合成>2^53用户，建单/CC/分派三路payload为精确字符串，建单回读与CC chip均通过（joint-target-e2e-v3.log）。前两轮S17因teleport旧option测试定位红，不计产品失败；已按combobox aria-controls+键盘选择更正并提交 `e853323405eb9c7856d80605fd724c414e65bd5b`，同笔含S3真实menuitem与S8预览DOM定位。状态：待固定提交全门禁。

### B08-F05 / P2：类型配置仍显示已废止的“三态不可达”提示

原快照锚点（完整hash见基线）：[src/views/ticket/type/index.vue:738](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/views/ticket/type/index.vue#L738) 的 `类型编辑弹窗alert`。

原基线 `src/views/ticket/type/index.vue` 编辑弹窗alert与两处API/组件注释声称in_progress/pending_verify/rejected三态经API不可达；S3a转换入口/动态可执行动作已交付且有真实E2E，与当前产品相矛盾。

修复 `f23d081080cf49354fd7017b92c59ffecbb735f6` 提示改为类型图决定转换、实际动作受权限/条件/校验器约束，审批节点尚未启用。P5-S2候选真栈断言新提示及旧文案消失通过。状态：已提交待固定复验。

### B08-F06 / P1：活动数据未选择类型仍显示无目标写入入口

原快照锚点（完整hash见基线）：[src/views/al/data/index.vue:175](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/views/al/data/index.vue#L175) 的 `openCreate / 空态模板`。

原基线 `src/views/al/data/index.vue` 的空态分支/openCreate：未选类型有选择提示，却同时显示“暂无数据/写入首条”，可打开类型名为空的写入框。截图 `visual-20261106/1024x900-_al_data.png`；新增W5初始反断言原样失败（按钮count=1）。

修复 `f23d081080cf49354fd7017b92c59ffecbb735f6` 以selectedType门控数据空态/表格，openCreate同时重验有效且未废弃类型。W5正反旅程候选两例通过（写入/cursor分页/软删恢复、废弃只读；未选类型没有写入口）。状态：已提交待固定复验。

### B08-F07 / P1：表格聚焦/密度的键盘动作被Tooltip截走，聚焦图标缺名称

原快照锚点（完整hash见基线）：[src/components/ProTable/Tools.vue:16](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/components/ProTable/Tools.vue#L16) 的 `Tooltip按钮模板`。

`src/components/ProTable/Tools.vue` 包原生按钮的Element Plus Tooltip默认triggerKeys含Enter/Space，keydown preventDefault阻止按钮本身激活。新增真实S2聚焦Enter断言失败；修复 `f23d081080cf49354fd7017b92c59ffecbb735f6` 给Tooltip trigger-keys=[]并提供aria-label/pressed，Enter入聚焦/Esc出聚焦真栈通过。既有视觉探针五个列表页发现同一共享无名按钮，非五条重复发现。状态：已提交待固定复验。不据此声称WCAG全面符合。

### B08-F08 / P1：表格初挂unobserve(undefined)抛错，后续观察注册中断

原快照锚点（完整hash见基线）：[src/common/hooks/src/useTableFit.ts:41](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/common/hooks/src/useTableFit.ts#L41) 的 `syncObserve`。

原基线 `src/common/hooks/src/useTableFit.ts` 的syncObserve：首次已有observer但observedEl未赋值，无条件unobserve(undefined)。S17真实浏览器控制台同签名TypeError，新增初挂/换表/卸载回归在同一行红。

修复 `80c34da918fb29cc5c60845c215567a522cd5bf2`：旧目标非空才unobserve，继续observe新目标并挂resize。回归绿；移除守卫独立副本8/8目标TypeError红；基础门禁28文件225例通过。状态：待最终固定快照与浏览器回归。

### B08-F09 / P2：在线图标组件与离线注册表隔离，工具图标依赖外网且空白

原快照锚点（完整hash见基线）：[src/components/ProTable/Tools.vue:8](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/components/ProTable/Tools.vue#L8) 的 `Icon局部导入`；[src/components/NavbarBadge.vue:1](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/components/NavbarBadge.vue#L1) 的 `Icon局部导入`；[src/views/system/setting/index.vue:1](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/views/system/setting/index.vue#L1) 的 `Icon局部导入`。

原基线 `src/components/ProTable/Tools.vue`、`src/components/NavbarBadge.vue`、`src/views/system/setting/index.vue` 的局部Icon从`@iconify/vue`导入，全局registerIcons却写`@iconify/vue/offline`。包的两个入口函数不相同，SSR同图标offline输出path、online输出空SVG。真实浏览器阻断外网后四布局亮暗工具的svg path=0，发出api.iconify.design两组取图请求；注册表虽完整仍不生效。

修复 `4908d699a1a74b74246f074d0619a462a49a45a1` 三处导入offline；新增全src视图入口扫描原样精确命中三处红，修复后28文件226例/lint/type/build绿，计数README/AGENTS同批同步。证据icon-registry-before.json、icon-import-before.log、visual-extra-v3-20261106/observations.json。状态：已提交待阻断外网固定产物复验。

### B08-F10 / P2：亮色状态/优先级徽标对比度偏低

原快照锚点（完整hash见基线）：[src/views/Home/index.vue:170](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/views/Home/index.vue#L170) 的 `最近工单状态/优先级Tag`。

经root视觉复核提出并独立核DOM：Home最近工单字样完整，静止opacity=1，不是缺字或状态映射丢失。warning前景230/162/60与背景253/246/236比值2.03898；info144/147/153与244/244/245比值2.80166，12px语义文字低于[W3C常规文字4.5:1说明](https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html)。暗色同前景配暗底7.18259/5.24193通过该单项。取computed颜色而非截图像素，未声称全站WCAG不合规。

修复 `c83a53ded725da167eb1e9b3d408320336e25fb7` 限定light/plain Tag较深语义文字，保留背景、主色、dark与实色effect；候选warning6.33214/info6.87556。S2新增真实computed断言在旧产物精确红（review_custom的info2.80166）；lint/type/build绿。证据tag-contrast-calculation.json、tag-contrast-before.log及截图、亮暗17图DOM。状态：已提交待固定产物正向复验。

## 视觉、习惯与基本可访问性覆盖

本轮真实测试浏览器20条路由×1366×900、1024×900、2560×1440，60张页面截图+DOM观察见`visual-20261106/observations.json`。覆盖Home、工单列表/发起/类型/分派规则、任务、AL类型/数据、审计、情报、通知配置、字典、用户/角色/菜单/组织/设置、Profile、MyOrg、消息中心。该批观察外层document/section滚动均为0，路由无404；截图人工复核了首页、任务表、建单窄窗、用户宽屏、设置、消息和AL空态等代表页。

风格结论：紫色主操作、卡片/筛选/分页位置和抽屉/对话框有统一范式；宽桌面留白属于当前封顶列表规则，不当作纯审美bug；AL双空态、流转误导提示、键盘缺口与挂载异常为上述可验证问题。另补四布局×亮暗×Home/工单列表的1024宽16图及暗色类型弹窗1图（visual-extra-v3-20261106），可真实键盘开布局、切主题、Esc关弹窗，无外层滚动/pageerror；长标题用例最后详情定位漏#ID前缀而失败，长详情尚待补完。前两轮分别因隐藏input点击等待和路由切换未落定命中旧Home两行而失效，保留日志，不算产品缺陷；布局面板退场动画被截到，后续等待隐藏。60张默认Tab快照与17图不代表所有Tab/表单都通过。输入的DOM标签缺失探针仅作线索，EP组合控件须按真实可访问名称复核，不机械当成批量合规缺陷。

CUA本轮尝试iab不可用、Chrome建窗不支持、getApp超时，未冒充新人工CUA旅程；原B08已有CUA截图保留，新增截图来自仓内真实Playwright审计探针。浏览器功能测试与人工视觉判断分列。

## 门禁、失败记录与当前限制

旧基线六项中的lint/typecheck/单元202/供应链audit/build通过；旧全E2E40例35通过5失败保留，死信两角色500由taskrunner修复承接，S3菜单定位与S8c开发期Vue内部状态依赖已校准。新联合候选9例首轮7绿（含S8c真实RustFS贴图），新S17/S2测试准备/定位问题如上；S2最初新夹具assigned→closed违反图、随后误写to_status，均保留并改用真实to契约。当前候选S17已过，S2+W5三例已过；不能据此代替完整41例或产品验收。

一次本地E2E误并行启动已结束；错误部署过/dist-test/前缀产物导致测试准备无效，主动中止并恢复正确base=/生产产物，相关日志保留，不作为产品缺陷。嵌套工作树第一次unit未设置ZHUZHAO_REPO而失败，显式路径后通过。最终固定提交验证须在无并行Playwright条件下，完整重建、核产物哈希后运行六件门禁；原始证据不覆盖。

UI对AL导入仍明确1MiB前置，nginx默认限制与后端网关边界须独立对账；AL直连修到1GiB不能自动扩大UI/网关承诺。未擅自改此边界。RichtextView catch直接拼HTML是不可达组件线索（全src无生产导入），不报可达XSS；当前md-editor-v3默认XSS处理已核实，服务端渲染SSOT仍需主仓证据。

## 接续

exclude、UX/AL、真实测试、离线图标与对比度均已提交；固定提交完整六件门禁、变异与同类浏览器再验；补主题/布局/长数据视觉和账号/失败面。本报告与11号控制台由主会话统一维护；过程草稿不纳管。所有raw日志、token/真实数据均不入库；样例内容全合成。

## 模块覆盖清单与演进边界（阶段对账，不等同每输入全覆盖）

- 壳层/IAM：phase4/01 §3–6→boot/permission/router、user/tokenStorage/request、动态菜单和v-permission；已有S1/S16/FE3/FE4及新F02延迟交错，服务端三层最终判定由主仓批次承接。
- 首页/标签：phase4/03 S2与phase6/08→Home/AppView/TagsView/NavbarBadge；现阶段六卡+最近表是既定能力，身份条/任务消息合卡属phase6，不报缺功能；当前深链/缓存/键盘/口径修复F03/F07。
- 用户/角色/菜单/组织：phase4/03 S3–S7→system四页、my-org；只读菜单、严格勾选、主组织/组织透明度、owner委托按已有真实S3–S7断言；尚未重新逐字段全验所有校验边界。
- 工单：phase4/03 S8–S12及phase5 S1/S2/S3a/S5/S10→发起/列表/详情/类型/规则、UserPicker/Richtext/附件/FlowGraphDesigner；已做草稿、贴图、图编辑提示、转换/CC标签/大ID等交互，最终全套待跑；S3b审批、语义引擎v2与类型可见性随phase6，未越界实现。
- 任务中心：phase4 S13+phase5/04→三Tab（定义按manage门控）、schema提交、取消重试、死信、cursor/详情；FE4旧500由taskrunner修复，完整真栈重验需固定最新兄弟运行态；jobs无DELETE是设计边界，无删除按钮不报缺陷。
- 活动列表：phase4 S14→类型/schema/字段/演进、两栏data、敏感掩码、cursor/软删恢复/废弃只读/导入导出；W5真栈两例已过，F06已提交。1MiB UI/网关与1GiB直连是分层边界，root另核网关超限502→413。
- 审计/情报：phase4 S15+phase5/09→audit/Panic与intel查询历史/缓存；真实路由多视口与既有P4/情报E2E，错误细节/缓存强制刷新由主仓批次继续核。请求体折叠与脱敏不替代服务端安全；未做所有异常数据手工全组合。
- 消息/配置/个人：phase5/06、10→消息中心/铃铛、通知模板/规则/发送记录、字典、系统设置、profile/PAT；当前自服务消息不依赖菜单授权，管理Tab权限门控和FE4既有哨兵纳入最终复验；设置只读、上传限制由S8+主仓storage批次合证，非前端单独认证服务端。
- 未来：phase6 R1–R4风险闭环/首页/工单完善作为演进输入；phase7接入扩源/Action/非人身份；phase8脚本库/隔离执行/SOAR双向；phase9轻DAG/outbox。当前缺少这些页面不是phase5回归。仅阅读未来总纲与前端相关设计，不宣称所有愿景段稿全文核完。

未覆盖手机视口、200%缩放、屏幕阅读器实机与全量axe规则；未把基础键盘/名称/对比度单项等同可访问性认证。现行视口规范最低常用1024与内部滚动已按代表页观察；长文本/默认数据、主题与所有角色的交集仍须最终固定产物复核，不用默认截图替代功能/权限证据。


## 当前复验快照与证据归属

截至本次落档，UI最新修复提交为 `c83a53ded725da167eb1e9b3d408320336e25fb7`，固定归档六项门禁正在进行；不能把分项通过写成全部关闭。主仓联合修复 `12401aa52d5fa8842dd944b9366917b19ae7775e` 的完整集成验证也在进行，测试常驻app仍为f69，AL/TR最新联合产物已构建但尚未全部部署。源码提交、冻结归档验证、常驻运行状态分别记账。

原始浏览器截图、JSON、日志和产物清单位于本机 `.codex/audits/ECO-20261104/evidence/ECO-20261104-b08/`，按所有者要求不入Git/Docker；报告保留最小复现、实际结果、修复hash与限制。GitHub原快照链接仅用于固定源码锚点；本轮修复分支尚未推送，不假定远端已有这些修复对象。
