# B19：存储输出ID字符串契约

## 范围与基线

承接[B15-F01 / P2](./zhuzhao-ecosystem-audit-b15-contract-migration-boundaries-2026-11-06.md)：主仓 `996c167f4567e39362226937b25368b55b7f44bd` 的 `internal/model/storage.go` 中FileObject.ID、TicketAttachmentView.ID输出numeric JSON，而UI `01c5deae23e677853316f1ee2dd692c708246651` 的 `src/api/storage/index.ts` 手写声明string。两仓均在Phase5修复；AL/TR/utils没有该浏览器输出消费者，不增加无关改动。不合main、不开始Phase6、不发布utils版本。

固定原问题锚点：[storage.go 的 `FileObject / TicketAttachmentView`](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/internal/model/storage.go#L6)、[UI storage的 `ConfirmResponse / AttachmentView`](https://github.com/tracerbiubiubiu/zhuzhao-ui/blob/01c5deae23e677853316f1ee2dd692c708246651/src/api/storage/index.ts#L24)。生成类型原本正确描述numeric服务端，不是“仅生成物过期”。

## 修复与兼容性

主仓模型统一十进制JSON string（包括内部孤儿清理投影，关联实体本来已是string），真实再生三份Swagger产物；UI真实再生schema，仅对应两个输出ID由number变string，storage输出类型接入生成定义并对实际必出字段加Required Pick，不再靠手写string掩盖wire类型。增加模型/真实HTTP与UI契约回归，测试数量同步为29文件230例；输入类型统计变为5/16、输出storage新增1/16、去重联合6/16，其他域未假称接线。

**破坏性变更**：ConfirmUpload、BindAvatar、ListAttachments中的上述`id`从JSON number改为string。DB仍BIGINT，已有ticket_id输入仍string-only，object_key仍是上传/头像/删除/下载寻址依据，未改端点/迁移/权限。只收numeric ID的旧Go/其他外部解码器需要同步升级；保留旧数字输出无法恢复JS已丢精度。本仓内消费者已经核对，仓外未知调用方未获全量清单，不宣称兼容所有外部消费者。两仓提交后同一推送/承接窗口更新；当前未部署此契约。

## 验证证据

- 专属真实PG写入2^53与2^53+1，通过真实TCP调用现有confirm/avatar/list handlers；L2检查与S3为针对序列化的stub，不能作为鉴权/S3集成证明。原numeric HTTP响应交给标准JS JSON.parse后Set.size=1，修复string响应Set.size=2；object_key始终两条精确且实际DB头像绑定仍按key。
- 真实HTTP守卫移除string标签连续8轮稳定失败，修复8轮通过；模型测试覆盖大ID、关联实体与原输入string-only边界。旧numeric Go解码器对新string按预期报错，正确string-tag解码精确，明确证明兼容性成本而非掩盖。
- UI storage lint/3条Vitest、真实vue-tsc通过；分别将生成FileObject.ID或TicketAttachmentView.ID改回number，vue-tsc均exit2，恢复exit0。不是只在测试里断言一份手工伪造类型。
- 含B19/B23的353源固定组合通过原生Go1.26.10完整lint/unit/cover/integration与原样253/0/0、81迁移往返零漂移。实际主容器ELF为`82179b0bc48a4337e78fabdb2448110c4ede038e4b634e6b486c4fcd9dfd95f0`，逐项匹配构建源清单；AL/TR实际ELF亦核对。Swagger/Wire真实再生无额外漂移。
- 同组合UI完整门禁的lint/typecheck、29文件230单测、生产依赖audit及build通过；41个E2E为40过、1个B20画布用例失败（含一次配置内重试），附件上传/删除/网络失败重试用例通过。UI整体仍不记完成，B20修复后须重验完整门禁；不以此次部分通过替代最终前端验收。

原始前后HTTP/JS、生成命令、源码SHA、变异、编译兼容和联合失败记录保留在ignored `.codex/audits/ECO-20261104/phase5-closeout-20261107/{storage-contract,joint-b19-b20-b23}/`，不进Git/Docker。当前仅完成代码/专项及后端组合出口；两仓完整修复hash、准确CI与部署状态提交后另绑，不提前报告生产事故或已部署修复。
