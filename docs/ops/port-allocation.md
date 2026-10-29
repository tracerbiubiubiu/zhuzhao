# 端口分配表（全生态权威——zhuzhao / taskrunner / activelist / zhuzhao-ui）

> **定位**：全仓端口分配唯一权威。新增服务、新增端点、新增测试基础设施先查本表登记再取号——防端口冲突（所有者 2026-10-26 发起）。
>
> **三原则**：
> 1. **固定端口**（dev compose / acceptance / 前端）从本表取号，新占用先登记——冲突时用环境变量覆盖（如 `APP_PORT=33334 make dev`）。
> 2. **动态端口**（testcontainers / CI 集成测试）不加约束——Docker 随机分配天然零冲突，39000–39999 是预留段而非绑定规则。
> 3. **跨仓**（taskrunner / activelist）在同一张表登记（所有者维护，三仓共用），各仓 compose 自管映射。

## 端口分配总表

### zhuzhao 主仓（dev / 测试 / 验收）

| 端口 | 用途 | 使用环境 | 占用方 | 可覆盖 env |
|------|------|----------|--------|------------|
| **33333** | zhuzhao app HTTP（API + 代理下载） | dev / acceptance / E2E | compose app 容器 + `make dev` 本地 | `APP_PORT` |
| **33334** | zhuzhao app 第二实例（并行会话 / 调试） | dev 并行 | `APP_PORT=33334 make dev` | `APP_PORT` |

> **33334 状态更新（2026-10-15）**：机制已接线（config.go `server.port`↔`APP_PORT` BindEnv +
> Makefile kill 行参数化）——`APP_PORT=33334 make dev` 实测可双实例共存（33333/33334 各自
> 独立服务互不误杀）。两会并用时：并行会话占 33333,本表第二实例走 33334。
| **48080** | zhuzhao-ui 容器（nginx） | dev compose | compose ui 容器 | `UI_PORT` |
| **4000** | vite dev server（前端本地开发 / E2E webServer） | dev / E2E | vite（ui 仓，**strictPort 已启**） | `VITE_PORT`（vite.config 消费） |
| **5432** | PostgreSQL（dev compose） | dev | compose postgres 容器 | 映射 `"5432:5432"` 已启用（docker-compose.dev.yaml L16 实测；原「已注释不映射」描述与文件不符——本机无原生 5432 时该映射无害） |
| **5433** | PostgreSQL（**测试专用**——本机独立容器跑 migration 验证） | audit / migration 验证 | 手动 `docker run -p 5433:5432` | — |
| **6379** | Redis（dev compose） | dev | compose redis 容器 | — |
| **9000** | RustFS API（S3 兼容端点） | dev | compose rustfs 容器 | `RUSTFS_API_PORT` |
| **9001** | RustFS Console（管理面板） | dev | compose rustfs 容器 | `RUSTFS_CONSOLE_PORT` |
| **9002** | RustFS backup API（第二实例 S3） | dev | compose rustfs-backup 容器 | 不映射（compose 内网互访） |

### 跨仓服务（dev compose 内网互通）

| 端口 | 用途 | 使用环境 | 占用方 | 仓 |
|------|------|----------|--------|-----|
| **18080** | taskrunner HTTP API | dev compose | taskrunner 容器 | taskrunner 仓（⚠ 2026-10-15 核对：compose 未发布宿主端口（C3 内网），容器态亦未常驻——现状为宿主过渡形态，见下节） |
| **18081** | activelist HTTP API | dev compose | activelist 容器 | activelist 仓（⚠ 同上——现状为宿主过渡形态，见下节） |

### 宿主过渡形态（2026-10-15 核对登记——现状实际占用，与 compose 形态并存期）

当前 dev 实跑为宿主 go-run 形态（compose 的 app 容器态未建成/未常驻），实际占用登记如下：

| 端口 | 用途 | 占用方 | 说明 |
|------|------|--------|------|
| **8081** | taskrunner 宿主直跑（SQLite 零依赖） | `TASKRUNNER_HTTP_ADDR=:8081 go run ./cmd/taskrunner` | E2E W5/任务中心数据源；compose 容器态建成（18080 内网）后退役 |
| **8080** | activelist 宿主直跑 | `ACTIVELIST_PG_PORT=15432 go run ./cmd/apiserver` | E2E W5/活动列表数据源；PG 走 activelist-dev compose（15432） |
| **15432** | activelist dev PostgreSQL（宿主调试口） | activelist compose.dev `"15432:5432"` | compose.dev.yaml 自带映射，非本表新增 |

### 预留段

| 端口段 | 用途 | 说明 |
|--------|------|------|
| **39000–39999** | testcontainers 随机端口（Docker 分配） | 集成测试的 PG / RustFS / 临时 HTTP——Docker 自主分配，非强制绑定 |
| **33335–33399** | zhuzhao 未来多实例（当前零使用） | 并行测试 / 多环境 dev |
| **48081–48099** | ui 未来多实例（当前零使用） | 并行前端 dev |
| **5434–5439** | PG 测试多实例（当前零使用，5433 已用） | 并行 migration 验证 |
| **9100–9199** | 对象存储测试多实例（当前零使用） | RustFS 第三实例等 |

## 冲突处理指引

### 常见冲突场景

| 场景 | 症状 | 解法 |
|------|------|------|
| 本机装了 PostgreSQL | compose postgres 起不来（5432 已占） | compose 注释了端口映射（内网互访），无需映射——若需 psql 直连改映射 `- "5433:5432"` |
| 本机装了 Redis | compose redis 起不来（6379 已占） | 同上——compose 未映射 6379；本地 `make dev` 走容器内网 |
| dev app 没跑 | acceptance 脚本全红（33333 连不上） | 先 `make dev`；或换端口 `BASE_URL=http://localhost:33334 bash scripts/acceptance-phase1.sh` |
| 两个 vite 项目同时跑 | 4000 被占（~~vite 自动跳 4001~~ **2026-10-29 strictPort 后显式失败**） | 杀遗留实例（`lsof -ti:4000 -sTCP:LISTEN`）或 `VITE_PORT=48081 pnpm dev` 换预留段——原「自动避让无害」口径已证伪（见变更记录 10-29：auto-increment 静默漂移掩盖遗留进程四天，E2E reuseExistingServer 一直打旧实例） |
| IM 软件（QQ/微信）动态抢端口 | vite 起在 4001 后 QQ 抢注 127.0.0.1:4001 IPv4——浏览器空响应（QQ specific 绑定压过 vite 通配绑定） | IM 本地监听**动态随机无可避开区间**；对策=strictPort 显性失败 + `VITE_PORT=48081+` 换预留段 + 重启 IM（其端口会重摇） |
| dev app 换了端口 | E2E 连不上 33333 | `E2E_API_BASE=http://127.0.0.1:33334` 覆盖 |

### 宿主形态的域名解析（2026-10-15 追加——本机开发/E2E 免改 config）

compose 形态下 zhuzhao 网关上游为 `http://activelist:8080`（容器网络名）。**宿主机直跑
zhuzhao（make dev）时该域名不可达** → /al 反代 502。两种解法：

1. **本机别名（推荐，一次性）**：`sudo sh -c 'echo "127.0.0.1 activelist" >> /etc/hosts'`
   ——此后 config.yaml 保持仓内默认，**不再需要「改→测→还原」循环**（2026-10-15 前
   的宿主态临时改动即此痛点的历史形态）；仅本仓使用该名字，机器级无害；
2. config 文件选择（未实现，备用）：`APP_CONFIG` env 选加载文件——main.go 的
   `config.Load` 路径当前硬编码，需要时三行改造。

注意：容器内 DNS 与宿主 /etc/hosts 相互独立——compose 形态（zhuzhao 容器态）不受
该别名影响；宿主形态（make dev）下 activelist 走宿主 8080 直跑，与别名解析一致。

### 验收脚本端口可覆盖（已支持）

所有 acceptance 脚本第一行已支持环境变量覆盖：
```bash
# 默认 33333
bash scripts/acceptance-phase1.sh

# 换端口
BASE_URL=http://localhost:33334/api/v1 HEALTH_URL=http://localhost:33334/health/ready bash scripts/acceptance-phase1.sh
```

### 新增服务取号纪律

1. 查本表确认端口未被占用
2. 优先用预留段（33335+ / 48081+ / 5434+ / 9100+）
3. 在本表登记（用途 / 环境 / 占用方 / env 变量）
4. compose 加映射时引用 env 变量（对齐 `"${APP_PORT:-33333}:33333"` 既有模式）

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-10-26 | 建档（所有者发起：测试/验证端口未提前分配致经常冲突）；首版分配表 + 三原则 + 冲突处理指引 |
| 2026-10-29 | **vite 端口事故批**：10/25 遗留 vite 占 4000 致全部实例 auto-increment 漂移 4001（四天未察觉——E2E reuseExistingServer 一直打旧实例，同仓代码同源未炸属侥幸）；QQ 又抢注 127.0.0.1:4001 IPv4（specific 绑定压过 vite 通配绑定）致浏览器空响应。整改：ui 仓 vite.config +strictPort（显式失败替代静默漂移）+VITE_PORT 覆盖口（对齐本表原则 1）；冲突表修正「无害自动避让」证伪口径 + 新增 IM 动态端口应对行 |\n