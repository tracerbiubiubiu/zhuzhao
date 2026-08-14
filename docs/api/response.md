# HTTP 响应体约定（后端 SSOT）

> **后端权威契约**。所有 JSON API（Phase 1 管理端）必须经 `internal/pkg/response` 输出；前端按本文对齐，字段名以本文为准（使用 `message`，不是 `msg`）。
>
> 实现：`internal/pkg/response/response.go`  
> 错误码分段与 HTTP 映射：[`errcode.md`](./errcode.md)  
> 路由清单：[`architecture.md`](../design/architecture.md) §17

---

## 1. 统一 Envelope

除下文「例外」外，**每个 JSON 响应**均为同一外层结构：

```json
{
  "code": 0,
  "message": "success",
  "data": {},
  "request_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `code` | int | 是 | 业务码。**`0` = 成功**；非 0 = 失败（见 errcode.md） |
| `message` | string | 是 | 面向用户的简短中文说明；成功默认 `"success"` |
| `data` | any | 是 | 成功时为业务载荷；**失败时固定 `null`** |
| `request_id` | string | 否 | 请求追踪 ID（中间件注入；无则省略 JSON 字段） |

### 1.1 命名约定（后端已定，不改）

| 采用 | 不采用 | 原因 |
|------|--------|------|
| `message` | `msg` | 与 Go struct、OpenAPI 描述一致，可读性更好 |
| `code`（int） | HTTP 200 当成功 | 业务码与传输层分离，便于同一 HTTP 状态表达多种业务语义 |
| `request_id` | — | 串联 access log、审计、slog，排障必需 |

---

## 2. 双层语义：HTTP Status + body.code

客户端须 **同时读 HTTP 状态码与 body.code**；**业务分支以 body.code 为准**。

| 层 | 职责 | 示例 |
|----|------|------|
| **HTTP Status** | 传输/网关/缓存语义 | 401 未认证、403 禁止、429 限流、503 依赖不可用 |
| **body.code** | 具体业务原因 | 20001 密码错、20003 token 失效、70003 未分配角色 |

典型：登录失败与 token 无效可能都是 **HTTP 401**，但 `code` 分别为 **20001**、**20003**。

---

## 3. 成功响应

### 3.1 普通成功

HTTP **200**，`code: 0`，`message: "success"`，`data` 为对象或数组。

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "1",
    "username": "admin"
  },
  "request_id": "..."
}
```

- 无业务数据时：`data` 可为 `{}` 或 `null`（推荐 `{}`，表示「成功但无附加载荷」）。
- 需要自定义成功文案时：使用 `OKWithMessage`，仍保持 `code: 0`。

### 3.2 分页列表

HTTP **200**，`data` **固定**为下列形状（字段名 snake_case）：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "list": [],
    "total": 100,
    "page": 1,
    "page_size": 20
  },
  "request_id": "..."
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `list` | array | 当前页数据 |
| `total` | int64 | 总条数 |
| `page` | int | 当前页码，从 1 开始 |
| `page_size` | int | 每页条数 |

实现：`response.OKPage()`。

### 3.3 主键与 BIGINT

`data` 内凡 `BIGINT` 主键/外键，JSON **序列化为 string**（Go tag `json:"...,string"`），避免前端精度丢失。

---

## 4. 失败响应

HTTP 状态按场景选取（见 errcode.md §3），body 形如：

```json
{
  "code": 20001,
  "message": "工号或密码错误",
  "data": null,
  "request_id": "..."
}
```

规则：

1. **`data` 必须为 `null`**——不在失败响应里塞 `{ "errors": [...] }`（Phase 1 保持简单；字段级校验若需要，Phase 2 再扩展并修订本文）。
2. **`message` 仅用户可见文案**——禁止返回堆栈、SQL、Redis key、内部路径。
3. Handler / 中间件 **禁止** 直接 `c.JSON` 裸业务结构；统一走 `response.Fail` / `response.Error` / `response.ForbiddenError` 等。
4. 未识别错误：HTTP **500**，`code: 10000`，`message: "服务器内部错误"`。
5. **AuthN 拒绝**（混用凭证、token 无效、AK 验签失败等）：`data` 仍为 `null`；禁止返回「哪条凭证错了」；混用 Bearer + AK/SK 用 **HTTP 400 + code 20008**。处理细则见 [phase1/02-auth.md §非法认证请求的处理](../phase1/02-auth.md#非法认证请求的处理实现必读)。

---

## 5. 后端实现约束

```
请求 → Middleware（JWT / Casbin / Recovery）
         ↓ 失败：response.* 直接写 Envelope + Abort
       Handler
         ↓ 调用 Service
       Service → 返回 error（*errcode.Error 或 wrap）
         ↓
       Handler → errors.As → response.Error(c, httpStatus, err)
```

| 包 | 职责 |
|----|------|
| `errcode` | 定义 `code` + 默认 `message` 常量 |
| `response` | 唯一 JSON Envelope 出口 |
| `middleware/recovery` | panic → 500 + 10000 + Envelope |

新增 API 时：

1. 成功 → `response.OK` / `OKPage` / `OKWithMessage`
2. 失败 → `response.Error` 或语义化 helper（`BadRequest`、`ForbiddenError`…）
3. 新业务码 → 先写 `errcode.md`，再写 `errcode.go`

---

## 6. 例外（不使用 Envelope）

| 场景 | 说明 | 阶段 |
|------|------|------|
| 健康检查 | `GET /health/live`、`GET /health/ready` 返回 `{"status":"ok"}` 或 `{"status":"unhealthy","component":"db"}` | Phase 1 |
| 文件下载 / 预签名跳转 | 二进制流或 302，非 JSON API | Phase 2+ |
| Swagger UI | 静态 / OpenAPI JSON | Phase 1 后期 |

除上述外，**不得**新增「半套 JSON」接口。

---

## 7. 与常见 `{ code, msg, data }` 的对应

| 常见写法 | 本项目 |
|----------|--------|
| `msg` | **`message`** |
| `code: 200` 表成功 | **`code: 0`** 表成功 |
| 无 `request_id` | **有**（推荐客户端在报错 UI 展示，便于提工单） |
| 只看 HTTP | **必须看 body.code** |

前端对齐检查清单：

- [ ] 拦截器读 `response.data.code`（axios 下为 `res.data.code`）
- [ ] `code === 0` 判成功
- [ ] 401 且 `code === 20007` 跳转改密页
- [ ] 403 且 `code === 70003` 提示联系管理员分配角色
- [ ] 503 且 `code === 10008` 提示服务暂时不可用

---

## 8. 相关文档

| 文档 | 内容 |
|------|------|
| [errcode.md](./errcode.md) | 错误码清单、HTTP 映射、Phase 1 验收对照 |
| [architecture.md §16](../design/architecture.md#16-统一响应与错误处理) | 架构层概述（细节以本文为准） |
| [phase1/README.md §1.3](../phase1/README.md) | 端到端验收路径 |

---

## 变更记录

| 日期 | 说明 |
|------|------|
| 2026-08-14 | 初版：后端 SSOT，固定 `message` / `code=0` / 失败 `data=null` |
