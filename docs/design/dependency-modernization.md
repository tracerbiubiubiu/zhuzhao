# 工具链、依赖与运行组件现代化

日期：2026-11-12。状态：所有者要求将G07A/G07B提前为Phase6业务开工前的维护批次，五仓独立工作树候选已完成本地实现与验证，尚未合入main或替换共享服务/原数据卷。所有者已取消“与公司现有版本保持一致”的前提；自行打包与内网分发允许采用受支持版本，具体候选仍须组合验收。

## 1. 升级前main基线与当时推荐目标

下表保留升级调查时的main声明及当时推荐，不是本地维护候选的当前锁值；已实施候选见[§6实际组合](#6-本轮实际组合与交付口径2026-11-12)及组件锁。版本快照按当日仓库声明和官方一手资料核验。浮动标签不是实际镜像版本；实际部署需inspect摘要另记，本文不以声明推断当前容器运行版本。后续升级实施前再次核验补丁、安全公告与镜像可用性。

| 对象 | 升级前main声明 | 当时推荐处理/候选 | 窗口 |
|---|---|---|---|
| Go服务/公共库 | 四仓go.mod=1.26.10；三服务构建镜像同钉点 | 三服务1.27.4；utils最低1.26.11、推荐toolchain1.27.4，双SDK验证；不因服务升级强抬公共库语言线 | G07A/业务开工前 |
| Node/UI工具 | .nvmrc20.19.0、engine>=20.19、镜像node:20-alpine；@types/node26.2.0 | 推荐26 LTS当前补丁（本次核验26.13.1）；若依赖阻塞可临时24 LTS，须有解决期限。nvmrc/engine/CI/Docker/类型测试一起更新 | G07A/业务开工前 |
| pnpm/Vite/Vue/TS等 | pnpm12.3.4、Vite8.3.4、Vue3.5.43；其余按lockfile | 先验证现有组合在目标Node；保留lockfile，扫描并分小批升级，不把全部依赖强行升major | G07A/业务开工前，后续G05 |
| PostgreSQL | dev/test/三服务组合postgres:15浮动major，ltree等扩展 | 先明确15当前补丁+digest；目标18受支持补丁（本次官方表18.6），独立副本迁移演练后切换。不追19RC | G07B/业务开工前副本验证；实际环境另行受控切换 |
| Redis | redis:6.2-alpine，供会话/限流/缓存及TR/Asynq | 先固定6.2受支持补丁；候选8.2 extended线当前补丁，先经受支持7.x中间线演练。客户端/Asynq/脚本/ACL一起验证；许可证核验单列 | G07B/业务开工前副本验证；实际环境另行受控切换 |
| OS/Web镜像 | core/AL Alpine3.20、TR3.21；UI nginx1.27-alpine | 查当前受支持稳定线、平台架构与CVE，独立固定patch+digest；不凭同为Alpine推定相同CA/libc | G07A/业务开工前 |
| 对象/备份/迁移工具 | RustFS1.0.1、rclone1.75.2、migrate4.17.0；开发RedisInsight3.8.0 | 建立版本/许可/漏洞清单，先验证现行功能与恢复，再选受支持候选；不因新版本号直接变更存储产品 | G07B/业务开工前实际组件与副本恢复；G05仍交可复用发布机制 |

Go官方当日稳定线为1.27.4与1.26.11，支持最近两条主线；补丁和主线更新分批执行。[Go下载](https://go.dev/dl/)、[发布政策/历史](https://go.dev/doc/devel/release)

Node20已于2026-04-30退出支持；官方26.13.0在2026-10-28转LTS，支持计划至2029-04。11月10日已发布安全补丁26.13.1，本轮采用该补丁。推荐26是本项目维护期判断，不能以engine最低声明证明所有依赖已兼容。[Node发布计划](https://github.com/nodejs/Release/blob/main/schedule.json)、[26.13.0发布说明](https://nodejs.org/en/blog/release/v26.13.0)

PG15官方支持至2027-11，18至2030-11；支持表当日仍列18.6为稳定最新major，19为RC信息。采用18是减少短期再次major迁移的建议，不是现有驱动/扩展已验收结论。[PG版本政策](https://www.postgresql.org/support/versioning/)

Redis官方版本管理页把6.2列extended（至2027-04）、8.2列extended（至2030-09）。8的升级指南列7.x→8，不能据此承诺6.2直接原地升8。支持日期不替代具体制品的安全状态；实施时核验对应Redis OSS而非企业版生命周期。[版本管理](https://redis.io/docs/latest/operate/oss_and_stack/install/version-mgmt/)、[单实例升级](https://redis.io/docs/latest/operate/oss_and_stack/install/upgrade/standalone/)

## 2. 一个版本来源，多处消费

G07A建立生态运行/构建版本清单，分别列最低兼容版本、推荐版本、准确patch、OCI摘要/架构、工具版本、许可证、来源与核验日期。go.mod/packageManager/lockfile仍为各自生态权威；公共清单引用它们而非另维护一套不一致数值。CI、Docker、dev/test/生产模板和文档由检查器对账，差异须显式说明。

镜像固定patch与digest，release产物使用内容摘要，不以`:latest`证明组合。开发允许按明确策略更新引用；业务release固定五仓SHA、builder/runtime镜像摘要、生成工具、migrations摘要和合同版本。工具链升级也会改变二进制/压缩/时区/证书行为，必须关联同一组合回执。

公共utils保留消费者最低Go版本测试；新服务工具链可先升。提高最低Go版本是消费者兼容变更，必须有明确版本/迁移公告；不能把服务Docker升级顺手写入utils最低要求。UI至少覆盖目标Node和临时受支持回退线，移除过期线不当成API破坏。

## 3. 分批升级与验证

G07A先处理EOL Node、Go安全补丁与基础镜像；本轮用户已明确前置G07B，不再等待P6.1。工具链、依赖与数据库按子步骤分别留证，仍不把数据迁移和应用DDL变更混成一次不可定位的操作。先运行现有lockfile的lint/typecheck/unit/build/audit/E2E，再单独处理真正不兼容依赖。Go lint/swag/wire等生成工具需兼容目标SDK，重生成diff必须解释，不能用格式/生成巨diff夹带业务变化。每个候选在独立worktree和私有栈验证，不替换共享开发容器。

G07B以同一生产形状的脱敏/合成副本建立PG与Redis升级组合：pgx/ltree/迁移81对及后续、锁/事务/CAS、AL动态DDL/导入、TR/Asynq队列与cron、会话撤销/RT轮换、Lua脚本/TTL/限流全部覆盖。覆盖现有make acceptance、各仓检查与真实UI关键旅程；只跑“服务能启动”不足以放行。

升级工单必须分别记录preflight、备份可恢复、旧/新制品、数据格式、停写/水位、切换、观测和退出触发。G05承担可复用制品/发布恢复机制，G07只计本轮版本调查、候选和组合升级增量，不重复计价。对安全紧急补丁使用更短窗口，但不能删去最小兼容/恢复验证。

## 4. 数据服务的真实回退

PG major数据目录不向旧major兼容，官方要求dump/reload或pg_upgrade；可跨major升级但需读所有中间发布说明。推荐先用副本做逻辑导出/导入和校验，再按数据规模评估pg_upgrade。必须含角色/扩展/序列/权限/ltree/字符集/排序规则/时区及备份恢复；验证实际行和关键约束，不只比迁移版本。[PG升级政策](https://www.postgresql.org/support/versioning/)

PG回退不能“换回旧镜像挂同一卷”。切换前保留已验证旧版备份与恢复环境；切换后如产生新写入，必须有数据差异/重放/业务补偿方案，否则明确需要停写并由所有者接受RPO。应用迁移expand/contract与数据库major升级分窗口，避免无法定位和无法恢复的双变化。

Redis先盘点哪些是可重建缓存、哪些是会话、队列/计划和幂等证据。仅缓存可以按受审策略重建，不能FLUSH整个库使任务/会话证据消失。每跳保存RDB/AOF及配置并实测加载、ACL、持久化、重启和pending任务；新版本AOF/RDB不能假设可由旧版本读取。回退用升级前备份并对账切换后任务/会话，不把恢复旧快照当作零损恢复。[Redis持久化](https://redis.io/docs/latest/management/persistence/)

## 5. 自行打包与内网分发

按[基础先行边界](foundation-release-boundaries.md)先选一个实际入网目标OS/架构：准备运行制品、配置/迁移、许可证信息、校验和与启动检查，以及所选adapter开发的必要依赖；只有实际二开前端才准备对应Node/npm工具。首次试验不以全部架构、Playwright/字体、所有Go工具和完整签名流水线齐备为前置。完整离线重建、额外受支持平台、SBOM/签名及发布自动化沿G05或公司明确要求的接入门逐项交付，覆盖所承诺的构建/检查组合。内网GOPROXY/npm registry/OCI仓库/CA是配置，不在源码硬编码公司地址；公共构建不读取私有SDK或凭据。

许可证/发行条件是具体升级包需核验的输入，尤其Redis跨版本以及运行组件的社区/商业版区别；本规划不作法律结论，也不默认替换Valkey等另一产品。若确有分发限制，另开有限选型包，对命令/Asynq/持久化兼容实测后决定，不把“Redis兼容”当作现成替换证明。

每月补丁/依赖调查、严重可达漏洞紧急处理、季度受支持版本复核纳入G05；开工者对精确版本再次核验。当前版本维护从P6.0启动，不再整体等P7，也不因公司版本不同而永久挂账。


## 6. 本轮实际组合与交付口径（2026-11-12）

完整OCI index、Linux amd64/arm64 manifest摘要及核验日期见[component-lock.json](../../deployments/component-lock.json)，原生依赖仍以各仓go.mod/go.sum和UI lockfile为准。当前候选为Go1.27.4、Node26.13.1 LTS、PostgreSQL18.6-trixie、Redis8.2.11、Alpine3.24.4、Nginx1.30.6、RustFS1.0.5、rclone1.76.0、migrate4.20.1；RedisInsight仍为当前3.8.0，补固定digest。

主仓`make lint`中的`check-component-versions.py`校验OCI组合、编译器、部署及真实测试夹具的一致性。生成器Wire0.7.0仍为上游版本；Swag CLI升到1.16.6，与已用运行库对齐，继续核验sumdb、编译器和平台，未关闭来源检查。GitHub Actions更新为官方固定commit，托管runner满足新Node24运行时；远端实际事件结果须待PR，不以本地YAML检查冒充。

服务保留已公开可下载的utils依赖pin；本轮没有公共API变更，不制造跨仓未推SHA依赖。utils自身推荐/最低SDK与服务真实依赖图分开测试，不能把服务测试称为已消费新utils提交。生产代码不借版本升级实施Phase6新能力；mock数据、合同和真实接口分层保持。

PostgreSQL18挂载父目录`/var/lib/postgresql`，PG/Redis新版本使用新命名卷，旧卷保留。新卷为空不代表升级完成；存量必须先备份、恢复、校验，再切换入口。具体副本、停写、回退和结果见[数据契约](component-upgrade-data-contract.md)。本轮验证栈自有且隔离，默认dev/test服务、系统Go/Node、账号和旧数据没有自动切换。

TypeScript7需要OpenAPI生成依赖链同步支持，暂保留5.9.3；Cropper2有组件API变化，留给已排期的个人资料改造。每项延后均需注明理由、影响与窗口，不能称全部依赖已升至最大major。实际结果以本轮交付记录为准；验证未通过前不将G07标记完成。


工具实际执行目标已改为受控重包：migrate4.20.1只编PostgreSQL/file，rclone1.76.0保留工具能力并固定crypto0.59，两者用Go1.27.4保留符号、模块清单与许可证。上游OCI摘要只作比较，不能把其stripped JSON扫描退出0当安全通过；新Compose构建和CI见`deployments/tools/`及`helper-tools.yml`。[本轮记录](../review/reports/version-modernization-2026-11-12.md)汇总真实main/ELF text扫描和81迁移、备份恢复结果。
