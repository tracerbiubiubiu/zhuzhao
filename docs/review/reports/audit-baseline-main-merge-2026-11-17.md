# 存量审计修复三仓推送与main合入记录

> 状态：**三仓功能PR已正常合入main，准确head的分支/PR CI及功能main CI均成功**。合并树与已验head完全一致，未使用admin bypass，未部署。后继状态文档的提交与适用检查另由同批Git记录及收据定位。
> 本页只记录远端承接与后继状态；[本地修复与组合验收报告](audit-baseline-repair-2026-11-17.md)保留原时点、冻结输入、首次失败及验证边界。后继材料ID：`maintenance/audit-baseline-main-merge-20261117/`。

## 1. 固定修复范围与消息修正映射

原三笔fix提交的路径数为主仓166、UI30、TR36，共232文件；本轮另外修正2path的CI自测夹具与在途说明，后继收尾更新17份Markdown（主仓12/UI4/TR1）；不扩大业务范围、不增加价格、DAG或剩余工程日，也不改原冻结来源hash。

- 主仓：`5768faebea19c80d966fd1d23b56a2012f70f6ee` → `a2632efa596878acdaecdf40e35c713db715dcbc`，分支`codex/iam-baseline-repair-20261117`。
- UI：`1870d89fd8e4d5e172f8a7b462917dfcc5d10d05` → `6ba9a1a2a920b4699edb825042d82b5cf28a49a1`，分支`codex/iam-ui-repair-20261117`。
- TR：`790565ebdc13c1ff6a9a9a56b0436d1ca497abe0` → `83ed6e902841df9659fa9f50cefc1b2b40d5570f`，分支`codex/task-chain-repair-20261117`。

三仓均仅补提交消息的变更说明、同类扫描、Refs及兼容性footer；每仓tree与parent完全不变，受控树干净。修正前确认远端main无新增、目标远端分支不存在、索引为空且HEAD归本会话；修正后才推送。以上映射承接相同源码的本地门禁，不能替代新head对应的远端CI结果。

## 2. 推送前复核与首次工具失败

首推主仓`a2632efa`时，受控1029文件逐一匹配收尾manifest；相对已测试r2只有23份Markdown状态差异，业务、配置、依赖、迁移和产品测试零漂移。该时点运行验证索引的453份原始证据SHA复核一致，三仓1578个受控输入无漂移；此结论不覆盖下述后续CI夹具修正的新head。本地门禁沿原报告：主仓七项完整门禁、253验收、82对迁移一次往返零漂移及默认单测语句覆盖率20.5%；UI322单测/45真实E2E；TR226测试通过，另2个父测试实际启动的子进程helper在顶层按设计跳过，cron E2E20通过。

首次推送前检查调用全局swag，二进制由Go1.26.6构建，解析Go1.27标准库`math/rand/v2`泛型方法语法失败，退出1；受控生成物未改动。随后使用仓库钉住的Go1.27.4构建swag v1.16.6，受控工具安装、commit-lint、wire及diff检查均退出0，没有跳过生成器或调整产品源码。该工具失败与修正原样留新材料，不回写或覆盖原验收封存。

### 远端CI首失败与后续core重验

首轮远端CI发现并经本机复现：`scripts/test-migration-template-preflight.py`的旧自测仍把82当作unknown，但本批已新增000082，真实预检实现按迁移目录正确接受82。root仅将该测试夹具的unknown编号改为实际down迁移最大编号加1，并补catalog增长、known clean、unknown与dirty对照；Go、业务配置和迁移本身不变。

该修正独立提交为`42b47261a4d4105b802d43193180cdeb93f7aea7`（父`a2632efa`），原232文件口径不回改。首轮失败run为[PR 44044359257](https://github.com/tracerbiubiubiu/zhuzhao/actions/runs/44044359257)及[push 44044119949](https://github.com/tracerbiubiubiu/zhuzhao/actions/runs/44044119949)：只有lint内的旧夹具失败，quality聚合随之失败，其余产品检查通过。修前本机12测试复现1失败；修后13/13，独立目录82/增长83/回退82/81四形态均13/13，恢复旧断言仍失败。实际预检实现及原FK、dirty、只读SQL、秘密保护断言保留。

新冻结`repair-merge-20261117/runtime/r1-ci-fix`的1578文件manifest SHA为`1c14025e321f80b68c32f661fbb28a0bd4ed2fb2e9fa16a5ef32631d5dd7cc63`；仅CI自测脚本和控制台Markdown两处变化，Go产品/测试、配置、迁移、UI/TR/AL零漂移。42提交逐文件等该快照。新专属release骨架重新取得core七make全0、253验收0失败/0跳过、82对一次全链迁移往返零漂移，unit/integration0跳过；CI精确Python命令另跑13/13，不能用make lint替代该独立step。

本轮普通单元语句覆盖率实测**20.4%**，旧r2的20.5%保留历史，差0.1个百分点。Go源码、插桩块集合与权重相同；2个覆盖状态块相差4条语句，均在`internal/pkg/audit/policyeval.go`，不把其余原始执行次数变化等同新覆盖缺口，也不臆测未单独核验的时序原因。新三Go Linux二进制与原产品字节相同；UI45真实E2E/TR全量及S3/JSONL/实际PG恢复沿原准确来源，不重复执行或冒称新跑。 合同合入后又以实际功能main的Swagger执行UI `pnpm codegen`及`pnpm typecheck`，生成类型SHA前后相同、消费工作树干净；该项证明契约再生零漂移，不冒称新跑UI六件。

新owner=`zz-repair-merge-20261117`的8容器/7卷/3网络已精确回收，宿主/owner资源零残留、7端口可bind；共享15运行容器/13卷/3网络身份零变化，无镜像prune。旧owner收据及旧封存均保留。

## 3. 三仓功能PR、CI及main实证

以下是功能合入时点的准确快照；后继说明文档不反写该merge SHA。检查中的不适用job依分类跳过，所需job与quality聚合均成功。

### 主仓

- PR：[10](https://github.com/tracerbiubiubiu/zhuzhao/pull/10)；已核head：`42b47261a4d4105b802d43193180cdeb93f7aea7`。
- [pull_request CI 44049035143](https://github.com/tracerbiubiubiu/zhuzhao/actions/runs/44049035143)（success）、[push CI 44049026451](https://github.com/tracerbiubiubiu/zhuzhao/actions/runs/44049026451)（success）。
- 合并提交/功能main：`edb2a5a32e8a5985013754a19c4b2e4135a747bc`，时间`2026-11-17T08:59:36Z`；第一父为`822346fea620d934d084d27c77cf421dca6a3327`、第二父为上述head，merge tree与已验head逐Git tree完全一致。
- [push CI 44051468750](https://github.com/tracerbiubiubiu/zhuzhao/actions/runs/44051468750)（success），head精确为该merge SHA。
- 适用检查：changes、commit-policy、document-check、reference-check、lint、test、test-integration、Current source and Linux binary vulnerability scan及quality-gate；helper-security本候选不适用。


### UI

- PR：[7](https://github.com/tracerbiubiubiu/zhuzhao-ui/pull/7)；已核head：`6ba9a1a2a920b4699edb825042d82b5cf28a49a1`。
- [pull_request CI 44044370323](https://github.com/tracerbiubiubiu/zhuzhao-ui/actions/runs/44044370323)（success）、[push CI 44044142243](https://github.com/tracerbiubiubiu/zhuzhao-ui/actions/runs/44044142243)（success）。
- 合并提交/功能main：`e93d8e0c580ce5f71876b4bd774665b49ebff68e`，时间`2026-11-17T09:00:06Z`；第一父为`8c4abadd9f86e4c8d1e8e9d90eb71b281e1f2d1a`、第二父为上述head，merge tree与已验head逐Git tree完全一致。
- [push CI 44051544862](https://github.com/tracerbiubiubiu/zhuzhao-ui/actions/runs/44051544862)（success），head精确为该merge SHA。
- 适用检查：changes、commit-policy、document-check、lint、test及quality-gate；无参考项时reference-check不适用，真实栈E2E以本地45项独立收据为准。


### TR

- PR：[8](https://github.com/tracerbiubiubiu/taskrunner/pull/8)；已核head：`83ed6e902841df9659fa9f50cefc1b2b40d5570f`。
- [pull_request CI 44044386963](https://github.com/tracerbiubiubiu/taskrunner/actions/runs/44044386963)（success）、[push CI 44044176569](https://github.com/tracerbiubiubiu/taskrunner/actions/runs/44044176569)（success）。
- 合并提交/功能main：`077adc573d3f3e2e8d1128b473526d27dfb64062`，时间`2026-11-17T08:59:12Z`；第一父为`7dc40e05e0d1cc633104d33b40915dddd2d958bb`、第二父为上述head，merge tree与已验head逐Git tree完全一致。
- [push CI 44051414006](https://github.com/tracerbiubiubiu/taskrunner/actions/runs/44051414006)（success），head精确为该merge SHA。
- 适用检查：changes、commit-policy、document-check、test及quality-gate；test含实际PG/race、cron E2E和源码/二进制govuln，无参考项时reference-check不适用。

## 4. 后继文档、材料与未交边界

后继状态文档在三仓具名短分支`codex/audit-baseline-merge-closeout-20261117`准备，先将准确17份自有草稿逐SHA备份，再快进到上列实际功能main并重放，只改Markdown。当前控制台、计划、模块/运维及UI/TR入口承接主线事实；原本地验收报告只加页头后继指针，不反写旧状态、首次失败或旧覆盖率。

该文档批按实际reference/document分类完成quality-plan/check、文档/针对检查与提交规则验证，其PR、文档main和最终本地同步/归档收据按同批材料ID及各仓Git记录定位，不在本页循环写自身提交/merge SHA。文档检查不冒称重新执行产品全门禁；三功能main的完整CI另有上列准确run。

新材料ID`maintenance/audit-baseline-main-merge-20261117/`保存消息映射、推送/PR/CI/合并、提交与来源复核及本次状态文档收据；旧`maintenance/audit-baseline-repair-20261117/`封存不改。原始日志、凭据、私有地址和本机路径只在受审本地材料中，Git只保留必要摘要、准确提交及公开PR/CI链接。

本次不关闭全部审计发现。完整IAM-C/C05、C03/F2b、HA、真实UAA/企业消息、完整SLA及未取用P2/P3仍按原包推进；真实业务启用仍需绑定人员、数据、支持责任、维护批准及恢复参数。单实例限制保持，同机逻辑恢复与代表对象/JSONL恢复不等于异机、PITR或跨介质一致快照。合入main不表示生产部署、共享环境更新或真实业务已经启用。

## 5. 临时工作树独立收尾

此前三仓合入后，主目录虽干净，仍遗留8个旧实施/评审工作树，共395项未提交状态；这是工作区收尾遗漏。逐项比对中261项与最终main同字节，85项匹配旧归档历史字节，49项未匹配旧归档完整文件SHA。后49项归并27路径、34个草稿版本，源码/合同差异复核未发现遗漏实现；唯一额外的`internal/service/task_casefold_review_test.go`场景，已由main的`internal/service/task_json_http_repair_test.go`中`TestTaskJSONContractRepair_CaseAliasesCannotChangeAuthorizedDefinition`覆盖并扩为3例，均验证拒绝且零下游写入，不将旧探针重复提交产品。

按所有者后续“收尾”指示，先保全全部8树文件、Git状态/索引和binary patch，另保全5个已合并干净工作树及三仓15个本轮分支的Git bundle。共33,698个普通文件、11,320个唯一SHA对象；原8树无排除，另5树仅排除两个UI根`node_modules`依赖安装目录，`dist-pro`、taskrunner `bin`和全部本地执行材料仍保留。独立复验核对源字节/模式、bundle引用、inode隔离和权限，并实际从bundle还原全部13树：395项原始状态逐字节一致，其余5树仍为零状态。

首次删除遇到Go依赖缓存目录0555权限，Git部分移除首树后拒绝删除；先确认残留文件是已验证清单子集且内容/模式不变，再仅将明确目标的所有者目录加写权限，不跟随软链接、不改文件模式。后续清理13个临时工作树与15个本地分支，分支均核可达远端main；UI因活跃主目录仍在旧main，使用准确tip的CAS删除其已合并临时引用，未移动主目录。三仓6个已合并远端功能/文档分支按已核PR和准确tip清理；本段说明自身提交及PR分支的后继状态由Git/CI收据定位。

清理前确认目标未被进程句柄或容器挂载使用，保留现有开发服务、UI活跃主工作树及最新main独立检出；更早分支及其它会话材料不在范围。材料ID`maintenance/audit-worktree-closeout-20261117/`保存保全、首次失败/恢复、实际删除和文档门禁收据，旧两批封存SHA不变。此轮无产品源码、接口、权限、依赖或迁移改动，按实际参考档运行文档和规划针对检查，不重跑或冒称新跑产品全量门禁，覆盖率沿前述实测。
