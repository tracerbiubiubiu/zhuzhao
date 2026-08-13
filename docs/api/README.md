# API 文档

接口规格说明，供前后端联调使用。

## 文档

| 文档 | 说明 |
|------|------|
| [errcode.md](./errcode.md) | 业务错误码分段、HTTP 映射、Phase 1 验收对照 |
| [../design/architecture.md](../design/architecture.md) §17 | API 路由总表 |

## 文档来源

- **RESTful API**：由 `swag` 从代码注解自动生成 OpenAPI/Swagger 文档
  - 生成命令：`make swag`（见根目录 Makefile）
  - 生成路径：`/swagger/*`（运行时访问）或指定输出到此目录
- **非 RESTful 接口**（如 WebSocket、gRPC）：手写文档放在此目录

## 接口总表

完整的 API 路由清单见 [../design/architecture.md](../design/architecture.md) 第 17 章。
