# 五仓文档归位与内容校准 — 2026-11-08

> 状态：五仓文档归位、内容与格式复核、本轮必要门禁均已收口。首轮超时/中断与默认环境门控未覆盖项保留在下表；未推送、未合main或部署。

## 1. 范围与决定

所有者选择“五仓各自docs/、统一结构”。本轮以主仓phase5的f3630f93、UI phase5的bf61c863、taskrunner审计候选b51d224f、activelist审计候选2ebafca1、utils审计候选46a5605d为迁移前基线。未推送、未合main、未打tag、未切Phase6或更新共享运行服务。

迁移前清点225份受控Markdown，先逐份理解/分类，再合并迁移。根README、AGENTS及法律文件保留其入口/机器/法律用途；既有本地doc/保持原样。统一分类与状态规则见[文档维护约定](../../documentation.md)。

| 仓库 | 维护入口 | 主要归位 |
|---|---|---|
| zhuzhao | docs/README.md | 部署/备份/运行手册、错误码登记、报告与规划；保留模块和阶段职责 |
| zhuzhao-ui | 本仓docs/README.md | 贡献/发布/部署、hooks与请求层长说明、前端规范、13份正式报告 |
| taskrunner | 本仓docs/README.md | 当前执行/回调与API、部署/备份、ADR及必要演进历史 |
| activelist | 本仓docs/README.md | 当前PG模型与契约、运维；Mongo旧设计和旧实施记录明确归档 |
| zhuzhao-utils | 本仓docs/README.md | 从长README提取包使用、API边界、版本与发布维护指南 |

各仓只创建有内容的分类，不造空阶段。主仓Phase1–9原本均有README，本轮统一其状态说明、阅读顺序和当前/历史边界；普通文件/目录统一小写连字符。主仓旧VISION、ADR文件名及债务台账归一大小写，决策ID不重排。

## 2. 保留、合并与停止跟踪

- 主仓52份报告/研究/旧索引归位：40份正式日期报告入review/reports，10份规划来源入proposal/secops-platform，旧overview/执行卡入review/archive。控制台日期流水抽离，能力矩阵、§4迁移地图及§6/§8入口保留。
- 主仓现行runbook与部署/备份说明统一到ops，根api错误码正文归入docs/api；旧指针和两份过时运维示例被现行权威替换，原件另留本机及Git历史。
- UI与兄弟仓保留有独有决策/失败/验证价值的报告和旧设计。长历史不是当前操作手册；当前合同抽出后回链历史，不把Mongo、Watcher、任意callback URL或未来引擎当作现成功能。
- 各仓Git/Docker同时忽略doc、旧deliverables、outputs、audit-reports与.codex。本机阅读ledger、逐项去向map、原始证据、完整旧文与SHA只留ignored过程目录，不再进入受控文档索引。

历史报告保持其原基线、失败、NOT_RUN、40位hash及固定SHA链接。B12中的旧根api登记与docs指针仍按原时点区分，不能把今天的新路径反投到旧缺陷描述中；现在可点击的导航则指新目录。

## 3. 内容校准与阅读体验

模块入口改为“职责 → 实际调用链 → 权限/数据 → 失败边界 → 排查”，清理重复过时DDL和不可复制伪代码。补开发导读、按任务索引及11张Mermaid图，解释五仓边界、请求、菜单策略、审计、组织移动、存储和消息等链路。

核心校准包括：JWT四项Redis检查与PAT边界；HTTP审计/权限判定日志不同可靠性；活跃用户部分唯一索引；组织角色来源和scope/委托分离；菜单只读/B案；Move真实校验和级联范围；网关全局1MiB限体并不因跳审计捕获而放宽；UI真实ready、列表解包和专属测试账号。

阶段与提案保留已批准决策、历史变更和后续范围，集中披露未实施或未定约束。例如HR真实同步、风险指纹DDL校准、转换级完整UI、逐事件通知“应发/实发”对账不能虚标交付；本轮不替业务拍板或实现这些能力。

## 4. 实际消费者与兼容性

Swagger三份生成物保留原路径，Go导入和UI codegen输入不变。错误码AST对账测试改读新登记路径；CLI帮助、脚本输出及跨仓注释跟随新路径。UI15个源码文件仅文档引用、注释或测试描述变动。没有依赖、SQL迁移、权限/API行为或运行配置值变更。

本轮对当前Go候选重新构建linux/arm64运行二进制并核源码manifest、镜像ID和容器内ELF。首次本地构建包装器把image ID误作FROM名称失败，原日志保留；改用已核精确ID的本地tag后重建并核基础层和运行配置一致，不将初次失败算通过。

## 5. 验证证据

| 检查 | 本轮结果 |
|---|---|
| 五仓文件/锚点、命名、H1、围栏、目录README、散落Markdown | 274份Markdown、2532处本地文件/锚点引用检查零问题（主192/UI30/TR21/AL22/utils9） |
| Git忽略与Docker实际引擎合成context探针 | 五仓Git规则与Docker实际导出探针均通过；AL原有策略另排除整个docs，未擅自放宽 |
| 11张Mermaid | 全部解析、离线SVG/PNG渲染及独立目视检查通过 |
| 主仓lint / unit / cover | 通过；覆盖率20.4%，未变化 |
| 主仓integration | 首轮仅用户安全状态测试超时；未改源码/超时的定向复验通过，完整串行make test-integration 580.115秒通过，0 skip/failed，源码无漂移 |
| migrate-up / 原样acceptance | 自有隔离栈81 clean；253通过/0失败/0跳过，81对往返零漂移 |
| UI原样pnpm gate | 六件全过；241单测、41 E2E、0 flaky/skip/实际重试，阶段内HTTP 429为0 |
| 三Go兄弟原Make目标 | utils lint(含test)/build、TR lint/test/build/默认e2e、AL lint/test/test-integration/build均通过；TR默认E2E为18断言通过/0失败 |
| 隔离环境回收 | 联合验收8容器、专属卷/网络已精确回收，共享环境零变更 |

首轮integration的四个子场景共用90秒context，最后reset/admin_profile在总耗时96.48秒时超期。现场同时有多仓空缓存编译，记录到Load80.51、CPU无空闲和内存压缩压力；源码/断言未变。停止本轮额外编译后，原失败路径以未改超时的fresh执行通过。该关联是资源压力归因证据，不能据一次复验宣称消除所有flaky；原失败与中断均保留。完整串行门禁随后580.115秒通过，四个原失败相关子场景全绿；没有通过延长测试期限掩盖初红。Docker忽略探针首轮错误地要求AL导出docs/README，实际AL既有规则排除整个docs；改用独立允许sentinel验证后五仓全过，这是探针预期校正，未修改该策略。

未覆盖：TR默认make test在没有专用PG/Redis连接时跳过5个PG场景、callback panic lifecycle及delivery-fence的环境门控子场景；默认E2E为专用SQLite+Redis链，可选cron未运行，不能代替上述PG专项。主仓unit/integration及UI E2E无skip；AL/utils无新增门控遗漏声明。

业务门禁针对精确Go/依赖及UI运行源码快照；后续仅文档导航与说明收尾，另以最终静态/差异校验绑定。不能拿旧审计报告代替本轮结果，也不把本地门禁说成远端CI或生产部署验收。

原日志、manifest、各文件阅读/处置与独立复核收据位于本机ignored `.codex/audits/docs-reorganization-20261108/`。迁移前五仓候选已由此前无前置依赖bundle保全；本轮不清原分支、原卷或旧证据。
