# 生态全面审计 B08：前端功能、交互、视觉与实践（阶段报告，2026-11-06）

> **最新固定UI后笔**：F12窄契约绑定、F13长URL及F14暗按钮均已提交，UI最终候选 `9f76a1b1eba80434107d1797e9b7401aa0ef6623` 已用259文件SHA核对的准确镜像部署test。F14真实Chrome正常/悬停八轮绿、删除规则八红，工程S2及实际按下三态八轮/变异八轮完成，后文续记分开说明工具与覆盖。F12固定后端bc245专项及前端227单元/类型变异通过。新组合完整41尚未跑，三项保持待最终组合验收；原dev b205保留已知F08。以下f104/61及“未修”是历史批次时点。

> **同日后续增量**：F13已独立提交 `61b35bf75596c79bf42c7c7d26544ac584716ba2`，长描述 scoped `overflow-wrap:anywhere`；原八红/修复八绿/移除规则八红，S10与基础五项通过。root已将其干净archive重建为正确版本镜像并明确测试徽标，实际Chrome通过登录、首页深链、列表、必填反馈、任务记录与AL空态代表走查，1280/1024整页未见横溢；F14暗按钮仍复现，F12尚未接线。新61镜像未重跑完整41，不能借用下文f104整套成绩为新产物背书。原dev b205实际Chrome仍报F08已知异常，修复源未合main。见[B13环境/Chrome记录](./zhuzhao-ecosystem-audit-b13-environment-chrome-2026-11-06.md)；以下“F13未修/CUA不可用/旧镜像标签”均为之前批次时点，原证据保留。

## 结论与范围

**本批原F01–F13加单次视觉补充F14，共14项（P1×8、P2×6）：F01–F11已提交并在固定产物复验通过；F12/F13/F14三项P2现已修复并通过专项，待最终组合完整41验收。** UI六件门禁与完整41例真实栈通过，不代表整个生态审计结束、生产无错误或全站WCAG认证。未合main、未发布；远端CI本批未运行。正式报告与11号控制台由主会话统一维护；过程草稿与raw证据不纳管。

原UI基线 `b20594cc744a61e2bcb41535b0ea668a711aa4df`；修复窗口止 `f1043a12e74e929dd223a5b718e0a27b3781c253` 共12笔，分支codex/fix-ui-audit，原UI仓他方.gitignore未改。原设计/API对照起点主仓 `5c03581d5999139a3e8e5ead5972c026868aa181`；跨仓原基线沿root执行卡，不另虚造兄弟hash。

本批最终动态运行来源：主仓app `f69b7cbefebff943972d24c647776375fb8d1f90`、taskrunner `5e3f5f3720a54fad9f5cec4a08481364693b8f7d`、activelist `19a4c1d5cd84cc892b82ad52554db320bfad8f7f`、自有RustFS59000。服务镜像/二进制来源由root固定联合批实核，见joint/test-runtime-siblings-result.json等。UI48081/API33334；dev9000及既有dev不动。UI容器镜像名仍旧基线，但259个静态文件的固定source/dist/容器SHA匹配、所有JS/index再经HTTP逐一匹配；先前c83轮曾对259文件全部HTTP验证，最终f104有独立manifest，不能用旧镜像名替代产物来源。

**基线计数校正**：b205 AGENTS实为28spec/39E2E、25文件202单测。早期40例35通过5失败，是已增S8d后的修复候选轮，最初草稿误写“旧基线40例”现明确纠正；原raw轮次/日志不回写。最终f104为30spec/41E2E、28文件226单测。

## 固定产物验证

- git archive固定f104独立目录安装 `pnpm install --offline --frozen-lockfile` 成功，非依赖原UI工作树symlink充当最终安装。
- lint/typecheck/226单测/官方registry prod audit/build均exit0；audit无已知漏洞，构建仅既有chunk>500k提示。证据fixed-f1043a12-gates.json与各log。
- **完整E2E 41 passed，0 skipped/0 unexpected/0 flaky，150.191秒，retries=0/workers=1**；fresh无.state直接启动，globalSetup正常先行。`--list`因不执行globalSetup而缺.state的尝试不当产品缺陷。B08未自行改主仓源码或跑Go acceptance；后端验收见root联合批证据。
- 同修复源c83早轮完整41为39过2红：导出被真实折叠；S3a reload menus真429。后者经request_id/AT哈希/服务端时序查证后，独立test提交只在夹具/强制reload前预留完整补桶窗，不失败后重试、不放宽限流、不改FE4零错误断言。
- F02三异步分支移除修复各8轮目标红，F08移除空目标守卫8/8精确TypeError红；F11从f104独立副本移除hasAttribute、还原dataset判断，真实部署8个串行新route实例均export hidden/display none，恢复固定源后8/8 visible/block。各mutation源码/构建/运行SHA与raw保留，不改纳管源。
- `commit-lint b205..f104` 12笔全绿。原始日志/截图/探针/变异副本仅本地.codex，永久回归用例在UI仓必要源码中。

## 分项发现、建议与复验

### B08-F01 / P1 / 修复复验完成：草稿跨账号/类型恢复，丢弃/成功建单未清

原快照锚点（完整hash见基线）：[src/components/RichtextField.vue:74](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/components/RichtextField.vue#L74) 的 `watch / onMounted / clearDraft`；[src/views/ticket/create/index.vue:1](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/views/ticket/create/index.vue#L1) 的 `draft-key / handleSubmit`。

原b205的RichtextField只以字段key存储；真实同origin账号A类型A草稿被账号B类型B同名字段恢复，丢弃仍留内容。范围是浏览器本地内容，未声称后端越权。证据draft-A-edit.png、draft-B-other-account-restored.png、draft-discard-retains-content.png。

修复 `fc1068b66ea4f33f2f1a02e9a9ef557b488c7560`，账号/类型/字段完整作用域、旧无属主key安全删除、timer/model/丢弃/提交后复原；锚点[src/components/useRichtextDraft.ts 的 richtextDraftKey/clearDraft/discardDraft](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/.codex/audits/ECO-20261104/worktrees/ui-r03/src/components/useRichtextDraft.ts:6)及create.handleSubmit。最终S8d真实栈+持久回读与相关单测通过。

### B08-F02 / P1 / 修复复验完成：旧异步结果覆盖或拆除新会话

原快照锚点（完整hash见基线）：[src/common/request/src/index.ts:63](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/common/request/src/index.ts#L63) 的 `refreshAccessToken`；[src/store/modules/user.ts:1](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/store/modules/user.ts#L1) 的 `loadSession / resetState`。

原b205刷新成功/终态失败、loadSession三种deferred交错可分别覆盖B token/清B会话/写旧A profile和权限菜单；未突破后端鉴权。修复 `604b96c2fb6133fa75b2800f7121d5badd0c966c`：generation隔离刷新单飞、响应、store和守卫写入。锚点[src/common/request/src/index.ts 的 refreshAccessToken/response interceptors](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/.codex/audits/ECO-20261104/worktrees/ui-r03/src/common/request/src/index.ts:67)、user.loadSession及tokenStorage.getSessionGeneration。

三目标移除修复各8轮红（r03-session-mutation-summary.json），最终请求单测与S16换账号/S8d通过。早期跨目录mock解析失败实验无效，未计入有效证据。

### B08-F03 / P1 / 修复复验完成：Home/Badge深链与缓存筛选、计数语义不一致

原快照锚点（完整hash见基线）：[src/views/Home/index.vue:103](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/views/Home/index.vue#L103) 的 `goTickets`；[src/components/NavbarBadge.vue:36](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/components/NavbarBadge.vue#L36) 的 `goTodo`；[src/views/ticket/list/index.vue:1](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/views/ticket/list/index.vue#L1) 的 `URL筛选初始化`。

原b205：先点待处理卡→列表手改closed→回Home再点同一fullPath，URL=open但缓存筛选仍closed；不同query首次进入对照正常。两待办入口又只传assignee=me，把闭单混入。Home原统计assigned/in_progress，与BadgeService排除closed/rejected（含custom）本来不同，不能硬统一计数。

首修 `ad5874590dd3ccee9f878541e629cc1a3ff075df`（主仓cd79374 statuses）+ `a1f8476c1102328a0c39dbe132f34befdb0506b9`（主仓9e2fef6 exclude_statuses）：显式深链onActivated兑现、无query正常页签保筛选、集合回显/单态改选/重置与list/export同源。锚点[src/views/ticket/list/index.vue 的 applyRouteFilters/statusSelection/statusFilter](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/.codex/audits/ECO-20261104/worktrees/ui-r03/src/views/ticket/list/index.vue:52)、NavbarBadge.goTodo/Home.goTickets。

最终S2造assigned/closed/review_custom真实图状态：Home两态包含assigned排除闭单/custom；Badge保留custom、count=list.total；两路导出精确参数200；相同缓存路径再入/回显通过。证据早期home-query-before-v2.log/八轮正向及最终fixed-f1043a12-e2e-result.json。

### B08-F04 / P1 / 修复复验完成：supervisor数字恒400、CC大ID舍入失败

原快照锚点（完整hash见基线）：[src/api/ticket/index.ts:239](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/api/ticket/index.ts#L239) 的 `AssignTicketInput`；[src/views/ticket/detail/index.vue:215](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/views/ticket/detail/index.vue#L215) 的 `addCC / addSelfCC`。

原b205 supervisor经Number而后端标量json,string；CC把9007199254740993变9007199254740992。旧app真实合成：supervisor numeric1→400/10001，同值string→200；实际大ID收件用户的JS舍入CC→400“抄送用户不存在”，精确numeric control→200，旧DTO string[]→400。**仅证明选人失败，未证明错误抄送他人。** 原始id-wire-baseline.json保留。

修复 `2d4afd06ff703cf26609af7eca7cd5f0ae6ed32c` + 主仓91d7b90兼容Int64Slice + 测试定位 `e853323405eb9c7856d80605fd724c414e65bd5b`。建单/分派/CC保持string，排序BigInt。锚点[src/api/ticket/index.ts 的 CreateTicketInput/assignTicketApi/addTicketCCApi](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/.codex/audits/ECO-20261104/worktrees/ui-r03/src/api/ticket/index.ts:234)与两页面handleSubmit/doAddCC。旧supervisor单测真红；CC传输单测只是正向control，不谎称能单独抓调用方旧Number。最终S17真浏览器>2^53合成用户，三路payload精确string、建单回读/CC chip均通过。

### B08-F05 / P2 / 修复复验完成：类型页“三态不可达”说明过时

原快照锚点（完整hash见基线）：[src/views/ticket/type/index.vue:738](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/views/ticket/type/index.vue#L738) 的 `类型编辑弹窗alert`。

原b205编辑alert/注释未跟随Phase5 S3a。修复 `f23d081080cf49354fd7017b92c59ffecbb735f6`；[src/views/ticket/type/index.vue 的 编辑说明](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/.codex/audits/ECO-20261104/worktrees/ui-r03/src/views/ticket/type/index.vue:738)如实说明图+权限/条件/校验器与审批未启用，API注释同扫；最终P5-S2新旧文案正反断言通过。Phase4历史稿不倒改成当时已有S3a。

### B08-F06 / P1 / 修复复验完成：AL未选类型同时显示无目标写入入口

原快照锚点（完整hash见基线）：[src/views/al/data/index.vue:175](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/views/al/data/index.vue#L175) 的 `openCreate / 空态模板`。

原b205未选类型的选择提示旁又显示“暂无数据/写入首条”，能开空类型写框。原截图visual-20261106/1024x900-_al_data.png与反断言count1保留。修复 `f23d081080cf49354fd7017b92c59ffecbb735f6`；[src/views/al/data/index.vue 的 openCreate/selectedType空态](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/.codex/audits/ECO-20261104/worktrees/ui-r03/src/views/al/data/index.vue:175)校验有效非废弃类型、门控空态/表格。最终W5写入/cursor/软删恢复/废弃只读两例通过。

### B08-F07 / P1 / 修复复验完成：Tooltip拦截键盘聚焦/密度，聚焦图标无名称

原快照锚点（完整hash见基线）：[src/components/ProTable/Tools.vue:16](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/components/ProTable/Tools.vue#L16) 的 `Tooltip按钮模板`。

原b205 Tooltip默认triggerKeys阻止原生Enter/Space；新增S2聚焦Enter曾真实红。修复 `f23d081080cf49354fd7017b92c59ffecbb735f6`，[src/components/ProTable/Tools.vue 的 两处trigger-keys/aria-label/aria-pressed](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/.codex/audits/ECO-20261104/worktrees/ui-r03/src/components/ProTable/Tools.vue:16)；最终Enter入聚焦、Esc退出及状态回显通过。五页探针命中同共享组件，非五条重复发现。

### B08-F08 / P1 / 修复复验完成：首次unobserve(undefined)中断挂载观察

原快照锚点（完整hash见基线）：[src/common/hooks/src/useTableFit.ts:41](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/common/hooks/src/useTableFit.ts#L41) 的 `syncObserve`。

原b205 useTableFit.syncObserve无旧目标仍unobserve，真浏览器与新增同符号单测TypeError。修复 `80c34da918fb29cc5c60845c215567a522cd5bf2`；[src/common/hooks/src/useTableFit.ts 的 syncObserve](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/.codex/audits/ECO-20261104/worktrees/ui-r03/src/common/hooks/src/useTableFit.ts:41)仅旧目标非空时注销。移除守卫8/8精确TypeError红，最终初挂/换目标/卸载单测、整套E2E与视觉通过；pageerror事件与Vue errorHandler的console.error须分开理解，不能单靠pageerror0证明所有JS无错。

### B08-F09 / P2 / 修复复验完成：online组件未共享offline图标注册表

原快照锚点（完整hash见基线）：[src/components/ProTable/Tools.vue:8](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/components/ProTable/Tools.vue#L8) 的 `Icon局部导入`；[src/components/NavbarBadge.vue:1](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/components/NavbarBadge.vue#L1) 的 `Icon局部导入`；[src/views/system/setting/index.vue:1](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/views/system/setting/index.vue#L1) 的 `Icon局部导入`。

原b205 Tools/NavbarBadge/setting局部online Icon、全局offline注册隔离。真实SSR offline有path/online空svg；阻断外网原产物四布局亮暗focus path=0并发api.iconify两组请求。修复 `4908d699a1a74b74246f074d0619a462a49a45a1` 三处offline，icons.test新扫描原样精确三处红；最终226单测通过。固定产物视觉v4阻断三图标API域，requests0、八组列表focus path各1。证据icon-registry-before.json/icon-import-before.log/visual-extra-v4-20261106。

### B08-F10 / P2 / 修复复验完成：亮色语义Tag小字对比度低

原快照锚点（完整hash见基线）：[src/views/Home/index.vue:170](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/views/Home/index.vue#L170) 的 `最近工单状态/优先级Tag`。

root视觉线索经独立DOM核：文字完整、静止opacity1；亮warning2.03898/info2.80166，dark7.18259/5.24193。取computed前景/背景，12px文字参照[W3C常规文字4.5:1说明](https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html)，不是全站合规裁定。

修复 `c83a53ded725da167eb1e9b3d408320336e25fb7` 限light/plain较深语义文字，背景/主色/dark/实色不改；[src/common/styles/src/var.css 的 亮色Tag文字变量](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/.codex/audits/ECO-20261104/worktrees/ui-r03/src/common/styles/src/var.css:88)。S2原产物info2.80166精确红，最终真实S2与亮暗DOM验证warning6.33214/info6.87556、dark不变。

### B08-F11 / P1 / 修复复验完成：data-ignore空布尔属性仍被折叠

原快照锚点（完整hash见基线）：[src/components/FilterBar/index.vue:20](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/components/FilterBar/index.vue#L20) 的 `controlEls(`。

完整c83/S2导出点击超时揭出原b205共享FilterBar用!dataset.ignore，空属性值为''仍算控件，重置/导出在收起态隐藏。全src三调用点：工单、任务、审计均为空属性。修复 `5ecba4b37709acef63224abfeb477f1a0e707878`；[src/components/FilterBar/index.vue 的 controlEls](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/.codex/audits/ECO-20261104/worktrees/ui-r03/src/components/FilterBar/index.vue:20)改hasAttribute，S2增加真实可见性断言。

从后续固定f104副本只移除此守卫并部署，8个串行新route实例均hidden/display none；恢复后8/8 visible/block（filter-mutant-eight.json/filter-fixed-eight.json及源码/产物manifest）。最终S2含两路导出均200；原c83整轮39/2保留。限流节奏另独立提交 `f1043a12e74e929dd223a5b718e0a27b3781c253`，不混入F11独立fix。

### B08-F12 / P2 / 未修挂账：工单请求仅部分兑现生成契约的编译保护

原快照锚点（完整hash见基线）：[src/api/ticket/index.ts:230](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/api/ticket/index.ts#L230) 的 `CreateTicketInput`。

[src/api/ticket/index.ts 的 CreateTicketInput/assignTicketApi](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/.codex/audits/ECO-20261104/worktrees/ui-r03/src/api/ticket/index.ts:234)手写类型，未绑定generated AssignTicketRequest/CreateTicketRequest。隔离f104副本只把schema两路supervisor:string改boolean，客户端仍string，vue-tsc exit0（generated-contract-mutation.json/log）。不是声称所有codegen消费者失效：system/user/role/org等4模块已有绑定。

已知CustomData RawMessage在生成schema为number[]、UI注释也明确承认此失真；主仓ticket_request.go CreateTicketRequest.CustomData（21行）为契约前置。因此建议先校准Swagger对象形状，再逐请求绑定/加编译断言，不能直接全量alias。此项是防漂移能力缺口，F04真实故障已单独修复；未修改纳管生成物，变异只在独立副本，正常运行/构建保持f104。

### B08-F13 / P2 / 未修挂账：长无断点描述在详情内部横向溢出

原快照锚点（完整hash见基线）：[src/views/ticket/detail/index.vue:584](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/views/ticket/detail/index.vue#L584) 的 `<pre`。

原b205与f104均保留[src/views/ticket/detail/index.vue 的 描述pre](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/.codex/audits/ECO-20261104/worktrees/ui-r03/src/views/ticket/detail/index.vue:584)仅whitespace-pre-wrap；合法合成长URL在1024暗色dual：pre横向溢出1983px、cardbody1963px，document/section仍0。证据固定源visual-extra-v4-20261106/observations.json的dark-dual-long-detail-1024，以及visual-extra-v5-20261106/dark-dual-long-url-end-1024.png末端截图；长中文/标题可换行不等于所有长文正常。建议限定描述长词/URL换行，验证亮暗、1024/宽屏与内容不被裁剪；本批不扩修复源。

## 视觉、使用习惯与覆盖限制

原轮20条路由×1366×900、1024×900、2560×1440=60张默认Tab截图与DOM观察：Home、工单列表/发起/类型/规则、tasks、AL两页、audit、intel、notification/dict/user/role/menu/org/setting、profile、my-org、messages。代表截图有人眼复核，全部截图不等于全Tab全输入已手工过。默认外层滚动0/无404只针对该批观察，不能扩成任何内容无溢出。

最终固定f104另做四布局×亮暗×Home/长标题列表16图+类型弹窗/长详情共18图，追加同组console捕获与长URL末端补图合19图（visual-extra-v5-20261106，不将重复轮叠加覆盖数）；布局切换按Enter、theme真实可见控件、Esc关闭类型弹窗。document/section滚动0、pageerror0/console.error0；语义Tag对比与离线Icon上述实核，F13明确为内部内容例外。宽屏留白符合填充/封顶规则，不作纯审美bug。视觉风格沿紫色主操作、统一筛选/卡片/分页/对话框范式，不作无需求换皮。

CUA本轮iab不可用、Chrome创建不支持、getApp超时，已正常结束工具会话；不冒充新人工CUA走查，新增图来自真实Playwright探针。未覆盖手机/200%缩放/屏幕阅读器实机/全量axe规则，基础键盘/命名/对比度单项不是认证。RichtextView原catch拼HTML是零生产导入的不可达线索，不报已达XSS；实际md-editor默认过滤与服务端安全需结合主仓批次，不用代码注释代替验证。

## 环境、账号与清理

UI六件过程中E000001密码只由既有E2E globalSetup幂等流程收敛为admin12345，未人工改密；大ID测试用户均专属合成且回读/清理，旧首轮notifications FK清理失败与后续准确修复保留。

W5既有helper默认宿主SQLite不适用于当前test PG，日志有不可达，按既定规范跳过定义自动清理（不是E2E跳过，41例仍全执行）。本两轮准确description与job_id共2定义已从zt-tr-postgres删除，复核0残留；已核jobs/job_runs无FK，仅PK/UNIQUE，**不删job_runs运行历史，不按前缀批删**。证据full-e2e-tr-fixture-cleanup.json。旧本会话失败准备/探针明确7张单与3类型经API逐项核后精确清理，复核0残留：known-failed-fixture-cleanup.json。其他既有保留测试工单/日志/数据不冒领已清。

一次早期本地探针误并行、错误/dist-test/部署前缀、跨目录mock/env路径、S17隐藏teleport定位、S2非法assigned→closed与误写to_status、theme隐藏input点击及早于路由落定匹配旧Home两行、详情标题漏#ID前缀等无效尝试均保留，不当产品发现或正向通过。最后正式全套与变异/视觉均串行，产物来源固定。AL UI/网关1MiB与AL直连1GiB是分层边界，不擅扩；超限HTTP映射由root联合批承接。

## 全文设计阅读与后续挂点

本次另独立全文补齐phase4/01（1–231）、phase4/03（1–108）、phase6/08（1–105），固定主仓HEAD6199ef631050b4547fe4248b3e0cf5d661182106，SHA256/绝对路径/边界映射在design-full-read-manifest.json，已向main设计代理回传；没有把章节浏览算full。其他已全文phase5稿由main代理合并覆盖，phase3/12归siblings。不宣称所有docs全读。

## 模块覆盖清单与演进边界（阶段对账，不等同每输入全覆盖）

- 壳层/IAM：phase4/01 §3–6→boot/permission/router、user/tokenStorage/request、动态菜单和v-permission；已有S1/S16/FE3/FE4及新F02延迟交错，服务端三层最终判定由主仓批次承接。
- 首页/标签：phase4/03 S2与phase6/08→Home/AppView/TagsView/NavbarBadge；现阶段六卡+最近表是既定能力，身份条/任务消息合卡属phase6，不报缺功能；当前深链/缓存/键盘/口径修复F03/F07。
- 用户/角色/菜单/组织：phase4/03 S3–S7→system四页、my-org；只读菜单、严格勾选、主组织/组织透明度、owner委托按已有真实S3–S7断言；尚未重新逐字段全验所有校验边界。
- 工单：phase4/03 S8–S12及phase5 S1/S2/S3a/S5/S10→发起/列表/详情/类型/规则、UserPicker/Richtext/附件/FlowGraphDesigner；已做草稿、贴图、图编辑提示、转换/CC标签/大ID等交互，固定f104完整41例已过；S3b审批、语义引擎v2与类型可见性随phase6，未越界实现。
- 任务中心：phase4 S13+phase5/04→三Tab（定义按manage门控）、schema提交、取消重试、死信、cursor/详情；FE4旧500由taskrunner修复，最新固定兄弟运行态下FE4/W5完整真栈已过；jobs无DELETE是设计边界，无删除按钮不报缺陷。
- 活动列表：phase4 S14→类型/schema/字段/演进、两栏data、敏感掩码、cursor/软删恢复/废弃只读/导入导出；W5真栈两例已过，F06已提交。1MiB UI/网关与1GiB直连是分层边界，root另核网关超限502→413。
- 审计/情报：phase4 S15+phase5/09→audit/Panic与intel查询历史/缓存；真实路由多视口与既有P4/情报E2E，错误细节/缓存强制刷新由主仓批次继续核。请求体折叠与脱敏不替代服务端安全；未做所有异常数据手工全组合。
- 消息/配置/个人：phase5/06、10→消息中心/铃铛、通知模板/规则/发送记录、字典、系统设置、profile/PAT；当前自服务消息不依赖菜单授权，管理Tab权限门控和FE4既有哨兵已在最终完整轮复验；设置只读、上传限制由S8+主仓storage批次合证，非前端单独认证服务端。
- 未来：phase6 R1–R4风险闭环/首页/工单完善作为演进输入；phase7接入扩源/Action/非人身份；phase8脚本库/隔离执行/SOAR双向；phase9轻DAG/outbox。当前缺少这些页面不是phase5回归。仅阅读未来总纲与前端相关设计，不宣称所有愿景段稿全文核完。

未覆盖手机视口、200%缩放、屏幕阅读器实机与全量axe规则；未把基础键盘/名称/对比度单项等同可访问性认证。现行视口规范最低常用1024与内部滚动已按代表页观察；长文本/主题已做固定产物代表页复核，所有角色×所有主题×所有数据的全交集未穷举，不用默认截图替代功能/权限证据。


## 修复提交窗口（逐笔hash）

- `fc1068b66ea4f33f2f1a02e9a9ef557b488c7560` fix(ticket): 隔离富文本草稿并清理提交后的表单
- `604b96c2fb6133fa75b2800f7121d5badd0c966c` fix(auth): 阻止旧会话异步响应覆盖新登录
- `ad5874590dd3ccee9f878541e629cc1a3ff075df` fix(home): 统计卡深链恢复筛选并对齐待办集合
- `2d4afd06ff703cf26609af7eca7cd5f0ae6ed32c` fix(ticket): 人员标识保持精确字符串
- `80c34da918fb29cc5c60845c215567a522cd5bf2` fix(table): 首次挂载不注销空观察目标
- `f23d081080cf49354fd7017b92c59ffecbb735f6` fix(ui): 校准空态能力说明与表格键盘操作
- `e853323405eb9c7856d80605fd724c414e65bd5b` test(e2e): 使用真实菜单选人与富文本交互
- `a1f8476c1102328a0c39dbe132f34befdb0506b9` fix(ticket): 待办角标入口对齐排除终态口径
- `4908d699a1a74b74246f074d0619a462a49a45a1` fix(ui): 统一使用已注册的离线图标入口
- `c83a53ded725da167eb1e9b3d408320336e25fb7` fix(ui): 提高亮色状态徽标文字对比度
- `5ecba4b37709acef63224abfeb477f1a0e707878` fix(filter): 保留布尔标记的常驻动作
- `f1043a12e74e929dd223a5b718e0a27b3781c253` test(e2e): 在密集夹具后预留完整限流预算

## 单次视觉补充：B08-F14 / P2 / 未修挂账（不扩f104源）

原快照锚点（完整hash见基线）：[src/common/styles/src/var.css:52](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b20594cc744a61e2bcb41535b0ea668a711aa4df/src/common/styles/src/var.css#L52) 的 `--el-color-primary: #818cf8`。

root复核暗色主按钮浅紫底白字后，按要求只做一次真实computed测量：工单列表“新建工单”active，14px/500、opacity1、disabled=false，前景rgb(255,255,255)、背景rgb(129,140,248)，对比2.982903277。证据dark-primary-button-computed.json/dark-button.log；这是主动按钮的常规文字，参照前述W3C4.5:1，未按截图像素裁定，也不扩成全主题审计。

源锚f104的[src/common/styles/src/var.css 的 html:root.dark](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/.codex/audits/ECO-20261104/worktrees/ui-r03/src/common/styles/src/var.css:52)（主色#818cf8）与ElButton默认文字变量。建议后续只核暗色主按钮专用text/hover/active变量或对比合格的底色，不全局加深主色而伤害链接可读性；亮Tag的F10已完成仍保留原范围，本补充独立挂账。此项追加于已交root的13项版之后，原13项/六件结论与raw时点不回写。

## 新后端组合二次验证（2026-11-06）

保持同一固定UI `f1043a12e74e929dd223a5b718e0a27b3781c253` 和AL19/TR5e/RustFS，app另升 `7389ed79bdf409e466542992d9f1befa0c1a8529`（新增通知身份/情报单飞/消息接线/审计脱敏修复）。完整41例重新实跑：41 expected、0 skipped/0 unexpected/0 flaky，152.039秒，retries=0；旧f69组合150.191秒41绿作为上一组合保留，未覆盖。新证据fixed-f1043a12-7389-e2e-result.json/log。没有重复未变UI源码的安装与前五项门禁，也未叠加截图覆盖数。

第三轮W5定义按准确job_id+description及本轮时间窗核后删除，0残留，保留job_runs历史；证据full-e2e-7389-tr-fixture-cleanup.json。新app容器二进制/image核对及仅目标升级见主审test-runtime-7389-result.json；其它test与dev容器保持。整体文档/历史债对账继续，三项P2仍待后续整改。


## F12–F14固定后笔与实际浏览器复验

来源窗口在原12笔后追加：F13 **`61b35bf75596c79bf42c7c7d26544ac584716ba2`**；F14 **`324bd5fba0806b45d024237b45598298c96582c1`**；F12 **`9f76a1b1eba80434107d1797e9b7401aa0ef6623`**。F12后端对象元数据 **`bc245edfdcd3d2eabf4a64b84e7c34aa92c298ea`** 固定698文件零差异，lint3.858秒、unit10.518秒、schema专项race八轮通过；UI仅以该固定Swagger再生custom_data对象合同，assigned_to允许null，supervisor仍绑定生成契约。两路supervisor:string→boolean分别八次实际页面与wire类型测试编译失败，227单元/28文件及lint/type/audit/build通过。不是全局忽略生成错误，也没有any兜底。

F14仅在暗色primary按钮常规/hover/active三文字变量使用深色#111827；不全局加深主色，plain/text/link/禁用形态不机械覆盖。root通过已启用的Chrome扩展，实际测试域专属E800006登录会话，在工单创建页1024/1280视口量computed样式：fixed常规/hover八轮合格；仅删除三变量的独立mutant八轮稳定不合格，恢复fixed后console error0。实际Chrome这一轮未测持续按下态，不能混写成三态均由Chrome扩展验证。截图和原始computed收据留在忽略目录 `evidence/ECO-20261104-b08/p2/chrome-f14/`，未上传用户数据或将图片纳管。

另用项目原有 `pnpm test:e2e` 专用headless工程上下文，在真实持久S2中repeat-each=8、workers1、retries0：fixed **8 expected/0 skip/0 flaky，53.724秒**；移除三变量的mutant **8个目标断言失败，42.087秒**。独立三态probe各八轮均实测:active=true：fixed默认**5.947**、hover**4.902**、active**7.537**；mutant对应**2.983/3.619/2.354**，八轮均低于4.5。工程上下文与用户Chrome分开，不用脚本接管个人浏览器。

准确fixed镜像 `zhuzhao-ui-audit:9f76a1b1eba8`，ID `sha256:94f88b6c4c6ca74a5782b2634b75ad13694af0f8e1d7dee6fe400c05e456ee96`；259静态文件SHA及nginx模板均匹配，HTTP index SHA `addc4a93d8c2a38464f344de4244765e219ffb4affd000deb08b97e3953d638c`。对照完成后恢复该fixed镜像、light主题与默认视口；后台八容器ID/image/启动时间全部不变。S2本轮新增ticket/type残留0，原四单/一类型与审计/通知历史保留，Playwright工作进程残留0。私有canonical已收敛到9f准确来源，未再执行重启。证据 `p2/{image-9f76a1b1/result.json,active-standard-e2e/result.json,active-standard-e2e/three-state-summary.json}`。

本节是修复和目标变异证据，不代替最终主仓/AL/TR候选组合的完整41例。F17/F18导致后端Swagger版本说明变化，最终UI需按固定新源再生并确认差异；最终六件门禁/完整浏览器回归与部署状态另记。全角色/全主题交集、手机、屏幕阅读器等未覆盖边界仍保留，不声明全站可访问性认证。所有新增源码未合main。
