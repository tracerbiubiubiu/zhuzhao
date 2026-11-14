# 部署与联调：主应用、UI 和两上游

> 命令默认从**主仓根目录**开始；下面显式 `cd deployments` 后才使用省略 `-f` 的 Compose。配置与 Dockerfile 仍在 `deployments/`，文档迁移不移动部署文件。先按 [runbook §7](runbook.md) 确认资源归属，本页命令用于已确认的目标环境。
> 容器部署使用 `deployments/docker-compose.yaml`；宿主开发使用 dev Compose 依赖加四个宿主进程。`scripts/dev-stack.sh` 不代表完整宿主联调已就绪。上游独立部署细节由各自 `docs/ops/deployment.md` 维护。

## 〇、开发依赖与宿主应用（2026-11-12版本维护对齐）

**2026-11-13现行前提**：所有者确认现有开发栈/数据卷均为可丢弃测试环境，可按项目精确重建，再在空库执行迁移，不要求保留旧测试数据。PG18/Redis8模板使用新卷，同project不代表同一数据源；重建要核清Compose声明及旧遗留卷，不把空库称作存量迁移成功。只有需要保留数据的目标环境才须先执行[数据升级契约](../design/component-upgrade-data-contract.md)；生产及其他项目不在本次测试重建授权内。

常用宿主开发形态：主仓 PG/Redis（5432/6379）、activelist PG（15432）；应用为主仓 `make dev`（33333）、AL `make build` 后宿主进程（18081）、SQLite taskrunner 宿主进程（18080），以及 UI `pnpm dev`（4000）。各仓配置/密钥按各自文档注入，环境差异走 env，不改 `configs/config.yaml` 基线。E000001 终态与改密权限以 [runbook §4](runbook.md) 为准。

```sh
# 仅在全新环境或已恢复校验的新卷上运行；PG18/Redis8使用dev_pg18_data/dev_redis82_data
make docker-dev-up
# AL仍用project=deploy，但PG18卷改为deploy_pgdata18；原deploy_pgdata不能直接挂给PG18
(cd ../activelist && docker compose -p deploy -f deploy/compose.dev.yaml up -d --wait postgres)
# 迁移由操作者显式执行，再分别按各仓文档起宿主应用
make migrate-up
make dev  # 端口已被监听时拒启并显示归属；需要并行实例可APP_PORT=33335 make dev
```

`bash scripts/dev-stack.sh up` 的范围是：①主仓 dev PG/Redis，②AL dev PG，③TR **容器部署栈**（生产compose、仅内网、不发布宿主18080）。它不启动主仓/AL/vite宿主进程，也不会为宿主开发启动SQLite taskrunner。宿主开发需按 TR 文档另起原仓SQLite进程，并核对回调与SK；不要把容器TR健康等同宿主任务/回调链可达。纯宿主开发用上面两组依赖即可。

脚本要求Docker Compose v2（支持`up --wait`），全部配置/归属预检在任何资源变更前完成；main/AL/TR任一失败均非0，保留已成功启动的资源供检查，不回退主仓生产compose、不自动回滚或删卷。`down`按TR→AL→main停止，`reset`=down后up，**均不删数据卷或共享外部网络**；`make docker-dev-reset`是另一条显式删卷命令，不能当无损重启使用。

三组显式project默认分别为`zhuzhao-dev` / `deploy` / `taskrunner`；版本维护已更换PG/Redis新卷名，须单独完成数据迁移，不能承诺沿用旧数据源；`COMPOSE_PROJECT_NAME`全局值不能覆盖这些显式参数。可用`ZHUZHAO_DEV_PROJECT` / `ACTIVELIST_DEV_PROJECT` / `TASKRUNNER_PROJECT`指定其它、互不相同的project，或`ACTIVELIST_DIR` / `TASKRUNNER_DIR`指定同级布局之外的仓路径，`DEV_STACK_WAIT_TIMEOUT`为健康等待秒数（默认90）。**改project会改默认卷名**：已有固定容器归属或同project的Compose来源不匹配时脚本拒绝接管，先由所有者核对标签/卷与备份迁移，不自动改名、清理或换库。

离线回归：`make test-dev-stack`，仅shell/Python命令桩，不访问真实Docker或服务。本批尚未用此候选脚本做真实启停/完整acceptance；实机恢复记录与候选脚本验收分别记账。

## 一、单起 zhuzhao 栈（不含两上游）

```sh
cd deployments
# 首次参考本目录.env.example手工准备.env，已有文件保留；现有卷沿用原DB口令。
# 必填JWT_SECRET/GATEWAY_SK/TASKRUNNER_SK/INTERNAL_JOBS_SK，跨仓SK按下方对值表。
# PG/Redis虽有dev兜底，生产必须配置强值；不要向已有.env盲目追加不同口令。
docker compose up -d --wait postgres redis
# migrate带profile，普通up不会自动运行；先明确应用迁移再起app
docker compose --profile migrate build migrate
docker compose --profile migrate run --rm migrate
docker compose up -d --build app pgbackup ui
# 验证：curl http://127.0.0.1:${APP_PORT:-33333}/health/ready
```

- `migrate` 服务需显式profile/run（受控Go1.27.4重包的migrate4.20.1/PostgreSQL专用镜像，完成即退），普通`up`不含它；app依赖PG/Redis healthy，不会代执行迁移。
- `ui` = 前端控制台（nginx 静态 + `/api`·`/al/api` 精确反代 app；构建上下文相对本目录跨仓 `../../zhuzhao-ui`——**两仓同级 checkout**）。
- `APP_SERVER_TRUSTED_PROXIES`（默认 `172.16.0.0/12`）：经 ui 反代后 ClientIP 还原依赖此配置，空值=限流桶全站共享。

## 二、三栈拼装（顺序敏感）

```sh
# 从主仓根开始。① 先确认两条共享网是否已存在及归属；仅首次缺失时创建
docker network create zhuzhao_to_activelist
docker network create zhuzhao_to_taskrunner

# ② activelist 栈（细节：其 docs/ops/deployment.md；先依赖、迁移/种子成功，再确认 ready）
cd ../activelist/deploy && docker compose -f compose.prod.yaml up -d --build

# ③ taskrunner 栈（细节：其 docs/ops/deployment.md）
cd ../../taskrunner/deploy && docker compose up -d --build

# ④ zhuzhao 栈最后起（app 已配置挂接两共享网；taskrunner 回调经别名 http://zhuzhao:33333）
cd ../../zhuzhao/deployments
docker compose up -d --wait postgres redis
docker compose --profile migrate build migrate
docker compose --profile migrate run --rm migrate
docker compose up -d --build app pgbackup ui
```

## 二·五、测试环境（单文件 docker 化，2026-10-13）

**用途**：稳定回归/演示环境——与宿主进程 dev 栈（vite HMR 快改）**双形态并存**，生命周期与 AI 会话/宿主进程完全解耦（会话回收不再连坐服务进程，2026-10-12/13 app·taskrunner 反复下线的根治）。

```sh
cd deployments
docker compose -f docker-compose.test.yaml up -d --wait postgres redis activelist-pg taskrunner-pg
docker compose -f docker-compose.test.yaml build migrate
docker compose -f docker-compose.test.yaml run --rm migrate
# 逐项确认上一步退出0、schema clean；上游服务自身迁移/绑定种子成功后再核 ready
docker compose -f docker-compose.test.yaml up -d --build activelist taskrunner app ui
# 入口 = http://localhost:48081（ui nginx：静态 + /api·/al/api 双反代 app）
# 全新库的种子口令只属初始化态；E000001收敛仅走runbook §4既定例外流
# 人工/agent冒烟用专属账号，不自行登录改写E000001
# 三链验证：登录 → /al/api/v1/admin/types（活动列表链）→ /api/v1/runs（任务链）
```

- **组成**：默认 8 个常驻服务，加 migrate 一次性任务；storage profile 另起 RustFS。包括主 PG/Redis/app、AL PG/服务、TR PG/服务和 ui；独立网 `zhuzhao_test`、独立卷 `zt_*` 前缀，入口 ui 48081 / app 直连 33334。
- **数据**：当前PG18卷为`zt_pg18data/zt_al_pg18data/zt_tr_pg18data`；旧`zt_*pgdata`保留作迁移来源，已有环境先恢复校验并切换。升级完成后继续复用新版本卷。测试自举只走批准的 acceptance/globalSetup 幂等流；不得为重新演示而清共享数据。
- **配置**：`deployments/config.test.yaml` 卷挂载 app，地址、存储及 cron 等可能有环境差异；核实实际文件、Env 与运行产物，不假设只差一个 gateway target。
- **徽标**：ui 构建参数 `VITE_APP_ENV=测试环境`——登录页环境徽标显式标注，防与生产混淆；
- **E2E**：默认目标和身份准备以 UI 测试指南及脚本为准；切换目标须同时核 baseURL、账号/库归属和依赖，单改 URL 不等于完整隔离。

测试 Compose 的 migrate 虽无 profile，app 也没有等待其成功的依赖。不要并发全量 `up` 后仅凭服务后来重启成功，就宣称首次迁移/种子时序正确。`/health/ready` 走 app 直连，不在 UI 的 `/api` 反代前缀内。

## 三、密钥对值表（跨栈，拼装前核对）

| 本栈变量 | 对端变量 | 对端栈 |
|---|---|---|
| `GATEWAY_SK` | `ACTIVELIST_CALLER_ZHUZHAO_SK` | activelist |
| `TASKRUNNER_SK` | `TASKRUNNER_CALLER_ZHUZHAO_SK` | taskrunner |
| `INTERNAL_JOBS_SK` | `TASKRUNNER_SELF_SK` | taskrunner |

> taskrunner 栈五变量均 fail-closed 拒建（2026-10-01 硬化）；activelist 两变量同款。
> 网关 `/al` target 默认 `http://activelist:18081`（分配表 18081——原 8080 高危口 2026-11-02 迁）；`TASKRUNNER_BASE_URL` 默认 `http://taskrunner:18080`——共享网别名寻址，均无需改配置。

## 三·五、PostgreSQL 连接预算（2026-11-06 ECO-B06-F08）

`database.max_open_conns` 只约束每个 app 的业务连接池。CronScheduler 的会话级 advisory lock 使用独立短命连接，同一调度器串行执行、每次最多额外一条；握手/取锁/Handle 共享两分钟执行预算，解锁和关闭各有独立两秒清理预算。失败、panic、取消和解锁异常均关闭专用会话，不把带锁连接放回业务池；锁竞争跳过不算失败，取锁/建连/清理失败记录 cron 状态并触发既有失败回调。

部署估算 PostgreSQL 总连接至少按 **app副本数 ×（每副本业务池上限 + 同时活跃cron数）+ 迁移/备份/运维连接 + 余量** 预留。当前最多七个维护调度器，部分按开关启用；业务池默认25时每副本上界按25+7估算，不宣称全生态所有PG服务共享同一个上限。实际cron周期错峰通常低于此值，但不能依赖错峰代替预算。请结合数据库实例的`max_connections`与其他服务预算核对，不能只把业务池调大来规避嵌套取连。

## 四、全链冒烟（拼装后）

```sh
UI=http://127.0.0.1:${UI_PORT:-48080}
curl -s -o /dev/null -w '%{http_code}\n' $UI/                      # SPA 壳 → 200
TOKEN=$(curl -s -X POST $UI/api/v1/auth/login -H 'Content-Type: application/json' \
  -d '{"employee_no":"<工号>","password":"<密码>","device_id":"smoke"}' \
  | python3 -c "import sys,json;print(json.load(sys.stdin)['data']['access_token'])")
curl -s "$UI/al/api/v1/admin/types?page_size=1" -H "Authorization: Bearer $TOKEN" | head -c 120
# → {"code":0,...}：四跳全链（nginx→app→网关 AK/SK→activelist）通
curl -s "$UI/api/v1/runs?page=1&page_size=1" -H "Authorization: Bearer $TOKEN" | head -c 120
# → {"code":0,...}：app→taskrunner 代理链通
```

> 本机已按此口径实测（2026-10-01，四仓联合验证批）：UI 镜像五项冒烟全绿。
