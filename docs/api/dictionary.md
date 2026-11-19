# 字典删除合同

字典删除沿用现有 POST 路由及 `dict:manage` 授权。请求均为 JSON body；响应封装见[响应合同](response.md)，错误码见[登记表](errcode.md)。实现入口为 `internal/handler/dict_handler.go` 的 `DeleteType` / `DeleteItem`、`internal/service/dict_service.go` 的同名方法，以及 `internal/repository/dict_repo.go` 的 `DeleteTypeByCode` / `DeleteItemByID`。

## 类型：POST /api/v1/dicts/delete

旧请求 `{"code":"severity"}` 继续按 code 删除。新客户端可发送 `{"code":"severity","id":"123","version":7}`，将删除绑定到用户确认的稳定身份和版本。

- `code` 必填，最多 64 字符。
- `id` 与 `version` 可选，但必须同时提供；只提供一个或显式传 null 均返回 400。
- `id` 是正 int64 的 JSON 字符串，字符串长度最多 19；`version` 是正 int64 的 JSON number。上限均为 9223372036854775807；number ID、字符串 version、小数、零、负数、溢出均返回 400。
- CAS 路径的最终 DELETE 同时匹配 `code + id + version`。同 code 已换 ID 或当前版本不匹配时不删除，返回既有 HTTP 409 / code 10005（`ErrConflict`）。
- 类型不存在时保留 HTTP 404 / code 92301。字段引用预检仍优先执行：引用 `dict:<code>` 的字段仍返回 HTTP 400 / code 92304，且不进入删除 SQL。

## 条目：POST /api/v1/dict-items/delete

旧请求 `{"id":"456"}` 继续按 ID 删除。新客户端可发送 `{"id":"456","version":3}`。

- `id` 必填，其 JSON 类型、长度和 int64 范围与类型删除一致。
- `version` 可选，提供时必须为正 int64 JSON number；显式 null、字符串、小数、零、负数、溢出均返回 400。
- 提供 version 时，最终 DELETE 同时匹配 `id + version`。当前版本不匹配时不删除，返回 HTTP 409 / code 10005；条目不存在时保留 HTTP 404 / code 92302。
- 条目删除沿用原语义，不增加类型级字段引用预检。

## 并发和兼容边界

CAS 路径在同一 SQL 中锁定当前目标并执行条件 DELETE，返回目标存在与删除结果；冲突不会回退到无条件删除。旧客户端省略 CAS 字段时仍保持旧语义，因此不具备这项 stale 目标保护。更新接口的既有 404 版本冲突形态不变。

类型的字段引用预检仍在删除 SQL 之前单独执行，未与引用写方建立端到端互斥。类型删除继续通过既有 FK 级联删除条目；类型 version 不代表条目列表世代，不能证明确认后条目列表未变化。本合同不宣称这两类竞态已闭合，也不引入 schema/DDL 变更。
