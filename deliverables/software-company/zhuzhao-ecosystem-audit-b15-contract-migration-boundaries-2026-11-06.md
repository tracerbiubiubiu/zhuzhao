# 生态全面审计 B15：接口、工具与迁移边界（2026-11-06）

> **来源路径迁移（2026-11-07）**：仅迁移旧审计工作树的源码链接；目标文件逐字一致，原审计基线、历史 hash 与结论不变。

## 范围与基线

有界横切收口，冻结主仓 `dc3d540a48e67bf56244efdc0f71cf32cbcb5e80`；接口探针源 `bd05cfd39cf1448f58283f3f2129a0d6f0701a6b`、UI `9f76a1b1eba80434107d1797e9b7401aa0ef6623`，相关模型/消费者在后续冻结版本未改。迁移静读固定ba3ecb6，162 SQL与dc3逐字相同。记录三项当前P2偏差、一项已动态确认的回滚前置；未确认线索与撤销候选另列。未改产品实现、依赖或历史SQL。

## B15-F01 / P2：存储输出ID与字符串标准及手写消费者不一致

锚点：[storage.go 的 FileObject.ID / TicketAttachmentView.ID](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/internal/model/storage.go:6)、[UI storage API 的 ConfirmResponse / AttachmentView](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao-ui/src/api/storage/index.ts:24)、[TicketAttachments 的 att.id键](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao-ui/src/components/TicketAttachments.vue)。真实Confirm、BindAvatar与ListAttachments handler/PG都输出numeric ID；Swagger/生成类型也是number，并非生成物陈旧。standards §3-7/§3-14⑦要求出站int64 ID字符串，手写UI却声明string且只有unknown断言，实际没有转换。

当前FileObject.ID未用于业务寻址，上传/头像/富文本等都用object_key；附件ID用于Vue key，删除/下载仍按object_key。自有PG显式造2^53与2^53+1两附件ID，实际响应经标准JS解析后两numeric键碰撞，object_key仍精确两条；未挂载Vue观察DOM、未证明当前数据已到该边界或实际错对象。因此按合同/类型偏差P2，不升P1，不冒称生产精度事故。

后续挂点：统一存储输出DTO的字符串ID、再生Swagger/UI并核外部数字消费者兼容，补真正跨HTTP序列化守卫；不能只改手写TS类型。按既定P2统一排期，本冻结修复批不扩大。

## B15-F02 / P2：make wire入口缺工具校验和

锚点：[Makefile 的 wire目标](https://github.com/tracerbiubiubiu/zhuzhao/blob/dc3d540a48e67bf56244efdc0f71cf32cbcb5e80/Makefile#L43) @ dc3d540。原5c与本冻结源均pin Wire v0.7.0，缺google/subcommands及go-difflib内容sum。正常GOPROXY/GOSUMDB下，原/新两版的默认与显式readonly go run共4次均先报missing go.sum，模块SHA未变；不是本批升级utils引入，也不是只因离线代理。

root首次完整门禁运行器在该额外生成步骤exit2，原日志保留。CI实际使用go install固定v0.7.0后调用CLI；已装工具的go version -m明确同版，root在不改冻结模块的前提按CI同款CLI生成1.930秒通过，wire_gen.go零漂移。lint、业务unit/integration/cover原样目标另有独立结果，不把首次make入口失败涂成全绿。

后续最小建议：Makefile对齐固定版本工具入口和安装指引，避免向业务模块塞工具私有依赖；独立修复该入口及无工具时提示。当前P2记账，不为此重开冻结生产代码。

## B15-F03 / P2：定制模板引用是迁移回滚前置

锚点：[000061建立规则→模板NO ACTION FK](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/migrations/000061_message_rules_fk.up.sql:8)、[000079 down删除种子](/Users/bujibuji/code/src/github.com/tracerbiubiubiu/zhuzhao/migrations/000079_action_notify_seed.down.sql:3) @ dc3d540。自有PG两库全81迁移，经真实MessageAdminService.UpsertRule合法创建ticket.transitioned→ticket_action_notify；81降至79成功，再down79实际23503。079整段SQL回滚，模板、默认规则、定制规则全字段保留；migrate checkpoint为v78 dirty=true，数据仍v79态。fresh默认对照同路线成功，v78 dirty=false，种子正常撤销。

**撤销原MIG-S04/P1候选**：FK在保护合法用户依赖，不能为让down绿而删管理员规则；当前证据证明回滚需检查定制引用，不证明在线业务损坏。079旧注释的not-found Warn不能替代FK回滚前置。操作前应列出被撤模板的非本迁移规则引用，先裁定保留种子或显式改指；失败后须按实际数据态排查dirty，不能盲force checkpoint再继续。只读预检SQL及完整前后行在本地证据，不执行自动清依赖。

## B15-F04 / P2：密集E2E夹具与页面初始化共享限流预算

固定UI c9的w5-al-pages密集Node夹具后直接uiLogin，没有其它场景已使用的waitUserRateBudget。实际attempt2同主体前5秒77条请求，随后/profile与/permissions各429（RetryAfter1）导致初始化失败，原retry1恢复；最新原样41首过仍有AL数据POST429由原helpers.api的1.1秒退避一次恢复。是可观察的测试节奏风险，不是已证明错误限流或业务权限失效。最终0Playwright retry/0flaky不能写成全程0HTTP429。

后续测试小批在密集夹具后核共享预算，沿既有helper为初始页面留足额度；不提高DUT限额、不延长失败断言或以无限重试涂绿。当前P2记账，原spec未改；真实request_id、access时序及三轮分开结果见B08最终收据。

## 未确认线索、撤销项与覆盖证据

162/162 SQL共3223行已逐文件全文读、81对crosswalk与11号编号地图一致，无缺号/冲突；各文件blob/SHA/完整读段可查。既有81往返仅证明fresh seed终态投影（menus/menu_apis/role_menus/casbin及P4四表），不证明任意管理员定制状态或每对中间降级的数据保真。075–077页面写回滚与旧代码耦合已被AGENTS/runbook接受，不重复报缺陷。

迁移静态三线仅待核，不计已确认运行P1：031 intent-preserving JOIN未关联同角色ptype/v0；054 down与同笔3条POST绑定的中间逆操作口径；055/059按任一路由来源保护整组规则的粒度。须在历史/定制夹具中进一步裁定，不因全81 fresh往返绿直接排除，也不凭静态推断称当前越权。当前批不改历史迁移。

MessageTemplate.VariableSchema的对象wire缺陷候选已撤销：真实[]byte/base64往返，UI无显式语义消费，现有Object.assign+spread回传200且DB JSON语义保持；没有已定对象HTTP合同，不能据JSONB存储推断正常编辑失败，也不把base64擅定为已批准二进制协议。保留元数据/Swagger合同边界。通知处理人快照时刻亦只留P2设计边界，未证提交瞬时合同或动态泄露，不扩成P1修复。

证据仅忽略 `.codex/audits/ECO-20261104/`：`migration-source-coverage.json`、`migration-design-source-review-2026-11-06.md`、`data-coverage/contract-candidates/closure-result.json`、`data-coverage/wire-entry-closure/closure-result.json`、`data-coverage/mig-s04-closure/{closure-result.json,preflight-readonly.sql}`；root冻结门禁的原wire失败与CI同款成功分存joint/dc3d540。所有自有PG/匿名卷精确清理，未改共享dev/test或E000001。只纳管必要报告，raw/过程不入Git或Docker。
