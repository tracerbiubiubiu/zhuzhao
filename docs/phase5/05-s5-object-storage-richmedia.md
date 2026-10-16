# 05 — P5-S5 对象存储底座与富媒体：RustFS（S3 兼容；2026-10-16 后端拍定——原 MinIO，见变更记录）（设计稿）

> **段属性**：基线段（phase5 收口对象）。**建议实施序**：**第一棒（底座先行）**——S1 富文本调色板、S6 以后各富媒体消费件的依赖；顺序约束：底座 → 头像 → 附件 → 富文本。
> **量级**：1.5–2 周（底座 1–2 天 → 头像 0.5–1 天 → 附件 2–3 天 → 富文本 3–5 天）+ 卷备份随行 0.5–1 天。**迁移**：有（`file_objects`/`ticket_attachments` 两表族；编号启动时按 [review/11 §4](../review/11-project-control.md) 对账，**不写死下一编号**，A2 规则）。
> **文档定位**：段级设计稿（2026-10-15 建档，phase5 分支）。**底座设计 SSOT = [phase2/10-storage.md](../phase2/10-storage.md)**（IW2 独立窗口收编，本段为其**实施载体**——底座只做实施不另设计；富文本字段为新增设计，不在 10 号范围）。立项 = dd §27。

## 1. 定位与由来

全仓无对象存储/附件能力；个人头像 = URL 字符串手填；工单字段七类型无富文本/文件。头像、富文本、附件三类富媒体诉求**共用同一存储缺口**——对象存储作公共底座收编实施（2026-10-08 所有者指示，dd §27；**后端 2026-10-16 拍定 RustFS**——原 MinIO：官方已无开源镜像[八源拉取全败实证]且 AGPLv3，RustFS = Apache 2.0 + S3/MinIO 双兼容，镜像已拉已冒烟）。phase6 弱依赖（风险证据附件，未交付则降级外链）。

## 2. 现状基线（代码级锚点，2026-10-15 核实）

| 项 | 现状 | 锚点 |
|----|------|------|
| 对象存储 | **全仓零实现**（file_objects/ticket_attachments/storage 模块均无，2026-10-15 grep 核实） | — |
| 头像 | URL 字符串手填（patch 更新，max=500） | `internal/model/user.go:18`（Avatar 列）+ `internal/model/user_request.go:71`（`UpdateProfileRequest.Avatar` binding max=500） |
| 字段类型 | 七类型白名单，无 richtext | `internal/service/ticket/type_admin.go:24` |
| 错误码段 | 91000–91999 存储段**未占用**（[phase2/00 §9](../phase2/00-implementation-plan.md) 既有登记随批落） | docs/phase2/00 §9 |
| compose | 无 MinIO 服务（部署时增，或 config 指向已有 S3） | deployments/compose |

## 3. 底座（10 号蓝本实施——要点转录，全文以 10 号为准）

- **S3 兼容客户端**：开发 **RustFS**（2026-10-16 拍定）/ 生产云 OSS 仅**配置切换**非代码分支；SDK 选型预置 **minio-go v7**（对任意 S3 兼容端点工作——对 RustFS 的兼容性随 B5-1 首日 spike 实测，备选 aws-sdk-go-v2）。**后端切换依据（2026-10-16 所有者）**：MinIO 官方已无开源镜像（八源拉取全败实证）且 AGPLv3；RustFS = Apache 2.0 + S3/MinIO 双兼容（**实测 2026-10-16**：`/minio/health/live` 200、`RUSTFS_ROOT_*` 兼容 `MINIO_ROOT_*`、未签名请求 403 fail-closed）。
- **预签名上传/下载**：前端直传，后端不扛文件流；上传流程 = 预签名 → 前端直传对象存储 → `attachments/confirm` 写元数据（服务端验证对象存在与 size）。**协议口径注（2026-10-15 公约对表，standards §3-1）**：直传环节的上行（PUT）与服务端对象探测（HEAD）均为 **S3 兼容对象存储协议层动作**（RustFS/MinIO 同构）——第三方存储既有协议、不经平台 API 面，不在「方法仅 GET/POST」公约辖域；**平台自有 API 全部 GET/POST**（presign upload/download + attachments confirm/list/delete 五端点，10 号 §4 即此形态）。
- **配置面**（10 号 §2 样例为准）：`storage:` 段（endpoint/region/bucket/AK/SK/use_ssl/上下行 TTL/`max_upload_bytes: 10MB`/`allowed_mime_prefixes`）——endpoint/凭据 config 化禁硬编码（README §6 总纪律③）。
- **对象 key 规范**：`{domain}/{yyyy}/{mm}/{uuid}.{ext}`；**禁止客户端自选路径**（随机 UUID 防穿越）。
- **桶与 key 布局展开（2026-10-15 所有者问询，蓝本骨架细化；现成例子 = 10 号 §2 示例）**：
  - **单桶 + 域前缀**（10 号示例即此形态）——bucket 全局一个（config `bucket`），域前缀区分用途：`avatars/`、`tickets/`（附件与 richtext 内嵌图同域，purpose 区分）；生产云 OSS 同构切换。**多桶登记不做**：当前量级下 IAM/策略复杂度无收益。
  - **桶策略全私有**：零公共读（含头像——下载走拍板点 1/2 结论）；访问仅两条通路 = 预签名直传（上行）+ 代理/预签名（下行）。
  - **key 细节**：uuid4 **服务端生成**（每次预签发新 key，旧 key 不可再签——天然防覆盖）；`ext` 从签名钉死的 Content-Type 推导、**不取用户文件名**；原始文件名只落 `file_objects.file_name`（下载回显时 RFC 5987 编码防 header 注入）；**purpose↔key 前缀一致性校验**（avatar purpose 必须 `avatars/` 前缀，confirm 复核）。
  - **桶级版本化/ILM 生命周期**：登记不做（触发驱动）——DB 元数据 + CASCADE/GC 已覆盖删除语义，桶版本化反增恢复复杂度。
  - **孤儿对象**（预签后未 confirm）：**已定基线（2026-10-16 外审对齐——附录 A.3 修正②/A.9 按基线设计，正文统一）**：B5-1 随批轻量 GC（cron 扫 pending + `orphan_gc_days=7` KV 可调）；10 号"GC 后移"口径就此收编。
- **上传限制矩阵展开（2026-10-15 所有者问询；建议值 = 启动批确认制）**：

  | 维度 | 建议基线 | 执行点 |
  |------|---------|--------|
  | 大小 | 全局 10MB（10 号既定）；per-purpose 差异化可选（avatar ≤5MB / 附件 ≤10MB / richtext 内嵌图 ≤5MB） | presign 声明值预检 + **confirm HEAD 实测 content-length 比对**（直传链路后端不见字节，confirm 是尺寸防谎报的唯一强制点） |
  | 个数 | 单工单附件 ≤20；**单次批量 presign ≤20（2026-10-16 所有者问询上调：与单工单上限对齐——batch 低于总限额无额外收益只逼分轮选，防滥用主手段=rate_limit 桶非此值；presign 接口本身单文件设计，batch 框的是前端一轮批量上传）**；richtext 内嵌图软限（字段 schema meta 可配） | confirm 计数校验 |
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
  - **按 key 编辑层级（2026-10-15 所有者问询"superadmin 改的 admin 能否覆盖"——业界模式对表后采纳静态分层）**：每个配置 key 在**代码目录里登记最低编辑层级**（`min_editor_tier`: superadmin | admin），服务端按层级放行**写操作**（POST update）——**存储限制类 key 默认 superadmin-only**（admin 无写路径，"覆盖"结构性不存在；UI 对低层级只读展示"需超级管理员"）。业界锚点：Google Workspace「锁定设置」（超管锁定后下级置灰不可改）/ GitLab 实例默认值 + enforce 锁（下级不可覆盖）/ GPO「强制·禁止覆盖」/ Atlassian System Admin vs Admin 两级——**GitLab 演进注**：其近年部分硬锁回退为级联默认（硬锁维护成本实证），支持我们取静态分层而非运行时动态锁。**动态锁定**（超管运行时按 key 上锁/解锁+锁管理 UI）**登记触发驱动**（真实场景如多团队委托出现再升级）。
  - 量级 +0.5–1 天（KV 表一枚迁移 + GET/POST 两端点 + 小管理页 + 钳制/审计/层级测试）。**切出口径（2026-10-16 外审对齐）**：切出时静态 config 形态先行交付，配置化以独立小批收口——**仍计入 S5 段/一期收口**，不另立段。
- **表族**（10 号 §3 DDL 为准）：`file_objects`（bucket/object_key/file_name/mime_type/size_bytes/created_by，UNIQUE(bucket,object_key)）+ `ticket_attachments`（ticket_id FK CASCADE + file_object_id FK + uploaded_by，UNIQUE(ticket_id,file_object_id)）。
- **API 面**（10 号 §4 五端点为准）：presign upload/download + attachments confirm/list/delete——**附录 A.4 全景另含代理下载与 settings KV 两枚（2026-10-16 外审对齐措辞）**；confirm 校验 `created_by == 当前用户` 且对象未绑定。
- **错误码**：91001 FileTooLarge / 91002 FileTypeNotAllowed / 91003 FileNotFound / 91004 AlreadyBound（区间 91000–91999）。
- **删除语义**（10 号 §9 既定建议）：删附件仅删关联，对象 GC 后移（GC Job 触发驱动）。
- **compose 增对象存储服务（RustFS + rustfs-backup 双实例草案已落 deployments 两份 compose——profile=storage 隔离默认不起，2026-10-16 工程预备批；起容器冒烟通过）**；迁移编号启动对账（A2）。

## 4. 头像上传（自服务面）

- profile 头像由 URL 手填改**预签名直传 + 回写对象引用**；上传组件 + 即时预览（`purpose: avatar`，10 号 §4.3 契约）。
- **下载形态三选一（启动拍板点，README §2 选型表转推荐）**：
  1. **后端代理下载端点（`GET /files/{key}` + 鉴权中间件）——推荐**：稳定 URL（img src 非时效链）+ 权限收口（登录态 + L2 管线）；
  2. 预签名 GET——有时效，img src 非稳定，过期裂图；
  3. 公共读桶——暴露面大，不取。

  **业界模式对表（2026-10-15 核验）**：业界三种形态——①纯预签名（带宽卸载；但授权只在**签发时刻**、下载不过应用 = 无逐次审计/RBAC）；②后端代理（逐次 RBAC + 审计，吃带宽）；③**授权后短时效预签发 + 302 重定向**（业界折中：每次下载过鉴权留痕，签发 60s–5min TTL 预签名跳存储）。**内网带宽充裕 + L2 逐次判定 + 审计诉求 → ②为主流答案**；③登记为未来带宽压力时的升级选项，不预做。

  **升级成本评估（2026-10-15，拍代理前问询"以后改 302 改动大吗"）**：**很小，≤0.5 天**——两种形态的架构完全同构（同一端点/鉴权中间件/L2 判定/审计埋点），差异仅在最后一投递段：代理 = 后端拉流转写；302 = `PresignedGet`（60s TTL）+ 302 重定向。浏览器对 `<img>`/下载自动跟随 302，**前端零改动**；响应头（nosniff/disposition）经签名参数（`response-content-*`）随预签名携带。实现要求：handler 按**投递策略缝**写（delivery 策略可替换），升级时只换一个函数。**前提注（2026-10-16 外审补遗）**：img 场景依赖拍板点 #8 的认证通道——cookie 回退落地后 `<img>` 才能到达本端点并原生跟随 302（header-only 认证下 img 直接 401，两形态都不免疫）。

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
- **自动保存（2026-10-16 所有者问询；同日扩评估至发起页整页与"草稿箱"必要性）**：三候选编辑器均提供 onChange/onSave 钩子——自动保存逻辑落**封装层**非编辑器本体，不影响择库。**三形态评估结论**：
  - **① 表单级本地草稿（✅ 做进 B5-4 随行，≈1 天；实现载体 = 发起页 `create/index.vue`，S1 域随 B1-1 增强）**——整页全字段（标量 + custom_data 序列化）防抖（≈2s）写 localStorage（key = type_code），重开页面检测草稿提示恢复、提交成功清除；**附件联动**：草稿存 pending object_key 列表，恢复时自动重新 confirm **复用已传对象**（GC 窗口内省重传；confirm created_by=本人成立），超窗标"需重传"——业界标配（Jira/GitHub issue/GitLab 均本地草稿、无草稿箱）。原字段级方案（仅 richtext）升级废弃。
  - **② 服务端草稿箱（`ticket_drafts` 表+列表页+保留期，2–3 天）：不做默认，登记触发驱动**——触发信号 = 真实出现"跨设备/跨天填单"诉求（长审批类表单场景；业界锚点 = 宜搭/飞书审批草稿箱）。①的存在使其降级为锦上添花。
  - **③ 工单 draft 状态态：维持不做**——动状态机默认图 + L2 可见性/S6 触发/角标计数/报表四处排除逻辑，全局涟漪远超场景强度（与批量下发"草稿态留二档"同款结论）。

## 7. 对象存储备份随行件（2026-10-09 登记；2026-10-15 业界方案展开；**2026-10-16 随后端切 RustFS——工具链 mc → rclone**）

PG 有 pgbackup+WAL+PITR 完整体系，对象存储（RustFS）**零备份设计**——附件丢失 = 工单证据链断裂；且对象存储**无 WAL 类事务日志可 ship，PITR 原生不可得**，RPO = 复制/镜像周期（附件场景日级可接受，显式登记不默认）。

**业界方案四类（2026-10-15 核验；均为社区版 mc CLI 能力，不涉控制台裁撤面）**：

| 方案 | 机制 | 优点 | 局限 |
|------|------|------|------|
| **服务端桶复制（`mc replicate`，active-passive）——业界首选** | MinIO 原生异步复制到目标 MinIO | 近实时 RPO；**版本历史/元数据保全**；服务端执行不占业务机 | 需目标 MinIO 实例；仅 MinIO→MinIO |
| **`mc mirror` 定时** | 客户端单向同步，任意 S3 兼容目标（NAS MinIO/Ceph 均可） | 最轻、可断点续传、对齐既有 cron/RETAIN_COUNT 惯例 | **只同步 latest（无版本历史）**；周期性 RPO |
| Batch Replication | 批量复制作业 | 大存量一次性迁移 | 非常态备份机制 |
| 卷快照/停机 tar | PV 级快照 | 完整 | 一致性风险/需停机——**不取为主方案**（可作恢复演练辅助） |

**落地建议（拍板点 5）**：目标实例可得 → **A = 服务端桶复制 + 双端桶版本化**（版本化防误删/勒锁，业界标配——⚠ RustFS 服务端复制能力随 B5-1 spike 验证）；单机起步期目标实例暂缺 → **B = `rclone mirror` 定时（独立卷第二 RustFS 实例，镜像已拉）+ 目标桶版本化**起步（**工具随 2026-10-16 后端切换定 rclone——`rclone/rclone` 镜像已拉已验证，替代 mc mirror**；mc 对 RustFS 端点的可用性随 B5-1 spike 一并验证），登记升级触发（跨主机目标就位/量级上来切 A）。**恢复演练强制随行**（对齐 activelist pgbackup 首验+恢复演练惯例：不演练 = 没备份）；目标侧保留策略沿 RETAIN_COUNT 惯例。生产云 OSS 场景 = 版本化 + 跨区域复制 + 云备份计划（配置切换形态，内网后另议）。量级 +0.5–1 天。

## 8. 启动拍板点

| # | 事项 | 建议 |
|---|------|------|
| 1 | 头像下载形态 | ✅ **已拍（2026-10-15）**：后端代理端点——「授权后短 TTL 预签发+302」升级路径已评估：handler 投递段留策略缝，切换 ≤0.5 天（路由/鉴权/L2/审计全复用，§4） |
| 2 | 附件下载形态 | ✅ **已拍（2026-10-15，与 #1 联拍）**：统一后端代理——预签名形态弃用（L2 绕过+无审计），302 同 #1 升级路径 |
| 3 | 附件权限码 | ✅ **已拍（2026-10-15）**：复用 `ticket:update` 零新码——所有者「先不需要单独权限」 |
| 4 | 富文本编辑器形态 | ✅ **已拍（2026-10-15）**：**Markdown 编辑器**——所有者确认；md-editor-rt / bytemd / milkdown 择一（W0 spike 定） |
| 5 | 对象存储备份 | ✅ **已拍（2026-10-15）：直接做进 S5 必做件**——起步形态 = compose 第二 RustFS 实例（独立卷）+ **`rclone mirror` 定时**（2026-10-16 随后端切 RustFS 调整，原 mc mirror；镜像已拉已验证）+ 目标桶版本化 + **恢复演练一次**；跨主机目标（灾难级）= 部署拓扑升级触发；量级 +0.5–1 天 |
| 6 | 上传限制基线 | ✅ **已拍（2026-10-15 基线认可）**：按 §3 限制矩阵建议值；业界对表补 presigned POST policy 强制点（实现形态随批拍，见 §3 业界对表注） |
| 7 | 限制值配置化开放 | ✅ **已拍（2026-10-15）：归属 S5 段内做**（底座批后随行/段尾，**可切出**——排期吃紧先静态 config 形态交付零返工，配置化切独立小批）；形态 = 开放·分层（config 默认 + `system_settings` KV 运行时覆盖 + 管理面 UI）+ 硬底线不可配 + 按 key 编辑层级（存储限制类 superadmin-only）+ 变更审计/钳制——详录 §3 |
| 8 | **img 认证最后一跳（2026-10-16 外审实锤缺口——已核代码）** | ✅ **已拍（2026-10-16）：Cookie 双凭证**——登录/刷新旁设 `HttpOnly; SameSite=Lax; Path=/api/v1/files` 短时效 cookie（TTL 对齐 access token），files 路由中间件 cookie 回退（header 优先）——`<img>` 原生可用、浏览器缓存/304 生效、前端零渲染改造、302 升级路径顺带解锁；与逐次审计兼容（cookie 映射用户）；tokenStorage 已预留「B7 cookie 会话演进」向。实施设计 = 附录 A.11（含验收负向三件）；业界锚点 = GitHub/GitLab 应用内 cookie 阵营（GitLab 文档原话：同源 `<img>` 自动带 session cookie 是应用内私有图片可渲染的原因）；Slack/Notion/Discord 签发时效链阵营的动机（跨服务嵌入/CDN 卸载/防公网滥用）内网语境均不存在。证据：`middleware/jwt.go` 仅读 Authorization header + ui token 在 localStorage——`<img src>` 裸请求必 401 |

## 9. 边界（不做）

维持 10 号"不做"清单：病毒扫描/内容审核、>100MB 分片上传、CDN 独立域名——均按需另立；云 OSS 生产替换 = 配置切换非代码分支。

## 10. 影响面

- **迁移**：两表族（编号对账）；**权限**：预置零新码（拍板点 3）；**配置**：`storage:` 段新增 + compose RustFS 双实例（草案已落）；**错误码**：91000–91999 段启用（91001–91004 四码随批登记 `api/errcode.md`——standards §3-4 对账位）；**端口**：对象存储客户端（S3 兼容抽象即端口，README §6 清单第 7 行）。
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

## 附录 A：详细技术设计（实施级，2026-10-16 补充）

> 定位：把 §3–§7 的决策与 10 号蓝本整合为可直接开工的实施设计。标注 ⚠ 的是实施级设计建议（启动批确认制，不新增拍板点）；与 10 号冲突处**以本附录为准**（收编注同款纪律）。

### A.1 模块架构与代码落点

```
internal/pkg/storage/              # 端口层：S3 兼容抽象，零业务语义（README §6 端口第 7 行）
  client.go                        #   Client 接口：PresignPut / PresignGet / StatObject / RemoveObject
  minio.go                         #   minio-go v7 实现（生产 OSS = 换实现文件，接口不动）
  config.go                        #   StorageConfig 结构 + viper BindEnv（endpoint/AK/SK 等 config 化，禁硬编码）
internal/repository/storage_repo.go    # file_objects CRUD（pgx，事务接口最小依赖惯例）
internal/service/storage/          # 业务服务层：presign/confirm/limits/GC
  service.go                       #   PresignUpload / Confirm / ListByTicket / Unlink
  limits.go                        #   限制矩阵执行器：config 默认 + system_settings KV 覆盖 + 代码硬底线钳制（§3 配置化三层）
  proxy.go                         #   代理下载投递策略缝：deliverProxy()（现拍）/ deliver302()（升级选项，§4 评估 ≤0.5 天）
internal/handler/storage_handler.go    # presign/confirm/files 代理/settings KV 路由
internal/service/ticket/attachment_service.go   # 附件业务归 ticket 域（领域隔离，10 号 §8 蓝本同款）
internal/pkg/jobs/… + cronloop     # 孤儿对象 GC（骑既有 cron 设施，§3）
deployments/compose.yaml           # minio + minio-backup（B5-5 第二实例）两服务
```

- **Wire 装配**：`storage.ProviderSet`（Client→Repo→Service→Handler），与既有 provider 分层惯例一致。
- **端口方法面刻意收窄**（4 方法）：presign GET 仅 B5-5 备份与 302 升级用；业务链路不出现预签名下载（已拍代理）。

### A.2 config 完整结构（出厂默认；运行时覆盖见 A.7）

```yaml
storage:
  endpoint: http://rustfs:9000     # RUSTFS_ENDPOINT（compose 服务名；dev 栈 localhost:9000）
  region: us-east-1
  bucket: zhuzhao
  access_key: ${RUSTFS_ACCESS_KEY}  # env 注入，禁烤镜像（值与 compose RUSTFS_ROOT_* 同源）
  secret_key: ${RUSTFS_SECRET_KEY}
  use_ssl: false
  presign_upload_ttl: 15m
  max_upload_bytes: 10485760       # 10MB 全局（KV 可覆盖，天花板 100MB 代码钳制）
  max_attachments_per_ticket: 20
  max_presign_batch: 20            # 前端一轮批量上传上限（=单工单上限；防滥用由 rate_limit 桶承担）
  allowed_mime_types:              # ⚠ 显式枚举（修正 10 号 allowed_mime_prefixes 前缀白名单盲区）
    [image/jpeg, image/png, image/webp, image/gif, application/pdf, text/plain,
    application/zip, application/msword, application/vnd.openxmlformats-officedocument.*]
  blocked_mime_types: [text/html, image/svg+xml]   # 硬底线：KV 不可白名单化（§3 配置化护栏）
  inline_mime_prefixes: [image/]   # 代理下载 Content-Disposition 分型（inline vs attachment）
  orphan_gc_days: 7                # 孤儿对象 GC 阈值（KV 可覆盖）
```

**匹配语义（2026-10-16 外审补遗，写死防歧义）**：`allowed_mime_types` 中尾缀 `.*` 的条目按**前缀匹配**（如 `application/vnd.openxmlformats-officedocument.*`）、其余条目**精确匹配**；`blocked_mime_types` 恒**精确匹配**且**优先于白名单**——执行器单函数实现，KV 运行时覆盖值同语义。

### A.3 数据设计（迁移全文级）

> **编号口径（2026-10-16 外审对齐补注）**：本文 DDL 注释中的 000045/000046 为 2026-10-15 对账结论（review/11 §4：000001–000044 占用），**非锁定**——取号当天按 A2 规则再对账，撞号让位整体重排（11 号 §5.9 同口径）。

**迁移 000045（`file_objects`，含 ⚠ 两处对 10 号的实施级修正）**：

```sql
CREATE TABLE file_objects (
    id          BIGSERIAL PRIMARY KEY,
    bucket      VARCHAR(100) NOT NULL,
    object_key  VARCHAR(500) NOT NULL,          -- {domain}/{yyyy}/{mm}/{uuid}.{ext}，服务端生成
    purpose     VARCHAR(30)  NOT NULL,          -- ⚠ 修正①：显式 purpose 列（avatar/ticket_attachment/ticket_richtext）
                                                --   ——purpose↔key 前缀一致性校验的查询锚点（§3 key 细节）
    status      VARCHAR(20)  NOT NULL DEFAULT 'pending',
                                                -- ⚠ 修正②：pending(已预签未确认)/confirmed/deleted
                                                --   ——10 号原设计 confirm 才写库，孤儿对象（预签未 confirm）DB 无记录、GC 无从发现；
                                                --   改 presign 即写 pending，GC 扫 status=pending AND created_at < now()-orphan_gc_days
    file_name   VARCHAR(255) NOT NULL,          -- 原始文件名（仅回显；下载头 RFC 5987 编码防注入）
    mime_type   VARCHAR(100) NOT NULL,          -- presign 钉入签名的类型（防谎报锚点）
    size_bytes  BIGINT,                         -- confirm HEAD 实测回填（声明值预检存 presign 阶段）
    created_by  BIGINT NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    confirmed_at TIMESTAMPTZ,
    UNIQUE (bucket, object_key)
);
CREATE INDEX idx_file_objects_gc ON file_objects (status, created_at) WHERE status = 'pending';
```

**迁移 000046（`ticket_attachments`，同 10 号 §3）**：ticket_id FK CASCADE + file_object_id FK + uploaded_by + UNIQUE(ticket_id, file_object_id) + idx(ticket_id)。**richtext 内嵌图不建关联表**——图片引用存在于 MD 源文本（`custom_data`），生命周期：删工单不级联删图，随 GC 触发驱动再议（登记）。**编辑时泄漏（2026-10-16 外审补遗，登记第三条 GC）**：编辑 MD 删掉某图后，confirmed 对象永久无主（无关联表可查引用）——登记**定期对账扫描**为触发驱动件：cron 解析存量 custom_data 的 `/files/` 引用集，标记超期（N 天）无引用的 confirmed 对象 deleted；启动信号 = 附件量级/磁盘水位触发，与「删工单不级联」同批再议。

**B5-6 KV 表**：`system_settings`（key VARCHAR PK / value JSONB / updated_by / updated_at）——存储限制为首个消费者，审计保留期为已登记的第二个。**迁移 000047（对账结论非锁定）**——S5 合计三枚迁移（000045–000047，2026-10-16 外审对账补齐第三枚）。

### A.4 API 契约（全端点；信封 `{code,message,data,request_id}`，standards §3-3）

| # | 方法/路径 | 权限 | 用途 |
|---|-----------|------|------|
| 1 | POST `/api/v1/storage/presign/upload` | per-purpose（见 A.7 权限矩阵） | 返回 upload_url/method/headers/object_key/expires_at；**服务端同时写 file_objects(status=pending)**（A.3 修正②） |
| 2 | POST `/api/v1/tickets/attachments/confirm` | `ticket:update` + L2 | HEAD 实测（存在/size/content-type 比对声明）→ 翻 confirmed + 写 ticket_attachments；校验 created_by=当前用户 + 对象未绑定（91004）+ purpose↔key 前缀一致 |
| 3 | GET `/api/v1/tickets/:id/attachments` | `ticket:read` + L2 | PageData 分页 |
| 4 | POST `/api/v1/tickets/attachments/delete` | `ticket:update` + L2 | 删关联行 + file_objects 翻 deleted；对象物理删除走 GC（10 号"仅删关联"口径） |
| 5 | GET `/api/v1/files/*key` | 登录态 + per-purpose 鉴权（头像=本人或任意登录态展示场景；附件=工单 L2 可见性；richtext 图=同附件） | **统一代理下载**（已拍 #1/#2）：鉴权→L2 判定→**审计落行**→投递策略缝（deliverProxy：流式转发 + `nosniff` + 分型 disposition + RFC 5987 文件名） |
| 6 | GET `/api/v1/system/settings/:key` + POST `/api/v1/system/settings/update`（B5-6——**2026-10-16 外审修正：原 GET/PUT 违 §3-1，改 house style**） | GET=admin 读 / POST=按 min_editor_tier（存储限制类 superadmin-only），key/value 入 body | KV 读写 + 钳制校验（越界 400 非静默修正）+ 变更审计 |

> 直传环节的 PUT/HEAD 为 S3 协议层动作，不经平台 API 面（§3 协议口径注）。错误码 91001–91004 随 B5-1 登记 `api/errcode.md`。
>
> **动词段对表（2026-10-16 外审问询，已核无需豁免）**：`attachments/confirm`·`attachments/delete` 与仓内 house style 同构——POST `/ticket-types/update`·`/ticket-templates/delete` 等（P4-W1 BK-18 方法整改后的**合规目标形态**）即「POST 资源/动词 + 标识入 body」；§3-2 管的是业务标识（本表全部端点标识均在 body），taskrunner/activelist 的 §3-5 登记系「URL 标识+动词段」组合或跨服务契约，非本形。

### A.5 关键链路时序

**附件上传链**（B5-3）：
```
前端选文件 → ① POST presign/upload（声明 purpose/ticket_id/file_name/mime/size）
  服务端：权限+限制矩阵预检（size≤上限 / mime∈白名单∧∉黑名单 / 个数≤20）
        → 生成 uuid key → INSERT file_objects(pending) → 签发 PUT URL（Content-Type 钉入签名）
→ ② 前端 PUT 直传 RustFS（类型被签名钉死，改类型=签名失效 403）
→ ③ POST attachments/confirm
  服务端：HEAD 实测（存在/size 与声明比对/content-type 与签名一致）
        → UPDATE status=confirmed, confirmed_at → INSERT ticket_attachments → 事务提交
失败路径：②未发生或③超时 → 对象滞留 pending → GC cron 扫描删除（A.3 修正②闭环）
```

**粘贴图片直传链**（B5-4）：编辑器 paste → Blob → ① presign(purpose=ticket_richtext) → ② PUT → ③ confirm → 编辑器插入 `![](…)`——**src 用代理端点相对路径** `/api/v1/files/tickets/…`（稳定 URL 硬约束，§6；`GET /files/*key` 为 GET 通配，与 §3-2 无涉且不与既有静态路由冲突——gin 静态 vs 通配冲突教训在案，启动批验证路由注册顺序）。

**下载投递策略缝**（B5-2 落地）：`proxy.go` 中 `deliver(ctx, obj, meta)` 单函数——现实现 = 流式转发；升级 = `PresignGet(60s)` + 302（响应头经 `response-content-*` 签名参数携带）。切换 ≤0.5 天、前端零改（§4 评估在案）。

### A.6 前端组件设计（zhuzhao-ui）

```
src/api/storage.ts                    # presignUpload/confirm/listAttachments/deleteFiles/settings KV
src/components/Upload/FileUploader.vue    # ①②③ 链路封装（进度/重试/类型前端预检——仅 UX，服务端为 SSOT）
src/components/Upload/AvatarUploader.vue  # 头像专用（purpose=avatar，即时预览——本地 Blob URL，非服务端回读）
src/components/MdEditor/index.vue         # md-editor-rt 🔶 封装：onUploadImg 桥接 presign 通道；禁 base64 内联（编辑器配置）；本地草稿自动保存（onChange 防抖 → localStorage + 恢复提示，§6）
views/ticket/detail/ 附件列表区块          # 下载走 /files/ 稳定 URL；删除二次确认
```

- 上传组件**不做**：分片/断点续传（>100MB 不做，§9）、图片裁剪（登记触发驱动）、拖拽多文件（单次批量 ≤5 由服务端钳）。
- MD 编辑器择库 spike（W0/B5-4 前置）判据不变：粘贴直传扩展对接 A.5 链路的改造量。

### A.7 权限矩阵

| purpose | presign | confirm | 下载（代理） | 删除 |
|---------|---------|---------|--------------|------|
| avatar | 登录态（本人 profile） | 本人 + created_by 校验 | 登录态（头像对登录态可见，§4） | 本人（换头像即弃旧，GC 收） |
| ticket_attachment | `ticket:update` + L2（工单可见） | 同左 + created_by | 工单 L2 可见性（`ticket:read` 管线） | `ticket:update` + L2 |
| ticket_richtext | 同附件 | 同附件 | 同附件（内嵌图随单可见） | 随工单（不单独删） |

- 零新权限码（已拍 #3）；代理端点鉴权中间件复用 gin 既有鉴权链 + L2 `resource.Authorize` 调用点（新资源接 L2 = BK-21 泛化预演，GetFilter 同管线铁律）。
- 审计埋点三处：代理下载（**范围收窄（2026-10-16 外审补遗）——仅 ticket_attachment / ticket_richtext purpose**：头像在每页右上角/列表/评论区高频渲染，逐次审计 = audit_logs 热读路径写放大；「证据链逐次审计」才是拍代理的核心理由——avatar 下载**免审计** + 响应带 `Cache-Control: private` 降重复拉取；谁/何时/key/结果）、attachments/delete、settings PUT（旧值→新值）。

### A.8 备份实施（B5-5，已拍 #5）

compose 增 `minio-backup` 服务（独立卷第二实例，零宿主端口）；`mc mirror` 定时任务（对齐 pgbackup cron/RETAIN_COUNT 惯例）+ 目标桶版本化开启；**恢复演练一次入 B5-5 验收**（删测试对象→mirror 回灌→比对）；RPO=镜像周期显式登记于 runbook（§7）。

### A.9 兼容与灰度

- **存量头像兼容**：`users.avatar` 旧数据为外链 URL——读取侧按前缀分流（`http(s)://` 开头直显 / 否则视为 object_key 拼代理路径）；列语义不变、零迁移。
- **灰度顺序**：B5-1 底座（无前端）→ B5-2 头像（单点自服务，炸了不伤业务）→ B5-3 附件 → B5-4 richtext（新字段类型，存量类型零影响——只增不改）。
- **验收补充**（在 §11 基础上）：pending 超时 GC 用例（presign 后不 confirm，orphan_gc_days 后对象与行均消失）；类型防伪负向三件（改 Content-Type 重放=签名失效 / 谎报 size=confirm 拒 / svg 改扩展名=mime 判定拦截）；并发 confirm 同一对象=仅一单成功（UNIQUE+状态机）。

### A.10 依赖与 utils 对表（2026-10-16 所有者问询；AGENTS「新增依赖须说明理由与替代方案」落档位）

**结论：utils 无需为 S5/S6 补充编解码工具**——utils 收编纪律 = 跨仓公共能力（aksk/jwt/errcode/response 的消费方为三四仓），本段全部编解码点均为 zhuzhao 单仓消费（taskrunner/activelist 无文件/消息场景），进 utils 无消费方。逐项对表：

| 能力点 | 出处 | 方案 | utils? |
|--------|------|------|--------|
| RFC 5987 文件名编码（Content-Disposition 防 header 注入） | A.3/A.4 | **stdlib `mime.FormatMediaType`**（内置 filename\* 编码），proxy 端单函数 | ✗ 单仓单点，留 `internal/service/storage/proxy.go` |
| **MD → 安全 HTML（服务端消毒 SSOT）** | §6 | **新增依赖 `goldmark`**（唯一 go.mod 增量）——理由：Go 生态 MD 事实标准（Hugo 同款）/CommonMark 合规/纯 Go 零传递依赖/**renderer 白名单钩子正好实现消毒 SSOT**（默认转义 raw HTML，属性/协议 allowlist 经钩子控制）；替代方案：blackfriday（维护停滞）/gomarkdown（其 fork）/自写 tokenizer（不现实）——落 `internal/pkg/mdrender/`（zhuzhao 单仓） | ✗ 单仓消费 |
| HTML sanitizer（bluemonday 类 allowlist） | §6 | **不需要**——MD 方案下 goldmark 默认转义 + 白名单 renderer 已覆盖；WYSIWYG（Tiptap 存 HTML）场景才需要，随其触发驱动 | ✗ |
| 文件类型嗅探（magic bytes/filetype 库） | §3 | **设计明确不做**——直传链路后端不见字节，类型防伪 = presign 钉 Content-Type + confirm 实测 + nosniff 三件组合（§3 修正②），嗅探无字节可嗅 | ✗ 零需求 |
| base64（图片内联） | §6 | **明确不引入**（防库膨胀纪律），粘贴直传走 presign 通道 | ✗ 零需求 |
| SHA-256 摘要（文件去重/指纹） | —— | 登记不做（S5 无去重设计；phase6 指纹为 risk 域内实现） | ✗ |
| zip 解压 | §9 | 不做（zip bomb 面；zip 仅作为上传白名单类型存储） | ✗ |
| 占位符替换 + escape（S6 模板渲染） | 06 号附录 A.4 | regexp + `html.EscapeString` stdlib 覆盖，单遍替换 | ✗ 单仓 |
| int64 string 序列化 / 信封 / JWT / AKSK | 全仓既有 | utils `jsonutil`/`response`/`jwt`/`aksk` **已有** | ✓ 已在位 |

### A.11 img 认证实施设计（拍板点 #8 已拍——Cookie 双凭证，2026-10-16）

- **Cookie 规范（⚠ 实施级建议，启动批确认制）**：name=`file_session`；值 = 服务端 HMAC 签名串（`uid+exp`，密钥复用 JWT 签名密钥——无状态、无 Redis 依赖）；属性 `HttpOnly; SameSite=Lax; Path=/api/v1/files` + 生产 `Secure`；TTL = 与 access token 有效期对齐。
- **签发/轮换/清除**：login 与 refresh 成功响应 `Set-Cookie`（重设即轮换）；logout 显式过期清除；密码修改/会话吊销（break-glass 链路）同步清除。
- **中间件回退**：files 路由认证顺序 = Authorization header 优先（既有 API 调用不受影响）→ 无 header 时读 `file_session` cookie 验签。**header+cookie 并存非 mixed-auth**（2026-10-16 外审修正：cookie 是文件通道的旁路凭证而非替代认证——`hasMixedAuth` 维持 Bearer+AK 互斥语义不变，二者随请求共存时以 header 为准、cookie 静默忽略）。
- **前端零改造**：`<img>`/`el-avatar`/MD 渲染器 src 直接用 `/api/v1/files/...` 相对路径（同源自动带 cookie）；axios 拦截器与 tokenStorage 无感知。
- **安全登记**：CSRF 面 = GET 只读 + SameSite=Lax + Path 限定，评估为极小；cookie 不承载刷新能力（refresh 仍走 header 流程）——文件 cookie 泄漏最坏后果 = 只读拉文件，HttpOnly 防 JS 读取；**phase1/02-auth 补第二凭证面登记**（本批已加尾注，B5-2 实施时同步实现细节）。
- **验收补充（叠加 A.9）**：负向三件——① cookie 过期后 `<img>` 401、重新登录恢复；② 登出后旧 cookie 立即失效（file 接口 401）；③ `Path` 限定断言（非 `/api/v1/files` 路径的请求不带该 cookie）。正向：axios API 调用（header）与 `<img>`（cookie）同端点双通道并存可用。

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
| 2026-10-16 | **详细技术设计批（附录 A，实施级——所有者要求各稿补架构/选型/DDL/API/链路细化，本稿为样板深度）**：A.1 模块架构与代码落点（端口层 4 方法收窄+Wire 装配）/A.2 config 全结构（显式枚举+黑名单+硬底线）/A.3 迁移全文级 DDL——**修正 10 号盲区第三条**（presign 即写 pending，孤儿对象 GC 才有扫描锚点；原设计 confirm 才写库=预签未确认对象 DB 无记录 GC 无从发现）+ purpose 显式列/A.4 全端点契约（六端点含代理下载与 KV）/A.5 三链路时序（上传/粘贴直传/投递策略缝）+gin 通配路由验证点/A.6 前端组件/A.7 权限矩阵+审计埋点三处/A.8 备份实施/A.9 兼容灰序+验收补充（GC 用例/类型防伪三负向/并发 confirm）——⚠ 标注项=启动批确认制，未新增拍板点 |
| 2026-10-16 | 自动保存批（所有者问询）：§6 补分层方案——本地草稿自动保存做进 B5-4（onChange 防抖→localStorage+恢复提示+提交清稿，≈半天，业界标配）；编辑已有工单的服务端自动保存不做默认（共享实体+version 乐观锁撞锁/审计噪音/半成品暴露——触发驱动另立）；A.6 组件行同步 |
| 2026-10-16 | 草稿评估扩批（所有者问"工单编辑页要不要草稿箱"）：§6 自动保存条升级为三形态评估——**①表单级本地草稿 ✅ 做进 B5-4 随行**（整页全字段序列化+附件 pending object_key 恢复时 re-confirm 复用已传对象，≈1 天，载体=发起页 S1 域；原字段级方案废弃升级）；②服务端草稿箱不做默认（触发驱动，信号=跨设备/跨天填单诉求）；③工单 draft 状态态维持不做（全局涟漪，与批量下发同款结论）——业界对表：Jira/GitHub/GitLab 均本地草稿无草稿箱 |
| 2026-10-16 | 批量上传参数批（所有者问询）：§3 限制矩阵+A.2 config——单轮批量 presign 5→**20**（与单工单上限对齐；presign 接口单文件设计，batch 框前端一轮批量；防滥用主手段=rate_limit 桶）——KV 配置化范围内随调 |
| 2026-10-16 | 依赖与 utils 对表批（所有者问询"utils 编解码工具要不要补"）：附录 A.10 新建——**utils 无需补充**（本段编解码点全为 zhuzhao 单仓消费，不满足 utils 跨仓收编纪律；RFC 5987=stdlib mime 包）；**唯一 go.mod 增量 = goldmark**（MD 消毒 SSOT——理由与替代方案按 AGENTS 纪律落档；黑名单：blackfriday 停更/gomarkdown fork）——文件嗅探/base64/zip 解压/SHA-256 去重均零需求（设计排除） |
| 2026-10-16 | **img 认证批（外审实锤，代码级核验成立——拍板点 +8）**：`<img>` 发不出 Authorization header（jwt.go 仅读 header + ui token 在 localStorage），现设计下 B5-2 头像/B5-4 内嵌图必 401 裂图、302 路径同不免疫——**#8 推荐 Cookie 双凭证**（Path=/api/v1/files 旁路 cookie + files 路由回退；tokenStorage 已预留 B7 cookie 演进向），B5-2 前必拍；§4 升级评估补前提注/A.7 审计范围收窄（仅 attachment/richtext——头像高频渲染免审计+Cache-Control private）/A.3 登记第三条 GC（编辑时泄漏=定期对账扫描，触发驱动）/A.2 MIME 匹配语义写死（`.*` 前缀+精确+黑名单优先）/A.4 动词段对表（无需豁免——house style 同构 #5 项不成立） |
| 2026-10-16 | **#8 拍定批（所有者）**：✅ Cookie 双凭证——业界两阵营核验落 GitHub/GitLab 应用内 cookie 主线（时效链阵营动机内网均不存在）；**附录 A.11 新建**（cookie 规范/签发轮换清除/中间件回退序/前端零改造/安全登记/验收负向三件）——**S5 八枚拍板全部闭环**；phase1/02-auth 补第二凭证面尾注 |
| 2026-10-16 | 外审对齐批：A.3 补编号口径注（000045/000046 = 对账结论非锁定——A2 撞号让位重排）+ **B5-6 system_settings 迁移 000047 登记**（S5 合计三枚——外审计数补齐） |
| 2026-10-16 | **第二批外审回声批（公约硬冲突优先修）**：①**A.4 #6 GET/PUT → GET+POST update（house style）**——原写法违 §3-1 且 §3-1 恰以本段为首批应用例（自指冲突，外审第 1 条）；②§3 配置化「放行 PUT/GET-PUT 两端点」措辞同步；③孤儿 GC 定位统一（正文采纳附录基线形态：B5-1 随批 GC+orphan_gc_days KV——正文/附录/A.9 三处对齐）；④「五端点」措辞补全景注（A.4 六行）；⑤A.11 hasMixedAuth 语义修正（header+cookie 并存≠mixed-auth，以 header 为准）；⑥B5-6 切出口径补「仍计一期收口」 |
