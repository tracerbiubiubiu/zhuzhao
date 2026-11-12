# MCP首批发布、外部消费与身份衔接合同

日期：2026-11-12。状态：**FD05本次新增补研／A03与I01/I02的设计候选，未实现、未运行协议测试**。它不追改2026-11-11基线的研究深度，不自动批准新的AS部署或扩展所有工具。当前范围/估算由[总账](../review/planning-backlog.md)维护；本页拆分已同步[AI总纲](ai-capabilities.md)、[阶段入口](../phase7/README.md)和[执行排期](../review/execution-schedule.md)，仍是候选而非实现收据。

## 1. 本次作出的最小选择

建议A03在**同一P7最薄范围**内交两条可验旅程：一个已登记客户端通过MCP查询自己可见的工单；平台作为MCP client消费一个已登记的只读情报工具，把来源和结果显示给当前有权用户。P8增强企业Agent、逐主体知识/情报ACL、多提供方、可选Tasks/Skills扩展及更多写工具；不把首个外部MCP调用整体留到P8。P7首批不直接写业务，后继可在§7全部条件具备时交精确确认写；本页不取消A02，也不把P7原受控写承接静默全部推至P8。

原发现是总账A03/P7包含外部MCP消费，而AI总纲把client整体列P8。本次已统一为P7读发布/消费、P7门后有限确认写、P8增强；A03-R粗估7–11日，A03-W的协议适配2–3日，合计9–14日，不重复计I01/I02/A02业务，也不隐含生产AS/登录桥和企业等待。P7的mock/contract与真实放行分开：无真实AS/服务时前两层可完成，真实出口保留pending；不能把测试AS或合成工具称为企业联通。

首批发布只包含下文两个工单查询工具，不开放任意SQL、任意HTTP、导出、附件原文、内部备注、改状态、转派或后台任务写工具。消费侧先选择有明确业务价值的只读情报结论，不支持从`tools/list`自动导入并执行所有工具。MCP不替代普通HTTP、U08表单或A01/A02界面。

## 2. 已核实现、权限及不能继承的假设

现行源码可复用的是领域用例，不是现成MCP或OAuth服务器：

- `internal/service/ticket/service.go`的`Service.List`、`Service.Get`及`resolveListFilter`已经调用工单资源范围；`internal/repository/ticket_repo.go`的`List`目前COUNT与明细分别查询、`id DESC`和页码分页，不是游标快照。
- L1依照`internal/middleware/casbin.go`的`CasbinAuth`思路，用`RBACService.GetRoleCodesByUserID`取得**当前有效角色**，对既有业务路由模板/方法执行Enforce。列表对应`GET /api/v1/tickets`，详情对应`GET /api/v1/tickets/:id`；两者当前GET绑定归`ticket_list`页面（`ticket:list`）。000031的B案明确读按钮可为零`menu_apis`，不能只因`ticket:read`字符串存在就当有详情路由权，更不能为MCP发明一组`mcp:*`业务权限。
- 情报消费映射现有`intel:query`及`GET /api/v1/intel/query`的L1资格；由`migrations/000067_intel_cache.up.sql`、`internal/router/router.go`与`internal/handler/intel_handler.go`的`Query`定位。这个资格不自动取得任意外部知识集合权限。
- `internal/middleware/jwt.go`的`JWT`处理本地AT/PAT和禁用/黑名单/密码纪元；没有MCP的资源发现、授权码/PKCE和OAuth audience。`X-Operator`及utils AK/SK说明内部服务和归因，不是用户OAuth授权。
- 当前`internal/middleware/cors.go`的`CORS`放行所有Origin；MCP专用入口不能直接继承它。当前`threatintel.Service.Query`以`intel_type/query_value`查全局缓存，不能拿它缓存逐用户ACL结果；缓存键也没有提供方/合同世代。

推荐MCP adapter首期在主仓可信进程调用同一领域服务，避免再造一个持有管理员JWT的HTTP代理。协议入口建议`POST /mcp`，由独立OAuth认证、Origin/Host与协议检查保护；不把整条`POST /mcp`挂到工单页面权限，也不配置一个大前缀豁免。实施时按[公约](../standards.md)§3的第三方协议防腐层在路由目录登记**精确协议路径及逐工具L1分派断言**，并让`AuditRouteCatalog`仍能发现未受保护的新路由。该登记不是跳过领域权限的selfService捷径。

## 3. 协议、SDK和transport候选固定下来

本次在线核到的规范是**MCP 2026-07-28**。SDK候选固定为官方Go SDK **v1.8.0**：官方发布页存在该tag，兼容表把v1.7.0+列为支持此版规范，tag的`go.mod`要求Go1.25.0，低于主仓现有Go1.26.10。这里仅选择实施基线，尚未改`go.mod`、下载/验sum、扫描依赖或做互操作；实际开始时若因安全修复换版本，更新精确组合与收据，不使用浮动`latest`。[官方发布](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0)、[兼容表](https://github.com/modelcontextprotocol/go-sdk#version-compatibility)、[固定tag依赖](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.8.0/go.mod)

P7默认只有一个Streamable HTTP profile：2026-07-28、HTTPS、单个`/mcp` POST端点、只读同步结果；服务端先发JSON，客户端同时能处理JSON和请求级SSE。**此版每请求携带protocolVersion/clientCapabilities，不做旧版`initialize`会话，不以Mcp-Session-Id保存用户权限**；GET/DELETE不作为MCP业务操作支持。协议元数据/必需镜像头交SDK编码与校验，不能把`Mcp-Name`头当未经body校验的授权事实。[规范transport](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/streamable-http)、[协议基础](https://modelcontextprotocol.io/specification/2026-07-28/basic/index)

已核固定tag源码的`StreamableHTTPOptions`有`Stateless`、`JSONResponse`、`MaxRequestBodyBytes`和`PropagateRequestCancellation`；实施候选分别为true、true、64KiB、true，关闭旧会话兼容调试开关。它的跨源保护需要显式包装，不能因采用SDK就假设安全默认已全开。client显式使用空能力集，避免SDK历史默认宣告roots；不设置sampling/elicitation处理器，不接受外部工具诱导主机读文件或二次调用模型。[固定transport源码](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.8.0/mcp/streamable.go)、[固定client源码](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.8.0/mcp/client.go)

P7组合至少固定“本仓adapter构建摘要＋SDK1.8.0＋规范2026-07-28＋参考Go client构建摘要＋测试AS配置摘要”。不用SDK宣称支持的全部旧版作为本项目支持承诺。2025-11-25或其它客户端若确需接入，P8增加独立兼容profile与同样的负例；首版收到不支持版本按规范返回支持集合，不静默降级为无认证的旧SSE。协议版本、工具主版本、schema摘要、核心程序版本和授权状态分别记录。

## 4. 首批工具与Schema

工具名与能力引用为候选合同ID，**不是新增功能权限码或现行已注册API**：

- `zhuzhao_ticket_list_v1`：映射I01只读工单列表用例，L1按既有列表GET，领域`Service.List`按当前actor注入范围。默认只查“处理人=我”；`assignee=visible`只去掉处理人条件，绝不去掉可见范围。首版不提供时间解析/动态排序/跨页聚合参数；自然语言澄清在A01完成后给这个工具确定参数。
- `zhuzhao_ticket_get_v1`：映射同一`Service.Get`，L1按详情GET，L2逐票。`ticket_id`必须可解析为正int64；不能从字符串拼出任意路径。返回正文纯文本投影、来源/取数时刻及平台相对链接；内部备注、附件、任意custom_data不默认进入模型。
- `mock_intel_lookup_v1`：是P7消费侧的**合成外部server工具名**，本地能力为`intel.conclusion.lookup.v1`、沿用`intel:query`。真实提供方可能使用不同名称/权限，须私有adapter显式映射。不得把这个mock名称当成已确认的企业接口。

输入是JSON Schema2020-12的封闭对象，不接收`actor_id`、角色、组织范围、token、endpoint、SQL或任意URL。工具描述的readOnly等注解只帮助呈现，工具目录的受信配置与实时授权才决定能否执行。输出提供`outputSchema`并验证`structuredContent`，文本文块仅为受权摘要，不额外放出比结构化对象更多的数据。[官方工具合同](https://modelcontextprotocol.io/specification/2026-07-28/server/tools)

### zhuzhao_ticket_list_v1 的 inputSchema

```json
{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "assignee": {
      "enum": [
        "me",
        "visible"
      ],
      "default": "me"
    },
    "type_code": {
      "type": "string",
      "minLength": 1,
      "maxLength": 50
    },
    "status": {
      "type": "string",
      "minLength": 1,
      "maxLength": 20
    },
    "priority": {
      "type": "integer",
      "minimum": 1,
      "maximum": 4
    },
    "keyword": {
      "type": "string",
      "maxLength": 50
    },
    "page": {
      "type": "integer",
      "minimum": 1,
      "maximum": 100,
      "default": 1
    },
    "page_size": {
      "type": "integer",
      "minimum": 1,
      "maximum": 50,
      "default": 20
    }
  },
  "required": []
}
```

### zhuzhao_ticket_get_v1 的 inputSchema

```json
{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "ticket_id": {
      "type": "string",
      "pattern": "^[1-9][0-9]{0,18}$"
    }
  },
  "required": [
    "ticket_id"
  ]
}
```

### mock_intel_lookup_v1 的 inputSchema

```json
{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "indicator_type": {
      "enum": [
        "ip",
        "domain",
        "hash"
      ]
    },
    "value": {
      "type": "string",
      "minLength": 1,
      "maxLength": 200
    }
  },
  "required": [
    "indicator_type",
    "value"
  ]
}
```

默认值由adapter在通过schema校验后确定性填入，不依赖JSON Schema validator自动补值。列表`status/type_code`是精确过滤；未知状态仅能在有权类型定义内解释，不将输入拼SQL。首期`priority`限定业务已有1–4；这是比原handler任意整数更窄的工具子集，不改旧REST语义。`page`大于100时提示收窄条件，不能偷偷导出全集。

### 完整输出类型规则

三个工具都用以下封闭结果结构，`status`只为`ok/error`，`contract_version`恒为`1.0.0`，`request_id`为1–128字符，`source_ref`为1–100字符，`observed_at`为带时区RFC3339，`warnings`最多8项且每项不超过200字符。成功时`error=null`、`data`为下列对应对象；失败时`data=null`、`error={code,message,retryable}`。`code`仅用现有10001/10004/10008/70001/92601/92602，message不超过200字符，不发明新的业务错误码。

- 列表data：`items`最多50个`TicketSummary`，`total`为非负int64的十进制**字符串**（`0`或无前导零正数、≤9223372036854775807）、`page`为1–100、`page_size`为1–50、`order="id_desc"`、`consistency="live_non_snapshot"`、`complete`为布尔。`TicketSummary`仅含`id`（正int64十进制字符串）、`human_key`（≤64字符，历史缺号为空）、`title`（≤200）、`status`（≤20）、`priority`（整数）、`updated_at`（RFC3339）、`link`（仅`/tickets/<id>`）。不返回员工姓名/工号、组织路径、secret或附件地址。
- 详情data：`ticket`为同一TicketSummary，`description_text`≤8000字符，`description_truncated`为布尔。富文本先按可信渲染/文本转换规则消毒；超长保留截断提示和平台链接，不让模型误当全文已读。`link`由本平台受信路由构造，点击仍需重新鉴权。
- 合成情报data：`indicator_type`同输入枚举、`normalized_value`为1–200字符、`verdict`仅malicious/suspicious/not_found/unknown、`source_record_ref/source_revision`各≤128字符、`fetched_at`为RFC3339、`stale`为布尔。`not_found`只表达来源查无，不等于安全；来源不能判断则unknown，不使用模型补评分。允许空的source_revision必须同时warning未知版本。

列表count与rows现状不在同一快照，`total`必须标为当前受权范围的近似同时观测值，不能承诺翻页过程中绝不变化；`complete=true`只在本次未截断且page=1、返回条数=total时给出，仍不冒称事务快照。首版不宣称摘要覆盖所有记录。若输出总字节超过256KiB，返回10001要求减小page_size或收窄条件，不截行后继续按原page_size翻页（否则会跳过未返回行），也不成功返回半截JSON。page/page_size与priority是有界普通整数；id与total按本项目int64字符串公约转换，禁止中转float64。

成功示例为设计用合成数据：

```json
{
  "contract_version": "1.0.0",
  "request_id": "sample-query-01",
  "source_ref": "zhuzhao-core",
  "observed_at": "2026-11-12T03:00:00Z",
  "warnings": ["实时分页不是跨页快照"],
  "status": "ok",
  "data": {
    "items": [{"id":"90001","human_key":"TK-DEMO-001","title":"合成整改核对","status":"open","priority":2,"updated_at":"2026-11-12T02:55:00Z","link":"/tickets/90001"}],
    "total": "1", "page": 1, "page_size": 20,
    "order": "id_desc", "consistency": "live_non_snapshot", "complete": true
  },
  "error": null
}
```

MCP返回时放入`resultType="complete"`、`structuredContent=<上述对象>`和不超范围的`content`，成功`isError=false`。[受控Schema bundle](mcp-first-contract.schema.json)含三个工具完整inputSchema/outputSchema、7个有效及10个无效Schema样本，以及14个尚未执行的语义负例；实施可从该文件生成工具声明，不依赖ignored收据。字符串格式还需在Go中核int64范围和业务约束，Schema通过不代表授权/协议互操作已通过。

## 5. HTTP身份、授权及无需企业AS的开发路径

### 5.1 角色选择：资源服务器与AS分开

推荐zhuzhao只做MCP资源服务器与业务授权属主，不把现有AT签发端点伪装成OAuth授权服务器。HTTP采用授权码＋PKCE S256的用户交互profile，客户端先预登记；首版不提供任意动态客户端注册或自动抓取CIMD。OAuth scope只作为委托上限，首批使用现有字面值`ticket:list`或`intel:query`；未启用写工具时不签`ticket:create`，更不签`admin`。scope与核心路由授权是交集关系，AS不能通过给scope绕过L1/L2。

选择理由：直接信浏览器JWT/PAT最省改动，但缺resource/audience、标准客户端发现/同意/撤销合同；自写完整通用AS把协议安全与账号体系再造一遍；独立成熟AS或企业AS提供OAuth，本平台保留身份绑定/实时业务授权，符合已有平台消费边界。P7用单AS、预登记client先验证，不为支持任意公共客户端扩大到DCR/CIMD市场。[官方授权](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization)、[客户端登记](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/client-registration)

这里**不假定企业AS已经存在或可用**：

1. P7 mock/contract使用仅监听回环地址的专用**测试AS夹具**，拥有独立issuer、合成用户、固定测试client/redirect和测试密钥，以已维护的测试组件或受限协议fixture覆盖code/PKCE/resource/scope/expiry及撤销负例。它是测试夹具，不是生产AS；不能靠固定任意Bearer字符串冒充走通OAuth。
2. 真正HTTP放行必须接入经验证的AS。企业AS可用且满足合同则接；否则以**独立、成熟OAuth AS组件＋本地IAM登录/同意桥**作为独立部署的推荐路径，不自行从JWT工具库拼一个生产AS。选定组件/固定制品、登录桥互操作和部署成本在I02/A03实施切片内单列，不假称已经有现成AS或“只改endpoint”。
3. 若生产环境既无可用AS、又不采用独立AS，该profile保持关闭，核心表单/REST继续用原认证运行；允许关mock/contract出口，真实MCP放行保持pending。后续取得AS的输入/验收条件与具体提供方记录在E01，不把整个A01或核心平台停工，也不改用HMAC冒名过门。

开发fixture与生产AS替换共享同一资源服务器验证/核心授权路径，只有issuer/token verifier和身份绑定输入不同。测试AS不能授予合成用户不存在的核心角色。正式采用哪一个AS实现尚未核验；这是明确的提供方组件选择，不再悬置“到底使用OAuth还是复用JWT”的架构取舍。

### 5.2 一次用户查询的真实授权时序

1. client向`/mcp`请求，缺token时返回401及Protected Resource Metadata位置；元数据声明canonical resource，例如合成`https://mcp.example.invalid/mcp`及唯一允许issuer。客户端核元数据后按预登记信息与AS发起code＋PKCE，authorization和token两步都带同一个resource。
2. AS进行用户认证/逐client同意，发MCP专用access token。企业AS主体经**已批准的`(issuer,subject)→local_user_id`绑定**解析；不按同名/邮箱自动并号。首次绑定需有可信本地登录或管理员证明，客户端不能提交local_user_id选择他人身份。
3. 独立AS没有公司身份时，浏览器走本地IAM登录桥：登录仍用已有账号/会话；桥只向AS签发一次性、短期、限定audience和OAuth request/client的登录证明，不把浏览器AT/RT/PAT或密码给MCP client。桥校验state/PKCE请求关联并展示client、目标resource、scope及到期；它只证明用户认证与本次同意，不给AS直接读取工单的能力。登录桥是I02待实现安全用例，不能把现有SSOProvider（本平台消费IdP）误认为已经支持反向AS登录。
4. 资源服务器验证token签名或受认证introspection、固定issuer、**audience精确等于此MCP resource**、有效期、scope、client绑定和撤销状态；拒绝ID token、发给REST/另一个MCP的token、本地`zpat_*`和X-AK身份混用。SDK`auth.RequireBearerToken`只提供承载，`TokenVerifier`内的这些校验和业务绑定不能省略。[固定SDK认证接口](https://raw.githubusercontent.com/modelcontextprotocol/go-sdk/v1.8.0/auth/auth.go)
5. 解析得到actor后按当前本地active/删除/安全状态和I02 grant复验；禁用用户、撤回client同意或grant后下一次请求拒绝。JWT自包含token有效期不等于允许延迟撤权；首版每请求查询AS grant/introspection及本地授权权威，不能确认时失败关闭，缓存只做同请求复用，不延长撤销期。
6. `tools/list`按当前scope、当前用户业务GET路由资格、server/场景允许集过滤并排序；scope充分但本地权限已撤销仍不可见。`tools/call`再次做同一检查，再以服务端actor调用`Service.List/Get`。客户端缓存目录、readOnly注解、上次连接及自报clientInfo都不赋权。结果/批准handle每次查所有者和当前范围，不是持有handle即可访问。
7. 结果只回传当前可见投影，审计关联client登记ID、issuer/subject的去敏引用、local actor、工具/schema摘要、输入摘要、request/trace、判断结果及数据源；不记完整token、敏感标题/正文或模型思维链。

源要求的资源发现、resource和audience校验来自[授权规范](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization)与[授权安全要求](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/security-considerations)。本页选择每请求撤销检查是平台更严格的实施策略；规范/SDK存在并不证明本平台已实现。

### 5.3 CSRF、跨源、重定向与内部签名

MCP endpoint只认Authorization头的MCP专用token，不接受环境cookie自动认证；有Origin时必须与受审白名单完全匹配，无Origin的原生client仍须token/Host/出站检查。开发只绑127.0.0.1/::1，生产TLS，拒绝DNS rebinding/伪造Host；CORS按预登记web client允许具体Origin和必需头，不开`*`或“有cookie即可调用”。

OAuth登录/同意桥与client回调使用单次state、PKCE S256、固定issuer与精确redirect URI，state绑本次浏览器会话/client/resource/PKCE请求且短期失效；同意表单另验CSRF token、Origin与点击劫持防护，不能GET访问即批准。换AS不得复用旧issuer下的client凭据；callback issuer不符先拒绝，不把授权码送去错误token endpoint。客户端打开授权URL仅允许受审HTTPS或开发回环HTTP，不经shell执行。[MCP安全实践](https://modelcontextprotocol.io/docs/2026-07-28/tutorials/security/security_best_practices)

MCP Bearer不转发到SOAR、模型或外部MCP。外部工具用自己的AS/token/secretRef；内部AK/SK只保护服务跳，不取代用户授权。若后续MCP adapter拆进程，须由I02/X01发短期、限定audience/工具/资源/用户的受信委托，纳入签名/可信传输；现有只签X-Operator不足，禁止把任意user_id附到原HMAC后就按该用户放行。首版同进程省去这条新分布式身份链，不省略核心L1/L2。

## 6. P7消费外部工具的完整最薄旅程

使用合成、已批准共享集合`mock-shared-intel`：有`intel:query`的用户在U08选择`intel.conclusion.lookup.v1`，输入`{"indicator_type":"domain","value":"indicator.example"}`，平台调用登记的`mock_intel_lookup_v1`，返回来源/版本/取数时刻与明确结论。首版无需LLM；A01/领域Agent以后消费同一Query。

### 6.1 连接声明与发现

连接配置在服务端保存`connection_id,endpoint_ref,data_zone,issuer_ref,client_ref,secret_ref,resource,allowed_tool,protocol_version,input_schema_digest,output_schema_digest,dataset_mode=approved_shared,limits,owner,enabled`，不从模型参数接收URL/token。登记者必须沿用已授权的受审配置/连接管理通道；此处不新造通配业务功能码，不因为有intel:query就允许注册server。

连接的server、AS元数据/JWKS/token端点、重定向和DNS目标都走出站允许集；内网可以明确允许特定私网服务，但这不是接受任意RFC1918地址。每次发现和重定向复核目的地，禁止访问云metadata、回环管理服务或越数据区；凭据按issuer+resource+client隔离。预登记测试client完成外部AS的同一code/PKCE流程，凭据留服务端；生产若用具名服务主体访问共享集合，需供应方明确支持该OAuth profile并单列合同，不能把本平台用户token换个URL直发。

`tools/list`最多10页/100个声明/1MiB发现数据；先核规范/工具名称和封闭Schema，再核**已登记工具与schema摘要**，只加载允许的一个。首版不启用远程`$ref`解析，不接受任意脚本/stdio命令启动，也不因readOnlyHint自动授权。发现额外工具只记录漂移；缺工具、输入/输出schema改变、AS/resource变化暂停该连接并报差异，不自动升级允许集。

### 6.2 调用、数据区与结果

每次调用前再次检查本地intel路由资格、当前用户/场景允许集、连接启用/数据区和共享集合授权；仅传经过白名单/长度与类型校验的指标。不向远端发送工单正文、员工信息、Cookie或核心token。上游结果按input/outputSchema验证并规范化为§4情报data；工具返回的指令/URL都是不可信数据，不能要求扩大scope、安装skill或发第二个工具调用。

首版外部Query**不进入现有S9全局缓存，也不默认发布为本平台的第三个MCP工具**。U08经I01用例返回给当前用户，内部Query投影和外部tool名称相互隔离。这样避免用现有全局`(type,value)`缓存承载不同source/schema/ACL的数据。P8若要复用缓存，须先把来源/集合世代、合同摘要、必要主体/权限epoch纳入缓存键及撤权/删除策略，或保持不缓存。真实服务只有逐主体ACL而无共享集合时，走独立用户授权/身份映射，不能以全局服务号+本地prompt裁剪冒充ACL。

预算候选：外部单次5秒、全用例10秒；输入64KiB、结果256KiB、每用户最多2并发、每连接最多8并发，实际值由实现容量验证校准。只读查询在未收到可用结果时最多重试一次，限429/临时网络与502/503并遵Retry-After，总预算不重置；认证/权限/schema/版本/其它4xx不自动重试。重试仍重新鉴权且不接受模型自行重试无限循环。业务数据写禁止通过这个只读连接混入。

### 6.3 失败、取消与恢复例

- 上游查无：成功`verdict=not_found`，不推断安全；上游无权或集合未知：失败，不映射成not_found或空成功。
- token过期：仅按已获授权的refresh能力刷新一次；刷新失败暂停连接/提示重新授权，不向别的AS递交旧凭据。用户撤权后旧result引用也重新检查，不靠结果存在继续开放。
- tool发现后schema变更：本次拒绝执行，保留去敏差异；受审重新发布映射再恢复，不能让模型适配未知字段。
- 结果不是JSON/超限/输出Schema非法/来源版本未知：非法结果失败；只有schema允许的unknown版本可成功并显式warning，不把任意原文返回模型。
- 超时/HTTP断开/进程退出：本次只读调用记failed/canceled，保留request/attempt，用户可重新查当前事实，不声称恢复成历史快照。取消由HTTP请求取消传到SDK/上游；即使供应方未停止计费，也不能显示业务动作已撤销。
- 多用户并发、连接停用与撤权竞争：任何结果发出前至少按当前本地授权再检查；请求开始后权限变化的残留响应由C04/I02约束，禁止用目录缓存或连接池中的旧用户身份串号。

## 7. 协议错误、业务失败和确认写入边界

HTTP认证缺失/无效为401并带resource metadata；OAuth scope不足为403及所需scope，当前核心角色/资源不足不会自动发新scope或建议申请管理员。Origin/Host非法403，body超限413，限流429；这些在工具执行前返回。JSON/JSON-RPC结构损坏用标准协议错误；不支持版本和头/body不一致分别用规范的UnsupportedProtocolVersion/HeaderMismatch（由固定SDK处理），不把10000业务码塞进JSON-RPC保留码段。

已识别工具但参数越界/未知字段、业务校验失败或来源不可用，用`isError=true`和§4 error对象；未知工具用JSON-RPC Invalid params。工单详情不存在与无权统一对外10004，内部审计仍分清；列表整体权限不足70001。10001要求修正参数，10008/92601/92602只有对应临时来源失败才可标retryable；70001/10004不自动重试。错误中无SQL、内部地址、token或受限对象存在性。正常空列表必须是`status=ok`，不能将来源错误包成空成功。[官方错误分层](https://modelcontextprotocol.io/specification/2026-07-28/server/tools#error-handling)

首批工具没有直接创建工单的能力。P7若接入A02的草案/preview，可先以受权深链打开平台确认页；这是草案/计划状态写，不是创建业务工单。client弹一个“允许工具”框、模型回复“已确认”、MCP elicitation accept或拥有`ticket:create` scope，都不能替代平台对精确计划的逐次批准。

后续可发布`ticket_create_preview/confirm`工具的放行门是：I02受限委托、A02 plan/hash/actor/目标/定义版本/expiry、C04事务内撤权守卫、R01/receipt幂等及结果查询全部完成。批准必须在本平台受信页面完成并绑定**真实用户、client、计划摘要和operation_id**；工具最多携带不可猜的批准引用，服务端确认引用所有者/有效性并原子消费，同key同命令返回原receipt。变计划/撤权/过期拒绝；返回丢失查receipt，不让模型生成新key重做。若客户端不支持这条确认协议，保留深链到平台，不开放远端confirm。无人值守/周期写继续归A04持续grant合同，不由本页读工具偷渡。

## 8. 实施验证矩阵、接入门与仍未验证的事实

P7最薄交付须证明，而不是仅实现SDK示例：

1. **协议合同**：固定规范/SDK/client，`server/discover`、tools/list分页与允许集、两个发布工具的正反Schema、JSON/SSE结果与取消、必需元数据、头/body漂移、未知版本/工具、超限、非法输出分别留回执。不宣告未实现的resources/prompts/sampling/elicitation/Tasks能力。
2. **本地AS链**：两个合成人物和两个client分别完成发现→code/PKCE/resource→token→核心身份绑定→工具调用；验证错误aud/issuer、过期/撤销、假subject、state错/重放、redirect漂移、混用PAT/HMAC、Origin/Host攻击及授权服务失联。mock只替AS/提供方，不替核心L1/L2。
3. **业务权限**：同一主体经表单/REST与MCP结果行范围一致；跨组织和类型查看范围、CC不扩处理权、list后撤权/get被拒、计数/正文/链接不泄漏；用专属合成账号，不动E000001。
4. **消费链**：U08→I01 Query→登记外部MCP→规范化来源/结果；验证发现分页限额、额外工具不启用、schema漂移暂停、SSRF/重定向阻断、错误/无权≠查无、注入文本不执行、取消/超时/并发不串用户、全局缓存不被误用。
5. **企业放行门**：实际AS发行者/客户端登记/资源值/token类型与撤销方式核验；主体映射、数据区/出站/TLS/代理、提供方集合ACL/Schema/配额/取消、去敏真实样本与业务效果分别验。一个真实client＋一个真实工具成立才登记该组合real通过，不泛称全部MCP客户端兼容。无真实条件时明确`mock/contract ready, real pending`。

P8增强以已证明的P7合同为基线，逐项增加实际企业Agent、用户授权/ACL、更多数据集合、缓存、长任务/Tasks、skill分发和写工具；每项重新登记工具版本/批准政策与兼容范围，不把P7当前两读工具的通过扩张成增强已验收。异步/长任务不能仅因工具HTTP200或SSE结束就当业务完成，仍走[I03/R01跨仓合同](cross-repo-execution-contract.md)。

本次完成的是FD05补研正文、受控工具候选Schema和合成回执例的静态自检：使用UI已有Ajv6.15.0验证本Schema所用的draft-07/2020-12共有关键词子集，6个Schema及17个正反样本均符合预期；这不是完整2020-12、MCP或OAuth conformance，14个语义负例未执行。未运行AS、MCP server/client、conformance、真实企业接口或任何业务门禁；没有模型/内部资料上传，没有新增依赖/配置/权限码/路由/迁移或启动实现。准确AS组件与真实企业配置仍待实施选定，但身份方向、无AS开发方式、首发只读边界、P7/P8消费范围及负例已不再只是一句“开工再冻结”。
