# B20 / G-12：前端即时反馈与失败恢复

## 范围与基线

UI Phase5 `b5cbf1596be2d2df98038b6534c38a6242515f25`，仅NavbarBadge、FlowGraphDesigner、TicketAttachments及对应三个现有E2E。B19生成类型接线另笔，不改后端API/权限/限流/错误码或依赖。不合main、不开始Phase6；本批以即时反馈、撤销一致性、可恢复错误改善操作体验，不宣称全站审美/无障碍认证。

## 发现与修复

- **B20-F01 / P2（G-12铃铛）**：原[NavbarBadge的refresh/setInterval](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b5cbf1596be2d2df98038b6534c38a6242515f25/src/components/NavbarBadge.vue#L17)独立取数据，消息页失效`['badge']`不能触达，已读后可能等60s才变。改用既有VueQuery同一key，写操作失效后立即重取，后台60s同步保留；错误沿统一请求层，失败保留上份成功数据，既有会话清理边界不变。E2E在reload前断言角标，避免刷新掩盖缺陷。
- **B20-F02 / P2（G-12画布）**：原[ensureLf/recompute/markInvalid](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b5cbf1596be2d2df98038b6534c38a6242515f25/src/components/FlowGraphDesigner/index.vue#L138)只订阅增删事件，undo整图替换后告警残留。最终监听公开`graph:updated`，并仅在invalid值变化时写model，避免装饰性写入污染redo。E2E真实键盘双轮undo/redo同时核节点与告警，不仅看serialize结果。
- **B20-F03 / P2（G-12附件）**：原[refresh/onUpload/onDelete](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/b5cbf1596be2d2df98038b6534c38a6242515f25/src/components/TicketAttachments.vue#L70)吞列表失败，用户看到空区或操作成功但列表未更新。增加原位错误/重试、保留旧列表；写已成功而刷新失败明确提示部分成功，避免误导重复提交。请求序列/ticket身份/卸载保护旧响应；统一Axios错误不再重复toast，原生直传PUT错误仍本地提示。E2E真实上传/删除，只对随后列表请求route.abort，恢复网络后真实重试，未伪造业务成功。

## 验证证据与已纠正的初版误判

SFC动态边界覆盖即时失效、旧列表保留、部分成功、过时响应与卸载；错误key/旧附件实现定向移除修复各8轮命中对应断言。先前独立fresh画布的8轮键盘验证没有覆盖JSON往返后的重复load，不能证明全部真实编辑流程安全。

第一版候选监听`history:change`，完整组合41 E2E中40过、画布首轮与配置内retry1均失败。真实页面诊断显示undos=[6态,6态,7态]，undo后实际6态11边，但History.add发现等于栈末而不emit，告警仍state_1两条；不是键盘没执行或等待过短。切换`graph:updated`后同页双轮undo/redo通过。最终候选连续8/8绿，仅恢复旧history listener则8/8均在原spec:62告警断言稳定红，专项retry0、无其它失败类型；恢复候选字节后再跑完整原配置门禁。

上述第一版完整门禁的lint/typecheck、29文件230单测、生产依赖audit与build均过；UI阶段0个429，三条profile/permissions失败分别对应PAT吊销401与首登改密gate403负控，已按request时间窗归因。初版失败trace、独立诊断尾钩因缺acceptance checker报错、后续新镜像执行位错误均保留，不冒称这些轮次完整成功。

最终354源G14/B19/B23后端组合已通过253/0/0与81迁移往返；最终原样pnpm gate已通过：29文件230单测、41/41 E2E，实际0 retry/flaky/skip，UI全部阶段0个429；候选8绿与旧history变异8同断言红。UI修复完整提交 `152258638abaa3bd3daa656255a794c5422a0e58`；准确CI、环境承接与Chrome代表走查另补。过程源码/trace/JSON/stdout/429归因/清理与元数据SHA仅保存在ignored `.codex/audits/ECO-20261104/phase5-closeout-20261107/{ui-residual,joint-b19-b20-b23}/`，不进Git/Docker。

G-12的intel裸fetch已由此前B08修复，本批不重复开发；消息Outbox、审批、完整SLA等未来能力不属于本批。视觉检查重点是失败横幅与旧列表共存、重试可达、角标和图状态一致；最终仅以实际代表页面为证，不扩大成全站覆盖。

独立wrapper在门禁之后因漏复制seed checker而completed=false，原记录保留；独立bcrypt终验与后续附加S3 control未执行。最后真实admin登录/成功任务查询提供行为证据，不冒称独立尾核完成。SQLite stderr为PG模式下taskrunnerDbAvailable对无效宿主路径的清理适配探测，只跳过宿主SQLite清理分支；PG任务链真实通过，私有卷已收回。外部testutil容器/reaper退出另列，8个自有容器/4卷/1网精确回收。
