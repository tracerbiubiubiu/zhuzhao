# 字典删除合同

字典删除沿用现有 POST 路由及 `dict:manage` 授权。请求均为 JSON body；响应封装见[响应合同](response.md)，错误码见[登记表](errcode.md)。实现入口为 `internal/handler/dict_handler.go` 的 `DeleteType` / `DeleteItem`、`internal/service/dict_service.go` 的同名方法，以及 `internal/repository/dict_repo.go` 的 `DeleteTypeByCode` / `DeleteItemByID`。

## 类型：POST /api/v1/dicts/delete

旧请求 `{"code":"severity"}` 继续按 code 删除。新客户端可发送 `{"code":"severity","id":"123","version":7}`，将删除绑定到用户确认的稳定身份和版本。

- `code` 必填，最多 64 字符。
- `id` 与 `version` 可选，但必须同时提供；只提供一个或显式传 null 均返回 400。
- `id` 是正 int64 的 JSON 字符串，字符串长度最多 19；`version` 是正 int64 的 JSON number。上限均为 9223372036854775807；number ID、字符串 version、小数、零、负数、溢出均返回 400。
- CAS 路径的最终 DELETE 同时匹配 `code + id + version`。同 code 已换 ID 或当前版本不匹配时不删除，返回既有 HTTP 409 / code 10005（`ErrConflict`）。
- 类型不存在时保留 HTTP 404 / code 92301。字段引用优先校验。引用 `dict:<code>` 的字段仍返回 HTTP 400 / code 92304，且没有字典或条目删除。

## 条目：POST /api/v1/dict-items/delete

旧请求 `{"id":"456"}` 继续按 ID 删除。新客户端可发送 `{"id":"456","version":3}`。

- `id` 必填，其 JSON 类型、长度和 int64 范围与类型删除一致。
- `version` 可选，提供时必须为正 int64 JSON number；显式 null、字符串、小数、零、负数、溢出均返回 400。
- 提供 version 时，最终 DELETE 同时匹配 `id + version`。当前版本不匹配时不删除，返回 HTTP 409 / code 10005；条目不存在时保留 HTTP 404 / code 92302。
- 条目删除沿用原语义，不增加类型级字段引用预检。

## 新建条目和更新备注

客户端新建条目时可提交 `type_id`（正 int64 的 JSON 字符串）及 `type_code`。服务端在插入前锁定并核对当前父类型。同码重建造成父身份不符时返回 HTTP 409。旧调用只提供 `type_code` 时继续使用当前父类型。

类型及条目更新均可省略 `remark` 或传 `null` 以保留原备注。传入字符串会写入该字符串，空字符串用于明确清空。仍须通过原有对象版本检查。

## 并发和兼容边界

删除在事务内以 `FOR UPDATE` 锁定字典父类型，再用下一条 SQL 重查存量字段的精确引用，最后核对稳定身份与版本。工单字段集替换按代码排序，以 `FOR KEY SHARE` 锁定其规范的 `dict:<code>` 父类型直至提交。存量非规范字段含同一精确引用时继续阻止删除。

例如，删除类型与把 `dict:<code>` 加入工单字段集同时发生时，结果取决于哪个事务先提交：

- 字段集替换先提交：删除重查时看到引用，返回 92304。
- 删除先提交：字段集替换发现父类型已不存在，整个替换失败。

现有 FK 保持级联删除条目。类型 version 代表自身版本。子项集合独立世代以及旧客户端切换按[续修交接](../review/reports/audit-remainder-2026-11-20.md#保留依赖与验收条件)。
