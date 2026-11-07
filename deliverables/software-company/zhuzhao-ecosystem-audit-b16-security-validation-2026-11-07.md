# 生态全面审计 B16：Phase5安全验证（2026-11-07）

> **执行中**：本批先补安全证据，再分批修复。UI生产依赖审计本轮无已知漏洞报告；四Go源码与三个真实ELF的本轮结果待真实扫描输出，不借普通CI绿宣称安全通过。B16-F01/P2已完成专项修复与反向变异，整仓门禁进行，尚未关单。本批无main合并、main PR、Phase6实施或正式发布。

## 范围与基线

五仓全部纳入：zhuzhao、zhuzhao-ui、taskrunner、activelist、zhuzhao-utils。开工时五个已验收审计头分别为cc8fd4ae4e2a4b91f77886d56f4ef6c38f1a81ae、51d5e9a2524c17e60536240913e341f05cddc5f5、21352322766c26ae9a35a45e3798f6155d6d3acd、50314a7dd0e77b9c27a2ef3f4cd1594cceb77b49、481c7ec3dc53c77cb23ac033985ea78593b0ca9c；与旧运行固定源的非文档/CI/忽略规则内容等值证据沿用并重核，不将头hash变化误说成运行产物已更新。

本批将主仓、UI实际Phase5与审计候选正常合流：主仓`09d224dca1fdeaf9e2e70e8bf7dd5aaedd3ee31b`，UI`01c5deae23e677853316f1ee2dd692c708246651`，两结果树分别逐path/object/mode等于原候选，保留双亲历史。**后续主仓/UI修改基于实际phase5**；三个Go兄弟没有常驻phase5分支，仍在Phase5用途的`codex/phase5-audit`修改，不触及其main。本操作不是准备main合入。

四Go源码扫描首个CI控制提交为`191d6e5efb3dd92d74f9943d803fd3f97f605f23`；兄弟源钉上述三个完整SHA，目标linux/arm64、CGO0、无test tag/replace、mod readonly。真实运行ELF分别仍为主仓/AL Go1.26.6、TR Go1.26.10，逐SHA核对通过；utils没有独立运行ELF，需回到主/AL当前pseudo-version和TR实际v0.4.2的消费图。

## B16-F01 / P2：Secure配置行位于错误命名空间

基线主仓09d224d（与前固定dc3生产内容相同）：[configs/config.yaml](../../configs/config.yaml)及[deployments/config.test.yaml](../../deployments/config.test.yaml)的`file_cookie_secure`位于`audit`段；实际消费是[config.go](../../internal/config/config.go)的`StorageConfig.FileCookieSecure`、`Load`及`storage.file_cookie_secure`绑定，最终由`SetFileSessionCookie`消费。Ruby YAML真实解析确认`audit.file_cookie_secure=false`而storage对应键缺失，不仅是注释或rg猜测。

当前dev/test是HTTP且缺省false，本批未证明泄露或绕过；`STORAGE_FILE_COOKIE_SECURE`环境覆盖和Secure setter已有实现。问题是编辑模板该行true不能到达真实Storage字段，按安全配置合同偏差P2，不升P1或推断生产已受影响。

修复仅将两行移入各自既有`storage`段，仍为false；不新增重复storage、不改enabled/端口/密钥/运行配置，不启用Secure或归档。新增[file_cookie_secure_template_test.go](../../internal/config/file_cookie_secure_template_test.go)经真实模板→`config.Load`→StorageConfig→Cookie头验证默认false、只改模板true、env true正控与env false覆盖；包外测试隔离Viper和环境，串行执行。

专项证据：最终测试SHA下，原两模板各8/8精确失败于Secure=false/want=true，其余6正控绿；修复8叶race串行8轮绿；ignored副本逐模板移回旧层各8/8红，其余正控保持绿，恢复后绿。config+middleware完整race单测、vet/gofmt/diff通过，Ruby语义比较确认除键路径外所有值不变。初版测试误判既有storage.enabled导致控制夹具红，已保留并剔除，最终测试全重跑；没有把夹具错误算产品发现。**完整lint/unit/integration/cover/acceptance尚待，修复提交与最终复验稍后补，不借旧门禁关闭本项。**

## Go工具与扫描证据状态

本机固定官方工具获取在约5分钟预算后停止：精确依赖ZIP不完整、h1未闭合，未产出可执行文件，未运行source或binary扫描；失败与partial保留，未替换依赖/关闭sumdb/改全局配置。后续改用[go-security.yml](../../.github/workflows/go-security.yml)在隔离runner采集四模块源码，官方v1.4.0工具及实际依赖h1独立校验；另交叉构建Darwin arm64官方工具并保存SHA/构建信息，供本机对三真实ELF扫描。

[parse-govulncheck.py](../../scripts/parse-govulncheck.py)完整解码JSON对象流，区分执行失败、findings待分级及限定配置无发现；source调用链与binary符号存在分开，binary Go版本按SBOM核。缺module frame仍保留待分级；不能按JSON exit0、仅有OSV元数据或CI采集成功宣称零漏洞。[离线回归](../../scripts/test-parse-govulncheck.py)20/20通过，覆盖截断/非法JSON、缺config/SBOM、源变化/错误Go/目标及exit0有发现等假绿路径。该workflow是证据采集，不是自动安全批准/升级或发布授权。

## 前端与部署安全差集

- UI本轮`pnpm audit --prod --json --registry=https://registry.npmjs.org` exit0，各级已知漏洞报告均0；metadata dependencies233/optional96/total329，锁文件和package.json前后SHA不变。结果来自51d5e9a，Phase5合流树与之完全一致，不扩大为所有前端输入安全认证。
- 登录已有12处显式`LogLogin`，logout位于authed AuditLog链；只读实际库存在login成功/失败及logout记录，request_id非空，旧“零审计”不能转成重复开发任务。部分前置拒绝/设备校验/内部错误早返仅列覆盖边界，未据此报新P1。
- 当前dev/test未启自动归档；TR只有manual/disabled cron，目录无归档文件。test及生产Compose模板缺归档持久卷；必须先核持久化、JSONL备份恢复与保留/删除策略再考虑启用。源码fsync后删DB批次的既有保护不等于已完成部署恢复；PGdump或AL/TR逻辑恢复不能代替主仓JSONL恢复。未启任何删除开关、未证已丢数据，不凭此新报P0/P1。

## 本地分支归档与恢复

56个本地heads逐项复核后保留10个活跃分支：主仓/UI各main+phase5；三个Go兄弟各main+Phase5审计短分支。46个旧head先复制到`refs/archive/phase5-audit/20261107/…`，五个完整bundle的SHA/verify/list-heads均闭合；21个祖先用branch-d，25个人工承接来源未谎称merged，在历史已完整归档后按预期tip CAS移出heads。40旧工作树仅同hash detach，54目录、ignored证据/备份/原型均保留，未删远端分支/改main。恢复可由archive ref或完整bundle重建旧branch；归档不是历史丢失。

## 记录与后续出口

原始命令/JSON/配置解析/只读DB聚合、分支逐ref表、归档bundle与恢复说明仅存主仓ignored `.codex/audits/ECO-20261104/security-20261107/`。正式结论入本报告及项目控制台；分批修复按发现编号/固定hash/原与变异证据记录，完成必要门禁后才关单。四源码/三ELF逐目标待补PASS/发现待处置/LIMITED，工具与DB时点、真实构建版本不缺省为零。本批不涉及稳定性下一批或Phase6实现。
