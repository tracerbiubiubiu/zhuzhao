# B24：三仓 PostgreSQL 冷启动就绪判定

> **报告口径（2026-11-08整理）**：本文保留注明时点的基线、原失败、发现编号和复验范围；代码路径是定位提示，历史内容以文内固定提交为准。后续变化见[正式报告索引](README.md)，当前能力、开放债与运行承接见[项目控制台](../11-project-control.md)。报告通过不等于main或生产已承接。
## 范围与基线

Phase5小范围稳定性批；主仓源码基线`996c167f4567e39362226937b25368b55b7f44bd`，AL `db6a17bb686f09287d8793d1a3f98783bcc2e09d`，TR `1e31e3e95dc442b8384f3582cfc2850bd57d260b`。UI/utils无Compose，不造额外修改。仅6文件8处`pg_isready`补显式TCP host，不改用户/库、时序、依赖、重启策略、端口、卷、镜像或配置基线；不合main、不开始Phase6、不应用常驻环境。

## B24-F01 / P2：socket探针可先于真实TCP就绪误绿

空卷初始化时，固定PostgreSQL镜像的`docker_temp_server_start`用`listen_addresses=''`起临时Unix socket服务器。原无`-h`探针已可返回0，跨容器TCP客户端仍拒绝连接；`service_healthy`因此可过早放行应用或一次性迁移。重启策略后来恢复不证明就绪门禁正确。

原问题锚点分别为主仓[dev](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/deployments/docker-compose.dev.yaml#L20)、[生产](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/deployments/docker-compose.yaml#L43)、[test的三PG](https://github.com/tracerbiubiubiu/zhuzhao/blob/996c167f4567e39362226937b25368b55b7f44bd/deployments/docker-compose.test.yaml#L40)，AL [dev](https://github.com/tracerbiubiubiu/activelist/blob/db6a17bb686f09287d8793d1a3f98783bcc2e09d/deploy/compose.dev.yaml#L17)/[生产](https://github.com/tracerbiubiubiu/activelist/blob/db6a17bb686f09287d8793d1a3f98783bcc2e09d/deploy/compose.prod.yaml#L45)，TR [生产Compose](https://github.com/tracerbiubiubiu/taskrunner/blob/1e31e3e95dc442b8384f3582cfc2850bd57d260b/deploy/compose.yaml#L38)的`postgres.healthcheck.test`。修复统一增加`-h 127.0.0.1`，现有`-U`/`-d`保持。

## 验证证据

- 六份Compose在干净环境/专属合成变量下，以默认及全部profiles展开均通过；从修后规范模型撤销唯一host参数，逐键等于原版。没有输出实际环境秘密。
- 动态使用固定PG镜像`sha256:724292da1f2e50bdccfc3302ce75bbba7f4a6076701b588cc795fcac65683550`和真实AL镜像`sha256:dd86a4829a5b7e6680313a5afafbce942121c4ec50eaf1a99d28a5ffacf47514`，AL实际ELF `b3ed59569cbc4fd7709a8c62f6dab8dadd120808c0ae37b8af8e0b84a2affc9b`、Go1.26.10、原User `app`、原DB用户/库 `activelist`。
- 8轮各用fresh PG卷/网络。仅fixture的init脚本延迟20秒，原侧每轮socket probe=0、TCP probe=2且Docker已healthy，真实AL首启均exit1/TCP refused；修正侧在同一临时窗口保持starting，只在final TCP probe=0后healthy，真实AL每轮readyz200。没有增加生产延迟、启动重试或放宽成功标准。
- 两次早期共享文件解释器权限失败保留，尚未进入业务断言，不计入8轮；改为docker cp私有fixture到自有容器后成功运行，镜像/包未变。
- 成功8轮的32容器、16卷、8网络及早期失败轮自有资源均精确回收；清理与起跑前资源交集为空，16个既有dev/test容器元数据未变。六份正式源码在专项期间固定。

## 修复建议与承接边界

代码配置修复的专项出口已满足：主仓 `dd5b54c65bc8dedea08e2b9a6f7e17a0a5fcc75d`、activelist `83909b86a697dd2fc69a71819a5e1327c9be1d03`、taskrunner `6d0e2ad592eccf0c12bb525958784200c478a45e`。主仓待推送窗口为 `f5a8a892579fd44f0bc5c0e633d541757d621c06..dd5b54c65bc8dedea08e2b9a6f7e17a0a5fcc75d`，AL/TR分别为上述基线至修复单笔；同批仅增加本报告绑定提交，推送前另核差集。准确CI已核全绿：[主仓81e9a3d](https://github.com/tracerbiubiubiu/zhuzhao/actions/runs/42360431942)、[AL83909b8](https://github.com/tracerbiubiubiu/activelist/actions/runs/42360432964)、[TR6d0e2ad](https://github.com/tracerbiubiubiu/taskrunner/actions/runs/42360436856)。这些CI不替代后续B19/B23/G14最新源码的准确提交CI。B18原样253/81已使用同类TCP探针fixture，但不借它宣称当前B19/B20/公式上下文新组合已全部通过；新组合仍须独立验收。上述为本批原时点；后续本机5PG已在备份及来源核对后以原镜像/卷承接TCP探针并通过独立SQL/ready核验，见[B27最终收口](zhuzhao-ecosystem-audit-b27-phase5-closeout-2026-11-07.md)。生产模板没有现场部署。

TCP返回0只说明该传输可接入，不能代替迁移/绑定种子、凭据、权限或应用ready探针。操作顺序见[runbook §9.1](../../ops/runbook.md#91-postgresql空卷初始化的tcp就绪判定)。过程脚本、逐轮结果、镜像入口快照、141项SHA清单和清理收据仅ignored `.codex/audits/ECO-20261104/phase5-closeout-20261107/postgres-readiness/`，不纳Git/Docker。
