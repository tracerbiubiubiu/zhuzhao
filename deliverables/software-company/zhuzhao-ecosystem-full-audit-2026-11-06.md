# zhuzhao 生态首次全面审计总报告（2026-11-06）

## 结论与交付边界

本轮覆盖 zhuzhao、taskrunner、activelist、zhuzhao-ui、zhuzhao-utils，从有效设计和阶段演进比对到实现、关键运行实践、修复及二次验证。当前已确认的 **7项P0、39项P1** 在下述固定修复范围完成代码或文档复验；P1含验收隔离与两项设计台账失真，并非39项都属在线业务故障。既有panic债单列去重，不与新发现重复相加。各项细节、锚点和原/修复/变异证据以对应分批报告为准。

**测试栈已承接修复，main/生产尚未承接。** 所有者最新要求不合main，本轮遵守；较早单独授权的utils v0.4.3发布见下节。原dev仍映射原工作树，UI b205的已知F08仍可复现，不能据测试版绿宣称原开发版或生产零错误。必要正式报告/指南/修复已分批本地提交，原始日志、备份、截图、探针与执行卡仅在忽略的`.codex`。

本报告陈述明确范围内的审计完成与复验结果，不是所有输入穷举、真实用户研究、全站可访问性或供应链安全认证。P2/P3与已接受未来取舍继续挂账；没有把未来Phase6–9未实施能力当当前缺陷。

## 五仓原基线、固定修复与运行来源

| 仓库 | 原受审完整提交 | 最终修复代码提交 | 状态 |
|---|---|---|---|
| zhuzhao | `5c03581d5999139a3e8e5ead5972c026868aa181` | `dc3d540a48e67bf56244efdc0f71cf32cbcb5e80` | codex/audit-integration-next；test已承接 |
| taskrunner | `1dcc83185f24b8dc2bed7bba6786272a5fdfb19f` | `eb3c0c07615c9646da9a9ada673de16843c75564` | codex/fix-taskrunner-compose-port；test已承接 |
| activelist | `af0b350da650517df2c9d15cca4892cdcd51be88` | `60e6b29ba8e03054a0badaac810a1ddda4b7c4c3` | codex/fix-utils-transport-consumer；test已承接 |
| zhuzhao-ui | `b20594cc744a61e2bcb41535b0ea668a711aa4df` | `c9f0429ea05badac228f8e7e523e98859434339d` | codex/fix-ui-final-schema；test已承接 |
| zhuzhao-utils | `5a3900fa6013750e912eca958666831f0a6f2c9d` | `cd759a0402455731f805ba507bf8401447e00e29` | 仅codex修复分支已推；消费者正式pin |

原消费者实际pin为v0.4.2，其tag为`21155a98e87a944b20600833f940698d2759c2e5`，不能用当时库HEAD替代。v0.4.3/tag→`9296ae170b71b06c623c54b6010a107406903346`已在较早单独授权下发布，后来“不合main”的限制继续生效；本轮Reader/Transport仅推codex分支供不可变pseudo-version解析，没有新main合并/tag。主仓与AL最终pin `v0.4.4-0.20261106080630-cd759a040245`，sum及下载源码核对通过、无replace/GOWORK；TR只直接Sign、无Transport消费，维持v0.4.2。

原仓收口SSH fetch确认：主仓origin/phase5仍5c、AL/TR原main仍原基线、UI phase5仍b205、utils main9296。UI原仓有既有`.gitignore`在途，未纳入本轮或替其提交；其他原仓干净。主仓共享phase5的报告提交不是上述修复代码来源。文档收口后审阅分支可以新增纯Markdown提交，其生产子树须与dc3完全相等，另有差集收据，不重贴运行镜像版本。

测试入口：[UI48081](http://127.0.0.1:48081)、API33334；dev为UI4000/API33333/AL18081/TR18080。实际四镜像分别为app `5f562f23…`、AL `6cfe5df4…`、TR `8de6d8ee…`、UI `fdd4b717…`（完整ID/二进制SHA见B13及本地manifest）。三PG备份目录经校验；四应用更新时九个基础/开发容器保持，原数据卷和其他cmpfull会话资源未清理。

## 阅读、实现与动态覆盖口径

- **设计**：130篇Markdown已全文阅读；最终代码快照对应103篇未变、27篇完整差异逐一复读，共764行带上下文差异。覆盖启动提案、Phase1–5、有效ADR/决策/历史审计及Phase6–9，不采信废弃`doc/`。不是宣称130篇都在最后一次重新全文读过。
- **主仓实现**：cmd/internal的156个非test Go（含1个生成Wire）有全文基线与完整差异；b306前14篇差异后，至dc3另151不变+5篇完整差异（4逻辑、1DTO注释），无整文件未读。生成Swagger按模型/注解及再生零漂移核验，源码阅读不等于所有函数动态全覆盖。
- **数据库**：162 SQL/81对共3223行全文、逐对设计/兼容/逆操作crosswalk；编号与11号地图一致，无缺号冲突，下一号82。动态fresh往返只验证终态投影，不证明任意定制数据可逆；MIG-S04另以定制/fresh两库实测裁定，见B15。
- **兄弟仓**：全部生产模块均有设计→入口→边界→证据映射，含类型/Schema、数据与导入导出、执行存储/调度/worker/callback/outbox回收、公共认证与错误/序列化工具。没有把11个模块映射虚称每个既有测试文件逐行审完，也未以主仓Go计数替代兄弟仓覆盖。
- **前端**：功能/权限/会话/数据精度、导航表单、键盘焦点、图标、主题对比、长内容和桌面布局均纳入。最终28文件227单元、完整41E2E和真实Chrome代表旅程分层记录；未进行手机/屏幕阅读器实机、全站axe、200%缩放或真实用户研究。
- **历史债**：101个正式编号逐行映射；旧“102件”不当成102条独立当前故障。原基线分类48partial/31future/16open/6component是映射状态，不是本轮尚有16项P1、也不是101全销账。B-11/G-3/L-6补分支复验证据，L-5主体、通知增强、观测/可靠性等仍按现行台账。

独立出口复核确认五仓、当前模块及演进没有整块必审范围漏项。局部受限与未来项已显式列明，不以找到若干问题或测试绿代替覆盖表。

## 从启动至Phase9的设计演进与现行边界

1. 启动与Phase1：Go模块化单体、PG/Redis、Wire分层，登录/刷新/吊销及用户/角色/菜单/组织；Casbin绑定菜单API构成L1。管理动作priority/系统角色守卫与有效角色继承是不同判定，不能混为全局豁免。
2. Phase2：资源级工单、评论/备注/事件/关联/模板；2b/2c补ltree组织关系、透明读/隔离、委托scope/org_roles/角色继承。L1后以有效org_path和关系约束L2，资源canOperate最终deny-by-default；创建者/处理人/CC读权与操作权分开。
3. Phase3：平台底座与兄弟服务集成；taskrunner执行、AL反代、HMAC、提交/回调幂等、审计归档/字典/轻量观测。当前事件面接受进程内at-most-once，不能把早期轮询/Outbox草图直接当现行承诺。
4. Phase4：Vue SPA、壳层/三层路由、Pinia与服务态查询、OpenAPI生成消费；GET/POST公约、页面仅GET/按钮挂写与全角色权限成为机器守卫。兄弟服务共享认证契约，未另建一套面向用户IAM。
5. Phase5：S5富媒体、S1字段/公式取数、S6通知/inbox、S2图配置、S10协作超时导出、S4任务管理、S9缓存及S3a条件/validator/post_actions/分派/SLA0.5。S7 AI/S8大屏仍为条件/概念边界，“十段”不表示十套完整业务都上线；engine/driver拆包、流程版本实例语义未由S3a交付。
6. Phase6：已立项规划R1汇聚去重、R2评估处置、R3风险与工单联动、R4度量，另有已拍的P6-R5/D30收口后IR扩展批（战时动作/战后组装，尚未实施）；启动批补流程V2/预检、mock HR/IdP和委托地基。README§5与07挂账索引为边界；按2026-11-07所有者最新顺序，须先Phase5检查/审计/修复收口并经授权合main，再从main切Phase6；旧允许从phase5先切的例外已取代。本轮不合main、不切Phase6或宣称其已实施。
7. Phase7：扩源/路由维护窗、Action注册、非人身份、调度职责、资产门面与观测；暂不自建通用编排引擎，依赖量级和资产契约触发。
8. Phase8：脚本契约/隔离执行、审批库/预置任务、SOAR集成、动作面板、自助任务、凭据与结果摘要；本阶段不建设自研DAG，外部接口和审批输入仍是前置；Phase9轻DAG MVP是已拍的后续顺序，并非未承诺或仅触发驱动。
9. Phase9：自动处置闭环、值班升级、runbook/证据中心、轻DAG与PG outbox持久化；与当前状态机共用Action词表但执行语义不同，不用未来at-least-once要求倒判当前接受的at-most-once。

## 模块能力与关键实践核验

| 模块 | 现行能力与边界 | 本轮重点证据 |
|---|---|---|
| 认证/会话/PAT | JWT双token、Redis撤销/轮换、文件cookie与PAT独立身份；HR/IdP未当现有能力 | B02纪元、真实撤权13步、B14安全写；非只看commit正文 |
| L1/管理权限 | Casbin有效角色/菜单API；priority/系统角色管理守卫独立 | B11：19主体×170case×8，25840请求/5440ServeHTTP交叉、差异0；不把哨兵500算业务成功 |
| 组织/L2/L3 | 物理/虚拟组织、有效成员/scope、隔离/透明读、资源canOperate | B09过期→拒绝→续期15步与六过滤变异；独立owner权源未被取消 |
| 工单核心/评论/关联 | 状态配置、custom_data增量patch、归属与事务审计、双端关联授权 | F05权限快照、F11消息接线/commit失败控制；并非所有跨域角色撤权原子化 |
| 类型/字段/表达式 | 字段meta、dict/file/richtext、formula/fetch；预检与写同代 | F15 tx字典/MIME复用；F17图另一半持久快照、F18nil/旧/未来版本与硬锁 |
| 流程/动作/分派 | S3a condition/validator/post-actions、任务提交、notify/reassign | 锁后资源/类型版本验证、动作/回调幂等；任务成功不等于工单自动成功，结果映射仍未来 |
| 存储/设置 | presign-confirm-bind、配额、头像/附件、GC、全量KV快照 | RustFS原两误删/修复实物；F14多键/取消，F20头像与资料CAS；多实例传播不在保证内 |
| 通知/站内信 | 模板/规则CAS、节流/偏好、dispatcher、inbox | creator与actor、无目标不烧水位/零额度不发、F19真实身份链；真实IM无凭据不冒充已送达 |
| CC/标签/角标 | CC扩大读、标签共享、本人工作台统计 | 读与操作隔离、深链筛选一致；旧轮询/缓存债未因导航修复全关 |
| 超时/SLA/调度 | 进入时刻、提醒水位/人工supervisor、专用cron锁会话 | F06/F07/F08/取消与panic控制；at-most-once、完整工作日历/HR链仍按规划 |
| 导出 | List/Export同过滤、15列/5万、自审计 | F13真实XLSX/Flush/Close，F16实际流上限/实际计数、静默截断变异 |
| 情报缓存 | provider只读Port、7/30天TTL、LRU/SWR/singleflight | F10 cleanup交错真实PG/阻塞Provider八轮；外部情报源未因此全联调 |
| 审计/运维 | 脱敏、判定日志、JSONL归档、轻metrics、备份 | F12连续数组脱敏，连接预算/目录权限/恢复演练分层；未声称PITR/防篡改认证 |
| activelist | 动态Schema/CRUD/keyset分页/软删/导入导出、HMAC | pool/1GiB边界/全量导入revision围栏；未承诺多租户细粒度IAM或百万行压测 |
| taskrunner | SQLite/PG、Asynq调度、callback、重投/死信/reclaim | delivery身份围栏、panic兜底、空死信、真实PG/Redis及生产18080探针；非通用DAG/恰一次 |
| utils/网关 | JWT纪元、双AK/SK协议、Reader/Transport、信封与日志 | 正式版本/sum、ReadAll错误关闭、Reader错误分类、网关本地413；未凭库HEAD推消费者已升级 |
| 前端体验 | 角色旅程、草稿/异步会话、精确ID、深链、空态/键盘/离线图标/对比 | 14项原/修复证据、窄生成契约、227单元/41真实栈；Chrome1024浅暗及退出重登 |

## 发现、修复与二次验证汇总

| P0编号 | 已证实问题与影响边界 | 主要修复来源 |
|---|---|---|
| [B02-F01](./zhuzhao-ecosystem-audit-b02-auth-revocation-2026-11-04.md) | 同秒/重登后的旧AT复活；非匿名访问 | `10f3f9485f1bbe5722192e2e367e735f162653ac`及正式utils v0.4.3 |
| [B06-F01](./zhuzhao-ecosystem-audit-b06-storage-transactions-2026-11-05.md) | enforce头像GC与绑定误删实物；默认observe不同 | `d07f560c19e311d8c5255a8f7eb5867cc55f488a` |
| [B06-F04](./zhuzhao-ecosystem-audit-b06-storage-transactions-2026-11-05.md) | 默认工单confirmed GC旧快照误删已绑附件 | 同d07f560，独立对象域/入口/默认开关与回归 |
| [B06-F05](./zhuzhao-ecosystem-audit-b06-flow-reminders-2026-11-06.md) | 同状态交错使锁前工单/条件权限快照过期 | `fb0fff29da5774f557e51d714e1ee7c0b1b8d879` |
| [B07-F03](./zhuzhao-ecosystem-audit-b07-sibling-services-2026-11-06.md) | 全量导入重置version导致旧客户端ABA覆盖新数据 | `afc3700eb80c3d2a5b9c4c5e948f32f1e2c3dafb`后汇入AL60 |
| [B09-F01](./zhuzhao-ecosystem-audit-b09-membership-expiry-2026-11-05.md) | 过期成员/委托仍通过部分读写链 | `d2ab771292980c6bf40930a6bfec326b5bf6513d` |
| [B14-F01](./zhuzhao-ecosystem-audit-b14-user-security-state-2026-11-06.md) | 资料旧读回写禁用/强改，后续新登录通过；旧AT仍拒绝 | `f917c64242e08fdc59bd29179602148a9e8d00fd`，F20另补头像字段代际 |

修复按模块分批组织，同时包含已确认P1与伴随P2：B05验收入口/临时文件隔离；B06池嵌套/额度、提醒调度、收件人/评论、脱敏、导出/设置、配置/身份/头像；B07池/导入/死信/旧delivery/本地超限/生产探针；B08草稿/会话/筛选/ID/空态/键盘/表格/图标；B10/B12设计台账与错误码登记；B13启动project/回退/假成功/端口互杀/构建上下文。各项等级以分批报告为准，不能把此模块清单全部算作P1。原修复提交与联合承接hash逐项保留，没有用后笔覆盖原复现或把既有债再开一遍。

修复守卫按本项目习惯做移除修复串行八轮与同类扫描；变异必须失败于目标不变量，编译、初始化、选空测试等均剔除。关键新同类包括schema已传tx却字典/MIME仍取池、只修nil CAS仍可未来version绕过、移除安全字段回写后头像专用写还需共享代际；不是只在修复文件单点跑绿。全量真实PG/RustFS、HTTP、浏览器及产物解析相互补足，commit message仅索引，不作为验证事实。

## 固定联合门禁与运行证据

| 检查 | 实际结果 |
|---|---|
| 主仓 lint/Swagger | 3.718秒通过；generated三文件零漂移 |
| 提交守卫/验收隔离/启动编排回归 | 1.736/12.685/9.142秒通过 |
| Wire | 原make入口缺工具sum失败保留；CI同款固定v0.7.0 CLI 1.930秒通过、wire_gen零漂移 |
| 主仓单元/原生全量集成 | 10.746秒 / 558.571秒通过；原Makefile integration/race/-p1、无overlay |
| 主仓覆盖率 | 11.547秒通过；默认tag总20.4%，不是集成覆盖率或相对提升 |
| 显式migrate-up/原样acceptance | 81 clean；253 PASS/0 FAIL/0 SKIP、81对fresh投影往返零漂移 |
| 真实生命周期 | R01 13步、B09 15步均过；B14/F20永久原生实际服务回归亦过 |
| AL60原生全PG/race | 138.814秒；41PG顶层逐源匹配、含子测196 PASS/0 FAIL/0测试SKIP；3包无测试文件分列 |
| UI六件 | lint/type/227unit/依赖audit/build + 最终真实栈41首过139.286秒；配置retry1、实际0retry/flaky/skip |
| Chrome扩展 | E800006；重载最终源、首页/16行列表/表单/退出重登；1024浅暗整页无横溢、暗按钮对比5.947、console error0；恢复浅色/默认1920×855 |

早两轮UI夹具失败分别是admin权限不足及专属号缺root成员/精确用户名；原globalSetup默认种子例外后全部首过，密码指纹未变。临时E800007已回收。前轮W5AL初始化/profile/permissions429及最终一次AL POST429（原HTTP helper退避恢复）照实留存，0Playwright重试不等于0HTTP429。临时TR job和本轮S8工单/type按多重ID/时间/actor精确清理，历史runs/审计保留，24容器和四宿主PID无漂移。

主/AL运行镜像为固定源码宿主Go1.26.6交叉Linux arm64后复用准确运行层，source/blob/sum/binary均核；TR固定源码canonical build目标Go1.26.10新ELF与旧/新runtime逐字相同；UI固定源码Node22构建、准确nginx digest/259资产。没有冒称全生态canonical Dockerfile各阶段都离线重建成功。新AL60第一次构建误带祖先仓VCS字段的产物已隔离、未部署；历史AL19父镜像也有此元数据偏差，其真实来源以固定源码/二进制收据核对，不能仅凭VCS字段或误称其从未部署。最终主/AL以buildvcs=false及blob/binary收据锚定。FROM ID及离线apk失败亦保留，不计构建通过；Docker镜像ID与config digest分列。

**Go已知漏洞可达性：受限，四范围均未执行（NOT_RUN）。** 本机未安装govulncheck；官方v1.4.0缓存本体及签名记录可核，但精确x/tools v0.46.0/telemetry依赖缺失。官方代理及备用代理超时，固定工具安装预算退出；另一官方SSH路径虽取得telemetry精确提交，tools精确tag浅取90秒超时，未能闭合h1，按既定边界停止。没有更换依赖、关闭sum校验或构造兼容版扫描器。官方漏洞库index可达不等于扫描执行，不能给GO-ID、可达数量或“零漏洞”结论。

仅已验证依赖pin/完整性/产物来源和UI生产依赖audit。主/AL实际ELF为Go1.26.6，TR实际Go1.26.10，utils为库；网络条件满足后应在相同固定源及实际目标补source/binary扫描，区分调用链与符号存在。完整收据 `joint/final-govulncheck/{result.json,report.md}` 保留两次获取尝试、源码零漂移及无自有进程残留；不把受限项计作通过。许可证逐库裁定、完整SBOM和供应链认证亦未由pnpm audit/sum替代。

## 后续台账与未覆盖项

1. B15四P2：存储numeric ID与字符串标准/手写类型偏差；make wire工具入口sum；合法定制模板引用的回滚前置（FK完整保护用户数据、P1候选撤销）；密集W5夹具共享限流预算。按既定P2节奏另批，不在冻结窗口反复抛光。
2. 迁移MIG-S01/02/03为历史/定制状态静态线索，未报当前越权；fresh往返不是每个中间down或任意业务数据保真的证明。自定义引用down79会23503并留dirty checkpoint，须按实际数据态/依赖预检，不盲force或删用户配置。
3. Phase5 L-5正文回填、通知增强/动态词表、inbox/规则泛化、观测与outbox等按现行阶段台账。内部 at-most-once/水位策略、平台分工、未实现的Phase6–9为已接受边界，不重复计缺陷。
4. 未做百万行/1GiB实际吞吐压测、全角色×全页×全主题穷举、真实企业IdP/HR/IM/情报源全联调、PITR/全生产恢复与正向全栈up/down/reset演练。B13有19黑盒与真实三动作归属拒绝，不扩成全拓扑生产演练；真实Chrome专家实查不扩成用户研究/全站认证。
5. 对收件人“业务读取时/提交时”快照等未明确合同只保留边界；MessageTemplate变量元数据base64的运行缺陷猜测已撤销，没有把未启用字段猜成当前编辑故障。新发现须先对齐SSOT并动态证实，不能仅凭日志措辞或commit正文。

建议所有者先按固定codex修复分支审阅承接，再决定后续发布/Phase6启动；本轮不执行main合并。后续审计可直接复用[审计指南](../../docs/review/audit-playbook.md)，另建自己的基线与执行卡，不能沿用本轮旧hash或运行结论。

## 报告与证据入口

分批正式报告：B02认证、B05验收隔离、B06存储/事务及流程/提醒、B07兄弟服务、B08前端、B09过期成员、B10文档、B11路由矩阵、B12错误码、B13环境、B14用户安全状态、B15横切边界，均在本目录。每项发现含原固定hash、文件/符号/行锚及原/修复证据，历史失败与后笔关闭分列。

证据目录仅本地`.codex/audits/ECO-20261104/`：`evidence/ECO-20261104-joint/dc3d540/`、`evidence/ECO-20261104-joint/final-pg-integration-al60/`、`evidence/ECO-20261104-b08/final-schema/`、`evidence/ECO-20261104-b08/final-e2e/`，以及`final-source-coverage/`、`final-doc-delta/`、`migration-source-coverage.json`、`final-coverage-exit-review`和`data-coverage/final-ledger-check.json`。相关manifest记录SHA/环境/命令/清理；目录中的过程文档、原日志、SQL备份、截图不纳Git，也不进入Docker上下文。

## 修复窗口逐笔hash

以下范围分别为主仓37、UI16、AL5、TR6、utils4笔；不是把未包含于目标的其它分支改动混入。主仓后续纯报告提交不改变本轮生产代码，修复来源通过补丁承接者另有source→integrated映射。

### zhuzhao：`5c03581d5999139a3e8e5ead5972c026868aa181..dc3d540a48e67bf56244efdc0f71cf32cbcb5e80`

```text
01a59b06976c275b9a2c0ae2ac18beb791154846
0f262242d3b447c81924ad9e30c42f02a94e1f37
5883f147a0a4e2c3586ac0cff58529cac8840b5a
b95d050ffe445a3d192413863951b6186a833a92
d345858754005cd43d23534afc37870df608165e
4de9070ab444137689774f8b7e93dc69145b1157
62e40244b1667e47c3f729250b36c5edcbf445aa
586d63b61c53079151b574e2b6e5fbe15f69c15f
aa0b8ef1b84ecf3596245b8424803f23609431f4
173406af27aefd0d0af907116d1ae90311f1b6ad
42da71b9e37b9494427e8aeda60370269675fd0f
bf1e2d138e581378af0e660090598858d458a283
3fd219fa38500cbf3e29382de33d34547693b280
cd79374b18ea3e1aae63189d706d1b1751e91e13
91d7b90aca40a9dbf2aff59845f5def381b789f1
d38f10f9c60a8457417c2630afba0926b103797d
9e2fef62077a683049d16f0e0acba6295fd74cee
02a6e2cdaf00df89f63c95f573675f2fa1896f9e
f69b7cbefebff943972d24c647776375fb8d1f90
12401aa52d5fa8842dd944b9366917b19ae7775e
1a99a2534372da2106828b75962d162d98fdadac
7389ed79bdf409e466542992d9f1befa0c1a8529
6d66067b9534de555e10b1336093516ff10d8537
d16cb35d1afe58d958e5ebcff8d3459a58c2ab1b
7feb1c2ed6179a6a628464cc73c27eeb041c418b
e312ac0b4f82ec5ad615b27ece639fee0403a66a
1b8b96d64eeba555703a9630aa225339b403cefc
b4435022d00ee789b9a62f74036824bd7d3791b7
b306eed09db369b194c3216e05c72d16b597c2fd
fe99ae15e797d968584c10a7458a39c20e32be76
fbb1642a002c3bf404144a54a9b250556d9fefb5
dfc432fd5144a6ee6471c29f2dc1a4c6ec26c546
ba3ecb6dc643e529813198af97c8943245e7c500
38e6e442a8079bcc5aa4c1317a73f5df06931b28
6ad4cda60b321ee9420df20415e5d5c422b8a8f3
febafc592f21ef25e1ebf386a2c002ef89772697
dc3d540a48e67bf56244efdc0f71cf32cbcb5e80
```

### zhuzhao-ui：`b20594cc744a61e2bcb41535b0ea668a711aa4df..c9f0429ea05badac228f8e7e523e98859434339d`

```text
fc1068b66ea4f33f2f1a02e9a9ef557b488c7560
604b96c2fb6133fa75b2800f7121d5badd0c966c
ad5874590dd3ccee9f878541e629cc1a3ff075df
2d4afd06ff703cf26609af7eca7cd5f0ae6ed32c
80c34da918fb29cc5c60845c215567a522cd5bf2
f23d081080cf49354fd7017b92c59ffecbb735f6
e853323405eb9c7856d80605fd724c414e65bd5b
a1f8476c1102328a0c39dbe132f34befdb0506b9
4908d699a1a74b74246f074d0619a462a49a45a1
c83a53ded725da167eb1e9b3d408320336e25fb7
5ecba4b37709acef63224abfeb477f1a0e707878
f1043a12e74e929dd223a5b718e0a27b3781c253
61b35bf75596c79bf42c7c7d26544ac584716ba2
324bd5fba0806b45d024237b45598298c96582c1
9f76a1b1eba80434107d1797e9b7401aa0ef6623
c9f0429ea05badac228f8e7e523e98859434339d
```

### activelist：`af0b350da650517df2c9d15cca4892cdcd51be88..60e6b29ba8e03054a0badaac810a1ddda4b7c4c3`

```text
156ff034c1988509b517f246eb519ef81f829dc5
afc3700eb80c3d2a5b9c4c5e948f32f1e2c3dafb
e189f377cf6fc14cc5679773884fa3be209e6b6e
19a4c1d5cd84cc892b82ad52554db320bfad8f7f
60e6b29ba8e03054a0badaac810a1ddda4b7c4c3
```

### taskrunner：`1dcc83185f24b8dc2bed7bba6786272a5fdfb19f..eb3c0c07615c9646da9a9ada673de16843c75564`

```text
a1309cc0fd28a2c488a5ef7fdeb47dc68002d8bd
ed1a40e0dcf1ae6ae9c17e19720259132dc3bc88
df7ef36824f47eaa4832b20d214fed5a342b9b50
3b268dc350e2f7fb14941963cca902f946798ee7
5e3f5f3720a54fad9f5cec4a08481364693b8f7d
eb3c0c07615c9646da9a9ada673de16843c75564
```

### zhuzhao-utils：`5a3900fa6013750e912eca958666831f0a6f2c9d..cd759a0402455731f805ba507bf8401447e00e29`

```text
f6dbe69c668ee16a3c60f552539c03e52134a1eb
9296ae170b71b06c623c54b6010a107406903346
9ed38f791175a2324028a1cf386ef4318f3e5aa9
cd759a0402455731f805ba507bf8401447e00e29
```

## 剩余项分流与下一步建议（2026-11-07）

### 本次整理的边界

本节以原审计固定组合dc3/AL60/TR eb3/UI c9/utils cd759、正式分批结论及现行Phase5/6台账为依据。文档基线为主仓`fd21c38f75c69a69d7f57aa2e53ce9cc3dd7f249`、修复审阅分支`1f7e44fa52b4be306fb0486880f68426c0abe583`。本次仅排优先级与核对出处，**没有执行修复、漏洞扫描、服务重启、依赖升级、push或main合并**。下述“现在处理”是建议的下一批工作，不是本次已实施或额外门禁已通过。

执行优先级与缺陷等级分开：Go扫描未执行是安全证据缺口，不虚记为已发现P0/P1；P2也可能因回滚/对外契约临近而提前处理。101债编号中的48partial/31future/16open/6component是原覆盖映射，既不是101项未修故障，也不是当前风险排序。

已修无需再开发：B05-F02；B07-F07/F08/F09/F10（AL错误回显、TR两panic债、utils Close）；B08原六项P2；B10文档校准及B13旧P2。B-11/G-3的对象误删、后续F20共享代际和L-6内容登记已在修复组合关闭，**仍需后续主线/发布承接，不能把未合main误记为修复没做**。B08正文旧“未修挂账”是历史时点，按末节最终组合裁定，不重复开工。

### 现在应排的最小批次

| 项目 | 建议时点与原因 | 最小动作 | 验收与停止条件 |
|---|---|---|---|
| Go漏洞补扫（结果仍未知） | **第一优先，正式发布前补齐或显式评审受限风险**；未知不能由普通测试绿替代 | 独立工具目录/网络可用的隔离CI，固定官方工具及漏洞库时点，对四模块源码和三运行ELF分别核验 | 逐项记录工具/DB/source/binary/构建环境、finding/调用链与裁定；获取仍失败则保留NOT_RUN，不无限本机绕网；有实际可利用P0/P1则另开修复批 |
| [B15-F03](./zhuzhao-ecosystem-audit-b15-contract-migration-boundaries-2026-11-06.md)：定制模板回滚前置 | **现在补操作预检，任何实际down79前必须使用**；失败会留下dirty检查点，但FK本身在保护用户配置 | 只读列出待撤模板的定制引用，将“保留/改指由所有者裁定→按实际数据态处理dirty”写入回滚流程；本轮不自动删规则、不重写历史SQL | fresh默认库与有定制引用库均覆盖；预检零业务写入；不得以blind force或删除用户配置换取down绿 |
| B15-F02：make wire入口 | **下一工具小批优先**；新环境按文档命令会失败，修复面小 | 对齐CI固定版本Wire安装/调用入口与无工具提示；工具依赖独立于业务模块 | 新工具目录可用，生成物幂等零漂移；不将工具私有依赖塞入业务go.mod/go.sum |
| B15-F04：W5 E2E共享限流预算 | **下一测试小批优先**；本轮首过仍有HTTP429，不应把假红留给下一轮 | 密集夹具后沿现有等待/预算helper给首次页面初始化留额度 | 原断言/服务限额不变，串行多轮检查首轮结果、页面/profile/permissions及HTTP429；框架重试和HTTP退避分开记，不靠无限重试或全局延长超时 |
| B15-F01：存储ID出站为number | **近期独立合同批，最迟新增外部消费者或ID用途扩大前**；不是已证错对象事故，但类型债会扩散 | 存储输出DTO统一字符串ID，联动Swagger、生成类型及手写UI消费，盘点外部数字消费者的兼容方式 | 跨真实HTTP/JS验证2^53相邻ID保持区分、上传/附件/头像仍按object_key正确寻址；后端与UI同窗提交，不能只改TS声明 |

实际发布前还须核[C-14的生产HTTPS/Cookie Secure条件](../../docs/phase5/S6-S5-debt-ledger.md)、修复产物承接及备份/归档保留策略；当前未授权发布，本次不启用任何开关。它们属于上线条件核对，不是要求现在重建整套IAM/IdP。

以上四P2的等级和原证据均不改变。B15三条MIG静态线索只保留待核，不凭静读追加历史迁移改写；若下一批确实要降级到对应版本，再做定制状态复现。VariableSchema对象wire故障候选已撤销；通知收件人快照时刻尚无明确提交瞬时合同，先定合同再考虑实现。B07的Int64Slice数组含null仅P3错误层口径，随输入协议治理处理，不当成已证越权。

### 历史债按依赖排期，避免全部塞入当前修复

| 去重后的工作组 | 建议排期/触发 | 边界与出口 |
|---|---|---|
| 审计卫生：认证证据与归档 | **近期先核实际覆盖和部署策略**；依据[启动批§3](../../docs/phase6/05-launch-batch.md) | AuthService.Login已有多支LogLogin，logout挂authed AuditLog，旧“登录登出零审计”不再准确；先验证差集/修文档，不重复加日志。归档cron源码默认仍false，实际部署env本轮未核；启用前确认归档目的地、备份/恢复、保留策略和清理行为，不能盲切开关或误说运行态必关闭 |
| dbtx ticket域试点 | **建议下一轮大改ticket事务链前做**，可单独小批或Phase6随行；[13号§8](../../docs/review/13-bk11-pool-deadlock-2026-10-26.md)已批准，[05§3](../../docs/phase6/05-launch-batch.md)明确不锁启动 | 既有零行路径与字典/MIME池嵌套已修，不重复修；试点目标是防复发机制。小池并发/故意嵌套负向/取消回滚与全量集成通过后再决定其余repo推广。请求deadline仍未单独拍，不与dbtx强绑；计数行表化、schema缓存按量级/收益后排 |
| L-5长期模块/架构主体回填 | **近期文档批，优先于依赖这些正文做Phase6细化设计** | ticket/architecture按现行能力回填并保留演进边界；已完成menu/L1局部校准不销掉主体。无需先搬目录，也不顺手重构engine/driver |
| R1前的L2/装配/运行地基 | **Phase6 Week0既定出口**：C-3/BK-21、HR与源mock、事件装配守卫、C-17镜像侧车失败聚合可见面 | 新risk资源fail-closed与双org可见不能延后；mock HR/漏扫源先行，真实企业契约换adapter；mock IdP为可选随行，不能误记为R1硬门禁。新域验收随acceptance扩档 |
| C-1/C-2/G-1：DataFetcher与预填 | 四件已纳Week0；**真实外部源接入前必须具备超时、限流、审计**；运行时UI/三档错误提示随R3圈定 | prefill取数与B0-3 deploy preflight不是同件；当前空Registry不代表外部调用已安全。真实HR/IdP/CMDB契约不阻塞mock准备，但不能把mock绿当真实对接绿 |
| 流程V2与工单尾款 | 按[05/06](../../docs/phase6/05-launch-batch.md)交错实施：B-1→events真值diff；B-16→B0-3 deploy引用预检；B-21→B0-5，终态可配化前提到B0-2/B0-3 | B0-3定义快照先于B0-4自动路由；转换时字段采集先于R3复核态。G-4 Close/Assign与SLA口径随对应收敛批，不借已有Transition刻时全部销账 |
| 工单×任务数据面 | B-8/B-20与任务result消费按既定启动随行合并语义；C-21+是已升格核心能力，**近期单独圈定任务中心UX/结果批，不笼统扔回Phase7/8，也不擅自锁入Week0** | input mapping、result存储、output白名单及成功/失败状态映射跨主仓/TR/UI；只补run.succeeded事件不等于业务闭环；先明确本批四件范围与验收，再拆仓提交 |
| 通知基础债与H组增强 | B-3/B-4/B-5/B-13、G-7+G-29①在启动批随行；B-2/G-2/C-13分别并入H-3/H-2，**首条风险域通知前补H-5 owner解析** | inbox规则路由与用户opt-out两层同批；H-1按type_code、H-3字段变量/词表、H-4测试、H-6预览按真正消费者排。真实渠道须契约/凭据，G-9 ops站内收件锚点随该批；不能拿外发fixed成功证明inbox可达。G-29②全局actor排除已放弃，保持窄版白名单，不重启旧提案 |
| 事件可靠性C-20/outbox与G-6动作一致性 | 当前保留接受的进程内at-most-once；**丢一条即事故、需要重放或跨进程消费者时触发**，并与Phase9规划衔接 | R3若沿现事件面，其终态对账扫描必须同批落地，不能直接等Phase9；若无此兜底且丢失不可接受，C-20必须提前。reassign强一致/重试档先拍语义，不静默改原转换事务 |
| 当前小残留：G-9/G-12/G-14 | **最近同域小批，不等新触发**；不阻塞既有审计冻结/发布 | G-12的intel裸fetch已关闭，剩badge无效invalidate/60秒轮询、画布undo后lint态、附件刷新错误静默三个子项仍在；分别补有效失效更新、history事件联动与可操作失败反馈。G-14仅radio/dict收窄软提示，不影响G2提交强校验。G-9先定ops合法收件锚，随inbox规则/偏好批，不盲发全员；这三编号不是新增P1 |
| 量级/使用反馈/外部依赖型债 | B-9内嵌图引用、B-10索引、C-6/7/8/11/18、G-24/G-31等按台账具体触发 | 现有观测计数/SQL/用户反馈由维护者读取；用户反馈或容量信号出现即重新排序。已有索引/已修图标/GC不得重复开发；G-10日志表形态登记不修。手机/200%缩放/屏幕阅读器/全站axe在支持目标明确后另做，不冒充已覆盖或当前产品缺陷 |
| L-1–L-4目录搬迁 | **延后至以后明确允许合main、并满足台账触发后** | 本轮不搬deliverables、不扩引用改写；所有者已规定必要时点报告仍落deliverables，未来执行前先对齐现行规范和归属；历史24文件/36引用数不可当当前工作量。L-6已归一并补内容，不再开双份归一 |
| 07索引中已提入Phase6的专项 | 件18显隐求值同源、capability_registry及其它06/08/09事项按已拍批次逐项圈定 | 件18与件17/capabilities及表单批圈定优先顺序，按已拍Phase6随行，非新增全局R1门禁；原phase5过渡守卫方案已作废，不在本次擅自恢复。能力目录要求三步与utils执行SDK完整衔接，非只加表，命名/ActionMeta仍待批前定界；类型级可见性09的待拍项须在对应实施入口定界，而非默认已授权实现 |
| S3b审批/大屏/统计DSL/更远期能力 | 真实场景及Phase6–9既定顺序触发 | 不因审计收口把所有未来功能升成当前缺陷；同样不把已明确前提到Phase6的能力（如C-21+、capability_registry）放回遥远未来。具体能力按07索引分批，不能形成“全部历史债清零才开R1”的新门禁 |

来源：[Phase5统一台账](../../docs/phase5/S6-S5-debt-ledger.md)、[Phase6启动总账](../../docs/phase6/05-launch-batch.md)、[07挂账与触发索引](../../docs/phase6/07-backlog-and-triggers.md)。阶段日期/原工期是历史估算，不作为本次重新承诺的工期。完整101编号的旧映射与本次分组复核收据只存主仓ignored `.codex`，不新增第二套权威债台账。

### Go补扫的独立执行卡要求

1. 源码范围固定为主仓dc3、AL60、TR eb3、utils cd759（完整hash见本报告基线）；只读归档，不在原共享工作树安装工具或改go.mod/go.sum。官方工具独立固定版本/校验和，漏洞库记录抓取时点；不关闭sum校验，不替换工具内部依赖。官方精确依赖仍无法获取时，改在网络可用的隔离CI/已验证工具环境执行；不推main、不以受限计绿。
2. 生产构建维度`linux/arm64, CGO_ENABLED=0`；主/AL的实际ELF为Go1.26.6，TR为Go1.26.10。源码分析要使用各自目标工具链/构建标签，不用宿主1.26.6的标准库结论冒充TR。utils无独立运行ELF，库扫描须回到主/AL的cd759和TR仍v0.4.2的实际消费图。
3. 四模块source `./...`与三个实际ELF的`-mode binary`分开归档：源码调用链分析与二进制符号存在不是同一层证据；binary无调用栈，strip/buildvcs=false或模块版本未知时把边界写明，不能捏造业务主模块精确版本。扫描构建配置及这一区别见[官方govulncheck说明](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck)。
4. `-json`输出是流式对象，发现漏洞也可能exit0；必须完整解析finding、trace及扫描完成/错误，不能“exit0=零漏洞”。同次JSON若转text也须保留完整环境/数据库依赖。每个GO-ID记录受影响模块/版本、修复版本、入口/可达性、实际构建与处置；工具失败/仅取到DB index仍为NOT_RUN或FAILED，不能留空计零。该退出码约定见[官方文档Exit codes](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck#hdr-Exit_codes)。
5. 对确认可利用的安全/数据/门禁问题按项目P0/P1先修；仅模块出现但无当前可达调用链、或只在不部署构建中存在的条目单独说明，不因一个CVE数字无差别升级。不可达不等于永久豁免，升级/暴露面变化后重核。修复在codex分支，重建实际受影响产物并执行对应门禁/必要联合回归；发布状态另计。

### 建议执行顺序与本轮停止点

建议顺序：**Go补扫（与工具/E2E小批可并行）→回滚预检/操作说明→Wire/E2E及同域小残留批闭环→存储ID合同批→dbtx试点与L-5最小文档回填→Phase5收口并经授权合main后，再从main切Phase6按依赖实施**。Go扫描若发现高等级问题，插队修复；没有新发现则不扩大本次审计冻结范围。审计卫生先核实际部署再定实施，不把归档清理开关当无风险文案改动。

**阶段顺序已按2026-11-07所有者最新指示收束**：[00号执行前置](../../docs/phase6/00-flow-engine-v2.md)及[05号§6.1](../../docs/phase6/05-launch-batch.md)统一为“Phase5检查/审计/修改收口→后续经授权合main并核验→从main切Phase6”。允许提前规划，不再从phase5或审计短分支提前切Phase6。当前继续维护五仓Phase5修复，不执行main合并、正式发布或Phase6实施；未来整合要继承已验收修复并重核差集。

本次出口为排序、依赖、验收标准落档；剩余P2/P3的规划措辞不再开启无边界抛光。下一步优先执行Go补扫与上述小批，而不是重扫全部五仓、重做已修项或先推进目录搬迁。

## 工作区与分支收尾（2026-11-07）

范围仍为五个独立仓库：zhuzhao、zhuzhao-ui、taskrunner、activelist、zhuzhao-utils；工作树不是新增仓库。54个登记工作树逐一核对索引、跟踪/未跟踪和归属，清理只处理本轮自身状态。两处被正式修复替代的原型共12文件，完整内容/SHA与binary diff先存主仓ignored `.codex`，验证可恢复后再精确还原旧树；不提交过期原型。两个旧UI证据树只移除node_modules链接，目标依赖目录保留；其它ignored证据、日志、备份、构建产物和运行环境不按“清理”批量删除。

五仓本轮最终修复统一使用同名短分支 **`codex/phase5-audit`**；它是Phase5审计/修复分支，不是Phase6或生产发布分支。原修复来源分支与本报告固定hash保留。主仓/UI原phase5的已完成文档、CI及忽略规则提交也纳入推送核对；不把未包含全部修复的原phase5源码当成已验收修复组合。UI既有.VSCodeCounter忽略项规范保留，四兄弟仓补.codex的Git/Docker排除，过程材料不入库。

主仓/UI的现有CI增加codex/** push过滤，UI按同名主仓分支读取种子；其余Go仓本已全分支运行。push前逐仓SSH fetch、提交格式/门禁状态/窗口核对；主仓同名分支先于UI推送，远端ref与准确SHA的CI必须另核。分支推送不等于正式发版：本轮**不新增tag/GitHub Release/生产部署**，utils已发布v0.4.3及本轮固定pseudo-version保持，正式main/tag发版待以后整合与安全证据决策。Go扫描仍NOT_RUN，本次清理未补扫。

最新阶段顺序由所有者明确：继续以五仓Phase5检查、审计、修改为主；允许Phase6规划，实际须先Phase5合main并核验整合基线，再从main切Phase6。当前不执行main合并，不创建Phase6分支。原10-28“先从phase5切”的历史记录保留作演进证据，不再作为当前执行许可。精确工作树状态、归档清单、source→branch、SSH fetch/push窗口及CI收据仅存本地ignored `workspace-cleanup-20261107/`，不新增第二套设计或债台账。
