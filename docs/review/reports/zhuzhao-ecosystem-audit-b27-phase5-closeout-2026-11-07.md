# B27：Phase5五仓后续修复与环境收口

> **报告口径（2026-11-08整理）**：本文保留注明时点的基线、原失败、发现编号和复验范围；代码路径是定位提示，历史内容以文内固定提交为准。后续变化见[正式报告索引](README.md)，当前能力、开放债与运行承接见[项目控制台](../11-project-control.md)。报告通过不等于main或生产已承接。
## 范围与固定基线

本轮仅Phase5，承接五仓全面审计后的安全验证、四个P2、小范围稳定性、当前历史债与工作区/环境/分支整理。通用方法仍归[审计指南](../audit-playbook.md)，本报告记录本轮交付与边界，不替代设计SSOT。未合main、未准备main PR、未实施Phase6，也没有发布新tag或镜像到registry。

主仓准确CI基线 `4dbf4ffe1557346ba1a959329089267dfc1a8bb2`，352份Go文件（含测试/生成物）与go.mod/sum共354项与 `f87ab1aefcbf1843bd8714c5accf245033f99d8b` 逐Git对象一致。UI `152258638abaa3bd3daa656255a794c5422a0e58`；AL `83909b86a697dd2fc69a71819a5e1327c9be1d03`；TR `6d0e2ad592eccf0c12bb525958784200c478a45e`；utils `563785a83ce2aeb12c5f2b65c42defb58a48c331`。AL/TR后两笔仅Compose变化，Go字节分别等于db6a17b/1e31e3e安全扫描pin，不冒称工作流扫描了不同HEAD。

## 已交付与验证

- [B18工具/回滚](zhuzhao-ecosystem-audit-b18-tooling-rollback-2026-11-07.md)：Wire/Swag独立工具缓存、SDK/模块校验和、只读回滚前置检查、Git源码范围格式检查；定制模板真实PG和变异证据保留。预检通过不是DB回滚许可。 同报告记录B22 `596f625e8c678b26bce5bd96a8058cff91c17f5e`：仅修正限流跨秒和审计刷盘批次的两处测试时序假设，生产行为未变，原/修/去修各8轮验证。
- [B19 ID契约](zhuzhao-ecosystem-audit-b19-storage-id-2026-11-07.md)：主 `34d2c8bb4b3269bf75c5de55c9a94ef5facd5ed2` 与UI `306098df5970f731cd32f82c3ed1fe63eb140a10` 同窗承接。存储输出ID为string，真实HTTP/JS大ID及编译期漂移守卫通过；这是破坏性wire变更，仓外numeric消费者仍须协调升级。
- [B20/G-12前端反馈](zhuzhao-ecosystem-audit-b20-ui-feedback-2026-11-07.md)：角标真正订阅失效key、画布重复load后的undo/redo、附件失败保旧列表/重试与部分成功提示。真实页面候选8绿、旧history事件监听8稳定红；原初版40/41及retry失败没有删去。
- [B21/L-5主体文档](zhuzhao-ecosystem-audit-b21-design-docs-2026-11-07.md)：`f27994bcbd832b7a4162846f181044431b925516` 校准ticket/architecture正文与演进，保留历史裁定；独立语义/链接复核通过。L-1～4搬树不混销。
- [B23/BK11表达式](zhuzhao-ecosystem-audit-b23-expression-context-2026-11-07.md)：`82bda27e5a46c22d3d57daab4387dc1aa5dbae93` 复用当前q/ctx，关闭合法formula/visible_when在单连接池自锁；读取收尾错误、取消释放与缓存隔离均有回归。通用dbtx不是本批已实现能力。
- [B24 TCP就绪](zhuzhao-ecosystem-audit-b24-postgres-readiness-2026-11-07.md)：三仓6文件8探针，8轮fresh卷对照；本机5个PG已同原镜像/卷承接，生产模板没有现场部署。
- [B25归档](zhuzhao-ecosystem-audit-b25-archive-persistence-2026-11-07.md)：`2f9d94bcfd214a10eb1d06a495f5f7e5a75c1381` 增加prod/test JSONL持久挂载与独立PAX恢复说明；本机test已接卷。真实导出3+2旧行、替换/恢复内容与元数据一致；没有启用cron/新增周期备份服务或改变保留期。
- [B26/G-14软提示](zhuzhao-ecosystem-audit-b26-field-warnings-2026-11-07.md)：`f87ab1aefcbf1843bd8714c5accf245033f99d8b` 完整选项集合/radio/字典切换与读取不确定提示；原8红、修8绿、7变异各8红。相同字典引用的外部项变更无历史快照，仍属写侧合同边界。

完整原生Go1.26.10 lint/unit/cover/integration与真实Swagger/Wire再生通过；最终固定组合原样253/0/0、81对迁移往返零漂移。前端原样pnpm gate为29文件230单测、30 spec/41 E2E全部通过，实际0 retry/flaky/skip，UI阶段0个429。当前默认unit覆盖率20.309%，新增G14分支由integration覆盖，不宣称默认单测全面覆盖。

准确远端CI：[主仓4db三job](https://github.com/tracerbiubiubiu/zhuzhao/actions/runs/42363435972)、[UI152](https://github.com/tracerbiubiubiu/zhuzhao-ui/actions/runs/42363437676)、[AL839](https://github.com/tracerbiubiubiu/activelist/actions/runs/42360432964)、[TR6d](https://github.com/tracerbiubiubiu/taskrunner/actions/runs/42360436856)通过；utils5637既有准确CI不因本批无源码变化重复替代。报告尾随文档提交的CI另核，不倒写4db为包含本报告的提交。

## 安全补证

[准确4db安全取证](https://github.com/tracerbiubiubiu/zhuzhao/actions/runs/42363524261)五job及五artifact ZIP的API SHA全部核验；四source严格重放canonical parser、before/after/go-env，均Go1.26.10、Linux arm64、CGO0。主605/AL393/TR440/utils370个包均未导入OpenPGP，每仓仅GO-2026-5932模块提示，包/函数级发现0。当前354运行源逐项等于CI源对象。

实际主Linux ELF `60adf44c45f1868a499790c1d5cc6746bae9a48bbe8cef2d11d6add002372f9b` 补扫仍保留5932。stripped二进制的OSV包/函数通配符是scanner回退合成，整目标只有模块精度，不能称真实可达符号；初版按字段非空的中间分类被独立复核纠正，原始JSON/误分类/裁定均保留。CI绿色是证据采集完成，不是“零漏洞”或完整供应链/镜像OS/许可证认证。

## 开发与测试环境承接

维护前核其它会话、原进程/Env/CWD/实际ELF、全部卷与网络、任务DB/Redis无在途；先留5PG+SQLite一致备份，再停止入口与应用，停写后另留一组5个pg_dump及SQLite在线backup。跨库不是原子快照，不执行共享restore或shared acceptance，不改测试账号。E000001 dev/test只读bcrypt核验均匹配既定终态，mcp=false/status1；没有使用其登录或改密。

本机五PG同原不可变镜像/原data卷逐库重建，TCP与SELECT1均成功；主库两份v81、AL两份v2且dirty=false，TR无编号迁移表。本批受保护的原13卷与4网络保留，只有一个带owner的归档卷新增；test PG保留当前有效51692，仅本机运行快照冻结，不改变仓库动态端口基线。7个目标换容器ID；AL/TR保留原ID只stop/start，EndpointID/IP等动态身份变化不误称原始网络JSON零漂移。

test主镜像 `8cfe9ec7cbb0a4663915dc261c30fc486e8e8f7f831a67adb4b60b81bdd9d000` 的实际ELF为60ad；AL/TR实际ELF仍b3ed/69d，均Go1.26.10。UI镜像 `b283c00207366fa5f12b595b4a1a5a804e3710e2946a9f7955be3d675db59ba3` 的webroot恰266文件逐SHA一致，无旧assets，测试徽标/模板/启动配置均核对。原生三服务也换可信SDK1.26.10产物；系统Go1.26.6与全局PATH未改，Node仍既有22.14。

归档实际mount、33333:33333、可写与0文件独立核验；未知cool_keller、7个其它核心容器及Vite不动。五库对停写dump计数仅测试审计/判定日志随走查增长，其余受核业务计数一致；SQLite jobs2/job_runs290与integrity保持。计数不等于逐行/序列或跨库原子一致性证明。

原生Env取证校准：KERN_PROCARGS2不提供envc，原捕获器误把末尾8个apple平台执行项也当Env传入。依据原launcher common+override重建为93/75/78业务项，保留项逐值不变，平台项/置信边界独立保存；仅原生三进程受控重启，test不动。最终独立后验确认实际环境前93/75/78项与输入逐值一致，尾部8个kernel项单列；真实监听PID、ELF/Go/CWD/config与SQLite均再核。当前本机运行清单为ignored `environment-finalization/current-dev-manifest.json`，旧11-06 devctl与一次性维护脚本只作历史恢复证据。

## Chrome与操作体验

真实Chrome扩展、既有E800006会话复验首页、工单103图片/附件、类型图、消息、任务12记录和AL16类型。三张图片加载、内容内滚，代表页无外层横纵溢出、console warn/error为空。真实JSON往返后加临时节点→undo→redo→undo告警同步，取消后incident版本仍1，没有保存。失败恢复与未读写回的网络变异已由原样E2E覆盖，不在保留环境重复造故障或触发任务。

P3体验后续两点：新增节点可能到画布边缘、需“适应画布”；缺上传者姓名时退为数值ID。属于后续定位/展示抛光，不扩成本轮新的流程、权限或全站无障碍承诺。原始截图仅ignored本地取证目录，正式报告不上传运行数据。

## 保留失败、分支与下一步

独立wrapper在所有原样门禁后漏复制seed checker而completed=false，未改写；其独立bcrypt尾核和附加S3 control没有执行，后续保留环境的只读bcrypt证明另列，不能补造原隔离栈时点。新主镜像首轮丢执行位被拒启、SQLite停写后普通ro打开失败、native TERM两次ps间正常退出被脚本误判，也都保留原脚本/轮次。SQLite immutable只用于写方全退出且无WAL/SHM、源SHA稳定的静止点，未用于live库。

已归档/移除46个detached工作树、46条旧本地分支与4条已被祖先/归档/远端tip核验的旧远端分支；5份bundle和逐文件材料归档可恢复。保留8个attached工作树、10个本地分支，五仓活动目标和main分别保留；不将非祖先归档伪称合并。原镜像/二进制/备份及仍有重建引用的旧环境材料保留，未全局prune。过程证据/脚本/SDK仅ignored `.codex/`，Git/Docker均排除；正式结论与必要模块文档入Git。

剩余按原台账触发：通用dbtx试点/请求deadline未实现，不因B23最小修复混销；G-9合法运维收件人需业务合同，C-14需生产HTTPS前置；Outbox、完整SLA、审批、风险闭环等按后续阶段。L-1～4搬树与跨文件P2/P3整理仍后续，不把整个历史债清零。**Go1.27及Node24/26 LTS整体升级只在Phase5经授权合main后，再基于main单开批；本轮不提前实施，具体Node选型与patch届时核官方支持状态及兼容性。** 下一决策是Phase5最终评审与后续窗口，不自动合并或开启Phase6。

原始完整命令、源/ELF/工具/CI ZIP SHA、失败和清理/恢复证据仅在ignored `.codex/audits/ECO-20261104/phase5-closeout-20261107/`。最终运行独立后验已通过；尾随纯文档提交不改变上述被测代码；工作区/远端差集与main refs以推送后的独立实时检查为准，不用commit message替代事实。
