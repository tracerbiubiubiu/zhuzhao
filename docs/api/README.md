# API 文档

后端接口契约，供实现与前后端联调。**后端以本文档目录为 SSOT，前端按此对齐。**

## 文档

| 文档 | 说明 | 优先级 |
|------|------|--------|
| **[response.md](./response.md)** | **HTTP 响应体 Envelope**（`code` / `message` / `data` / `request_id`） | 必读 |
| [errcode.md](./errcode.md) | 业务错误码分段、HTTP 映射、Phase 1 验收对照 | 必读 |
| [../design/architecture.md](../design/architecture.md) §17 | API 路由总表 | 联调时 |

## 后端约定摘要

- 统一 JSON：`{ "code": 0, "message": "success", "data": {}, "request_id": "..." }`
- 成功：`code === 0`；失败：`data === null`，`code !== 0`
- 字段名用 **`message`**，不是 `msg`
- 业务分支看 **body.code**，HTTP 状态仅辅助
- 实现出口：`internal/pkg/response`（禁止 Handler 裸 `c.JSON` 业务结构）

## 文档来源

- **RESTful API**：Phase 1 后期可由 `swag` 从代码注解生成 OpenAPI（字段须与 response.md 一致）
  - 生成命令：`make swag`（见根目录 Makefile）
  - 运行时：`/swagger/*`
- **非 RESTful 接口**（WebSocket、gRPC 等）：手写文档放在此目录
