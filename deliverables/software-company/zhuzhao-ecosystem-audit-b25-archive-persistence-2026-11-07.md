# B25：审计JSONL持久化与独立恢复

## 范围与基线

主仓Phase5配置基线 `dd5b54c65bc8dedea08e2b9a6f7e17a0a5fcc75d`，检查现有 `AuditArchiveJob.Handle` 的文件承接与备份。仅prod/test Compose、backup README和backup.sh标题注释四处实现/说明改动；不引入新备份平台、调度、依赖、Go代码、迁移、权限或保留期。B24的TCP健康探针已独立提交，本批不改其参数。

## B25-F01 / P2：容器写层归档未接持久卷

`AuditArchiveJob.Handle`把audit_logs/policy_evaluation_logs过期行导出JSONL并Sync后删同批DB行；文件不在后续pg_dump或RustFS桶中。原[prod app](https://github.com/tracerbiubiubiu/zhuzhao/blob/dd5b54c65bc8dedea08e2b9a6f7e17a0a5fcc75d/deployments/docker-compose.yaml#L238)和[test app](https://github.com/tracerbiubiubiu/zhuzhao/blob/dd5b54c65bc8dedea08e2b9a6f7e17a0a5fcc75d/deployments/docker-compose.test.yaml#L124)没有覆盖`/app/data/archive`的持久挂载，实际test Mounts核对亦如此。容器重建无法承接该写层，不能由DB有备份推断归档文件有备份。

本轮native目录/test写层均0个文件，未证实既有归档数据已经丢失；不提升为已发生的数据损坏。native TR有2个disabled cron定义、test TR有1个enabled manual定义，app cron默认false不足以推出全部归档入口禁用。当前5个PG archive_mode均off且无运行pgbackup，文档中的备份能力也不等于已启用运行保障。

## 修复与恢复边界

prod app_archive、test zt_app_archive接现有目录；Docker默认copy-up保留镜像33333:33333。out_dir env覆盖必须同时调整挂载/备份位置，已有旧写层在重建前须停写、独立保真备份并迁移，不能用新空卷遮住文件。README增加只读PAX备份、fresh卷恢复、SHA/解析/权限/mtime核对及回退说明；backup.sh只把误称“物理”改回实际pg_dump逻辑备份。

DB180天、文件395天及task_runs/execution_logs永久保留均不改变。JSONL清理按顶层.jsonl的mtime，不按文件名或递归；恢复改变mtime会改变清理期限。离线副本的责任人/保留销毁/异机目的地另行登记；新增同宿主卷不等于异机备份。本批不启用app ticker、taskrunner job或生产备份侧车。

## 验证证据

- 独立PG应用81份真实迁移、显式可信Go1.26.10构建真实Archive helper；导出3条audit+2条policy合成过期行后DB各保留1条fresh。命名卷默认copy-up使33333非root用户可写；删除原导出容器再替换后，JSONL解析/条数/SHA/UID/GID/mode/纳秒mtime完全一致。
- attempt1负向证明本机Docker默认docker cp tar流虽保正文，却将uid/gid变root、mtime截到秒，未达到保真要求。attempt2改用在场固定PG镜像GNU tar1.35、PAX/数字所有者、源卷只读与fresh目标卷，恢复后全部内容/元数据相同。未安装包或修改共享卷。
- 395天清理只在自有合成fixture测试：400天mtime顶层jsonl删除，1天mtime但旧日期文件名保留，non-jsonl和子目录内容保留；未触及taskrunner保留数据。
- shell片段/backup.sh的sh-n、两Compose默认/全profiles规范展开、diff-check通过。两attempt的自建容器/命名与匿名卷/网络均精确清理，原16核心元数据和4个native进程保持；832项证据SHA已复核。
- 此专项没有运行make acceptance。root新组合真实镜像另通过原样253/0/0及81往返，但该组合使用自有Compose夹具，不声称它覆盖本批正式named-volume YAML。正式配置承接须在备份与来源核对后重建app、复核挂载/非root写权限/ready和关键路径，常驻尚未应用。

完整hash和部署承接状态在提交后补录。原始只读环境盘点、两次真实恢复、失败记录、源与证据清单仅在ignored `.codex/audits/ECO-20261104/phase5-closeout-20261107/{archive-persistence,environment-finalization}/`；不跟踪数据库转储、Env或原始个人数据。

## 固定提交与最终组合补证

本批主仓完整提交 `2f9d94bcfd214a10eb1d06a495f5f7e5a75c1381`。最终运行源码354项逐字绑定主仓 `f87ab1aefcbf1843bd8714c5accf245033f99d8b`，原样253/81及29文件230单测、41/41 E2E均通过；[组合补证与保留限制](./zhuzhao-ecosystem-audit-b20-ui-feedback-2026-11-07.md#验证证据与已纠正的初版误判)保留初版失败、wrapper缺checker与未执行附加control，不扩大本批原覆盖。后续主4db/UI152准确CI与dev/test承接已核，当前映射及Chrome代表走查见[B27最终收口](./zhuzhao-ecosystem-audit-b27-phase5-closeout-2026-11-07.md)；main/生产未变。上文保留本批原轮次与未执行路径，不用新时点倒补旧证据。
