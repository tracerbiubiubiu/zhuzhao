# 05 · 计划任务脚本节点契约（Python 为主）

> 性质：发散研究稿（见目录 README）。回答所有者问题："是否需要支持上传自定义脚本作为节点？脚本的输入输出要先定义好——以 Python 为主，是否要先规划日志、输入、输出参数等，有例子吗？"

## 0. 结论先行

1. **需要，但形态是"脚本即 Action"而非"上传脚本功能"**：脚本只是 Action 的一种 kind（03-A1），与连接器、内建动作共用注册表、表单生成、编排节点、AI 工具四个消费面。单独做一个"脚本上传跑一下"的功能是死胡同——没有契约的脚本既不能编排、也不能被 AI 调用、也无法审计输入输出。
2. **先定契约，再做执行面**：契约（本文）只依赖 stdin/stdout/stderr/env/exit code 五件事，与隔离档位（容器池 / 每运行一容器 / microVM）完全解耦——执行面可以后换，契约一旦发布就是对脚本作者的承诺，改不动。
3. **业界已有十年验证的共识**（01-§3.6 八条），本文据此给出规范 + 两个完整样例 + Python SDK 草图 + 安全基线。

## 1. 业界先例速览（详见 01-§3）

| 先例 | 输入 | 输出 | 日志 | 声明 | 隔离 |
|---|---|---|---|---|---|
| XSOAR automation | `demisto.args()` 字典 | `return_results()` 结构化 | `demisto.debug()` 独立通道 | YAML（参数/输出 context） | 每 integration 一镜像 |
| Cortex analyzer/responder | JSON 文档（观测值+配置） | JSON 报告（taxonomies） | stderr | JSON 规格（数据类型/级别） | 每分析器一容器 |
| StackStorm action | `run(**params)` | 返回 dict | 标准 logging | pack YAML 参数 schema | 进程 / 容器 runner |
| Rundeck job step | options → env / 参数 | 退出码 + 输出捕获 | 按 job 落盘 | job 定义（options 类型校验） | 节点执行 |
| AWS Lambda | event JSON | return JSON | 独立日志通道 | 配置（超时/内存/env） | microVM |
| MCP tool | JSON Schema 入参 | 结构化 content | — | tool 定义 | 由宿主决定 |

**蒸馏**：声明式 manifest · 类型化 I/O · 数据与日志通道分离 · secret 环境注入 · 工件目录 · 退出码语义 · 超时/取消归执行器 · 一切有上限。

## 2. 契约总览

```
脚本包（zip，存 S5 对象存储，版本化、审批后生效）
├── manifest.yaml        ← 声明：身份/kind/运行时/输入输出 schema/secrets/工件/网络/依赖/资源
├── main.py              ← 入口（或 manifest.spec.entry 指定）
├── requirements.txt     ← 依赖（可选）
├── requirements.lock    ← hash 锁定（审批时由平台生成或校验）
└── README.md            ← 给人看的说明（可选，UI 展示）

运行时五通道
  stdin   ◀── 输入 JSON（已按 inputs schema 校验）
  stdout  ──▶ 输出 JSON（单文档，按 outputs schema 校验）         ← 数据通道，只许放结果
  stderr  ──▶ 日志（行级捕获，级别解析，上限截断，secret 脱敏）    ← 日志通道
  env     ◀── ZH_* 运行上下文 + manifest.secrets 声明的秘密
  $ZH_ARTIFACT_DIR ──▶ 工件文件（manifest.artifacts 白名单，运行后上传 S5）
  exit code ──▶ 成功/业务失败/可重试失败/输入无效/超时/取消 语义
```

## 3. manifest 规范

### 3.1 完整示例（带注释）

```yaml
apiVersion: scripts.zhuzhao/v1alpha1
kind: Analyzer                 # Job（定时/手动作业）| Analyzer（富化：对实体打分打标）| Responder（处置：对实体执行动作）
metadata:
  name: ip.intel.enrich        # 即 actions.action_id；小写点分，全局唯一
  version: 1.2.0               # semver；与 actions.version 一致
  displayName: IP 威胁情报富化
  description: 查询内部情报（经 S9 缓存代理）并可选合并外部厂商结果，输出评分与判定。
  owner: sec-ops               # 责任团队/人（用于告警与审批路由）
  tags: [threat-intel, enrichment]
spec:
  runtime: python3.11          # 平台提供的运行时清单之一（固定基础镜像，内置 zh SDK）
  entry: main.py               # 默认 main.py
  output: stdout               # stdout（默认，整段单 JSON）| file——file 时结果写 $ZH_OUTPUT_FILE、stdout 全归日志（GitHub Actions / Tekton 结果文件式；09 号补强）
  resourceClass: small         # small|medium|large → CPU/内存/最长时限上限（见 3.4）
  timeoutSeconds: 60           # 必须 ≤ resourceClass 上限
  idleTimeoutSeconds: 0        # 0=不启用；>0 时连续无日志输出超此秒数视为卡死
  retryPolicy:                 # 仅对 exit code=2（可重试失败）生效
    maxAttempts: 3
    backoff: exponential
  idempotent: true             # 可安全重试（Responder 一般为 false）
  inputs:                      # JSON Schema（draft 2020-12 子集）；UI 表单与 AI 工具定义据此生成
    type: object
    required: [ip]
    additionalProperties: false
    properties:
      ip:      { type: string, format: ipv4, title: 目标 IP }
      sources: { type: array, items: { type: string, enum: [internal, vendor-a] }, default: [internal], title: 情报源 }
      asset:   { type: string, format: asset-ref, title: 关联资产, description: 可选，用于结果挂接 }   # 平台扩展 format → 资产选择器
  outputs:
    type: object
    required: [score, verdict]
    properties:
      score:    { type: integer, minimum: 0, maximum: 100 }
      verdict:  { type: string, enum: [clean, suspicious, malicious, unknown] }
      families: { type: array, items: { type: string } }
      sources_used: { type: array, items: { type: string } }
  emits: []                    # Job 常用：输出中的 findings 自动转为 security.alert（见样例 B）
  secrets: [VENDOR_A_API_KEY]  # 运行时以同名环境变量注入；值永不进 inputs/日志/输出
  secretsMode: env             # env（默认）| file——file 时注入到 $ZH_SECRETS_DIR/<NAME> 文件，避免 env 经 /proc、子进程继承泄露（随容器隔离第二档启用；09 号补强）
  platformAccess: [intel.query, assets.resolve]   # SDK 可调用的平台能力白名单（最小权限）
  artifacts: []                # 允许产出的工件文件名/通配白名单，如 ["report.csv", "evidence/*.png"]
  network:
    egress: allowlist          # none（默认，完全禁网）| internal（仅内网段）| allowlist
    allow: [intel-vendor-a.internal.example]
  dependencies:
    requirements: requirements.txt   # 审批时锁定 hash 并预构建 venv；运行时零安装
  logging:
    maxLineBytes: 4096
    maxTotalBytes: 1048576     # 1 MB，可上调至 8 MB
    format: text               # text | jsonl
```

### 3.2 字段参考

| 字段 | 必填 | 说明 |
|---|---|---|
| `kind` | ✓ | 三种语义：**Job** 无主实体、通常定时；**Analyzer** 输入含实体、输出评估结果（可挂到工单/风险单/资产）；**Responder** 对实体执行副作用（默认 `idempotent:false`、审批节点常前置）。kind 决定 UI 入口与编排节点可用位置 |
| `metadata.name/version` | ✓ | 即 Action 身份；重新上传同版本被拒，必须升版 |
| `spec.runtime` | ✓ | 平台维护的受支持运行时清单（第一档仅 `python3.11`；同协议可扩 `python3.12`/`node20`/`binary`） |
| `spec.inputs/outputs` | ✓ | JSON Schema 子集：type/enum/required/default/format/min/max/items/properties/additionalProperties。**禁止** `format: secret`——秘密不走输入 |
| 平台扩展 format | — | `asset-ref` `user-ref` `org-ref` `ticket-ref` `cidr` `datetime` `duration` → 对应 UI 控件与服务端解析；**校验责任在平台侧**：JSON Schema 2020-12 中 `format` 属注解性质、默认不校验，扩展 format 须由平台 validator 自定义实现（合并卡审计 P3 补注） |
| `spec.secrets` | — | 名称列表；凭据本体存平台凭据库，审批时校验存在性；runner 注入 env，日志脱敏按值匹配 |
| `spec.secretsMode` | — | `env`（默认）或 `file`；file 模式注入到 `$ZH_SECRETS_DIR/<NAME>` 文件，避免 env 经 /proc、子进程继承、崩溃转储泄露（随容器隔离第二档启用；09 号补强） |
| `spec.output` | — | `stdout`（默认，整段单 JSON）或 `file`（结果写 `$ZH_OUTPUT_FILE`，stdout 全归日志——GitHub Actions / Tekton 结果文件式；两通道并存由作者选；09 号补强） |
| `spec.platformAccess` | — | 白名单：`assets.resolve` `assets.list` `intel.query` `tickets.read` `events.emit`…；runner 签发运行期短令牌（principal=脚本的 service account，on_behalf_of=触发人，03-A6），SDK 用之调平台 API |
| `spec.emits` | — | 声明输出中哪条路径的元素按什么事件类型投递到 `/internal/v1/events`（样例 B）；`map` 值为 mustache 式 `{{ field }}` 模板——点路径取值、无逻辑、转义由投递器处理（方言钉死防各写各的；2026-10-20 双审计补注） |
| `spec.network` | — | 默认 `none`。`allowlist` 经内部出站代理放行；`internal` 为内网段整体放行（巡检类常用） |
| `spec.artifacts` | — | 白名单外的文件不上传且告警；单文件/总量上限由 resourceClass 决定 |

### 3.3 kind 与消费面的对应

| kind | 触发方式 | 编排节点位置 | 工单动作库 | AI 工具暴露 |
|---|---|---|---|---|
| Job | cron / 手动 / 工作流 | 任意 | 否（无主实体） | 可（需显式开启） |
| Analyzer | 工作流 / 工单页"富化" / 告警流水线 / AI | 任意 | 是（结果回贴工单） | 是（默认） |
| Responder | 工作流（审批节点后）/ 工单页"处置"（需权限码）/ AI（需确认） | 必须可达审批节点或显式豁免 | 是 | 可（默认需人工确认） |

### 3.4 资源档位（示意，可配）

| class | CPU | 内存 | 最长时限 | 工件总量 |
|---|---|---|---|---|
| small | 0.5 核 | 256 MB | 5 分钟 | 10 MB |
| medium | 1 核 | 1 GB | 30 分钟 | 100 MB |
| large | 2 核 | 2 GB | 2 小时 | 500 MB |

## 4. 运行时协议

### 4.1 环境变量（runner 注入）

| 变量 | 含义 |
|---|---|
| `ZH_RUN_ID` / `ZH_ATTEMPT` | 本次运行 ID / 第几次尝试（重试时递增；幂等键据此设计） |
| `ZH_ACTION` / `ZH_ACTION_VERSION` | 脚本身份 |
| `ZH_ARTIFACT_DIR` | 工件目录（唯一可写目录之一，另一个是 `/tmp`） |
| `ZH_API_BASE` / `ZH_RUN_TOKEN` | 平台 API 地址与运行期短令牌（仅当 `platformAccess` 非空） |
| `ZH_LOG_FORMAT` | `text`/`jsonl`，与 manifest 一致 |
| `ZH_TRIGGER` | `cron|manual|workflow|alert|ai`，JSON 串含触发者与上游引用（工单号/告警 ID/工作流 run） |
| `ZH_DEADLINE` | RFC3339 截止时刻（脚本可据此提前收尾） |
| `ZH_OUTPUT_FILE` | 仅 `spec.output: file` 时设置：结果 JSON 写入此文件，stdout 全归日志（09 号补强） |
| `ZH_SECRETS_DIR` | 仅 `spec.secretsMode: file` 时设置：秘密以 `<NAME>` 文件挂载于此（09 号补强） |
| `ZH_TRACE_ID` | 全链追踪 ID（OpenTelemetry 语义），贯穿 taskrunner → runner → 回调；SDK 日志自动附带（09 号补强） |
| `<SECRET_NAME>` | manifest.secrets 中的每个名字 |

### 4.2 通道规则

| 通道 | 规则 |
|---|---|
| stdin | 一份 UTF-8 JSON 文档 = inputs（已校验，默认值已填充）；脚本须读完再处理 |
| stdout | **整个 stdout 必须是且仅是一份 JSON 文档**（≤1 MB）。任何其它输出（print 调试）都会导致 `output_valid=false` 并以 exit code 3 语义处理——SDK 在运行期间把 `sys.stdout` 重定向到 stderr，正是为了防这类事故。**`spec.output: file` 时**：结果写 `$ZH_OUTPUT_FILE`，stdout 全部视为日志（GitHub Actions / Tekton 式；09 号补强，两通道并存由作者选） |
| stderr | 日志。行级捕获；每行 ≤ `maxLineBytes`（超截断）；总量 ≤ `maxTotalBytes`（超则截断并在运行记录标 `truncated=true`，全量落 S5 文件引用）。任何一行 stderr 都刷新"存活时间"（心跳） |
| 工件 | 运行结束后 runner 扫描 `ZH_ARTIFACT_DIR`，按白名单上传 S5，运行记录给预签名引用；可挂接到上游工单/风险单（`ZH_TRIGGER` 带上游引用时自动挂） |

### 4.3 退出码语义

| exit code | 含义 | 平台处理 |
|---|---|---|
| 0 | 成功 | 校验 outputs → 状态 succeeded |
| 1 | 业务失败（如目标不可达、判定为需人工） | failed，**不重试**；通知按策略 |
| 2 | 可重试的临时失败（网络抖动、上游 5xx、限流） | 按 retryPolicy 退避重试，耗尽 → dead |
| 3 | 输入无效 / 契约违反（脚本自检发现输入不合预期；或 stdout 非法 JSON） | failed，不重试，标记"契约错误"给作者 |
| 124 | 超时（runner 发 SIGTERM 后宽限 10s 再 SIGKILL） | failed，不重试（除 manifest 显式 `retryOnTimeout`） |
| 130 | 取消（用户/工作流取消 → SIGTERM） | canceled |
| 137 | 被杀（OOM / SIGKILL） | failed，标记资源不足建议升 class |
| 其它 | 未分类错误 | failed，不重试 |

### 4.4 取消与超时

runner 到 `timeoutSeconds`（或收到取消）→ 向进程组发 SIGTERM → 宽限 10 秒 → SIGKILL。脚本应处理 SIGTERM 做清理（SDK 提供 `zh.on_cancel(callback)`），并可读 `ZH_DEADLINE` 主动提前收尾。

## 5. 日志契约

- **text 格式**（默认）：自由文本行；若行以 `DEBUG|INFO|WARN|ERROR` 开头，runner 解析为级别并索引（UI 可按级别过滤）；推荐 `key=value` 尾随（示例见 §8）。
- **jsonl 格式**：每行一个 JSON 对象 `{"ts":…, "level":…, "msg":…, …}`，适合需要结构化检索的脚本。
- **脱敏**：runner 对 stderr/stdout/工件文本做"精确值替换"（manifest.secrets 的值 → `***`）；这是兜底，**脚本仍须不打印秘密**。
- **保留**：行级日志随运行记录保留 N 天（可配，默认 90）；超限全量文件在 S5 走对象存储生命周期。
- **UI 呈现**：运行详情页 = 元信息（exit code/时长/资源峰值）+ 输入/输出（schema 驱动的只读表单）+ 日志（分级过滤/搜索/跟随）+ 工件列表。

## 6. Python SDK（`zh`，内置于运行时镜像；非强制）

设计原则：**薄**——只封装协议，不引入框架；裸协议（不 import zh）同样合法。

```python
# zh/script.py —— 核心面（草图，~150 行实现）
def run(main):             # 入口包装：接管 stdout→stderr、捕获异常→exit code、SIGTERM→on_cancel、最终写结果
def input() -> dict:       # 读取并缓存 stdin JSON
def output(result: dict):  # 登记结果；run() 结束时写入真正的 stdout，或 spec.output=file 时写 $ZH_OUTPUT_FILE（只允许调用一次）
def artifact(path) -> str: # 返回 $ZH_ARTIFACT_DIR 下的绝对路径；也可 artifact.write(name, bytes)
def secret(name) -> str:   # 读 env，缺失抛 ContractError（→ exit 3）
def fail(msg, retry=False):# 抛 BusinessError（exit 1）或 RetryableError（exit 2）
def on_cancel(cb):         # 注册 SIGTERM 回调
log: Logger                # log.info/warn/error/debug(msg, **kv) → stderr，按 ZH_LOG_FORMAT 输出
ctx: Context               # run_id / attempt / trigger / deadline 等只读属性
ctx.cancelled: bool        # 取消信号已到（SIGTERM 后置真；样例 B 循环内检查——三审 E-F2 补齐）

# zh/platform.py —— 平台访问（受 platformAccess 白名单约束，走 ZH_RUN_TOKEN）
assets.resolve(identifiers) / assets.list(kind=..., org_id=...)   # 经平台解析门面调外部资产平台（平台不自建台账）
intel.query(type, value)                       # 经 S9 缓存代理
events.emit(type, payload)                     # 等价于 manifest.emits 的显式版本
tickets.get(id) / tickets.comment(id, text)    # 需 tickets.read / tickets.comment
class Unavailable(Exception)    # 平台能力不可用（配额耗尽且无缓存等）——样例 A 捕获用
class TransientError(Exception) # 外部上游临时故障（网络抖动/5xx/限流）——样例 A 用，配合 zh.fail(retry=True)→exit 2（三审 E-F2 补齐）
```

裸协议最小合法脚本（不依赖 SDK）：
```python
import json, sys
args = json.load(sys.stdin)
sys.stderr.write(f"INFO start ip={args['ip']}\n")
print(json.dumps({"score": 0, "verdict": "unknown", "families": []}))
```

## 7. 完整样例 A · Analyzer：IP 威胁情报富化

**manifest.yaml**：即 §3.1 示例。

**main.py**
```python
from zh import script as zh
from zh import platform

def main():
    args = zh.input()
    ip = args["ip"]
    zh.log.info("enrich start", ip=ip, sources=args["sources"])

    merged = {"score": 0, "verdict": "unknown", "families": set(), "used": []}

    # 1) 内部情报（经 S9 缓存代理；受 platformAccess: intel.query）
    try:
        r = platform.intel.query("ip", ip)
        merged["score"] = max(merged["score"], r.get("score", 0))
        merged["families"].update(r.get("families", []))
        merged["used"].append("internal")
        zh.log.info("internal intel ok", score=r.get("score"), stale=r.get("stale", False))
    except platform.Unavailable as e:      # 配额耗尽且无缓存等
        zh.log.warn("internal intel unavailable", reason=str(e))

    # 2) 外部厂商（可选；secret 经 env 注入，网络走 allowlist）
    if "vendor-a" in args["sources"]:
        key = zh.secret("VENDOR_A_API_KEY")
        try:
            v = query_vendor_a(ip, key)        # 自行实现：requests 调 intel-vendor-a.internal.example
            merged["score"] = max(merged["score"], v["score"])
            merged["families"].update(v.get("families", []))
            merged["used"].append("vendor-a")
        except TransientError as e:
            zh.fail(f"vendor-a transient: {e}", retry=True)   # → exit 2，平台按 retryPolicy 重试

    merged["verdict"] = ("malicious" if merged["score"] >= 80 else
                         "suspicious" if merged["score"] >= 40 else
                         "clean" if merged["used"] else "unknown")
    zh.output({
        "score": merged["score"],
        "verdict": merged["verdict"],
        "families": sorted(merged["families"]),
        "sources_used": merged["used"],
    })

if __name__ == "__main__":
    zh.run(main)
```

**stdin（平台注入）**
```json
{"ip": "10.0.0.8", "sources": ["internal", "vendor-a"], "asset": "ast_01HZX..."}
```

**stderr（日志）**
```
INFO enrich start ip=10.0.0.8 sources=['internal', 'vendor-a']
INFO internal intel ok score=35 stale=False
```

**stdout（结果）**
```json
{"score": 72, "verdict": "suspicious", "families": ["Scanner.Generic"], "sources_used": ["internal", "vendor-a"]}
```

**运行记录（平台落库/展示）**
```json
{
  "run_id": "run_01HZY...", "action_id": "ip.intel.enrich", "version": "1.2.0", "kind": "Analyzer",
  "status": "succeeded", "exit_code": 0, "attempt": 1,
  "started_at": "2026-10-20T09:00:01+08:00", "finished_at": "2026-10-20T09:00:03+08:00", "duration_ms": 1830,
  "resource": {"class": "small", "cpu_ms": 410, "mem_peak_mb": 48},
  "input": {"ip": "10.0.0.8", "sources": ["internal", "vendor-a"], "asset": "ast_01HZX..."},
  "output": {"score": 72, "verdict": "suspicious", "families": ["Scanner.Generic"], "sources_used": ["internal", "vendor-a"]},
  "output_valid": true,
  "logs": {"lines": 2, "truncated": false, "ref": "/v1/runs/logs?run_id=run_01HZY..."},
  "artifacts": [],
  "actor": {"principal": "svc:script-runner", "on_behalf_of": "E000002", "via": "ticket:T-2026-0188"},
  "runner": "sandbox-runner-1", "network_egress": ["intel-vendor-a.internal.example:443"]
}
```
结果回贴：因 `ZH_TRIGGER` 带 `ticket:T-2026-0188`，平台自动在该工单追加一条"富化结果"结构化评论（schema 驱动渲染），并在工单资产字段的快照上记录 `verdict`（资产本体在外部资产平台，本平台不维护资产标签）。

## 8. 完整样例 B · Job：证书到期巡检（定时，输出自动转告警）

**manifest.yaml（节选差异）**
```yaml
kind: Job
metadata: { name: tls.cert.expiry-scan, version: 0.3.0, displayName: TLS 证书到期巡检, owner: sec-ops }
spec:
  runtime: python3.11
  resourceClass: medium
  timeoutSeconds: 900
  idleTimeoutSeconds: 120
  inputs:
    type: object
    properties:
      days_threshold: { type: integer, default: 30, minimum: 1, maximum: 365, title: 到期阈值（天） }
      asset_kind:     { type: string, enum: [domain, service], default: service }
      org:            { type: string, format: org-ref, title: 限定组织（可选） }
  outputs:
    type: object
    required: [scanned, findings]
    properties:
      scanned:  { type: integer }
      failed:   { type: integer }
      findings: { type: array, items: { $ref: "#/$defs/finding" } }
    $defs:
      finding:
        type: object
        required: [asset_ref, host, port, not_after, days_left, severity]
        properties:
          asset_ref: { type: string }
          host: { type: string }
          port: { type: integer }
          not_after: { type: string, format: date-time }
          days_left: { type: integer }
          severity: { type: string, enum: [low, medium, high, critical] }
  emits:
    - from: $.findings[*]                 # 输出中的每个 finding
      type: security.alert                # 投递为告警（走 /internal/v1/events，source=script:tls.cert.expiry-scan）
      map:                                # finding → 事件字段
        external_id: "{{ asset_ref }}:{{ port }}"
        severity: "{{ severity }}"
        category: policy
        title: "证书 {{ days_left }} 天后到期：{{ host }}:{{ port }}"
        entities: [{ type: host, value: "{{ host }}" }]
        asset_ref: "{{ asset_ref }}"
  platformAccess: [assets.list]
  artifacts: ["report.csv"]
  network: { egress: internal }           # 要连内网主机 443
```

**main.py（节选）**
```python
import csv, socket, ssl
from datetime import datetime, timezone
from zh import script as zh
from zh import platform

SEV = lambda d: "critical" if d <= 3 else "high" if d <= 7 else "medium" if d <= 14 else "low"

def main():
    args = zh.input()
    assets = platform.assets.list(kind=args["asset_kind"], org_id=args.get("org"))
    zh.log.info("scan start", total=len(assets), threshold=args["days_threshold"])
    findings, failed = [], 0
    with open(zh.artifact("report.csv"), "w", newline="") as f:
        w = csv.writer(f); w.writerow(["asset_ref", "host", "port", "not_after", "days_left"])
        for i, a in enumerate(assets):
            if zh.ctx.cancelled: zh.log.warn("cancelled", done=i); break
            host, port = a["identifiers"].get("fqdn") or a["identifiers"]["ip"], a.get("attrs", {}).get("tls_port", 443)
            try:
                not_after = fetch_not_after(host, port)          # ssl 握手取证书 notAfter
            except Exception as e:
                failed += 1; zh.log.warn("probe failed", host=host, port=port, err=type(e).__name__); continue
            days = (not_after - datetime.now(timezone.utc)).days
            w.writerow([a["asset_ref"], host, port, not_after.isoformat(), days])
            if days <= args["days_threshold"]:
                findings.append({"asset_ref": a["asset_ref"], "host": host, "port": port,
                                 "not_after": not_after.isoformat(), "days_left": days, "severity": SEV(days)})
            if i % 50 == 0: zh.log.info("progress", done=i)    # 也是心跳
    zh.log.info("scan done", scanned=len(assets), failed=failed, findings=len(findings))
    zh.output({"scanned": len(assets), "failed": failed, "findings": findings})

if __name__ == "__main__":
    zh.run(main)
```

**平台侧后续**（脚本不感知）：runner 校验 outputs → 按 `emits` 把每个 finding 投递为 `security.alert`（指纹=source+external_id，重复到期不重复建单，只更新 last_seen/days_left）→ 路由规则：`severity in [high, critical]` 聚合进风险单并通知资产 owner；`low/medium` 仅记录 → `report.csv` 作为工件挂到本次运行，定时任务页可下载。

## 9. 执行器（sandbox-runner）职责清单

1. 取包：按 `action_id@version` 从 S5 拉 zip，校验 sha256 与审批状态；
2. 备环境：命中预构建 venv 缓存（按包版本+runtime 哈希），未命中拒绝运行并告警（审批时应已构建）；
3. 组装：解析 manifest → 校验 input 对 schema → 填默认值 → 解析 secrets（从 dispatch 请求中的短期加密载荷）→ 签发 `ZH_RUN_TOKEN`（scope=platformAccess，TTL=timeout+宽限）；
4. 起进程：非 root、只读 rootfs、`/tmp` 与工件目录可写、rlimit（内存/进程数/文件数）、seccomp 默认档、网络按 manifest（none / internal / allowlist 经出站代理）、cgroup 配额按 resourceClass；
5. 接通道：stdin 写入后关闭；stdout 全量缓冲（上限）；stderr 行级流式捕获+级别解析+心跳刷新+脱敏；
6. 守时：总超时与 idle 超时 → SIGTERM → 10s → SIGKILL；取消信号同路径；
7. 收尾：解析 stdout JSON → 校验 outputs → 扫工件白名单上传 → 组装运行记录 → 回报 taskrunner（AK/SK）→ 清理工作目录与 token；
8. 指标：每次运行上报时长/退出码/资源峰值/出网目标（可观测 M4 补齐的一部分）；Prometheus 暴露格式，运行记录带 `trace_id`（OpenTelemetry 语义）贯穿 taskrunner → runner → 回调（09 号补强）。

## 10. 安全基线（清单）

- [ ] 上传者权限：第一档仅管理员（D2），每次上传/升版走审批工单（复用自家流程，dogfooding）；
- [ ] 包完整性：sha256 + 审批时锁定 `requirements.lock`，运行时零安装；依赖仅内部镜像源；
- [ ] 最小权限：`platformAccess` / `secrets` / `network` / `artifacts` 四个白名单默认空；审批页逐项展示供审批人确认；
- [ ] 身份：脚本以专用 service account 运行（A6），`on_behalf_of` = 触发人；Responder 的出站动作审计归因到人；
- [ ] 隔离：非 root、只读 rootfs、无容器运行时 socket、资源配额、默认禁网；
- [ ] 秘密：默认 env、第二档起可选文件挂载（`secretsMode: file`，09 号补强）；日志/输出/工件兜底脱敏；运行后 token 作废；
- [ ] 上限：stdout 1 MB、日志 1–8 MB、工件按 class、单次运行时限按 class；
- [ ] 审计事件：`script.uploaded / submitted / approved / rejected / retired / run.started / run.finished`；
- [ ] 版本：同版本不可覆盖；retire 不影响在途运行；工作流引用钉 `major.x`；
- [ ] 供应链：基础镜像定期重建并扫描；SDK 版本随镜像，脚本不得自带 zh 包覆盖。

## 11. 与平台的接线

| 平台件 | 接线 |
|---|---|
| S5 对象存储 | 脚本包、超限日志、工件三类对象；预签名上传/下载通道。**非纯接线（2026-10-20 三审代码核实 E-F1）**：现 presign purpose 为封闭白名单五项（avatar/attachment_create/richtext_create/ticket_attachment/ticket_richtext）、代理下载仅放行 avatar/ticket 态、审计白名单仅两 purpose——三类新对象须扩 purpose 枚举 + 下载授权 + 审计白名单 + GC 绑定器（phase8 首批，+0.5 天） |
| taskrunner | `kind=script` 的 action 由 taskrunner 受理（幂等/重试/死信/cron/手动触发全部复用），dispatch 到 sandbox-runner |
| actions 注册表（A1） | 审批通过 → 自动从 manifest 生成 actions 行；inputs schema 驱动 S4 任务面的"参数表单"与 AI 工具定义 |
| 工作流（A5） | `script` 节点 = `action` 节点的特例；Responder 节点前置审批节点校验 |
| 工单 | Analyzer/Responder 出现在工单页动作库（S3a "提交任务"理想态的具体化）；结果结构化回贴 |
| 告警流水线（A8） | `emits` 把 Job 输出变成告警源；Analyzer 可作为告警流水线中的富化步骤 |
| S1 表单渲染器 | JSON Schema 子集 → 表单：**顶层标量字段**与 S1 自研七类型渲染器共用（扩展 format → 控件映射）；嵌套 object / array / `$defs`（如样例 B findings）超出七类型渲染面，用独立 JSON-Schema 只读视图承载（三审 E-F3 收窄原"共用"声称） |
| S6 通知 | 运行失败/dead/契约错误通知 owner；巡检 findings 经告警路由通知资产 owner |
| AI（phase11，范围后置讨论） | actions 即工具；Responder 默认需人工确认（D8） |

## 12. 演进路线

| 档 | 内容 | 依赖 |
|---|---|---|
| 一档 | Python 3.11 单运行时；Job/Analyzer/Responder 三 kind；管理员上传+审批；容器池子进程隔离；文本日志；S4 任务面展示 | A1、A2、S5 |
| 二档 | jsonl 日志与结构化检索；工件→工单/风险单自动挂接；Analyzer 结果 taxonomies 标准化（对齐 Cortex 语义）；每运行一容器隔离 | A8、运行时 socket 主机 |
| 三档 | 多运行时（node/二进制）同协议；脚本间组合（工作流内链式传参）；脚本市场（部门内共享与评审） | A5 |

## 13. 本文拍板点

> **状态（2026-10-20 晚）**：所有者"其余都按照建议"——S1–S8 **全部已拍、按建议**。下表保留选项与理由备查。

| # | 决策 | 建议 |
|---|---|---|
| S1 | stdout 协议：整段单 JSON 文档 vs "最后一行 JSON" | 整段单文档（SDK 重定向 print 兜底；"最后一行"在多线程/异常栈下极易破）；2026-10-20 晚补强：加可选 `spec.output: file` 结果文件通道并存 |
| S2 | SDK 是否强制 | 不强制（裸协议合法），但基础镜像内置 |
| S3 | 日志格式默认 | text + 级别前缀解析；jsonl 可选 |
| S4 | 网络默认 | `none`；巡检类显式 `internal` |
| S5 | exit code 表 | 按 §4.3；2 为唯一可重试码 |
| S6 | kind 三分（Job/Analyzer/Responder）是否第一档就分 | 是——它决定 UI 入口、审批前置与 AI 默认确认策略，后补代价大 |
| S7 | `emits` 声明式投递 vs 脚本自己调 `events.emit` | 两者都支持；声明式为推荐（脚本保持纯函数、平台统一指纹） |
| S8 | 脚本运行身份 | 专用 service account + on_behalf_of（A6/D8/D9） |
