# 平台工程公约（zhuzhao 生态）

> **适用范围**：zhuzhao / taskrunner / activelist / **zhuzhao-ui（前端——§3 API 契约的最大消费方）** / 后续所有内部项目。各项目仓库的工程文档引用本文件，**不必在各自文档中重复约定**。
> **地位**：跨项目工程公约的 **SSOT**。§25（design-decisions）为权限架构决策 SSOT；[16 号 §9](./phase3/16-external-integration.md) 为服务实例化落地表（允许差异/对齐清单），与本文件冲突时**以本文件为准**并回改本表。
> **修改纪律**：改公约 = 所有者拍板 + 本文件更新 + 受影响仓库文档同步（各仓变更记录留痕）。
> 建档：2026-09-04，由 16 号 §9 基线升格扩编而来。

---

## 1. 项目定位与分工

| 项目 | 形态 | 职责 | 权限 |
|---|---|---|---|
| **zhuzhao** | 单体（IAM 内核 + 统一网关 + 通用能力底座） | 认证/鉴权/用户/角色/组织/菜单/审计/事件发布；对外统一入口（网关） | 三层鉴权全量（L1/L2/L3）；权限管理面 |
| **taskrunner** | 独立仓库 + 独立部署 + 独立 Redis + 独立 PG | 事件/任务总线：调度、重试、死信、记录、看板（通用调度，**不做业务判定**） | 无用户权限模型；服务间 AK/SK |
| **activelist** | 独立仓库 + 独立部署 + 独立 PG | 动态数据模型薄层（类型注册/Schema 演进/数据 CRUD/导入导出） | 用户侧零权限（网关挡）；服务间 AK/SK |
| **zhuzhao-ui** | 独立仓库单页应用（Vue 3 SPA） | 前端控制台：消费 zhuzhao 网关全部业务 API（含 `/al` 反代与 taskrunner 代理端点），零权限判定（真实边界在后端三层，显隐仅体验层） | 纯消费方——遵 §3 全部 API 契约；权限码仅作渲染决策（2026-10-01 增行） |

- 微服务拆分不做（无多团队/M2M 需求）；**抄模式不抄架构**。
- 工单模块已封版（对接公司内部平台；重启条件见 design-decisions §23）。**状态注记（2026-10-01，§12-3 断言对账）**：「封版」指 §23 对接评估与审批翻案线挂起；基础工单域已于 Phase 2 交付、P4-W4 前端消费面上线——封版范围不含已交付的基础 CRUD/评论/关联面。

### 1.1 开源定位与公司信息隔离基线（2026-10-09 所有者定，全仓强制）

> **背景**：项目已从框架搭建进入**具体业务实现**阶段，所有者难免以公司内部文档举例/参考；但本项目定位为**开源项目**，且实现者将在公司内网完成公司特化部分。以下基线对**文档与代码同等强制**。

1. **公司信息零出现**：本仓（docs + 代码 + 配置 + 测试）不出现任何公司内部专有信息——内部系统名/接口契约/报文格式/组织与人员信息/内部文档内容。存量决策记录（dd/roadmap 等）中的"公司内部平台/内网迁移"属**定性背景描述**，保留不改写；**增量文档一律以通用抽象表述**（如"企业统一认证""内部工单平台"级抽象，不落到具体产品/系统/接口）。
2. **对接一律预留接口，不在本仓实现**：所有公司内部对接场景（认证/消息/情报/HR/取数等）= **Port + Adapter + config 挂点 + 桩实现（未配置 fail-fast）**，公司特化 adapter 在内网仓库实现。公共产品名（钉钉/飞书等）作渠道**示例**不受限，但不得绑定为本仓实现目标。**本仓 config 默认值与文档示例一律占位符化**（`example.com`/`<ENDPOINT>` 等）——真实内网地址/凭据只存在于内网侧配置（env/内网仓库），不入库；公司特化 adapter 的注入机制（多实现注册/构建期替换）随首个对接实现定型，本仓不预建。
3. **测试全 mock，数据全合成**：依赖外部系统的测试一律 mock 方式（本仓不持有、不模拟真实公司凭据与报文）；**mock 数据必须合成构造，禁止使用脱敏的真实公司数据**（脱敏不等于不可关联）。
4. **示例纪律**：以公司文档举例产出的设计，落档时必须**脱敏为通用抽象**（举例特征不得残留）。
- 违反处置：视同敏感信息泄漏处理（发现即修）；code review/审计将该基线列为检查项。

## 2. 服务间通信与鉴权

| 项 | 约定 |
|---|---|
| 协议 | **HTTP + JSON**；gRPC 不引入 |
| 服务间鉴权 | **AK/SK HMAC 签名**（utils `aksk`：按调用方发 SK、env 注入、常量时间比较、时间窗 ±5min 防重放）+ **专用 network 双防线** |
| 身份断言 | 明文 `X-Operator` 头 + **纳入签名覆盖**（不可伪造）；缺失兜底 `"system"`。AT 透传验签（方案 A）降为触发条件驱动 |
| 密钥管理 | 内部服务 = env 静态 SK；外部 M2M 平台凭据（api_keys 表/签发吊销）🚦 随调用方出现（09-platform，算法复用 `aksk`） |

## 3. API 设计约定

1. **方法仅 GET / POST**——PUT / DELETE / PATCH 不引入；**立法理由（2026-10-15 补记，防再议）**：资源语义由路径 + 权限码（`resource:verb`）承载，方法仅承载「幂等读 / 非幂等写」二分——引入 PUT/DELETE 不新增表达力，只摊薄「写操作必须过鉴权/审计」这一 HTTP 层结构性不变量；方法面变更为全生态破坏性契约变更（网关匹配 / menu_apis 绑定 / 前端请求封装 / 存量端点全量对账），内部平台 + 前端唯一消费方语境下无对应收益（对外公开放 API 的自描述诉求不存在，gRPC-Gateway/GraphQL 全 POST 同款先例）；**辖域界定（2026-10-15 所有者拍板）**：本条辖**平台自有 API 面**——第三方组件/协议规定的动作方法不属辖域（我方出站调用第三方时其协议要求的方法，如 S3/MinIO 预签名直传 PUT、HEAD 探测；第三方回调我方时其协议固定的方法，如扫描器/SIEM webhook、OAuth backchannel），按协议实现，**不算违约、无需豁免**。护栏三条：①**隔离**——出站经 adapter/client（utils aksk.Transport 先例）、入站挂专用路由组（`/internal`、`/compat` 先例），协议方法不得进入业务 API 命名空间，域内代码继续讲 GET/POST 语义（防腐层）；②**登记**——每个第三方协议面随段设计稿口径注或落 §3-5 豁免清单，防例外常态化；③**禁反向援引**——不得以第三方协议方法为由变更平台自有端点的方法设计。首批应用例 = [phase5 S5（05 号 §3/§4）](../phase5/05-s5-object-storage-richmedia.md)；**业务自有 API 新端点一律按本约定，不适用任何新例**。
2. **POST URL 不携带业务信息**——资源标识/动作参数全部在请求体（GET 的 path/query 参数不受限）；
3. 响应结构统一 utils `errcode` + `response`：信封 `{code, message, data, request_id}`——**code 为业务码（0=成功），不放 HTTP 状态码**；分页固定 `PageData{list, total, page, page_size}`；业务失败直接映射 HTTP 状态码（4xx 不可重试 / 5xx 可重试），**响应体不带状态字段**；**创建类端点统一 200（不使用 201/204）**；handler/middleware 不直接 `c.JSON`（探针 `/healthz` `/readyz` 类豁免）；**信封字段增删改 = 破坏性变更**，须评审并同步全部消费方（前端拦截器 + 测试断言）；
4. 错误处理走 errcode 映射，**禁止 raw 500 泄漏内部细节**；错误消息优先引用 errcode 常量，避免内联裸文案（无法与 api/errcode.md 对账）；
5. 存量豁免：zhuzhao 工单管理面 5 处 PUT/DELETE 已随 P4-W1 BK-18 整改为 POST（000030，2026-09-26——豁免消灭，仅留痕）；**taskrunner `/tasks/cancel`·`/tasks/retry` 动词段豁免**（同构违反 §3-2 但性质不同——纯内部 M2M 语义动词段，B 案/W5 已按此形态设计，改=推翻已定契约收益低，业界 RPC-over-HTTP 常态；2026-09-29 P4-W5 前置批注记）；**zhuzhao 侧 `/api/v1/tasks` 组的 `/cancel`·`/retry` 为该豁免契约的镜像路由（透传 taskrunner，2026-10-15 补登记）**；**activelist 数据域 `/:typeName/:id/update`·`/:id/delete`「URL 标识 + 动词段」同条豁免（2026-10-15 清欠批补登记）**——先于 C10 整改批的存量形态、与已上线 ui 消费契约耦合，对端 implementation-plan §4 头部注记在册，整改范围不追溯；activelist 三端点参数段（`/:typeName/deprecate`·`/:typeName/schema`·`/:id/restore`）已同批整改为标识入 body（zhuzhao 迁移 000032 同步 menu_apis）；新端点一律按本约定；
6. **列表端点必须分页**：page/page_size 服务端钳制 + 确定性排序键（通常 id DESC）——禁止无分页全量返回；分页形状统一 `PageData`（offset+total，管理台消费）；**游标/keyset 仅用于机器全量迭代链路（同步/导出），不进响应信封**。**特例在册（2026-10-15 全仓审计立约）**：activelist 列表端点为 keyset 游标分页且 `next_cursor` 进响应信封——动态表百万行量级下 offset 深翻页不可用、zhuzhao-ui cursorPager 已消费该契约；属 offset+total 形态不适用的领域特例，新端点勿援引。时间线备查（2026-10-15 git 考古）：keyset 设计 09-03（activelist `47e3644`）早于公约建纲 09-07（`c4e30b4`——建纲版本条仅要求"必须分页+钳制+确定性排序键"，keyset 实现对当时条款合规）；禁止性扩写 09-09（`f854e35` 三仓信封收敛批）晚于实现 09-08（activelist `dad1b12`）一天且未对存量对账——禁止性条款为既成实现环境后写，非先在约束。建档日期口径注：头注「建档 2026-09-04」与 git 首笔落库 09-07（`c4e30b4`）并存，本文档时间线采用后者（git 实证口径）；
7. **int64 ID 一律字符串序列化**（`json:",string"`）——规避 JS Number 精度丢失；
8. **时间统一 RFC3339**（传输）+ TIMESTAMPTZ（存储）——容器 TZ 统一（§4）；
9. **新增错误码必须登记 api/errcode.md**，编号沿用既有分段，不私造新段；**跨服务（taskrunner/activelist 等）自有错误码使用跨服务段 100000–109999**（activelist=100000–100999、taskrunner=101000–101999 预留；通用语义错误复用 10000 段现有码）——经网关/代理对外暴露前完成映射，6 位码与 zhuzhao 域内 5 位码数值可区分。**段位现状（2026-09-15 四仓审计对账）**：activelist 段已启用；taskrunner 段**预留未启用**（错误量级小，走 typed error + 通用段，见第 10 条，启用属触发驱动）；
10. **错误定义三形态与选择标准**（2026-09-15 审计定版）——按业务码量级三选一：① 码较多（≥5 个）：errcode 集中定义（zhuzhao 模式：`ErrXxx = util.New(码, "中文文案")` + api/errcode.md 登记）；② 需结构化 detail 上下文：apperr 结构体模式（activelist 模式：`Error{HTTP, Code string, Msg, Detail}`，detail 按键序折叠进 message——信封四字段约束下的既定解法；将 detail 升为信封字段仍须走破坏性变更评审）；③ 错误极少（<5 个）：typed error + `errors.As` 映射 + 复用 10000 通用段（taskrunner 模式）。**统一的是选择标准与纪律，不强制统一实现**；
11. **错误码引用纪律**：响应/映射处码值一律引用 errcode 常量（如 `errcode.ErrInvalidParams.Code`），**禁止内联码值字面量**（与 api/errcode.md 对账的前提，第 4 条文案纪律同理）；**服务端验签中间件不得各仓自研**——一律 `utils aksk.GinMiddleware` + `response.AKSKFail()`（2026-09-16 服务间验签统一批：统一信封 + 分档中文文案，归因键 caller/operator 由中间件统一写入，失败现场落 `Verifier.Logger`；aksk 包自带的零依赖默认响应 `{code,message,detail}` 仅限生态外独立使用，**detail 豁免就此撤销**）；**归因键消费方一律引用常量 `aksk.ContextKeyCaller/ContextKeyOperator`（v0.4.1），禁裸字符串**——键名漂移必须编译期可见（2026-09-16 三仓常量化批）；**存量协议兼容层豁免（2026-10-13）**：utils `aksk/compat` 为本条登记在册的唯一例外（承接存量 `x-auth-*` 调用方，独立重实现而非自研中间件复辟）——只挂专用路由组、不与 `aksk.GinMiddleware` 混挂，存量调用方迁移完成后整包删除（[02-auth §存量迁移翻案追记](./phase1/02-auth.md)）；
12. **字段格式补充（2026-10-01 四仓联合验证批）**：① **date-only 业务字段传输 `YYYY-MM-DD`**（如工单自定义字段 date——后端 `time.Parse("2006-01-02")` 硬校验），不适用第 8 条 RFC3339；发送侧须按字面格式产出（前端曾以 datetime 控件直发 ISO 时间恒 400，实测教训入册）；② **ID 数组元素与标量同规**——发送侧一律字符串（第 7 条）；`jsonutil.Int64Slice` 双形态兼容仅为**收方宽容**，不构成发送 number 的许可（存量 `.map(Number)` 属违反本公约的欠账）。
13. **500/内部错误必须留痕（2026-10-12 首例驱动：首页统计卡 `GET /tickets` 瞬时 500 事后无迹可查）**——① 未分类错误（非业务码）由 zhuzhao `writeServiceError` 中枢统一落 ERROR（`request_id/path/err` 明细，service error without http mapping 同规格）；② 服务源头**禁止吞错不落直接返回 ErrInternal**（存量零容忍：notification list 为首例补修；新增即审计项）；③ panic 不占本条——既有 Recovery 中间件口径（slog 堆栈 + PanicSink 指纹聚合落库 + 指标计数）；④ 前端/反代错误体归口各端点（blob 错误体解析属前端请求层职责，zhuzhao-ui 请求层已实现）。
14. **JSON 序列化形态公约（2026-10-14 三仓契约审计批立约；审计明细随 zhuzhao deliverables）**——① **同概念唯一 wire 形态**：同一业务概念在列表/详情/请求/响应各面编码必须一致（反面在册：taskrunner params 转义字符串 vs JobView 原生对象双形态——升级为 ui 协同决策项收敛）；② **「无值」按语义分组单表达**：关系指针类（`parent_id` 等文档拍板形态）出 `null`，回填/enrich 类（`*_name/*_no/last_*`）值类型 + `omitempty`（空即字段消失）——同组字段禁混两形态（反面在册：Ticket 回填四字段、Role.ParentID 偏离方已修）；③ **bool 恒出、数组恒不 null**：`false` 不得被 omitempty 吞（反面：Field.Sensitive 已修），空数组出 `[]`（`jsonutil.Int64Slice` nil 亦编 `[]`）；④ **零值有语义的数值禁 omitempty**（attempts/count/duration 类，反面：死信载荷 Attempts 已修）；⑤ **写响应回显规范化后的持久值**（Create/PATCH 响应与后续 GET 必一致，反面：relation_type 已修）；⑥ **PATCH 显式设计 null 语义**——指针/RawMessage 字段对显式 `null` 必须拒绝或明示忽略，禁直通落库（反面：taskrunner params 已修为 400）；⑦ 出站 int64 ID 恒 `,string`/`Int64Slice`（含分页游标/回显数组，反面：after_id、role_ids 已修）。**新增跨 HTTP 字段先对表本条**。

## 4. 工程结构与代码组织（所有服务同规格）

| 项 | 约定 |
|---|---|
| 分层 | handler → service → repository（handler 禁 DB、service 禁 HTTP、repository 禁外层依赖）。**判定口径（2026-10-15 审计成文）**：禁令禁的是能力越界（handler 触 DB 连接、service 发 HTTP、repository 依赖上层）——非越界用途的 stdlib 类型引用（如 `http.MaxBytesError` 判型、`sql.NullTime` 扫描）不判违；handler 引 repository 查询 filter 结构体属灰区，以 `architecture_test.go` 白名单制管理 |
| 目录结构 | 各服务同构骨架：`cmd/<server>/` 入口、`internal/app/`（wire 装配）、`internal/handler|service|repository|model|middleware/`、`internal/pkg/`（服务内公共构件，**升格 zhuzhao-utils 的候选区**）、`configs/ migrations/ scripts/ docs/`；同名目录跨服务职责一致 |
| 依赖注入 | google/wire（装配收敛于 `internal/app`） |
| 配置 | yaml + env BindEnv 覆盖（viper；env 前缀各服务自定：`APP_`/`TASKRUNNER_*`/`ACTIVELIST_*`）；敏感值（SK/密码）env 注入不入库不入 git；`${VAR}` 插值展开为可选增强（activelist 已实现，非强制） |
| **命名约定** | 文件名 snake_case（repository=`<域>_repo.go`、handler=`<域>_handler.go`）；package 名小写单词与目录同名；构造函数 `NewXxx`（包内单主类型可裸 `New`），wire provider `provideXxx`；接口定义放**消费方**（最小接口）+ `var _ Interface = (*Impl)(nil)` 断言；sentinel error 命名 `ErrXxx`、消息带包名前缀小写英文（`"store: job_run not found"`）；常量就近定义于使用包（不设集中 consts 文件），导出 CamelCase / 非导出 camelCase，禁 SCREAMING_SNAKE（2026-09-15 四仓审计将事实惯例升格成文） |
| 优雅启停 | 信号处理 + 依赖关闭顺序；**防孤儿进程**（重启先杀端口占用） |
| 门禁 | 统一 Makefile：`lint`（vet + gofmt）/ `test` / `build`；zhuzhao 另有 `test-integration`（-race -p 1）与 `acceptance` 四档链；**前端仓（zhuzhao-ui）另立六件门禁**（lint/typecheck/test/audit/build/test:e2e——E2E 打真实五栈不用 stub，规格见 phase4/01 号 §7），分仓各跑互不掺和 |
| 健康检查 | **zhuzhao**：`/health/live`（存活）+ `/health/ready`（检 PG/Redis 硬依赖）；**子服务（taskrunner / activelist）**：`/healthz` + `/readyz`（检各自硬依赖：Redis/PG） |
| 时区 | 容器固定 `TZ=Asia/Shanghai` |
| **代码注释** | **关键函数必须注释**：导出符号有 doc comment（说什么）；复杂逻辑/安全相关/非直观分支注释写**为什么**（不复述代码）；对外 API 带 swagger 注记。惯例：中文注释 |
| **文件规模** | 单文件 **超过 200 行即评估拆分**（按职责/子域切，不机械按行数硬切）；新增文件尽量低于该阈值。存量豁免：已冻结模块（ticket）与历史大文件不做突击拆分，触碰时顺手拆 |

## 5. 公共包（zhuzhao-utils）

- 清单：crypto / errcode / jsonutil / jwt / logger / postgres / redis / response / validate / aksk / aksk/compat（**11 包**）；**版本：v0.4.2（当前各仓 pin，2026-10-14 打标）**——v0.2.0 含 aksk、v0.3.0 含 RefreshClaims.Pwe、v0.4.0 验签统一批（AKSKFail/归因收编/Verifier.Logger）、v0.4.1 归因键常量 + AKSKFail 文案修正、**v0.4.2**：aksk/compat 存量协议兼容层（签名+验签+gin 中间件+出站 Transport，独立重实现、零新依赖、与内部 aksk 零耦合，见 [02-auth §存量迁移翻案追记](./phase1/02-auth.md)）+ jsonutil.Int64Slice 补 MarshalJSON（发送侧 ID 数组一律字符串）+ jwt ParseRefreshToken 过期错误 ErrTokenExpired 哨兵 + postgres 移除 MaxIdleConns 口径误映射；各仓 pin 去 replace（临时 replace 仅限联调窗口）；
- **抽取边界：只抽无数据依赖的纯工具**——绑定 zhuzhao 库 schema 的构件（如策略谓词）不进 utils（design-decisions §25.3）；
- 版本策略：语义化版本，发版后各仓 pin（去 replace）；临时 replace 仅限联调窗口；
- **新增第三方依赖必须说明理由与替代方案**（go.mod 铁律）；公共能力优先沉淀进 zhuzhao-utils 而非各仓自引。

## 6. 可观测性

| 项 | 约定 |
|---|---|
| request_id | `X-Request-ID` 头进出全程透传；zhuzhao 生成（`req-`+32hex）→ 透传下游 → 回调带回；与业务 body 内 request_id 同键关联（job_runs / trace_id） |
| 访问日志 | 统一中间件出口，每请求一行，行名统一 `access`；**标准字段**：method/path/query/status/duration_ms/request_id/operator/auth/caller（仅服务间验签场景）/ip；**`auth` 身份平面**（jwt/aksk/none，2026-09-16 归因口径拍板）：operator=谁在操作（username→签名透传 X-Operator→anonymous），auth=以什么形式接入——两维分列避免「未登录人类」与「服务动作」语义混叠，M-UAA 后 auth 可细分为 jwt:local/jwt:sso；**适用范围：仅多身份平面服务（zhuzhao 网关/IAM）需要 `auth`，单平面子服务（taskrunner/activelist 纯 AK/SK）可省略**；`caller` 三仓恒出（无验签场景空串占位，行结构稳定供 ES 索引）；耗时字段统一 `duration_ms`（2026-09-15 统一：zhuzhao 原 `latency`、taskrunner 原 `cost_ms` 已改名）；query 超 4KB 截断；网关对 proxy 路由**跳 body** |
| 审计 | **审计正本全在 zhuzhao**：请求级（audit_logs，脱敏+截断）+ 登录显式审计 + 业务操作点显式发布；服务自身零业务语义日志 |
| 判定日志 | L2/L3 判定落 `policy_evaluation_logs`（B11①，E-① 已实施）；归档超期导出 JSONL 后删行（B11②/E-③，保留期 180 天可配置） |
| 日志框架 | utils `logger`（slog + lumberjack，JSON Lines，字段稳定命名供 ES 演进） |

## 7. 权限架构（SSOT：design-decisions §25；三层详设：authz.md）

1. **PDP/PEP 分工**：身份/角色/组织关系**集中一份**（zhuzhao 库）；行级策略**归属主自治**；行级 PEP 必然在数据处——网关只做认证 + 路由级授权 + 身份断言透传，**不做行级判定**；
2. **三层**：L1 路由级（Casbin，policy=角色×path×method）→ L2 数据范围（策略库 builtin 或手写 Resource，Registry 统一接缝）→ L3 动作矩阵；
3. **平台策略库**（触发驱动）：`org-member`/`owner-only`/`role-gated` 三内置 + `Builtin()` 一行注册；**适用前提 = 资源表在 zhuzhao 库**；跨库资源走参数级组装（E-⑤ 模式）；
4. **Q5 禁令**：L2/L3 判定每请求实时查询，**不缓存**；任何缓存提案必须连同失效级联方案评审；
5. **ReBAC/PBAC 不演进**（触发器 = 11-authz §5 清单；命中时经~~ResourceAuthorizer~~ **resource.Resource/Registry 接缝**（`internal/pkg/resource/registry.go`，Authorize/GetFilter——⚠ 2026-09-22 虚标审计勘误：全仓无 ResourceAuthorizer 类型，实际接缝为此，L1 不动）；
6. 角色自定义全链（角色 CRUD / AssignMenus / org_roles 组织赋角 / BFS 三源展开）为平台本职，所有服务共用。
7. **权限码命名与注册**：码 = `resource:verb`（K8s verb 规范同款，禁复合码）；服务 API 的码↔路由绑定一律迁移申报（声明式，与代码同版本，design-decisions §26.2），运行时只开放菜单/角色绑定管理，**不开放 API 绑定编辑**。
   - 存量例外注记（2026-09-09 对码盘点；2026-09-11 泛化）：**层级子资源三段形（`resource:subresource:verb`）按同口径允许**——适用 `ticket:type:manage`（000018，早于本公约）及 activelist 权限面四码 `activelist:type:read|manage`、`activelist:data:write|read`（000024）——语义为层级子资源而非复合动词，保留不改（改码 = 破坏既有角色绑定）；`role::superadmin` / `role::admin` 为 Casbin 角色名命名空间，非权限码，不在本公约范围。

## 8. 数据与迁移

| 项 | 约定 |
|---|---|
| 迁移文件 | **up/down 成对**；编号**全局唯一**（zhuzhao 仓内按 A2 规则：谁先启动谁占用，后者整体重排）；**各独立服务迁移编号自成体系，与 zhuzhao 无关** |
| 写法建议 | 优先幂等（IF NOT EXISTS / ON CONFLICT / DO $$ 判列）；zhuzhao 集成测试容器按 testutil **显式清单**重执行——**新迁移必须同步登记该清单**，漏登 = 测试容器缺列全包炸 |
| 建表基线 | id 自增 / version 乐观锁 / created_at·updated_at / 软删 deleted_at（唯一索引配 `WHERE deleted_at IS NULL` 部分索引）；业务唯一键语义固定后建部分唯一索引。**taskrunner 豁免在册（2026-10-15 登记）**：schema 代码内嵌无版本化迁移文件（其 docs §10 条件登记——演进频率上升再评估）；job_runs/jobs 为任务域状态机表，无 version/created_at/软删属状态机语义而非业务实体，基线不适用 |
| 事务与并发 | 跨表写**必须同事务**（检查+写入同事务，防 check-then-act 窗口，B4-5）；悲观锁按场景选用（FOR SHARE 防写锁窗口读旧快照 / FOR UPDATE 队列竞争 / advisory lock 串行化长操作，BK-11/B3-2 模式）；**全表带 version 乐观锁**（并发写 → ErrConcurrentModification）；失败路径必须整体回滚不留部分写 |
| 跨库边界 | 服务间**不直连对方库**；身份数据需要时走事件同步副本（契约由数据接收方定义） |

## 9. 测试约定

| 项 | 约定 |
|---|---|
| 三档分层 | **单测**（无 build 标签，plain `testing`，不起容器）/ **集成**（`//go:build integration` + testify + `-race`，共享 PG 容器）/ **acceptance**（bash 黑盒 API 验收，四档链式——与 Go 测试互补：前者测权限语义，后者测端到端流程） |
| 容器与迁移 | 集成测试共享 PG 容器（testutil）+ **显式迁移清单**——新迁移必须同步登记该清单，漏登 = 测试容器缺列全包炸（000019 教训） |
| 数据隔离 | fixture **每跑唯一化**（uniqueSuffix）；清理用 `t.Cleanup`（软删释放唯一索引/删行）；跨测试残留数据 = 隔离债，修 bug **必带回归测试** |
| 负向用例 | 权限类功能**强制三断言**：可见 / 不可见→404 / 越权→403；错误注入（DB 错误→拒绝不留部分写） |
| 隔离复验 | `-count=1` 为门禁基线；`-count=2` 作数据隔离的复验手段（隔离债的探测探针） |
| 测试包选择 | 默认**同包**（可测非导出符号）；对外契约/集成行为验证用 `_test` 外部包；同仓允许两种并存、单文件二选一（2026-09-15 成文，四仓现状即混用） |
| 测试即验收 | 测试 = 可执行的验收标准：写不出测试说明设计未想清（回 §11 设计阶段） |

## 10. 安全基线

| 项 | 约定 |
|---|---|
| 登录 | 失败锁定（Redis Lua，fail-close）；登录事件显式审计（含 method=sso/local） |
| 密码 | 复杂度策略（网关化后=门户责任，待实施 C）；HR 同步账号密码为占位值，SSO 上线后默认禁账密（`auth.local_password_for_hr`） |
| 限流 | API 级限流（网关职责，复用 Redis Lua 基建）；登录限流 fail-close |
| 会话 | AT/RT 双 token；Logout/改密吊销；SSO 后 `source=hr` 禁账密旁路 |
| MFA | 未立项（触发驱动） |

## 11. 开发工作流（先框架 → 先测试 → 分批实现）

**设计原则：对齐业界实践**——实现可以先不完整，**设计思路必须与业界成熟形态一致**。设计评审的必答题是「业界怎么做」：说不清就先调研，有对照再动手（现有对照范例：PDP/PEP = NIST 框架、投递语义 = at-least-once、服务签名 = AWS SigV4 式 HMAC、可见性 = tag-based ABAC、组织树 = ltree；总方针 = **抄模式不抄架构**）。「业界没有先例」多数情况说明问题定义有误。

**标准节奏（2026-09-04 所有者拍板）**：

1. **决策面清零再动工**：动工前把拍板项/语义规则全部关闭并写进设计文档（参照 M-E 的 P1–P7 流程——决策清零后「剩余均为实施项」）；跨仓库的契约变更**先改契约文档、联调冻结前提出**（C11 教训）；
2. **先搭框架，不急着实现**：接口/类型/路由/装配/迁移骨架先立起来（保持编译通过），行为留空或显式返回「未实现」（如 501/`ErrNotImplemented`）——**禁止静默空实现**（半成品被误当完成是最难排查的坑）；
3. **测试先行**：按设计把测试用例**全量**写完（正/负向/边界/权限拒绝路径），先跑出红——测试就是可执行的验收标准，写不出测试说明设计还没想清；
4. **分批实现**：按设计分小批填肉到绿，**每批结束保持全门禁可过、主干可交付**，不留跨批次的半成品；
5. **小步快跑提交**：一个逻辑批次一个 commit（中文提交信息 + 验证证据），**禁止一次提交大段代码**——提交粒度以「可独立 review」为锚点（参照 b4cfe96：16 文件为量级感知上限）。

**配套纪律（本项目实测教训）**：

- 收口/替换类操作用 grep 全量清点对账，**不凭记忆宣告完成**（setupD9 漏网教训）；机械批量替换（sed）后必须全量 grep 残留并逐行核对（多行 VALUES 曾被吞括号）；
- 基础设施登记与代码**同批**：新迁移 → testutil 迁移清单 + 迁移地图；新端点 → 权限码/menu_apis + SSOT 索引；
- 改 API 契约后**重启常驻进程再验收**（acceptance 打 33333 常驻 server，旧进程 = 过期 API 假绿/假红）；
- 门禁未绿不算完成；门禁失败如实报告，不掩饰不跳过；
- 完成（DoD）= 门禁绿 + 文档回填（SSOT/变更记录）+ 变更评审说明三节。

## 12. 工程与协作纪律

1. **提交**：中文提交、分批提交（一个逻辑批次一个 commit）、**先验收后提交**（全门禁绿才可提交）、push 单独指示；
2. **变更评审说明**：任何 `internal/ migrations/ configs/ cmd/ docs/` 改动附带三节——改动摘要 / 影响面 / 验证证据；未跑门禁必须如实说明；
3. **文档体系规范**：docs 树 = `modules/`（能力文档）/ `phase1-3/`（阶段计划）/ `review/`（评审发现）/ `adr/`（重大架构决策）/ `design/`（设计推演与细节）/ `proposal/`（提案），顺序编号 + README 索引。**分工**：ADR = 重大不可逆架构决策（事件机制/任务执行器/集成形态），design-decisions = 演进中的设计细节拍板（编号顺延不回收），modules = 能力的长期文档。每个 SSOT 文档必须带**变更记录表**；**文档断言必须能与代码对账**（状态以代码为准）；**时点报告**（审计/复检/验证类快照文档）各仓集中存放随仓提交：各仓统一 `deliverables/`，命名 `<topic-kebab>-<YYYY-MM-DD>.md`（对齐主仓既有惯例；2026-10-03 统一）；
4. **编号 namespace**：W（Wave）/ IW（独立窗口）/ BK（backlog）/ E/D（16 号配套）/ P（待拍板）/ F/C/AB（各评审文档本地）——引用前必查归属；
5. **触发条件驱动**：暂缓 ≠ 搁置——未命中的能力不提前实现，触发器写清（冻结的工单模块同此纪律）；
6. **跨仓库契约变更**：先改契约文档（双方仓库同步）→ 再改代码；契约冻结（联调开始）后变更成本陡增。**改 wire 形态（字段类型/omitempty/指针化/序列化行为）五点必查（2026-10-14 契约审计批立约，三例在册：MaxIdleConns 跨仓破坏/A5 镜像 struct/schema.d.ts 三方矛盾）**——① 消费端 Go struct（grep 四仓字段名）② 测试侧镜像解码 struct ③ swagger 表达（数组 ID 字段带 `swaggertype:"array,string"`，CI 有 swagger 新鲜度守卫）④ zhuzhao-ui codegen 触发 ⑤ 文档口径（含「待发/已发」类措辞）；
7. **分支与版本**：功能分支开发（如 `feature/phase-2`），合入前全门禁；zhuzhao-utils 语义化版本，发版后各仓 pin。**维护态分支口径（2026-09-16 拍板，方案 a 后收敛）**：四仓 **main 单主干**——小批改动直接提交 main 推送（CI 即门禁）；隔离需求出现时（如 M-UAA）开短命功能分支，合入 main 后即删（远端+本地），不留阶段名常驻分支。**跨仓 phase 分支对齐（2026-10-19 立约——随 zhuzhao phase5 切分支触发）**：兄弟仓（utils/taskrunner/activelist/**zhuzhao-ui**）**默认不随主仓切同名 phase 分支**——预期改动 ≤1–2 小批时直接 main 直提（本款既有口径）；**跨仓改动必须互引登记**（zhuzhao 批次说明引兄弟仓批次号、反向亦然——防漂移；先例：04 号 §4 引 taskrunner.md 能力目录、9563c66 挂账引 taskrunner CallbackURL 清理）；某 phase 期间兄弟仓预期**多批联动**时，可选切同名分支随主仓收官节奏合并、合后即删（短命原则不变）。utils 发版 = main 打 tag，**零分支动作**；消费仓 pin 升级各一笔。盘点依据（2026-10-19 四仓盘点）：phase5 期间 **zhuzhao-ui = 多批联动档**（S1/S2/S4/S5/S6 五段前端件全落 ui 仓——**切同名 `phase5` 分支**）、taskrunner ≤1–2 小批（CallbackURL 清理/Inspector 端点均挂账触发驱动）、utils/activelist 零改动预期。
8. **CI 红窗纪律（2026-10-14 立约，10 月四例归因驱动）**——main 单主干下 CI 红窗即主干破损：① **push 后验绿再收工**：push 会话必须确认本轮 run 结论（`gh run list` / Actions 页）才算交付闭环——push 不是终点，绿了才是（§12-1「先验收后提交」的自然延伸）；② **红窗禁叠新批**：发现红先归因修复，修复提交信息引用失败 run 并注根因分类（契约未同步 / 门禁漏跑 / 依赖环境 / flaky）——分类积累方能识别真 flaky；③ **同 HEAD 重跑即绿 = 疑似 flaky**：登记后排查，禁止无归因 re-run 当修复；④ **拆笔不破主干**（细化 §11-4）：同批拆多笔时每笔独立可过门禁，类型/契约类双侧变更（类型定义 + 发送侧）禁拆笔；⑤ **跨仓契约变更双仓同窗推送**（§12-6 配套）：生产端先推、消费端后推的窗口期必致消费端红窗，契约批两仓提交齐再一起推。

9. **提交信息格式（2026-10-19 立约，所有者观察驱动——Conventional Commits 1.0 骨架 × 中文适配）**：
   - **结构**：`type(scope): 摘要` 主题行 → **空行** → 正文（分点自由）→ 可选 footer。主题行与正文之间**必须空行**——`git log` 短格式只取主题行；
   - **type 枚举**（小写）：`feat` 新功能 / `fix` 缺陷修复 / `docs` 文档 / `refactor` 重构 / `test` 测试 / `chore` 构建·工具·杂务 / `build` 构建系统·依赖 / `ci` CI / `perf` 性能。不确定时取主要意图，**一笔提交一个 type**；
   - **scope**（可选，小写）：域或模块——`auth` / `ticket` / `storage` / `jobs` / `audit` / `org` / 文档组如 `phase5` / `standards` / `review`。跨域批次取主落点或省略；
   - **主题行长度：中文 ≤40 字**（约英文 72 字符等量——`git log --oneline` 单屏可读）；**摘要只写「改了什么」一句话级**，细节（论据/级联清单/修法对比）全部进正文——主题行是索引不是正文；祈使语气、结尾不加句号；
   - **正文**：分点或分节自由；建议含「为什么改」（一句话）+ 变更要点——与 §12-2 变更评审说明三节的关系=**commit 正文是评审说明的持久化锚点**（回复中的评审说明随会话消逝，commit 不消逝）；**改动摘要多条时编号（①②③…）、单条不编**（2026-10-20 所有者立：三收益——逐条可引用[与批次 B5-1/拍板 S5#1/外审发现 P0-1 的编号生态同构，评审/回归时可直接指「摘要第 N 条」]、完整性可对账[编号缺号一眼暴露漏写，与 git diff 逻辑块数对账]、①式与段稿变更记录内嵌编号同形且 grep 友好）；
   - **push 决策点扫描（2026-10-20 立——发现链第 1 道）**：所有者下 push 指令前必跑 `make commit-lint`（默认扫待推送窗口 origin/<branch>..HEAD；`make commit-lint RANGE=HEAD~20` 扫指定范围），违例先本地 `git rebase -i` reword 再推——**本地历史是可改窗口，push 后才是不可改线**；并行会话的提交随主线 push 连带上远端，故扫描对象是待推送全窗口而非单会话自身提交；已 push 存量不追溯；CI 对推送范围校验为第 2 道兜底（发现及时但已不可改）；动因=格式违例屡次在被后续提交覆盖后才被人工发现（rebase 代价随覆盖深度增长），机械发现点必须前移到唯一的人眼决策时点；
   - **footer**（可选）：`BREAKING CHANGE: <说明>`（破坏性变更必标）/ `批次: P5-B5-1`（phase 批次关联）/ `Refs: <文档锚点>`；
   - **示例**：
     ```
     docs(phase5): S3a 升格排期（W13–15 纳入收官）

     所有者拍「升」：条件段→排期段，响应事件驱动统一模型+phase6 前核心引擎目标。
     - 11 号：Wave 表 W13–15 新行/§2.9 批次细化/台账 S3#4
     - 03 号：头注段属性/演进路线三处翻✅
     - README/review-11 级联五处
     收口口径：一期 12–13 周不变，含二期+引擎 16–17 周。
     ```
   - **存量不追溯**（§3-5 同口径）：既有长主题行提交留痕即史，不重写历史；**本条自立约后提交起生效**。

10. **全面检查类型学（2026-10-19 立约——本会话多轮检查实践沉淀，使检查可复用可比较）**：
   - **三档分层**：① **锚点抽查**（计划断言 vs 代码 grep——分钟级，随问随查）；② **横切一致性扫描**（跨文档口径轴：命名/计数/排期/分支/变更记录宿主引用——半小时级，phase 启动批与里程碑收口各一次）；③ **深读轮**（逐镜头代码审查，镜头清单固化：并发与事务边界/错误处理一致性/输入验证与安全面/迁移完整性/分页与契约/命名/场景覆盖/边界定义/演进平滑——每轮持一镜头，多轮成全面检查）；
   - **可复用检测模式**（已沉淀两条）：变更记录错位行 =「行无宿主结构引用」检测（本文件行应含 §引用，段稿行应含本段章节锚）；级联残留 =「批次→触碰文件映射」对照（`git log --name-only` vs 各文件 changelog 行）；
   - **发现分级与处置时限**：P1 设计缺口 = 对应批次启动批前必须落档 / P2 级联残留 = 随下一文档批 / P3 登记项 = 触发驱动；发现编号标注报告来源（A-1/B-3 式）可回溯；
   - **外审截断规则（辖域分流，2026-10-19 立——双钻复盘产出）**：外审两类勿混辖——①**规划抛光轮**（对已收敛文档的再审计，10-09 收口教训辖域）：开工前声明终止条件，本轮发现全为 P2/P3 或措辞记账级即终轮、剩余只记账不深修——判定锚=发现**性质**（结构性错误 vs 措辞精度）非轮次序数；②**实施期设计核验**（随批外审）：**不适用截断**，按上行分级处置执行（P0 随批修 / P1 随段启动批 / P2·P3 记账）——2026-10-19 P0×4（timeout 三方矛盾/执行端点缺失）即实施期外审高价值实证，截断若混辖必误伤；
   - **触发时机**：phase 启动批 / 里程碑收口 / 所有者问询 / 外部依赖变化（兄弟仓发版/上游契约变更）；
   - **产出归档**：按 §12-3 入 deliverables/ 时点报告；检查结论（含"未发现"）同样留痕；
   - **并行会话收尾互检**（fe720e1/c293f06 两次写冲突实证）：并行会话提交前必须回读自己写入的行仍在（grep 校验本批次锚点词），发现被覆写立即重做并最小窗口提交。

## 13. SSOT 索引

| 主题 | 权威文档 |
|---|---|
| 权限架构（PDP/PEP、网关、策略库、ReBAC 触发器） | design-decisions §25 |
| 软删组织委托语义 / SoD / Phase 3 重定位 / SSO | design-decisions §21 / §20 / §22–24 |
| 服务实例基线（允许差异、taskrunner 对齐清单 C 系列） | phase3/16 号 §9 |
| taskrunner 设计 / API / job_runs | taskrunner 仓库 docs/taskrunner.md |
| activelist 契约 / 存储模型 | activelist 仓库 docs/ADR-003-integration-contract.md |
| 排期（里程碑/人日/主链） | phase3/13 号 §1 |
| 事件机制 / Asynq | ADR-001 / ADR-002 |
| 工单（封版历史设计与对接参考） | phase2/09、phase3/10 |
| 前端工程设计（技术栈/壳层/契约消费面/门禁） | phase4/01 号 |
| 前端协作协议（六件门禁/提交规范/评审三节/文档规范） | zhuzhao-ui 仓 AGENTS.md |
| AI/协作者流程协议（zhuzhao 仓） | AGENTS.md |
| 借鉴/参考项目出处与致谢（新借鉴随批登记纪律） | acknowledgments.md |

---

## 变更记录

| 日期 | 变更 |
|---|---|
| 2026-09-04 | 建档（由 16 号 §9 基线升格扩编而来） |
| 2026-09-15 | 四仓一致性审计批：§3 补错误定义三形态（10）与码值引用纪律（11）+ taskrunner 段预留注记（9）；§4 补命名约定行 + 配置口径修正（yaml+BindEnv 为实况，${VAR} 为可选增强）；§6 补访问日志标准字段（duration_ms 统一）；§9 补测试包选择约定 |
| 2026-09-16 | 服务间验签统一批：§3.11 升格「服务端验签中间件不得各仓自研」——一律 aksk.GinMiddleware + response.AKSKFail()（统一信封+分档中文文案，caller/operator 归因键收编进 GinMiddleware，失败现场落 Verifier.Logger），aksk detail 豁免撤销；utils v0.4.0 发布 |
| 2026-09-16 | 归因口径拍板（选项 4）：§6 访问日志新增 `auth` 身份平面字段（jwt/aksk/none）与适用范围（仅多平面服务）、caller 三仓恒出（空串占位）——zhuzhao operatorOf 扩链/新增 authOf；同日 §3.11 补归因键常量纪律（ContextKey*，v0.4.1），三仓消费方完成常量化 |
| 2026-09-29 | P4-W5 前置批：§3-5 存量豁免更新——工单 PUT/DELETE 豁免消灭留痕（BK-18 已整改）+taskrunner cancel/retry 动词段豁免注记（纯内部 M2M 语义，规划拍板不返工）+activelist 三端点整改同步（标识入 body，000032） |
| 2026-09-30 | 二轮审计批：92xxx 三段码表（通知/字典/PAT）+httpStatusByCode 映射规范——新码必须三件套同步（errcode.go var + errors_test allErrCodes + errcode.md 含反引号常量列格式）；down 迁移精确逆红线重申（000033 空 down 教训）+roundtrip 快照扩 P4 四业务表 |
| 2026-10-01 | 四仓联合验证批：适用范围+§1 增 zhuzhao-ui 行（前端=契约消费方）+工单封版状态注记（基础域已交付被消费——§12-3 断言对账）；§3 补第 12 条（date-only `YYYY-MM-DD` 例外/ID 数组元素仍 string）；§4 门禁行补前端六件口径；§12-3 补时点报告存放命名（deliverables/·outputs/）；§13 SSOT 索引补前端两行 |
| 2026-10-03 | 时点报告命名升三段式 `<类型>-<范围>-<日期>`（同日多份无区分位——随 zhuzhao-ui outputs 四报告统一批同步）； |
| 2026-10-12 | 500 留痕纪律：§3 增第 13 条——未分类错误中枢落 ERROR（writeServiceError，request_id/path/err）+服务源头禁吞错（notification list 首例补修）；panic 既有 Recovery+PanicSink 口径注记。驱动=首页统计卡瞬时 500 事后无迹可查 |
| 2026-10-13 | 存量协议兼容层批：§3-11 验签条款增 `aksk/compat` 唯一登记豁免（只挂专用路由组、迁移后整包删除——02-auth §存量迁移翻案追记）；§5 utils 清单 10→11 包（aksk/compat，v0.4.2 待发）。驱动=存量 `x-auth-*` 调用方兼容口子（capabilities 对账见 compat 包注释） |
| 2026-10-13 | 借鉴出处纪律建档：§13 SSOT 索引增 acknowledgments.md 行——借鉴/参考项目集中致谢与溯源，新借鉴随批登记（借设计不借代码/无 LICENSE 禁拷码/公司内部参考不入页） |
| 2026-10-14 | JSON 契约审计批：§3 增第 14 条「JSON 序列化形态公约」（同概念同形态/无值分组单表达/bool 恒出数组不 null/零值语义数值禁 omitempty/写响应回显规范化/PATCH null 语义/出站 ID 恒 string）——三仓审计驱动（zhuzhao/activelist/taskrunner + utils Int64Slice MarshalJSON 补齐），修复随各仓同日提交，审计明细见 deliverables/json-contract-audit-2026-10-14.md |
| 2026-10-14 | 审计循环机制化收口：§12-6 跨仓契约变更增「wire 形态五点必查」清单（MaxIdleConns 跨仓破坏/A5 镜像 struct/schema.d.ts 三方矛盾三例驱动）；zhuzhao ci.yml 增 swagger 新鲜度守卫（make swag 同款命令 + git diff --exit-code，swag v1.16.4 钉版）——/compat/echo 注解漏生成首例驱动 |
| 2026-10-15 | 全仓公约审计批：§3-5 补 zhuzhao tasks cancel/retry 镜像段登记；§3-6 立 activelist keyset 游标特例（百万行 offset 不可用，ui cursorPager 消费中，新端点勿援引）；§4 分层禁令判定口径成文（stdlib 类型引用不判违/handler 引 filter 结构体走架构测试白名单）；§8 建 taskrunner 基线豁免登记（schema 内嵌+状态机表）；§12-8 立 CI 红窗纪律五点（10-14 四例 CI 失败归因驱动） |
| 2026-10-15 | 清欠批：§3-5 补 activelist 数据域 `/:id/update`·`/:id/delete` URL 标识+动词段豁免登记（存量形态不追溯）——至此字面违约仅剩在册特例三类；utils BK-25 六件 main 直提清零（tag 不动随 v0.4.3 收编，详见 review/11 §8 BK-25 行） |
| 2026-10-15 | §3-6 keyset 特例注记补 git 考古时间线锚点（`bc60e26`：设计 09-03/建纲 09-07/禁令扩写 09-09/实现 09-08 四提交备查）+ 建档日期口径注（头注 09-04 与落库 09-07 并存说明）——外审回声批补留痕 |
| 2026-10-15 | §3-1 扩写（phase5 启动批所有者拍板）：方法公约补**立法理由**（仅 GET/POST 的 why——语义由路径+权限码承载/HTTP 层读写不变量/方法面变更=破坏性契约/内部平台无自描述收益）+ **第三方协议方法辖域界定**（不属辖域、不算违约、无需豁免 + 隔离/登记/禁反向援引三护栏）——首批应用例 = phase5 S5 预签名直传（05 号 §3/§4） |
| 2026-10-19 | §12-9 立「提交信息格式」公约（所有者观察驱动）：Conventional Commits 1.0 骨架×中文适配——type(scope) 枚举/**主题行中文≤40 字**（oneline 可读性——既有提交主题行动辄二三百字全塞一行不可读）/空行分正文/正文=评审说明持久化锚点/footer（BREAKING CHANGE/批次号）；存量不追溯自立约起生效 |
| 2026-10-19 | §12-7 扩**跨仓 phase 分支对齐**（四仓盘点触发——zhuzhao phase5 切分支后兄弟仓对齐问题成文）：兄弟仓默认不随主仓切同名 phase 分支（预期 ≤1–2 小批=main 直提，本款既有口径）+ 跨仓改动**互引登记强制**（zhuzhao 批次说明引兄弟仓批次号、反向亦然——防漂移）+ 多批联动可选同名分支随主仓收官合并合后即删；utils 发版=main 打 tag 零分支。盘点分档：**zhuzhao-ui=多批联动档[切同名 phase5 分支]**（S1/S2/S4/S5/S6 五段前端件）、taskrunner ≤1–2 小批[CallbackURL 清理/Inspector 均挂账触发驱动]、utils/activelist 零改动预期 |
| 2026-10-19 | §12-10 立**全面检查类型学**（多轮检查实践沉淀——锚点抽查/横切一致性扫描/深读轮三档分层 + 可复用检测模式两条[变更记录错位行检测/级联残留映射对照] + 发现分级 P1/P2/P3 处置时限 + 触发时机 + 产出归档 deliverables + **并行会话收尾互检**[写冲突两次实证]） |
| 2026-10-19 | **宿主无关变更记录行清理批**：移除 2026-10-12 两行 zhuzhao 迁移批次日志（000041/000042 组织树读权限——纯仓内迁移+casbin 叙事，与本文件条款零关系、系并行会话误追加宿主）——正文权威记录在 review/11 §4 迁移地图与 10-12 文档批次注，零信息损失。检测模式=变更记录行**无宿主结构引用**（本文件行应含 §引用）；全仓+兄弟仓（taskrunner/activelist/utils）同模式扫描仅此 2 行 |
| 2026-10-19 | §12-10 增**外审截断规则（辖域分流）**——规划抛光轮（10-09 教训辖域）开工前声明终止条件：发现全 P2/P3 或措辞记账级即终轮只记账（判定锚=发现性质非轮次）；实施期设计核验**不适用截断**走既有分级处置（P0×4 反例实证防误伤）。出处=双钻模型复盘（用户流程×Design Council 四阶段对表） |
| 2026-10-20 | §12-9 正文条款增**改动摘要编号约定**（所有者提议）：改动摘要多条用 ①②③ 编号、单条不编——逐条可引用（与批次/拍板/外审发现编号生态同构）+ 完整性可对账（缺号即暴露漏写，与 diff 逻辑块对账）；原「批量大时可列编号清单」软建议升格硬约定；存量不追溯 |
| 2026-10-20 | §12-9 增**push 决策点扫描**（`make commit-lint`，发现链第 1 道）：push 指令前必跑（默认待推送窗口），违例先本地 rebase reword 再推——本地历史可改窗口内消灭格式违例；CI 推送范围校验为第 2 道兜底。动因=格式违例屡次被后续提交覆盖后才人工发现（scripts/commit-lint.sh 随批落地） |
