# zhuzhao 部署 runbook（含三栈拼装）

> 部署态 = `docker-compose.yaml`（本目录）；开发态 = `docker-compose.dev.yaml` + `scripts/dev-stack.sh`。
> 本文件是**三栈拼装的总 runbook**：activelist / taskrunner 各自栈的细节见其仓 `deploy/README.md`。

## 一、单起 zhuzhao 栈（不含两上游）

```sh
cd deployments
# env 口径（如实）：JWT_SECRET 无兜底必填；POSTGRES_PASSWORD/REDIS_PASSWORD 有 dev
# 兜底 zhuzhao_dev——生产务必经 .env 覆盖强值（弱兜底清理已登记主仓随手项池）
export JWT_SECRET=<强值>
echo 'POSTGRES_PASSWORD=<强值>' >> .env
echo 'REDIS_PASSWORD=<强值>' >> .env
docker compose up -d --build     # postgres/redis/migrate/app/pgbackup/ui
# 验证：curl http://127.0.0.1:${APP_PORT:-33333}/health/ready
```

- `migrate` 服务先行（migrate/migrate v4.17，up 完成即退）；app 依赖 PG/Redis healthy。
- `ui` = 前端控制台（nginx 静态 + `/api`·`/al/api` 精确反代 app；构建上下文跨仓 `../zhuzhao-ui`——**两仓同级 checkout**）。
- `APP_SERVER_TRUSTED_PROXIES`（默认 `172.16.0.0/12`）：经 ui 反代后 ClientIP 还原依赖此配置，空值=限流桶全站共享。

## 二、三栈拼装（顺序敏感）

```sh
# ① 预建两条共享网（一次性；本栈 app 容器加入后与两上游互通）
docker network create zhuzhao_to_activelist
docker network create zhuzhao_to_taskrunner

# ② activelist 栈（细节：../activelist/deploy/README.md）
cd ../activelist/deploy && docker compose -f compose.prod.yaml up -d --build

# ③ taskrunner 栈（细节：../taskrunner/deploy/README.md）
cd ../../taskrunner/deploy && docker compose up -d --build

# ④ zhuzhao 栈最后起（app 已配置挂接两共享网；taskrunner 回调经别名 http://zhuzhao:33333）
cd ../../zhuzhao/deployments && docker compose up -d --build
```

## 二·五、测试环境（单文件 docker 化，2026-10-13）

**用途**：稳定回归/演示环境——与宿主进程 dev 栈（vite HMR 快改）**双形态并存**，生命周期与 AI 会话/宿主进程完全解耦（会话回收不再连坐服务进程，2026-10-12/13 app·taskrunner 反复下线的根治）。

```sh
cd deployments
docker compose -f docker-compose.test.yaml up -d --build   # 首建 5-10 分钟（Go+前端全量构建）
# 入口 = http://localhost:4001（ui nginx：静态 + /api·/al/api 双反代 app）
# 首登引导：admin / admin12345（与全生态终态口令同系；种子首登亦走强制改密链路）
# 三链验证：登录 → /al/api/v1/admin/types（活动列表链）→ /api/v1/runs（任务链）
```

- **组成**：单文件全组件（10 容器）——zhuzhao PG/Redis/migrate/app + activelist PG/apiserver + taskrunner PG/taskrunner + ui（nginx 静态+双反代）；独立网 `zhuzhao_test`、独立卷 `zt_*` 前缀、宿主端口全错开（ui 4001 / app 直连 33334）；
- **数据**：全新干净态（与 dev 栈零共享）——业务自举走首登改密流程（E2E globalSetup 同款链路）；
- **配置**：`config.test.yaml` 卷挂载 app（与 configs/config.yaml 的唯一差异=gateway target 指本栈 activelist 服务名；configs 演进需人工对齐本文件）；
- **徽标**：ui 构建参数 `VITE_APP_ENV=测试环境`——登录页环境徽标显式标注，防与生产混淆；
- **E2E**：哨兵/全量仍打宿主 dev 栈（4000）；本栈的 E2E 打点切换=改 playwright baseURL 一行（触发驱动）。

## 三、密钥对值表（跨栈，拼装前核对）

| 本栈变量 | 对端变量 | 对端栈 |
|---|---|---|
| `GATEWAY_SK` | `ACTIVELIST_CALLER_ZHUZHAO_SK` | activelist |
| `TASKRUNNER_SK` | `TASKRUNNER_CALLER_ZHUZHAO_SK` | taskrunner |
| `INTERNAL_JOBS_SK` | `TASKRUNNER_SELF_SK` | taskrunner |

> taskrunner 栈五变量均 fail-closed 拒建（2026-10-01 硬化）；activelist 两变量同款。
> 网关 `/al` target 默认 `http://activelist:8080`；`TASKRUNNER_BASE_URL` 默认 `http://taskrunner:8080`——共享网别名寻址，均无需改配置。

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
