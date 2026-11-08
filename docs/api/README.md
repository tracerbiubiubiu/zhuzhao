# API 契约入口

前后端联调先读[响应封装](response.md)，再查[错误码登记](errcode.md)。错误码登记已统一至本目录，没有根 `api/` 的第二份权威。

- 业务响应为 `{code,message,data,request_id}`，成功code为0；业务分支结合HTTP状态和code判断。
- 常规出口由utils response与主仓错误映射维护。鉴权/限流、流式下载、健康检查等特殊出口按实际契约核对，不能套一条响应示例覆盖全部HTTP。
- `make swag` 根据Handler注解生成 `docs/docs.go`、`docs/swagger.json`、`docs/swagger.yaml`；运行时入口为 `/swagger/*`。生成物不手改，路径供Go导入和UI codegen使用。
- 生产路由注册以 `internal/router/router.go` 的 `New` 为准；架构页路由表用于导览，新增/退役接口同时更新注解、契约和对应模块。

本目录是查阅契约，开发操作见[开发指南](../guides/development.md)，权限设计见[权限地图](../modules/authz.md)。尚未提供的gRPC/WebSocket不因旧草图存在而视为支持。
