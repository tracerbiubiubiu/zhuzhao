# 致谢与借鉴出处（Acknowledgments）

> 本项目站在许多开源项目的肩膀上。此页集中登记**设计借鉴 / 参考对照过**的项目：出处、许可、借鉴点与代码内落点，一事一行，便于溯源与致谢。
>
> **登记纪律**：
> 1. **借鉴 = 借设计不借代码**。GPL 项目（如 Dagu）仅作设计研读，永不进入依赖树；**无 LICENSE 文件的项目默认保留所有权利——允许设计阅读，禁止拷贝代码**（eflow/eiam 属此类）。
> 2. **运行时依赖不逐一登记**（各仓 `go.mod` + 模块自带 LICENSE 已覆盖许可义务）；本页价值 = 溯源与工程礼仪，非合规工程。若未来对外分发，再评估用 go-licenses 类工具生成第三方声明。
> 3. **公司内部代码的参考不入本页**（脱敏纪律）——其出处写在落点包注释/文档的中性措辞里（先例：`aksk/compat` 包注释「出处说明」句）。
> 4. **新借鉴随批登记**：本页加一行 + 落点就地一句话指针，细节不重复。
>
> 许可核验日期 **2026-10-13**（各项目本地克隆的 LICENSE 文件 / 评估文档在档结论）；标注「随选型批核验」的条目在启用时补核。

## 1. 已实际借鉴（设计蓝本 / 接口形态 / 反面对照驱动修正）

| 项目 | License | 关系 | 借鉴点 | 落点 |
|------|---------|------|--------|------|
| [qingtao/aksk](https://github.com/qingtao/aksk) | MIT | 存量兼容协议蓝本 + 接口借鉴 | `KeyGetter`/`NonceStore` 接口形态、skew/TTL 对齐思路（借鉴不依赖，2026-08-14 评估在档）；存量 `x-auth-*` 线协议来源——本地存量框架实现系其衍生（回加 body-hash 复算，与 v2.1.0 细节不同源），线协议以 compat 冻结向量为权威 | utils `aksk/compat`（独立重实现）、[phase1/02-auth §评估](./phase1/02-auth.md) |
| [Bunny3th/easy-workflow](https://github.com/Bunny3th/easy-workflow) | MIT | 仅借鉴设计（不引库） | 工作流能力映射蓝本：batch_code/自由驳回 BFS/WhatCanIDo/事件→显式 TicketHooks/Go 侧表达式求值/双表归档等十二项 + 四条避坑（MySQL CTE/DB 表达式求值/反射事件/引擎内嵌身份） | [phase3/10 §4.2](./phase3/10-ticket-business.md) |
| [crazy-airhead/aifei-go](https://github.com/crazy-airhead/aifei-go) | Apache-2.0 | 仅借鉴设计（不引库） | §4.2 能力映射共同蓝本 | [phase3/10 §4.2](./phase3/10-ticket-business.md) |
| [Kun-GitHub/RuoYi-Go](https://github.com/Kun-GitHub/RuoYi-Go) | MIT | 反面对照 + 选择性借鉴 | 路由↔menu_apis 对账形态（BK-22 蓝本）、data_scope 锚点授予形态对照（勿抄角色派）、在线用户管理面登记（触发驱动） | review/11 §6/§8、[phase3/07](./phase3/07-security-enhance.md) |
| [lostvip-com/ruoyi-go](https://github.com/lostvip-com/ruoyi-go) | MIT | 反面对照 | L2「纯 UI 摆设」形态反证自研路线（行级判定必须落在数据访问处） | [phase2/11-authz 评审](./phase2/11-authz-architecture-review.md) |
| [tx7do/go-wind-admin](https://github.com/tx7do/go-wind-admin) | MIT | 反面对照 + 选择性借鉴 | L2 行级护栏论证（IW4 依据）、评估日志/审计归档佐证、Provider 降级语义辨析；L2 缺锚点退化对照 | IW4（review/11 §8）、[phase3/07](./phase3/07-security-enhance.md) |
| [qxkjsoft/ginfast](https://github.com/qxkjsoft/ginfast) | MIT | 反面对照 | L2 无锚点退化语义纠错（属主∪委托而非全拒）、GetDataScope 漏挂旁路佐证、阻塞策略配置维度借鉴 | [phase2/11-authz 评审](./phase2/11-authz-architecture-review.md)、review/11 |
| [Duke1616/ecmdb](https://github.com/Duke1616/ecmdb) | MIT | 反面发现 | 资源定义表乐观锁缺失反向发现 → BK-18 已实施 | review/11 §6 BK-18 |
| [axiaoxin-com/ratelimiter](https://github.com/axiaoxin-com/ratelimiter) | MIT | 库未采用，技术点吸收 | Lua `TIME`+`replicate_commands` 取服务端时间（多实例时钟免疫）——限流 Lua 脚本的时钟处理来源 | `internal/middleware/ratelimit.go` |
| Duke1616/eflow | ⚠ 无 LICENSE | 只读设计参考 | 工单终态联动/发起人撤回（WITHDRAWING 栅栏模式）对标——保留所有权利，**零代码拷贝** | [phase2/09](./phase2/09-ticket.md)、[phase3/10](./phase3/10-ticket-business.md)（对接参考） |
| Duke1616/eiam | ⚠ 无 LICENSE | 只读设计参考 | Casbin watcher 设计对照——同上纪律 | authz watcher 设计（design-decisions §25） |

## 2. 仅评估未采用 / 选型预置（反面教材与候选同样算贡献）

| 项目 | License | 关系 | 结论/用途 | 落点 |
|------|---------|------|-----------|------|
| [dagucloud/dagu](https://github.com/dagucloud/dagu) | GPL-3.0 | 🚦 仅评估 | 重编排外部引擎候选——**GPL 传染红线：若启用仅限外部进程集成，永不进依赖树**（历经 yohamta → dagu-org → dagucloud 迁移，此为 2026-10 现行规范路径） | [phase3/15](./phase3/15-script-platform-dagu-vs-inhouse.md) |
| gin 限流中间件生态（gin-contrib 所链 axiaoxin/gin-limiter/gin-redis-ip-limiter） | 各 MIT | 反面实证 | 官方 contrib 不含限流、所链三家全部弃养（含 panic 级缺陷实锤）——实锤「自研 + redis_rate 胶水」路线 | `internal/middleware/ratelimit.go`、2026-09-11 核验 |
| [flipped-aurora/gin-vue-admin](https://github.com/flipped-aurora/gin-vue-admin) | Apache-2.0 | 选择性借鉴（随批吸收） | 附件上传（local+MinIO）/通知/固定列导出/字典/MCP 透传鉴权模式；**勿抄清单在册**：代码生成器/热更新/引导初始化/ACL 路线 | P4 交付、phase5 S5 |
| [didi/LogicFlow](https://github.com/didi/LogicFlow)、[bcakmakoglu/vue-flow](https://github.com/bcakmakoglu/vue-flow) | 随选型批核验 | S2 画布 spike 候选 | transitions 同构零映射；X6/bpmn.js 已排除 | phase5 S2 |
| [ueberdosis/tiptap](https://github.com/ueberdosis/tiptap)（vs wangEditor） | 随选型批核验 | S1 富文本候选 | 低维护富文本编辑器对照 | phase5 S1 |
| [expr-lang/expr](https://github.com/expr-lang/expr) | 随选型批核验 | S1/S3 求值器预置 | 替代停更的 govaluate | phase5 S1/S3 |

---

## 变更记录

| 日期 | 说明 |
|------|------|
| 2026-10-13 | 建页：集中登记借鉴/参考项目（§1 已实际借鉴 11 项 / §2 仅评估未采用 6 项）；许可自本地克隆核验 |
| 2026-10-14 | 外链规范路径修正：dagu 现行仓为 dagucloud/dagu（历经 yohamta→dagu-org→dagucloud 迁移，GPL-3.0 不变）；LogicFlow 显示名笔误 didip→didi（URL 本正确）。驱动=外部审查残留核验 |
