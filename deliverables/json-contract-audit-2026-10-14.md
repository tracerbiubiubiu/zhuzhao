# JSON 契约一致性审计（zhuzhao / activelist / taskrunner / zhuzhao-utils）

> 日期：2026-10-14 ｜ 方法：三仓并行子代理全量扫描（跨 HTTP 边界 struct 逐字段核对 encoding/json 语义）+ utils 人工复核 ｜ 起点：zhuzhao `assignee_no *string` vs `created_by_no string` 形态不对称一例 ｜ 状态：**B1/B2/B3 已修复（各仓同日提交，commit 号见 §5），B4 公约已立（standards §3-14），遗留项见 §4**

## 1. 总体结论

公约大面守住（int64 ID 标量 `,string` 覆盖、RFC3339/date-only 校验链、列表空数组守卫、信封结构均确认干净）。问题集中于一个模式：**「空态」编码不统一**（null / 缺字段 / `""` 三种表达混用），外加 2 处真实数据缺陷、1 个发送侧系统性缺口。

## 2. 发现与处置（已修复 ✅）

### 2.1 发送侧系统性缺口（utils，一处修全生态收益）

| 位置 | 问题 | 处置 |
|---|---|---|
| zhuzhao-utils `jsonutil.Int64Slice` | 仅 Unmarshal（收方宽容），出站仍裸数字数组 | ✅ 补 MarshalJSON（`dd2d247`，元素一律字符串；nil/空均出 `[]` 数组恒不 null）；zhuzhao `Organization.OwnerUserIDs` 出站随 v0.4.2 pin 升级生效（打标最后做） |

### 2.2 真实数据缺陷（B2）

| 仓 | 位置 | 问题 | 处置 |
|---|---|---|---|
| taskrunner | `server.go:276` → `task_service.go:539` | PATCH `/v1/jobs` 显式 `"params": null` 击穿「nil=不改」——RawMessage 对 null 非 nil，字面量落库并随回调扩散 | ✅ 显式 null 400 拒绝（`ca9ac65`） |
| zhuzhao | `service.go:583` | CreateRelation 响应回显未规范化 `relation_type:""`，GET 同关联出 `"related"`——同资源读值漂移 | ✅ 回写规范化 relType（`7fd2fd1`） |
| activelist | `repository/transfer.go` | 导入路径 nil Data → jsonb null 落库，读/导出 `{}`/`null` 二义 | ✅ 归一空对象（`e807d5a`） |

### 2.3 机械性公约违约（B1）

| 仓 | 位置 | 处置 |
|---|---|---|
| zhuzhao | `user_handler.go:265` role_ids、`role_handler.go:177` menu_ids 出站 []int64 裸数字 | ✅ `int64IDsToStrings`（`ce74ecf`） |
| activelist | `handler/data.go:88` after_id 游标裸 int64（分页必经，超 2^53 错页）；`data_service.go:307` MaxID | ✅ 字符串化 / `,string`（`e807d5a`） |
| taskrunner | `task_service.go:183` RunView.ID 缺 `,string`（仓内唯一违约） | ✅（`ca9ac65`） |

### 2.4 空态形态统一（B3）

| 仓 | 项 | 处置 |
|---|---|---|
| zhuzhao | Ticket 回填四字段两形态（exemplar） | ✅ 四字段全值类型+omitempty（`439da1a`，指派存在性由 AssignedTo/assignee_name 承载，信息无损；W4 断言同步） |
| zhuzhao | Role.ParentID 缺字段 vs Org/Menu null | ✅ 去 omitempty 对齐 null 约定（`439da1a`） |
| zhuzhao + taskrunner | 死信载荷 RequestID/JobID/Attempts omitempty（0 值/空 id 消失，两侧成对） | ✅ 双侧恒出（`d9c16d0` / `ca9ac65`） |
| taskrunner | NextRun 消失 vs started_at null 同族两编码 | ✅ 去 omitempty（`ca9ac65`） |
| taskrunner | RunView 缺 source_ip（详情/列表字段集漂移，store 早已 SELECT） | ✅ 补齐（`ca9ac65`） |
| taskrunner | callback paramJSON null 分支被 omitempty 短路成死代码 | ✅ 删死分支留透传（透传 MarshalJSON 不可删——删则退化 base64，注释在案） |
| activelist | Field.Sensitive omitempty（false 消失）vs Required 恒出 | ✅ 去 omitempty（`e807d5a`） |
| activelist | scanDef 嵌套 fields nil→null | ✅ 空切片兜底（`e807d5a`） |

## 3. 业界实践对照

判断标准取 AWS API Guidelines / Google AIP / JSON:API / RFC 7396 共识：同概念唯一 wire 形态；「无值」按语义分组单表达且成文；数组恒 `[]` 不 null；bool 恒出（false ≠ undefined）；PATCH 显式设计 null 语义（本批 taskrunner 即教科书案例）；写响应回显规范化持久值。已全部立约 standards **§3-14**。

## 4. 遗留项（登记不修，触发驱动）

| 项 | 说明 | 触发条件 |
|---|---|---|
| **params 双形态**（taskrunner `TaskView/RunView.Params` 转义字符串 vs `JobView.Params` 原生对象） | 任一方向统一都破坏一个前端消费方，须与 zhuzhao-ui 联动改（推荐终态=RawMessage 原生对象，消灭双次解析反模式） | ui 排期批 |
| JoinedAt 三端点三形态（zhuzhao org/user 恒出 vs roster/myorg omitempty） | schema nullable 自辩在档 | 契约文档统一时拍 |
| Children 叶子缺字段（org/menu 树） | 设计自洽（叶子无 children 键），前端逐节点 .map 需知 | 登记进前端契约文档 |
| POST /tickets 响应无姓名回填（GET 有） | 读后刷新即一致 | 有消费方反馈再补 enrich |
| DeviceInfo 时间 string 载体（Phase 2 预留未启用） | 启用时定 RFC3339 | auth-enhance |
| task.Payload.TimeoutSecs 0=默认、TaskEvent.FromValue/ToValue created 事件双消 | 语义自洽，注释在案 | 无 |
| notify.Payload.FailedAt string 承载时间 | worker 内部 RFC3339 手工约定，误传无校验 | 契约升级时改 time.Time |

## 5. 提交索引（2026-10-14，均未打标）

- zhuzhao-utils：`dd2d247`（Int64Slice MarshalJSON，v0.4.2 收编）
- zhuzhao：`ce74ecf`(B1) `7fd2fd1`(B2) `439da1a`(B3) `d9c16d0`(B3) `594e585`(B4 立约) + 本报告
- activelist：`e807d5a`（四处）
- taskrunner：`ca9ac65`（六处）

各仓门禁：utils `make lint` exit 0；zhuzhao 全量单测 14 包 + TestW4_ 集成绿；activelist `make lint` exit 0；taskrunner `go test ./...` 全绿 + `make lint` exit 0。
