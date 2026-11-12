# 开工前五仓版本维护

日期：2026-11-12。状态：五仓维护候选已通过本地验证并收口；未推送、未合入main、未切换共享环境或生产。本报告只覆盖版本维护，不代表Phase6业务或全路线功能已实现。

本轮在五个独立`codex/prephase6-version-upgrade`工作树进行，承接已落档规划和Phase5源码。原main及规划工作树不写入；源码、SDK、锁文件、实际镜像/ELF与测试结果分别绑定，不拿旧Phase5收据证明新组合。原始证据在本机ignored审计轮次`version-modernization-20261112/`，长期结论在本页及各仓报告。

## 变更与边界

- 三服务采用Go1.27.4；utils最低1.26.11、推荐toolchain1.27.4，两SDK分别验证。服务保留可公开获取的原utils依赖pin，不写未推SHA或local replace。
- UI采用Node26.13.1、pnpm12.18.0、Vue3.5.45、Element Plus2.14.8、Vite8.3.8；最终40个直接依赖中20项锁定版本变化。minimatch修复采用11.0.7，保留既有发布年龄策略，没有增加豁免。
- PostgreSQL18.6-trixie、Redis8.2.11、Alpine3.24.4、Nginx1.30.6、RustFS1.0.5；RedisInsight保持当前3.8.0并补digest。公共镜像固定patch与OCI摘要，核对Linux amd64/arm64 manifests；本机实际运行验证为Linux arm64。
- pgx5.13、go-redis9.23、testcontainers0.45、miniredis2.40、x/crypto0.59等按修复/兼容证据更新；AL的集成测试压缩依赖确有可达公告，补到compress1.20.2。没有整体`go get -u`或把全部组件强升major。
- `migrate4.20.1`和`rclone1.76.0`使用固定官方源码重新打包，不维护fork或自写替代品。两者Go1.27.4、保留符号与许可证；migrate仅PostgreSQL/file，rclone保留工具能力并修复crypto依赖。上游OCI仅作对照，Compose运行受控本地构建。

Go业务实现、API、权限码、数据库schema和81对迁移均未改变。Go测试夹具更新镜像、端口就绪与可靠清理，AL补新pgx接口需要的测试桩；TR E2E只清理本次独立容器。UI的`src/`、`e2e/`、`scripts/`零改动，mock及既有合同保留。配置/CI/锁文件、版本说明及测试基础设施是本次影响面。

[组件组合](../../../deployments/component-lock.json)记录公共摘要及本地工具构建；[升级设计](../../design/dependency-modernization.md)、[数据恢复契约](../../design/component-upgrade-data-contract.md)、[工具构建](../../../deployments/tools/README.md)给出处与操作要求。

## 本地验证证据

主仓执行原`make swag`、`make wire`、`make lint`、`make test-unit`、`make test-integration`、`make test-cover`、`make migrate-up`与`make acceptance`。生成物无漂移；233个默认标签顶层单测通过，531个integration标签顶层测试通过（包含前述单测，不相加），无race、无skip。四档验收首轮253、最终幂等重跑252，81迁移往返通过，FAIL/SKIP均0。最终种子已处于admin12345，少一次首次强改密PASS；该分支已在首轮253验证，逐标签比对无其它缺项，未重置种子制造计数。最后夹具统一PG18/Redis8后完整门禁再次通过，代码/脚本/锁/配置输入hash无漂移。

UI干净安装、frozen replay、peers、lint、typecheck、31文件244单测、生产及含dev审计、build均通过。最终Node/Nginx Docker镜像构建、深链/静态缓存冒烟通过；原30 spec/42 E2E直接运行于最终Nginx生产产物和真实新app/AL/TR/RustFS组合，42首轮通过、0skip/0flaky，无实际重试。全量样式和已登记的Phase6前端问题不由这些42例代销。

AL最终真实PG18集成race为196个测试/子测试通过、0skip；TR真实PG/Redis为135通过、2个子进程哨兵skip，PG/SQLite路径均执行；Redis8/SQLite E2E含cron为19过0败；utils两SDK各142通过、0skip。三仓源码含项目测试及两服务实际ELF均通过默认text扫描。AL安全补丁重建后的ELF与UI已验证版本逐字节一致，SHA256为`6e747cc90a0690f7486026f6af620820492b5e67ce29177bbb0ec4ae52adc93c`。

主仓源码（含integration测试）、实际三服务ELF，以及两个受控工具的真实main源码/实际未strip ELF均由官方govulncheck1.9执行。JSON仅是原始证据，不能据exit0放行；当前CI使用默认text并严格保留失败退出码。UI生产与开发审计均无已知公告。此处不等同整镜像OS/SBOM、反射运行路径或生产环境的安全审批。

## 数据与恢复

- PG15→18：fresh18全部81迁移、39表行/序列/NOT NULL、约束与索引语义一致；归档恢复造成的4处数组cast反解规范化逐项保留原差异，并以值域/NULL真值和非法值CHECK验证。PG15原格式备份在空PG15中恢复通过，ltree、关联/附件join与新序列写入通过。
- Redis：RDB与AOF两路6.2→7.4→8.2；真实Asynq任务ID/载荷、pending/scheduled/retry/archived/completed、重复ID、会话/撤销epoch/TTL、刷新Lua只消费一次、停机过期、新写重启及旧6.2备份回退均通过。未FLUSH共享库。
- 对象存储：1.0.1停写副本→1.0.5读旧对象/metadata，新对象、11MiB multipart、预签名、第三空实例备份恢复和旧卷回退通过；原`rustfs-mirror.sh`覆盖/删除的归档及恢复通过。最终自打包rclone再次跑原脚本验证。
- core与AL分别用各自精确WAL命令归档，`archived_count=1/failed_count=0`；离线pg_checksums为Bad0。保持core“存在则失败”和AL覆写策略差异，不把成功复制一段WAL称为完整PITR。
- 最终自打包migrate在专属PG18执行81次up→全部down→81次up，schema一致。所有数据为合成副本；不据此宣称已验证真实企业数据规模或零损在线回退。

PG18挂载父目录`/var/lib/postgresql`并使用新卷；Redis也使用新版本卷。已有环境必须先停写/备份/恢复/校验再切换，新空库不是旧库升级成功。不能让旧镜像读取新格式卷；切换后新写入需要另行对账/补偿。原六个共享容器的ID/image/start/mount/running指纹已复核，未由本轮替换。停下的专属数据卷/备份保留作复核证据；这不是共享数据内容逐字节不变的承诺。

## 独立复核、失败与修正

保留首轮资源拥挤造成的短时限黑盒超时、缓存克隆中断及私有partial缓存校验失败；校验指出唯一不完整目录后重新取得并完整verify，未使用未经校验缓存。收敛并发后六组工程黑盒通过，没有加大这些检查超时或跳过断言。数据探针的临时Unix socket就绪、旧fixture列名、TR审计URI编码、Nginx探针错用后端health前缀等均保留原失败并修正探针，不冒充业务缺陷。

独立复核确证新CI曾只看JSON退出码、版本checker曾漏DockerHub别名/锁元信息不自洽；已改默认text与严格判定，并加入别名、未知repo、tag/index和本地源码锁反例。Testcontainers0.45双日志后端口映射仍可能迟到，AL已复现并补监听端口等待；core同类夹具一并补齐后重跑。全仓运行引用复核还发现一个用户安全集成夹具仍钉Redis7，已统一8.2并纳入版本checker，再跑完整531集成标签测试。最终重复验收的统计器原误把253写成恒定值，已按原login幂等分支确认252，保留原断言失败并另存判定；不把计数器失败误记成业务失败。最终全仓Actionlint还检出UI原CI把PR分支名直接拼入shell，已改step env数据传入；8个Git合法恶意分支、参数逐字传递和旧脚本正向对照均通过，运行时构建输入未变。G03/P6.0将工作流静态检查与此类脚本回归纳入常规门禁。没有为本地WAL UID假设添加无证补丁：实际root→gosu及两条归档命令均成功。

旧上游helper的stripped扫描会退化为模块级已知符号展开，不能把大量symbol帧当物理符号/运行可达，也不能把JSON0当清零。因此采用受控重包和真实main源码/保留符号ELF复核，旧镜像仅保留比较记录。

## 覆盖率口径与剩余窗口

同一Go1.27.4 `cover -func`口径，旧profile20.4%、本轮20.2%；不把它称为覆盖率提升。同一字节的router源码分别插桩，SDK1.26.11为237语句、1.27.4为214语句，恰好复现少23条，且两轮router未覆盖均16条。另policyeval少命中1条，未强归因SDK或删测。默认profile不含integration标签和外部253/42旅程；G04/P6.1固定该分支场景及统一profile基线，不把Go版本间百分比直接作业务覆盖结论。

TS7受OpenAPI生成链的5.x peer约束，暂留5.9.3；Cropper2需组件API改造，暂留1.7.0；Element Plus插件0.11引入未使用且peer不满足的Nuxt链，暂留0.10.0。U02/P6.1评审工具链兼容，个人资料改造P6.3复评Cropper；安全公告提前独立处理。完整Go模块目录中未进入生产/当前测试包的告警另行分类，不冒充可达或静默抹除；G05月度复核下一窗2026-12，新路径接入时现行含测试source gate立即检查。

远端PR/CI、amd64实跑、正式registry发布、共享环境切换、真实数据恢复及basebackup+连续WAL的完整PITR尚未执行。G05首次生产硬门继续负责SBOM/整镜像漏洞与离线供给、可复用发布/完整恢复；本轮一次升级演练不能代替这些交付。所有者已自行更新brew Go1.27.4与nvm Node26.13.1，已按基础二进制路径复核；本轮未代改其包管理配置。

## 对后续排期的影响

G07A/G07B按所有者要求前置为业务开工前维护；原S02/S04包ID仅追溯估算，不重复派单或计工。其余业务仍按[执行排期](../execution-schedule.md)；G03分档未启用，本轮跑现行完整门禁。首次共享/生产切换按数据契约独立实施，不随着源码合入自动切换数据库。
