# 运行组件升级、数据迁移与恢复契约

> 2026-11-12；Phase6 业务开发前的工程升级。本文约束数据组件切换，不代表已升级开发共享环境或已部署生产环境。五仓能力、排期和总验收状态仍以 [项目控制总览](../review/11-project-control.md) 为准。

## 1. 已确定的范围

项目所有者已取消与公司云数据库旧版本保持一致的约束，允许升级 Go、Node、依赖、容器、PostgreSQL、Redis 和对象存储工具。既有 mock 接口及其可替换接入边界继续保留；本次不启动 Phase6 业务功能，不接入内部真实数据源。

升级工作包含实际代码与配置适配、制品核验、已有功能门禁及独立恢复演练。现有共享容器和卷保持原状：将 compose 中版本更新到目标值，不等于已经完成存量数据迁移。PG、Redis、RustFS 的数据处理必须独立验收。

当前没有正式部署，允许主动更换旧接口和旧运行约束，不增加此前讨论的六个月兼容负担。但现有开发数据、会话吊销状态、未完成任务和附件仍不能因升级而默默丢失。需要清空的专属测试环境应由其所有者显式重建；不能借此推断其他环境也可清空。

## 2. 目标版本与可拉取制品

2026-11-12 从官方发布记录及 OCI Registry 共同核验。每个下列镜像都取得 index 和 Linux amd64、arm64 子 manifest，并计算响应体 SHA256 与 Registry 声明摘要比对；数据组件还实际拉取原生 arm64 制品。版本网页不是可拉取性证明。开发、测试、CI 和部署声明应使用同一目标版本，默认固定 tag 与 index digest。

选择 PostgreSQL 18.6 stable；19rc1 不作为默认。数据库使用 Debian trixie，避免把数据库大版本与 libc/distribution 切换捆绑；应用 runtime 使用 Alpine 不要求数据库也使用 Alpine。Redis 选择 8.2.11 Extended，官方当前维护期到 2030-09-01；8.10 短期版本线不因版本号更高而自动替换。7.4.12 仅作恢复演练和迁移桥接。Redis 8.2 官方 Alpine 制品基于 3.22，不能臆造 `8.2.11-alpine3.24`。

```text
golang:1.27.4-alpine3.24@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf
node:26.13.1-alpine3.24@sha256:6bd9ab43c0ec0190ca7b2d193906cb8b7edf6e7f4afa43ddeccd6452710bffb5
nginx:1.30.6-alpine3.24@sha256:583b7f2fce1cdb8da5ad5c39ebd3c33b98b87d493190e548bdee1fa8c619c311
alpine:3.24.4@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
postgres:18.6-trixie@sha256:e3d24f6906d0760bf4da0dae872415391ededefad23fd471c47dc53b0ddf3c42
redis:8.2.11-alpine3.22@sha256:da23c2478319ceb4903857d77a2fd7aa5137d9e9d4c8165dafbacedac6403e95
rustfs/rustfs:1.0.5@sha256:b81d16a90ee25bd9511280127ca1471788c6e874cf0f0da759645f46b2dc1d74
rclone/rclone:1.76.0@sha256:7cbeec24a44f206067597c0368be4de90ae19a64d5113c7d1938bd737a81e466
migrate/migrate:v4.20.1@sha256:76cc2074cb6642631f34a898ced71e6aeaa6b1a4d78c4daa743275a22e0c5be7
redis/redisinsight:3.8.0@sha256:b5e19ee240abef6edb435871b90ff8a210995422e8e018ab61c0339d318a1f84
```

旧数据演练基线与桥接制品：

```text
postgres:15.19-trixie@sha256:1aac5b49cefd627f02816141ca4ad012d6cc19460dfe02a9b5168a105be3fc53
redis:6.2.24-alpine3.22@sha256:3f9c6e23ed6bea3b41d0ef709fc164c1a0b7c00d5a21a60e0e9c33ada5496881
redis:7.4.12-alpine3.22@sha256:7f7d30b7fdd09467bac58198a1f166dd69789d348d4fe859cccb20e0cf287fbe
```

Go 从 1.26.10 升至 1.27.4，Node 从 20 线升至 26 LTS，Nginx 选择 stable 线；RustFS 1.0.1→1.0.5，rclone 1.75.2→1.76.0，migrate 4.17.0→4.20.1。RedisInsight 3.8.0 在核验时仍为官方最新发布，保留其版本并固定 digest，不为制造变更而替换。

官方依据：[Go 发布](https://go.dev/dl/?mode=json)、[Node 发布线](https://nodejs.org/en/about/previous-releases)、[PostgreSQL 支持周期](https://www.postgresql.org/support/versioning/)、[Redis 版本策略](https://redis.io/docs/latest/operate/oss_and_stack/install/version-mgmt/)、[Nginx 发布](https://nginx.org/en/download.html)、[Alpine 发布](https://alpinelinux.org/releases/)、[RustFS 发布](https://github.com/rustfs/rustfs/releases)、[rclone 发布](https://github.com/rclone/rclone/releases)、[migrate 发布](https://github.com/golang-migrate/migrate/releases)、[RedisInsight 发布](https://github.com/RedisInsight/RedisInsight/releases)。OCI tag 构建来源可在 [Docker Official Images](https://github.com/docker-library/official-images/tree/master/library) 逐项追溯。

## 3. 数据库契约：PG15 到 PG18

### 3.1 数据目录与备份边界

PG18 官方镜像默认 `PGDATA=/var/lib/postgresql/18/docker`，持久卷挂在父目录 `/var/lib/postgresql`；PG15 原数据目录为 `/var/lib/postgresql/data`。新 compose 应使用新的 `pgdata18` 一类卷名，旧卷保留，禁止让 PG18 直接打开 PG15 文件。Testcontainers postgres module v0.45.0 的 `Run` 本身不写死旧 PGDATA 或默认持久目录；自有 `WithMount`、tmpfs 或 compose 显式路径仍须单独核对。

逻辑备份和物理备份不同：旧版本物理目录、WAL 和 base backup 不可作为 PG18 的恢复输入。核心仓使用新的 `wal18_archive` 卷和 `/wal18_archive` 目标路径；ActivList 的 WAL 归档、数据卷、备份目录也按主版本隔离，例如 `wal_archive18`、`backups18`。历史 PG15 备份仍标明 PG15。物理恢复必须使用匹配主版本的工具和实例。

WAL 目标目录须通过实际 PG 进程写入和 `pg_stat_archiver` 验证。不能仅由“全新卷可能 root 属主”推定存在故障。本轮本机轻探针中，不同容器用户、乃至同一 root 容器内 `gosu postgres` 前后观察到了目录 owner 呈现变化，降权写入仍成功；随后默认官方 root entrypoint、全新 WAL 卷和生产归档命令的端到端验证也通过，未复现权限故障，因此没有添加额外 entrypoint 包装。这只能作为该运行环境的观察结果，不能外推其他 Linux 环境的权限。

两仓现有归档命令有区别：核心为 `test ! -f /wal18_archive/%f && cp %p /wal18_archive/%f`，ActivList 为 `cp %p /wal_archive/%f`；后者依照既有事故修复允许覆写同名半截目标段。本轮分别验证成功写入，保留原有策略。中断复制、半截目标段重试处置、base backup 与 WAL 连续回放属于既有 G05 的生产前恢复门禁，不能由此推断已具备完整 PITR。

数据库创建角色、扩展和排序规则属于恢复前置项。实际环境应核对 encoding、locale/collation、timezone、数据库列表、owner/role 与授权、`ltree` 等扩展。角色全局备份和数据库归档须在受控本地路径保存，不上传真实数据或密码摘要。核心、ActivList、TaskRunner 都要纳入同一停写窗口的业务一致性核对。

### 3.2 推荐切换顺序

1. 记录代码版本、镜像 digest、配置、卷归属、数据库列表、迁移版本和备份校验值。预留新卷和独立恢复环境，确认旧数据目录不会被自动复用。
2. 先停止业务写入、调度器和 worker。保留查询或维护页时不得存在旁路写入；记录各队列和 PG 业务状态的基线。停止顺序以实际部署依赖为准。
3. 使用 PG18 `pg_dump` 客户端从 PG15 生成 custom archive，并另存角色/授权信息；同时保留 PG15 原生工具生成的回退归档及原卷。备份成功须以退出码、归档可读性、恢复结果共同确认。
4. 初始化空 PG18 实例，配置目标角色/扩展；把归档恢复到新库。`--no-owner` 或 `--no-privileges` 仅在明确重建 owner/授权时使用，不能把少恢复授权当成成功。
5. 核对全部表、行、关键关联、序列、外键、唯一约束、索引、`ltree` 运算和迁移版本，执行 `ANALYZE`，再做新版本真实业务读写与五仓门禁。PG健康检查成功、表数量相近、只运行 `pg_restore --list` 均不足以放行。
6. 验证通过后显式切换应用 DSN/服务地址，再恢复 worker 和调度；写入恢复前保留可回退窗口。确保旧业务实例不会误连新库或继续写旧库。
7. 切换后核对 outbox、任务受理、回调、附件元数据和审计；观察通过前保留旧卷、归档和版本记录。不能用“新卷是空库但应用能启动”作为升级完成证据。

回退优先采用停写后重新指向未变更的 PG15 原卷，或在 PG15 新实例恢复 PG15 原生归档。PG18 已接收新业务写入后，回退可能丢失这些写入；必须先停写并决定前向修复、对账重放或专门反向逻辑迁移，不能直接把 PG18 数据目录挂回 PG15，也不能承诺无条件一键回滚。

官方依据：[PG18 官方镜像目录约定](https://hub.docker.com/_/postgres)、[PostgreSQL 升级说明](https://www.postgresql.org/docs/18/upgrading.html)、[pg_dump](https://www.postgresql.org/docs/18/app-pgdump.html)。

## 4. Redis 契约：缓存、会话与队列分别处理

Redis 不只承载可丢弃缓存。登录刷新 token、禁用标记、密码纪元、吊销/限流状态、幂等键、任务队列和调度状态具有不同丢失后果。切换前按实例与 DB 列出使用者、key prefix、TTL、持久化模式、淘汰策略和写入方；不能统一 `FLUSHDB` 或无条件清空新卷再启动。

本次采用 6.2→7.4→8.2 的独立副本恢复路径。官方 Redis 8 文档支持从 7.x 升级到 8；本文不把 6.2→8 直接载入视为官方保证。RDB 和 AOF 各做一次完整演练，先恢复 6.2 数据到 7.4，确认逻辑状态后保存 7.4 数据，再恢复到 8.2。AOF 从 6.2 单文件到 7.x multipart manifest 的变化应被实际覆盖，不能只证明 `PING`。

1. 先停止任务受理、调度器及 worker，等待在途任务按应用排空策略结束。无法排空的任务按 delivery fence、DB-first 受理记录和回收机制对账；Redis 不承担 exactly-once 保证。
2. 确认实例的 `dir`、`dbfilename`、`appendonly`、AOF 文件/目录配置及每个 DB 数据量。等待 RDB/AOF 保存完成后停止源端，复制完整数据目录及配置，记录校验值。大实例的保存方式和停机窗口先按实际数据量压测，不在生产盲目执行阻塞 `SAVE`。
3. 每一步在新卷载入副本：缓存值可以重建；会话、吊销/纪元和幂等状态默认保留；TTL 按绝对到期时间继续减少，停机期间过期的 key 不复活。若明确选择全员重新登录，需同时使旧访问 token 失效，不能只丢刷新 token。
4. Asynq 需验证 pending、scheduled、retry、archived、completed 的任务 ID、payload、状态和调度时间，重复 TaskID 拒绝行为、实际 worker 消费和业务幂等栅栏。补跑策略仍服从既有任务设计，不能把升级过程变成批量自动重放安全动作的理由。
5. 新版本持久化写入、重启恢复与旧格式备份恢复都通过后，再切换唯一写入端并恢复业务。队列 Redis 保持既有 `noeviction` 约束；鉴权状态故障按既有 fail-closed 逻辑处理。

回退只使用未被新版写入的 6.2 原卷或 6.2 备份。不能把 8.2 新生成的 RDB/AOF 直接交给 6.2；新旧业务同时消费同一份复制队列也不允许。恢复接收任务后需要结合 PG 受理记录对账，重新判定任务是否已生效。

官方依据：[Redis 8 standalone upgrade](https://redis.io/docs/latest/operate/oss_and_stack/install/upgrade/standalone/)、[Redis 持久化](https://redis.io/docs/latest/operate/oss_and_stack/management/persistence/)。

## 5. 对象存储与备份工具契约

RustFS 1.0.1 的原卷先停写并离线复制，在独立卷用 1.0.5 启动，核对旧对象二进制内容与元数据，再验证新对象写入、下载与重启。验证 rclone 1.76.0 对升级后源端和独立备份端的实际同步，并从备份恢复到第三个空实例，通过下载校验比较文件内容；不能只检查对象列表或备份脚本退出码。

备份同步语义继续遵守既有对象版本/保留策略。`sync` 会传播删除，备份目标必须有对应历史保留，不将镜像同步本身宣称为不可变备份。停写副本探针能证明本次合成单节点格式与调用兼容，不能自动推定多盘/集群升级、生产容量、实际损坏恢复或历史所有对象均已验证。

升级后仍保留 1.0.1 原卷；只允许用旧版本读写该未改动卷。新版已经写入新附件后，退回旧快照会造成对象与数据库元数据不一致，必须联同 PG 和应用的写入边界对账。不得默认 RustFS 数据格式可向后兼容。

## 6. 本轮证据与验收边界

本轮恢复演练全部使用 `zzu-data-*` 命名、`zz.owner=data-upgrade` 标签的独立容器/网络/卷与合成数据，不暴露宿主机端口，不操作共享容器中的 DB/key/bucket。源代码只读取本轮新工作树，现场数据不上传。

证据保存在原主仓 ignored 路径 `.codex/audits/version-modernization-20261112/data/`：官方发布快照、Registry manifest、`selected-registry-pins.json`、镜像拉取与运行身份、恢复脚本、归档/数据文件摘要、逐条命令日志和结果 JSON。恢复脚本不纳入运行产品或自动执行迁移入口。

已验证结果：

- 制品：13 个目标/桥接镜像的 index 和双架构 manifest 内容摘要一致；9 个数据/运维镜像已原生拉取。`selected-registry-pins.json` 与 raw manifest 已再次逐项校验。rclone 与 migrate 的实际二进制分别报告 1.76.0、4.20.1；镜像中 ELF 已提取并记录 SHA256，供本轮漏洞审查使用。上游 rclone 构建使用 Go1.27.3，与本项目应用 Go1.27.4 分别记账，不以最新 tag 代替漏洞扫描。
- Redis：`zzu-data-redis-1794464145/result.json` 为 PASS。RDB、AOF 两套完整路径均覆盖 6.2.24→7.4.12→8.2.11、真实 Asynq v0.26.0 五类状态和 payload/ID、重复 ID 拒绝、新版 worker 消费与合成业务幂等栅栏、core 原始刷新 Lua 一次消费、会话/禁用/纪元/限流/幂等值、TTL 绝对期限与停机过期、新版新写重启恢复、未改动 6.2 格式备份回退。AOF 包含单文件向 multipart manifest 的实际转换。此探针不替代 TaskRunner 自身 delivery fence、DB 对账与完整业务测试。
- 对象存储：`zzu-data-object-1794464231/result.json` 为 PASS。1.0.1 离线目录副本被 1.0.5 读入，旧对象内容与元数据一致；新对象与 11 MiB multipart 对象、预签名 GET、新写重启恢复通过。rclone1.76 对独立备份端同步并 `check --download`，再恢复至第三个空实例，三个对象逐字节一致；旧版原卷仍可读取原对象。另以原仓 `rustfs-mirror.sh` 原样执行两轮，覆盖和删除前的对象被 `--backup-dir` 留存，均从归档恢复并逐字节核对通过，证据 `zzu-data-object-1794465880/result.json`。
- PostgreSQL：`pg-upgrade-final-result.json` 为 PASS。在本轮工作树实际 81 对迁移上，PG18 空库执行到 `81:false`；PG15→18 custom archive 恢复后，全部 39 表的行内容、序列与 NOT NULL 一致。约束和索引仅有一个 CHECK、三个 partial-index predicate 的 varchar 字面量数组强转反解差异，同样出现在 PG15→PG15 归档恢复；对这些表达式做明确限域归一，并执行取值域/NULL 真值比较及实际非法 visibility 拒绝验证，未忽略其他 DDL 差异。PG15 原生归档恢复到同一独立 PG15 实例的另一个空数据库，数据及语义同样一致。恢复后 `ltree1.3` 子树查询命中两个合成组织、工单关系与附件关联各一条、标签序列新写返回 id3；服务器为18.6，数据目录与默认 data checksums=on 均实测确认。初始化等待、fixture/readiness 和严格反解文本比较的中间失败记录全部保留，不作为通过证据。
- WAL 与页校验：`zzu-data-pg18-wal-1794466267/result.json` 为 PASS。复用恢复后的同一 PG18 数据卷，停库时 `pg_checksums --check` 扫描 2,030 文件、5,542 页，坏 checksum 为零；再用默认官方 entrypoint、全新归档卷及核心生产精确命令启动，`pg_switch_wal` 后 `pg_stat_archiver` 为 archived_count=1、failed_count=0，实际生成并校验一个 16 MiB WAL 文件，无目录权限修补。此证据只覆盖归档写入和恢复页校验。
- ActivList 精确归档命令：`zzu-data-pg18-wal-al-1794466489/result.json` 为 PASS。复用同一 PG18 数据卷，以全新 `/wal_archive` 卷及 `cp %p /wal_archive/%f` 启动，重置 archiver 统计后切段，archived_count=1、failed_count=0，另一个 16 MiB WAL 文件实际生成并校验；未另起空 PG 集群，也未并行启动同卷写入者。
- 资源隔离：`shared-container-fingerprint-result.json` 记录原有六个共享容器的 ID、镜像、启动时刻、运行状态和挂载保持一致。本轮全部自有探针容器已停止，恢复归档、合成文件和专属卷仍保留；该指纹不声称正在运行的原有业务数据始终逐字节不变。

尚未覆盖真实现场数据、实际内部平台、生产容量、多盘/集群升级、amd64 实机执行及完整 PITR。五仓构建、单元/集成、覆盖率、真实栈验收和 UI E2E 的最终结果由本轮总验收记录汇总，不能用本文的恢复探针替代。上述探针通过也不代表已切换共享开发栈或已部署生产。


## 最终工具执行制品补充

上文公共migrate/rclone OCI摘要保留为上游对照。本轮最终Compose执行[受控源码工具构建](../../deployments/tools/README.md)，与component-lock.json的`local_tool_builds`一致：Go1.27.4、migrate4.20.1仅PostgreSQL/file、rclone1.76.0加crypto0.59，均不使用fork/replace并保留符号及许可证。实际main源码与最终Docker ELF的四次text扫描通过；最终migrate再次完成81 up/down/up，最终rclone再次以原mirror脚本验证覆盖/删除归档、11MiB multipart备份与第三实例恢复。JSON0不作为安全门禁，旧stripped符号展开记录不代表真实可达。[汇总](../review/reports/version-modernization-2026-11-12.md)保留边界与后继窗口。
