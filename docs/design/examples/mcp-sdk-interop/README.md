# MCP 官方 Go SDK 本地互操作记录

> 日期：2026-11-13。SDK：github.com/modelcontextprotocol/go-sdk v1.8.0；协议：2026-07-28；主机工具链：Go 1.27.4 darwin/arm64。
> 本目录为受控设计审计样例，使用独立Go module，尚未纳入产品、生产依赖或CI。只用httptest回环HTTP、合成OAuth AS/主体/授权/工单/外部情报，不访问企业平台或真实模型，也未证明主仓现有Service/IAM已按本合同接线。

## 复跑

从主仓根目录先进入本目录，再运行。Go依赖按go.mod/go.sum校验；首次缺模块时仅由Go标准模块机制下载公开依赖，不改产品go.mod或系统工具链。

```bash
cd docs/design/examples/mcp-sdk-interop
env GOMAXPROCS=2 GOFLAGS=-p=2 GOMEMLIMIT=768MiB go test -mod=readonly -count=1 -timeout=90s -v ./...
env GOMAXPROCS=2 GOFLAGS=-p=2 GOMEMLIMIT=768MiB go test -mod=readonly -race -count=1 -timeout=90s -v ./...
```

原型从主仓`docs/design/mcp-first-contract.schema.json`读取三个真实候选工具Schema；原型本身可编译并创建官方SDK服务端与客户端、实际进行HTTP请求，不以Schema通过替代协议互操作。测试AS只实现本例需要的authorization code＋PKCE S256＋resource绑定和受认证introspection；fixture登录头、固定state/verifier、内存code/token表均不应移植到产品认证实现。

## 结果与边界

10个顶层测试、33个叶用例，普通与race均通过。两只读发布工具：zhuzhao_ticket_list_v1、zhuzhao_ticket_get_v1；只读消费工具：mock_intel_lookup_v1。两名主体的结果分别限制，JSON/SSE两种响应实际观测；无initialize、每请求version/capabilities、modern resultType、GET/DELETE405、64KiB输入/256KiB输出、HTTP取消传至handler都已实跑。

14个语义负例全部实际发送到SDK/HTTP或消费者受控配置门：WRONG_AUDIENCE、LOCAL_AT_OR_PAT、SCOPE_WITHOUT_L1、CROSS_ORG_GET、AS_OR_CORE_AUTH_DOWN、INPUT_ID_INT64_RANGE、TOTAL_INT64_RANGE、SCHEMA_DIGEST_DRIFT、PRIVATE_DATA_WITH_SHARED_CACHE、OVERSIZE_RESULT、CONFIRM_WITHOUT_APPROVAL、OAUTH_STATE_REPLAY、ORIGIN_REBIND、REMOTE_UNAUTHORIZED。

另验token撤销/过期/固定issuer/主体/client/scope拒绝、当前本地授权在响应前撤销不返回数据、每用户2/此连接8个inflight上限与429/Retry-After及permit恢复、protected resource metadata、禁止sampling/elicitation/input-required自动续跑。并发限流和当前授权是本地reference adapter，不是多实例共享限流或产品C04/I02的实现证据。

固定SDK的一个边界已记录：未知未来新版2099-01-01返回HTTP400与JSON-RPC -32022及支持集合；未知旧header1900-01-01走legacy前置检查，返回HTTP400纯文本。首轮把旧分支错误当成现代分支断言失败，去敏错误断言及实际响应保存在report.json的initial_assertion_correction字段，原始日志留本地审计区；随后增加两分支精确断言，不将改断言包装成产品修复。

客户端必须显式设置空Capabilities以及MultiRoundTrip.Disabled=true。v1.8.0的Capabilities=nil默认推断roots，MultiRoundTrip=nil默认安装自动input请求处理/重试；本原型用官方源码固定哈希并实测禁用后的sampling/elicitation不自动重入。现代resultType是SDK的顶层MCP结果判别，不应加入本项目structuredContent业务envelope。

## 证据与未覆盖项

[report.json](report.json)记录用例名、固定tag源码SHA256、样例/Schema摘要、去敏结果和范围；本轮原始test-output.log、race-output.log保存在忽略的本地审计区。测试AS和actor/L1/scope授权表全部合成。未验证真实UAA/AS、核心Service.List/Get/三层鉴权/受限grant、多实例撤销与共享限流、生产SSRF/DNS/TLS/容量、完整OAuth一致性/同意UI、自动重试退避和发布流水线。这些必须按所属实现包接真实组件后另验，不因本原型通过就写成“已接企业”或“生产可用”。

官方证据：

- [版本与无会话协议](https://modelcontextprotocol.io/specification/2026-07-28/basic/versioning)
- [请求元数据与resultType](https://modelcontextprotocol.io/specification/2026-07-28/basic/index)
- [Streamable HTTP](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http)
- [Go SDK v1.8.0 release](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0)
- [固定tag client](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.8.0/mcp/client.go)
- [固定tag MultiRoundTrip](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.8.0/mcp/mrtr.go)
- [固定tag HTTP transport](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.8.0/mcp/streamable.go)
- [固定tag认证承载](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.8.0/auth/auth.go)

## 依赖与许可核查

官方SDK用来实际验证transport/client/result行为，替代手写JSON-RPC仿真；jsonschema-go对本项目原始候选Schema做运行时输入/输出校验，替代仅对字段写断言；oauth2执行真实HTTP授权码/PKCE交换，替代直接把测试token塞进客户端。三者仅属于本目录的独立研究module；产品采用哪个adapter或OAuth UI仍按A03实现门决定。

直接依赖固定为Go SDK v1.8.0、google/jsonschema-go v0.4.3与golang.org/x/oauth2 v0.35.0。固定tag的Go SDK LICENSE声明正在由MIT迁至Apache-2.0：新贡献与已获同意部分采用Apache-2.0，未获同意的历史贡献仍为MIT；项目非规范文档另为CC-BY-4.0，不能把整版简记成MIT或“自由任选双许可”。[固定SDK许可](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.8.0/LICENSE)；[jsonschema-go](https://raw.githubusercontent.com/google/jsonschema-go/v0.4.3/LICENSE)为MIT；[oauth2固定模块LICENSE](https://raw.githubusercontent.com/golang/oauth2/v0.35.0/LICENSE)为BSD-3-Clause。许可与固定模块中的LICENSE实读核对；完整模块版本/校验和在本目录go.mod/go.sum。这里只保留公开模块引用和本例原创测试，不复制SDK源码、产品依赖或真实服务配置；固定源码及许可证SHA256在report.json，不代表完成企业法律审查。
