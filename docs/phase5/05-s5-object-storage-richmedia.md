# 05 — P5-S5 对象存储底座与富媒体：MinIO（设计稿）

> **段属性**：基线段（phase5 收口对象）。**建议实施序**：**第一棒（底座先行）**——S1 富文本调色板、S6 以后各富媒体消费件的依赖；顺序约束：底座 → 头像 → 附件 → 富文本。
> **量级**：1.5–2 周（底座 1–2 天 → 头像 0.5–1 天 → 附件 2–3 天 → 富文本 3–5 天）+ 卷备份随行 0.5–1 天。**迁移**：有（`file_objects`/`ticket_attachments` 两表族；编号启动时按 [review/11 §4](../review/11-project-control.md) 对账，**不写死下一编号**，A2 规则）。
> **文档定位**：段级设计稿（2026-10-15 建档，phase5 分支）。**底座设计 SSOT = [phase2/10-storage.md](../phase2/10-storage.md)**（IW2 独立窗口收编，本段为其**实施载体**——底座只做实施不另设计；富文本字段为新增设计，不在 10 号范围）。立项 = dd §27。

## 1. 定位与由来

全仓无对象存储/附件能力；个人头像 = URL 字符串手填；工单字段七类型无富文本/文件。头像、富文本、附件三类富媒体诉求**共用同一存储缺口**——MinIO 作公共底座收编实施（2026-10-08 所有者指示，dd §27）。phase6 弱依赖（风险证据附件，未交付则降级外链）。

## 2. 现状基线（代码级锚点，2026-10-15 核实）

| 项 | 现状 | 锚点 |
|----|------|------|
| 对象存储 | **全仓零实现**（file_objects/ticket_attachments/storage 模块均无，2026-10-15 grep 核实） | — |
| 头像 | URL 字符串手填（patch 更新，max=500） | `internal/model/user.go:18`（Avatar 列）+ `internal/model/user_request.go:71`（`UpdateProfileRequest.Avatar` binding max=500） |
| 字段类型 | 七类型白名单，无 richtext | `internal/service/ticket/type_admin.go:24` |
| 错误码段 | 91000–91999 存储段**未占用**（[phase2/00 §9](../phase2/00-implementation-plan.md) 既有登记随批落） | docs/phase2/00 §9 |
| compose | 无 MinIO 服务（部署时增，或 config 指向已有 S3） | deployments/compose |

## 3. 底座（10 号蓝本实施——要点转录，全文以 10 号为准）

- **S3 兼容客户端**：开发 MinIO / 生产云 OSS 仅**配置切换**非代码分支；SDK 选型预置 **minio-go v7**（官方维护、预签名 API 齐全）。
- **预签名上传/下载**：前端直传，后端不扛文件流；上传流程 = 预签名 → 前端直传对象存储 → `attachments/confirm` 写元数据（服务端验证对象存在与 size）。**协议口径注（2026-10-15 公约对表，standards §3-1）**：直传环节的上行（PUT）与服务端对象探测（HEAD）均为 **S3/MinIO 对象存储协议层动作**——第三方存储既有协议、不经平台 API 面，不在「方法仅 GET/POST」公约辖域；**平台自有 API 全部 GET/POST**（presign upload/download + attachments confirm/list/delete 五端点，10 号 §4 即此形态）。
- **配置面**（10 号 §2 样例为准）：`storage:` 段（endpoint/region/bucket/AK/SK/use_ssl/上下行 TTL/`max_upload_bytes: 10MB`/`allowed_mime_prefixes`）——endpoint/凭据 config 化禁硬编码（README §6 总纪律③）。
- **对象 key 规范**：`{domain}/{yyyy}/{mm}/{uuid}.{ext}`；**禁止客户端自选路径**（随机 UUID 防穿越）。
- **桶与 key 布局展开（2026-10-15 所有者问询，蓝本骨架细化；现成例子 = 10 号 §2 示例）**：
  - **单桶 + 域前缀**（10 号示例即此形态）——bucket 全局一个（config `bucket`），域前缀区分用途：`avatars/`、`tickets/`（附件与 richtext 内嵌图同域，purpose 区分）；生产云 OSS 同构切换。**多桶登记不做**：当前量级下 IAM/策略复杂度无收益。
  - **桶策略全私有**：零公共读（含头像——下载走拍板点 1/2 结论）；访问仅两条通路 = 预签名直传（上行）+ 代理/预签名（下行）。
  - **key 细节**：uuid4 **服务端生成**（每次预签发新 key，旧 key 不可再签——天然防覆盖）；`ext` 从签名钉死的 Content-Type 推导、**不取用户文件名**；原始文件名只落 `file_objects.file_name`（下载回显时 RFC 5987 编码防 header 注入）；**purpose↔key 前缀一致性校验**（avatar purpose 必须 `avatars/` 前缀，confirm 复核）。
  - **桶级版本化/ILM 生命周期**：登记不做（触发驱动）——DB 元数据 + CASCADE/GC 已覆盖删除语义，桶版本化反增恢复复杂度。
  - **孤儿对象**（预签后未 confirm）：随批轻量 GC（cron 扫 N 天未确认对象删除）或登记触发驱动——随启动批拍（10 号"GC 后移"口径维持）。
- **上传限制矩阵展开（2026-10-15 所有者问询；建议值 = 启动批确认制）**：

  | 维度 | 建议基线 | 执行点 |
  |------|---------|--------|
  | 大小 | 全局 10MB（10 号既定）；per-purpose 差异化可选（avatar ≤5MB / 附件 ≤10MB / richtext 内嵌图 ≤5MB） | presign 声明值预检 + **confirm HEAD 实测 content-length 比对**（直传链路后端不见字节，confirm 是尺寸防谎报的唯一强制点） |
  | 个数 | 单工单附件 ≤20；单次批量 presign ≤5；richtext 内嵌图软限（字段 schema meta 可配） | confirm 计数校验 |
  | 类型 | **显式枚举白名单**（image/jpeg、image/png、image/webp、application/pdf、text/plain…）+ **显式黑名单 `text/html`、`image/svg+xml`**（存储型 XSS 载体）；**presign 钉 Content-Type 入签名**（客户端改类型即签名失效——直传链路类型防伪主力）；扩展名仅展示不参与判定 | presign 签名约束 + 白/黑名单校验 |
  | 下载侧兜底 | `X-Content-Type-Options: nosniff` 恒带；`Content-Disposition` 分型——白名单 image/* **inline**（richtext 内嵌图渲染需要）+ 其余 **attachment**；文件名 RFC 5987 编码 | 代理端点响应头 |
  | 频次/滥用 | presign/confirm 挂既有 rate_limit 桶；per-前缀 body 上限维持 16 号口径 | 中间件 |
  | 配额 | per-user 存储配额 | 登记不做（触发驱动） |

  **蓝本盲区两条（本次展开修正）**：① 10 号 `allowed_mime_prefixes` 含 `text/`——前缀白名单会放进 `text/html`（XSS 载体），改显式枚举 + 黑名单；② 直传链路后端不见字节，类型防伪不靠服务端嗅探，靠「presign 钉 Content-Type + confirm 实测尺寸 + 下载侧 nosniff/分型 disposition」三件组合。

  **业界对表注（2026-10-15 核验：OWASP File Upload Cheat Sheet + AWS presigned POST 模式）**：① **presigned POST + policy 为业界标准强制点**——policy 条件含 `content-length-range`（尺寸在**存储侧**即被拦，强于 PUT + confirm 事后比对）、Content-Type 钉死、key 前缀约束；实现形态随批拍（POST policy 全家桶 vs PUT+签名钉 Content-Type+confirm 实测——功能等价，前者更强）。② OWASP 纪律对齐：扩展名 **allowlist 而非黑名单**（本矩阵白名单已隐含）、不信任客户端声明 MIME（已对齐）、随机文件名（已对齐）、**SVG 按 HTML 处理**（已黑名单）。③ 病毒扫描 / 独立域服务（cookie 隔离）：内网边界登记不做（10 号不做清单维持）。

- **限制值配置化（2026-10-15 所有者提出：admin/superadmin 前端可改——拍板点 7，建议开放·分层形态）**：阈值/清单 = 事实进 DB，强制机制/硬底线 = 语义进代码（§25 分界同构）：
  - **三层结构**：① config 文件持出厂默认（boot 兜底，总纪律③ config 化）→ ② `system_settings` KV 表存运行时覆盖（通用 KV：key/value JSONB + updated_by/updated_at——存储限制为首个消费者，**审计保留期管理端点（review/11 登记欠账）为已登记的下一个消费者**，复用同一面免二次迁移）→ ③ 管理面 UI（admin/superadmin）改 DB 层。
  - **可配**：大小上限（全局 + per-purpose）、个数上限、MIME 白名单（**只能在代码级安全目录内收放**）、孤儿 GC 天数。
  - **不可配（代码硬底线）**：`text/html`/`image/svg+xml` 硬拒（配置不可白名单化）、钳制天花板（如 size ≤100MB、个数 ≤100——DB 值越界服务端拒收）、限流不可关。
  - **护栏**：管理动作挂系统管理权限族（权限码随批对表）+ 变更审计（谁/何时/旧值→新值）+ 服务端钳制校验（非法值拒收非静默修正）。
  - **按 key 编辑层级（2026-10-15 所有者问询"superadmin 改的 admin 能否覆盖"——业界模式对表后采纳静态分层）**：每个配置 key 在**代码目录里登记最低编辑层级**（`min_editor_tier`: superadmin | admin），服务端按层级放行 PUT——**存储限制类 key 默认 superadmin-only**（admin 无写路径，"覆盖"结构性不存在；UI 对低层级只读展示"需超级管理员"）。业界锚点：Google Workspace「锁定设置」（超管锁定后下级置灰不可改）/ GitLab 实例默认值 + enforce 锁（下级不可覆盖）/ GPO「强制·禁止覆盖」/ Atlassian System Admin vs Admin 两级——**GitLab 演进注**：其近年部分硬锁回退为级联默认（硬锁维护成本实证），支持我们取静态分层而非运行时动态锁。**动态锁定**（超管运行时按 key 上锁/解锁+锁管理 UI）**登记触发驱动**（真实场景如多团队委托出现再升级）。
  - 量级 +0.5–1 天（KV 表一枚迁移 + GET/PUT 两端点 + 小管理页 + 钳制/审计/层级测试）。
- **表族**（10 号 §3 DDL 为准）：`file_objects`（bucket/object_key/file_name/mime_type/size_bytes/created_by，UNIQUE(bucket,object_key)）+ `ticket_attachments`（ticket_id FK CASCADE + file_object_id FK + uploaded_by，UNIQUE(ticket_id,file_object_id)）。
- **API 面**（10 号 §4 五端点为准）：presign upload/download + attachments confirm/list/delete；confirm 校验 `created_by == 当前用户` 且对象未绑定。
- **错误码**：91001 FileTooLarge / 91002 FileTypeNotAllowed / 91003 FileNotFound / 91004 AlreadyBound（区间 91000–91999）。
- **删除语义**（10 号 §9 既定建议）：删附件仅删关联，对象 GC 后移（GC Job 触发驱动）。
- **compose 增 MinIO 服务**；迁移编号启动对账（A2）。

## 4. 头像上传（自服务面）

- profile 头像由 URL 手填改**预签名直传 + 回写对象引用**；上传组件 + 即时预览（`purpose: avatar`，10 号 §4.3 契约）。
- **下载形态三选一（启动拍板点，README §2 选型表转推荐）**：
  1. **后端代理下载端点（`GET /files/{key}` + 鉴权中间件）——推荐**：稳定 URL（img src 非时效链）+ 权限收口（登录态 + L2 管线）；
  2. 预签名 GET——有时效，img src 非稳定，过期裂图；
  3. 公共读桶——暴露面大，不取。

  **业界模式对表（2026-10-15 核验）**：业界三种形态——①纯预签名（带宽卸载；但授权只在**签发时刻**、下载不过应用 = 无逐次审计/RBAC）；②后端代理（逐次 RBAC + 审计，吃带宽）；③**授权后短时效预签发 + 302 重定向**（业界折中：每次下载过鉴权留痕，签发 60s–5min TTL 预签名跳存储）。**内网带宽充裕 + L2 逐次判定 + 审计诉求 → ②为主流答案**；③登记为未来带宽压力时的升级选项，不预做。

  **升级成本评估（2026-10-15，拍代理前问询"以后改 302 改动大吗"）**：**很小，≤0.5 天**——两种形态的架构完全同构（同一端点/鉴权中间件/L2 判定/审计埋点），差异仅在最后一投递段：代理 = 后端拉流转写；302 = `PresignedGet`（60s TTL）+ 302 重定向。浏览器对 `<img>`/下载自动跟随 302，**前端零改动**；响应头（nosniff/disposition）经签名参数（`response-content-*`）随预签名携带。实现要求：handler 按**投递策略缝**写（delivery 策略可替换），升级时只换一个函数。

## 5. 工单附件（文档上传）

- 10 号 `ticket_attachments` 蓝本实施（§3 API 面）+ 详情页展示/下载。
- **权限码（✅ 已拍 2026-10-15：复用 `ticket:update` 零新码**——所有者「先不需要单独权限」；上传/删除挂 update，下载随工单读权限走 L2）。
- **下载形态须与头像同批联拍（2026-10-09 隐藏需求核查补遗）**：10 号原文"预签名 GET 直下"存在 **L2 判定被绕过**（时效内任何持有者可下载，URL 分享即越权）且无下载审计——与"PEP 在数据处"哲学冲突、与头像拍板不一致。推荐：**统一代理端点**，或"代理为主 + 预签名仅内网短时效"并显式登记风险接受。**与 §4 头像下载形态同一决策、一次拍完，防两处结论漂移**（F5.3 联拍纪律）。

## 6. 富文本字段类型（richtext，第八种——新增设计）

- 字段类型枚举扩 `richtext`（`validateFieldInputs`/G2 schema 校验同步扩，守"只增不改"）；与 S1 设计器联动（进可选字段类型调色板，依赖本段底座先行）。
- **编辑器形态：Markdown vs WYSIWYG（2026-10-15 所有者倾向 MD，设计复核支持——启动批正式拍）**：
  - **推荐 Markdown**：① 工单正文域（描述/复现步骤/结论）排版诉求 = 标题/列表/代码块/表格/图片，MD 全覆盖；② 目标用户 = 内网技术/安全团队，MD 上手零门槛；③ **存储态 = 纯文本**——体积小、可直接 ILIKE 检索、可 diff，HTML 存储则检索带标签噪声；④ **消毒面大幅缩小**——MD 渲染默认转义内嵌 HTML，渲染器白名单即消毒，vs WYSIWYG 存 HTML 需维护 allowlist sanitizer（标签/属性/URL scheme/style 全量）。
  - WYSIWYG（Tiptap）降为对照项：非技术岗大量写单场景出现再评估；字段类型不堵死，但**存储格式本批拍定后切换有迁移成本，宁窄勿宽起步**。
  - 选型候选（启动批 spike 半天）：Vue3 生态 MD 编辑器（md-editor-rt / bytemd / milkdown 择一），**核心验证点 = 粘贴图片直传扩展**。
- **图片粘贴直传链路（2026-10-15 所有者问询确认）**：编辑器 paste/drop 事件 → Blob → 复用 §3 预签名通道（purpose 归工单域）→ 直传 → confirm → 编辑器插入图片链接——**图片存对象存储**，与附件同受 10MB/MIME 白名单约束；**不引入 base64 内联**（防库膨胀，既定纪律）。
- **内嵌图片 src 硬约束：稳定 URL**——编辑器内图片若用时效预签名链接，历史工单图片**必然过期裂图**；内嵌图片 src 必须用后端代理端点（稳定 URL + 鉴权）。本条把 §4/§5「下载形态联拍」的代理端点推荐从"头像友好"升级为**编辑器内嵌图片的硬约束**（联拍时一并定）。
- **正文渲染/消毒 SSOT 不变**：服务端渲染为 SSOT——MD 方案下 = MD 源存储 + **服务端白名单渲染**出安全 HTML 供回显（前端不自行渲染用户输入）；防存储型 XSS 口径维持，实现较 HTML allowlist 简单。

## 7. MinIO 备份随行件（2026-10-09 登记；2026-10-15 业界方案展开）

PG 有 pgbackup+WAL+PITR 完整体系，MinIO **零备份设计**——附件丢失 = 工单证据链断裂；且对象存储**无 WAL 类事务日志可 ship，PITR 原生不可得**，RPO = 复制/镜像周期（附件场景日级可接受，显式登记不默认）。

**业界方案四类（2026-10-15 核验；均为社区版 mc CLI 能力，不涉控制台裁撤面）**：

| 方案 | 机制 | 优点 | 局限 |
|------|------|------|------|
| **服务端桶复制（`mc replicate`，active-passive）——业界首选** | MinIO 原生异步复制到目标 MinIO | 近实时 RPO；**版本历史/元数据保全**；服务端执行不占业务机 | 需目标 MinIO 实例；仅 MinIO→MinIO |
| **`mc mirror` 定时** | 客户端单向同步，任意 S3 兼容目标（NAS MinIO/Ceph 均可） | 最轻、可断点续传、对齐既有 cron/RETAIN_COUNT 惯例 | **只同步 latest（无版本历史）**；周期性 RPO |
| Batch Replication | 批量复制作业 | 大存量一次性迁移 | 非常态备份机制 |
| 卷快照/停机 tar | PV 级快照 | 完整 | 一致性风险/需停机——**不取为主方案**（可作恢复演练辅助） |

**落地建议（拍板点 5）**：目标 MinIO 实例可得 → **A = 服务端桶复制 + 双端桶版本化**（版本化防误删/勒锁，业界标配）；单机起步期目标实例暂缺 → **B = `mc mirror` 定时（独立卷第二 MinIO 实例）+ 目标桶版本化**起步，登记升级触发（跨主机目标就位/量级上来切 A）。**恢复演练强制随行**（对齐 activelist pgbackup 首验+恢复演练惯例：不演练 = 没备份）；目标侧保留策略沿 RETAIN_COUNT 惯例。生产云 OSS 场景 = 版本化 + 跨区域复制 + 云备份计划（配置切换形态，内网后另议）。量级 +0.5–1 天。

## 8. 启动拍板点

| # | 事项 | 建议 |
|---|------|------|
| 1 | 头像下载形态 | ✅ **已拍（2026-10-15）**：后端代理端点——「授权后短 TTL 预签发+302」升级路径已评估：handler 投递段留策略缝，切换 ≤0.5 天（路由/鉴权/L2/审计全复用，§4） |
| 2 | 附件下载形态 | ✅ **已拍（2026-10-15，与 #1 联拍）**：统一后端代理——预签名形态弃用（L2 绕过+无审计），302 同 #1 升级路径 |
| 3 | 附件权限码 | ✅ **已拍（2026-10-15）**：复用 `ticket:update` 零新码——所有者「先不需要单独权限」 |
| 4 | 富文本编辑器形态 | ✅ **已拍（2026-10-15）**：**Markdown 编辑器**——所有者确认；md-editor-rt / bytemd / milkdown 择一（W0 spike 定） |
| 5 | MinIO 备份 | ✅ **已拍（2026-10-15）：直接做进 S5 必做件**——起步形态 = compose 第二 MinIO 实例（独立卷）+ `mc mirror` 定时 + 目标桶版本化 + **恢复演练一次**；跨主机目标（灾难级）= 部署拓扑升级触发；量级 +0.5–1 天 |
| 6 | 上传限制基线 | ✅ **已拍（2026-10-15 基线认可）**：按 §3 限制矩阵建议值；业界对表补 presigned POST policy 强制点（实现形态随批拍，见 §3 业界对表注） |
| 7 | 限制值配置化开放 | ✅ **已拍（2026-10-15）：归属 S5 段内做**（底座批后随行/段尾，**可切出**——排期吃紧先静态 config 形态交付零返工，配置化切独立小批）；形态 = 开放·分层（config 默认 + `system_settings` KV 运行时覆盖 + 管理面 UI）+ 硬底线不可配 + 按 key 编辑层级（存储限制类 superadmin-only）+ 变更审计/钳制——详录 §3 |

## 9. 边界（不做）

维持 10 号"不做"清单：病毒扫描/内容审核、>100MB 分片上传、CDN 独立域名——均按需另立；云 OSS 生产替换 = 配置切换非代码分支。

## 10. 影响面

- **迁移**：两表族（编号对账）；**权限**：预置零新码（拍板点 3）；**配置**：`storage:` 段新增 + compose MinIO 服务；**错误码**：91000–91999 段启用（91001–91004 四码随批登记 `api/errcode.md`——standards §3-4 对账位）；**端口**：对象存储客户端（S3 兼容抽象即端口，README §6 清单第 7 行）。
- **涉及文件**（10 号 §8 蓝本）：`internal/pkg/storage/`、`internal/service/storage_service.go`、`internal/service/ticket/attachment_service.go`、`internal/handler/storage_handler.go`、compose、迁移两枚。

## 11. 验收口径

- 底座/附件 API 随四档 acceptance（10 号 §7 用例 S1–S6 为底稿：预签名上行/confirm 关联/无权限 403/超限 400+91001/下载时效/头像链路）；前端上传面走 ui CI + E2E。
- 补测：**代理下载鉴权负向**（无 L2 可见性用户拿不到对象——若拍代理形态）；存储型 XSS 负向（恶意 MD 内嵌 HTML/`<script>` 经**服务端渲染**被转义/白名单过滤，前端不自行渲染用户输入）；头像直传回显链路；**粘贴图片直传**（paste → 预签名 → 链接插入 → 回显不裂图）。
- 回归：既有 URL 手填头像兼容读取（存量列不破坏）。

## 12. 文档衔接

| 文档 | 关系 |
|------|------|
| [phase2/10-storage.md](../phase2/10-storage.md) | **底座设计 SSOT**（配置/DDL/API/错误码/用例全文） |
| [review/11 §8 IW2 行](../review/11-project-control.md) | 独立窗口收编注记（本段 = 实施载体） |
| [01-s1](./01-s1-form-designer.md) | richtext 进设计器调色板（依赖本段先行） |
| [README §6](./README.md) | 对象存储客户端端口预留行 |
| [06-s6](./06-s6-message-notification.md) | 无直接依赖；富媒体通知附件留二期评估 |

---

## 变更记录

| 日期 | 变更 |
|---|---|
| 2026-10-15 | 建档（phase5 分支十段设计稿批）——现状锚点逐段代码核实；phase2/10 底座蓝本全量转录；未新增决策 |
| 2026-10-15 | 公约对表批：§3 补直传协议口径注（PUT 上行/HEAD 探测 = S3 协议层动作非平台 API，不经 §3-1 辖域；平台五端点全 GET/POST）+ §10 错误码补登记位 `api/errcode.md`（§3-4） |
| 2026-10-15 | richtext 形态批（所有者问询）：编辑器倾向 Markdown（§6 复核四条收益：存储纯文本可检索/消毒面小/目标用户适配/体积）+ 粘贴图片直传链路落档 + **内嵌图片 src 稳定 URL 硬约束**（反哺拍板点 1/2 代理端点推荐）+ 服务端白名单渲染 SSOT 口径 |
| 2026-10-15 | 存储布局与上传限制展开批（所有者问询）：§3 补桶与 key 布局（单桶+域前缀/全私有/uuid 服务端生成/purpose↔前缀校验/版本化不做/孤儿 GC）+ 上传限制矩阵（大小/个数/类型/下载侧分型/频次/配额）——**修正蓝本盲区两条**（text/ 前缀白名单放行 text/html；直传不见字节的类型防伪改三件组合）；拍板点 +6（限制基线确认） |
| 2026-10-15 | 备份方案展开批（所有者问询）：§7 业界四类方案对比（服务端桶复制/mirror/batch/卷快照）+ 落地建议（A 复制+双端版本化为主 / B mirror 起步+升级触发 / 恢复演练强制随行 / RPO 显式登记）；拍板点 5 更新；phase2/10 补实施收编注 |
| 2026-10-15 | **拍板批 + 业界对表批（所有者）**：#3 已拍（附件权限码复用 `ticket:update`）、#4 已拍（MD 编辑器）、#6 已拍（限制基线认可）；#1/#2 补业界三模式对表（纯预签名/代理/授权后短 TTL 预签发 302——内网+L2+审计语境代理为主流，302 登记为带宽升级选项）；§3 补业界对表注（presigned POST policy 强制点/OWASP allowlist 纪律/SVG 按 HTML）——**剩待拍：#1+#2（联拍）、#5（备份）** |
| 2026-10-15 | 配置化批（所有者提出）：§3 限制值配置化分层形态（config 默认 + `system_settings` KV 运行时覆盖 + 管理面 UI；硬底线不可配；变更审计+钳制）；**拍板点 +7（开放与否确认）**——审计保留期管理端点（登记欠账）登记为同一 KV 面下一消费者；量级 +0.5–1 天 |
| 2026-10-15 | 编辑层级批（所有者问询）：§3 配置化条补**按 key 编辑层级**（代码登记 min_editor_tier——存储限制类 superadmin-only，admin 无写路径；业界锚点 Google Workspace 锁定设置/GitLab enforce 锁/GPO 强制——GitLab 硬锁回退演进支持取静态分层）；动态锁定登记触发驱动 |
| 2026-10-15 | **拍板收口批（所有者）**：✅#1/#2 已拍（统一后端代理——升级 302 成本评估 ≤0.5 天、handler 投递策略缝，§4）；#5 建议直接做进 S5（必做件：mirror 起步+版本化+演练，待确认）；#7 建议归属 S5 段内（可切出尾件，待确认）；19 项随段拍板制（所有者拍板：不整体授权，各段启动批逐段确认）——**S5 剩待拍：#5、#7 归属** |
| 2026-10-15 | **S5 拍板清零批（所有者「按建议来」）**：✅#5 已拍（备份直接做进 S5 必做件）、✅#7 已拍（配置化归属 S5 段内可切出）——**S5 七枚拍板全部闭环**；standards §3-1 同日扩写落档（辖域界定+立法理由，phase5/05 号为首批应用例）；W0 启动待所有者人工检查文档后另令 |
