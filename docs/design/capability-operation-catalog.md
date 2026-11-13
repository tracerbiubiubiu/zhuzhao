# 现有能力与逐操作边界清单

> 日期：2026-11-13。用途：补齐全模块/全部产品能力的实际入口清点，回答“已有API、内部动作、受控例外和缺失能力分别是什么”。状态：**当前源码与边界审计快照，目标共同通道未实现**。机器权威为[操作清单JSON](capability-operation-catalog.json)，源码以其中各仓HEAD和文件SHA为准；未来范围/估算仍归总账与[互通详设](capability-interconnect-detail.md)，不另立产品路线。
> 本文完成的是定义清楚分母的操作/模块检查，不是逐行漏洞认证或全部故障运行验收。业务代码、API、配置、迁移均未因此改变；最终集成者另报告本批实际门禁。

## 1. 范围与证据怎样取得

实际构造三仓Gin引擎并调用 `Routes()`，未启动HTTP服务或访问PG/Redis；core用合成密钥显式开启默认关闭的internal/compat入口。Go AST解析四Go仓全部218个非测试生产文件，记录文件/函数体SHA；提取298个导出service声明并逐符号裁定。另扫描UI全部非测试 `src` 中实际request调用，135个字面量/模板HTTP路径（134 request＋1 axios单飞刷新）均与当前core或AL公开绑定匹配。函数Port、静态元数据、纯渲染、探针等没有强行伪造service调用。另查原生fetch预签名PUT和浏览器files资源/下载通道，均单列数据面；external菜单导航是受控跳转，不算平台数据API。

| 实际分母 | 结果 | 不能据此声称 |
|---|---:|---|
| core固定路由，含条件internal 2＋compat 1 | 147 | 三个条件路由默认已开启，或网关未配置也有上游 |
| AL固定路由，含两探针 | 17 | AL已拥有逐用户/行/字段IAM |
| TR固定路由，含两探针 | 12 | 服务验签自动授予人类业务权限 |
| 合计Gin绑定 | 176 | 176个独立业务功能 |
| 去掉8个运维/协议例外后的产品/内部绑定 | 168 | 内部执行协议都应发布MCP |
| 去重能力设计映射（TR代理/执行协议合并） | 158 | 当前已注册158个共同门面 |
| service导出声明，含构造/机制 | 298，未分类0 | 每个导出方法都需要一个HTTP端点 |
| UI实际HTTP路径/模板 | 135，未匹配0 | 页面权限、所有运行分支已通过 |

网关 `gateway.Registry.Mount` 对每个配置前缀产生9个 `Any` 传输绑定，单独登记，不重复算上游业务能力；未配置prefix不构成现有入口。探针、`/metrics`、默认关闭的`/compat/echo`也单独列明，不用“internal”标签隐藏本应复用的产品能力。

清单字段分别表示：当前方法/路径、handler源码与符号、service或函数Port、数据写主、路由授权profile、现有UI消费者、当前条件开关、未来共同入口迁移载体。每个旧API的 `capability_id` 是本次设计映射，版本另列`contract_version="1"`；不存在的风险/HR/UAA/AI/设备/源feed仍明确为目标。全部12家族的“是否枚举”不再统一悬空false；**当前快照枚举完成与runtime实现完成是两件事**。

## 2. 逐模块审计结论

以下为机器表`modules`中每一模块的人工结论；其中操作、声明及例外全部可回到具体记录，不靠包名推断实现。

1. **身份/会话/PAT**：本地IAM现有API完整保留。PG账号与Redis会话跨存储，凭据发行不是统一分布式事务；Refresh/Logout重放和epoch回归保留。Captcha.Generate/Verify是认证步骤，不开放成业务工具；创建PAT/改密不自动授给Agent。C05/I02复用当前主体校验。
2. **用户/档案**：UserService拥有用户、角色、组织同库创建和profile；内部Org/RBAC协作属IAM同域受控事务。停用安全动作不等待交接；C04/D06补旧授权提交/派生角色撤销与HR权源。头像联合引用不应为解耦而拆成无原子性的网络写。
3. **角色与功能授权**：RBACService主写角色/菜单分配，层级与保护角色校验保留。直接、组织、继承三源都需撤权；组织status/deleted_at过滤尚欠，不能说停组织已撤全部派生权。受管投影与本地授予分权，不允许local回授central已撤权限。
4. **组织/成员/scope/委托**：委托selfService入口实际依OrgService L3，不靠页面L1放行。Get/SetUserOrgs、SetUserOrgsTx、Delete是受权IAM命令的步骤/同事务Port，不重新公开绕过actor的薄端点。移动/删除要保持两侧锁序、范围重验和历史归属。
5. **菜单/路由目录**：当前只读词表＋运行时角色菜单分配；启动对账与页面/按钮绑定守卫保留。MenuService.Update仍在源码但无路由，是退役写面；“所有能力有API”不能据此恢复任意菜单CRUD。其变更仍走迁移/发布治理。
6. **字典**：管理读写和全员enabled消费分开。包含disabled的内部查询用于旧值/配置解释，不成为普通用户绕停用查询。已有DB错误映射、引用检查与唯一约束必须保留。
7. **工单实例**：Create/Update/Close/Assign/Transition/Delete、查询/导出均清点。已有L2/L3和过滤不可因内部调用省略；必需状态＋责任由同事务用例协调。C01/C02/C04/R01补公式、引用、授权和不可丢后继；当前后Action提交后执行不等于已实现这些新语义。
8. **类型/字段/模板/自动分派**：管理面实际存在，发布快照/CAS/实例版本是D01/D02目标。当前部分管理service依赖handler路由资格，新内部入口要复用等价资格。ListTicketTypesFor带actor的方法未接当前列表，不能报类型新策略已上线。collect_fields/visible_when不代替字段ACL；真值diff和完整ACL另计范围。
9. **评论/备注/关系/CC/标签/富文本**：评论/备注/关系有现有事务与事件，CC/标签读写资格分开。现API仍单票；批量逐项结果归U03。RenderRichtext只是消毒，文件链接继续受下载权限，不能从渲染成功推断文件可见。
10. **对象/头像/附件**：S3网络在票锁之外；票/对象引用绑定需复用ticket拥有的tx和GC锁序。授权现在有一部分在StorageHandler.TicketAuthorizer，非HTTP调用StorageService不能省它。C02统一更新落绑、仍引用禁止显式删、失败回收；不为统一把任意对象store暴露出去。
11. **通知/模板/rules/偏好/收件箱**：管理面、本人偏好/收件箱和可信事件写分开。Hook仍是进程内低一致派发，R01才承担不可丢命令。渠道投递/已读不是业务完成；UIP桩不代表真实通道可用。收件未读计数由同源badge暴露，CleanupExpired是维护步骤。
12. **任务代理/执行/调度**：core权限与业务意图、TR定义/run/执行、Redis队列有不同写主。TR已有DB-first Submit，CLI/HTTP/cron共用；不是所有基础都缺失。但HTTP failFastAction未自动覆盖内部SpawnTaskAction，后者还丢弃Submit返回值，Submit后的core凭证失败仍是日志对账。C03/CE02统一准入、效果身份、任务关联与结果。
13. **AL类型/schema**：Register/Evolve/Deprecate/Get/List/History/Rules有实际入口。AL拥有metadata、动态表和history事务；core拥有逐用户业务授权，AL验签不复制IAM。R06守旧记录/破坏性演进，CE04记录类型/源世代；不能按接收顺序推导事实新旧。
14. **AL数据/import/export**：Insert/Update/SoftDelete/Restore/Import各在本库事务；Import是全量replace，不是普通patch。当前无operation效果receipt和源事务feed；R06补unique/EOF/世代，CE03/04补受权命令和持久事实。不把直库写包含进未证明的产品feed保证。
15. **情报/预取**：IntelHandler调用internal/pkg/threatintel.Service，后者有缓存写，不能因目录名下沉utils。数据预取registry当前为空，ApplyFetches的Background context及失败语义归D07；已有通用HTTP Provider不证明真实专业源已接通。I04核真实来源/时效/权限。
16. **审计/设置/运维**：Panic查询和Reconcile是函数Port闭包，静态变量集/作业目录不是遗漏的service。审计写不开放任意伪造；设置ConfigStore是单进程快照。维护cron/归档/GC/恢复必须有受控观测和操作通道，普通用户/模型不能自由删审计/claim worker。
17. **装配/网关/资源机制**：Wire/App是合理composition root，当前静态gateway不是动态插件。JWT/L1→可信头覆盖→AK/SK保留；新AL共同入口不能让legacy Any反代变成旁路。Shutdown drain不改变持久保证，启停/卸载/失败隔离仍归X01/X02。
18. **公共库**：utils的58个导出Go函数是机制API，签名不授用户权、PG构造器不拥有业务事务。内部helper、校验器、错误映射不逐个造HTTP/MCP。仅两个实际消费者证明共性后抽机制，业务状态、Policy和私有SDK不塞utils。

## 3. 已有非HTTP能力如何处置

- 四个已装配工单Action：notify/cc/reassign/spawn_task。均需工单、actor及发布版本上下文；对外是能力目录映射后的受权用例，禁止裸Expose Execute。notify与CC/分派共用原领域服务，必需本地责任变更与独立后继分别处理。
- jobs.Registry目前只注册audit_archive。经受权tasks提交和可信internal callback执行；App维护cron若也启同一任务，要避免双调度主。任意归档删除不作为普通业务工具。
- 七维护作业：audit_archive、storage_gc、timeout_scan、inbox_cleanup、intel_cleanup、storage_usage_sample、notify_volume_check。管理/配置、观测和受控维护面清点；不为了“全部有接口”给任何用户一键GC/恢复。
- 可信领域事件/收件投影是执行阶段；审计、来源事实和actor不能由任意外部JSON伪造。不可丢交接走R01/CE04，事件协议与业务command分开。
- 对象字节签名直传是受控数据通道；控制面仍经purpose/owner/大小/时效与确认，不开放底层任意bucket/key/SQL。
- 鉴权谓词、同事务Port、构造/装配、错误、序列化与纯函数是明确例外，均有维护者/理由，不隐藏未外露业务操作。

这补齐了“内部能力没有HTTP是否都算遗漏”的裁定：真正可复用产品操作应有受权入口；单一用例内部步骤、证据写主、维护机制与纯函数不等同产品能力。新增复用入口先登记当前资格/资源规则，再做薄适配。

## 4. 完整库存怎样渐进进入共同路径

当前158个映射均标`same_path_runtime_verified=false`，**不声称此次文档即已改造实现**。CE02分三出口：

- **S/P6.1**：共同同步上下文/版本/资格和精确参数校验，选工单查询/写、文件控制与本地事务Port建立代表接口；不网络回环，不复制领域实现。
- **D/P6.3**：可靠提交/执行阶段、稳定operation/调用号、链A关联和受权结果；复用C03/R01/D12，不重新造队列/状态账。
- **L/P6.1起分阶段、P9最终验收**：按本清单逐模块迁移剩余当前产品入口，HTTP/内部/私有模块/选定MCP共用应用层和领域实现；无需在P6一次重写全部存量。第一非HTTP消费者出现前必须完成本能力等价授权、body校验、额度和相同效果反例，不能建立旁路后再补。

AL旧`/al/api/v1/...`仍是已有绑定；迁移到共同受权AL用例时，legacy URI只能作为版本化输入adapter，从网关dispatch到相同用例，再调用AL公开API。原始反代必须对该能力/类型关闭或受同一adapter拦截；不在Gin现有`/*rest`下重复注册冲突路由，不允许保留第二条绕row/field资格的原始通道。TR `/v1`是内部执行协议，不成为绕core业务准入的用户入口。

## 5. 缺失产品家族与验证边界

风险、HR同步、UAA企业adapter/切换、设备、LLM/Agent/MCP、通用capability runtime、AL效果receipt、AL源feed、受控订阅和跨入口预算分别在JSON`future_or_missing_capability_families`，全部有原载体及当前missing，不列成现有API。AI样例的risk.query/ticket.linked.query/knowledge.query/risk.dispatch.preview/apply/operation.receipt版本另列，均是候选名。

本次实际运行：三仓router构造枚举、全生产Go AST解析、handler→service/helper映射、非HTTP声明逐项裁定、UI调用匹配和来源路径/符号/哈希核对。**未运行现行业务数据库路径/跨仓副作用/多实例故障链，不把库存和schema通过写成当前实现正确或未来三链验收通过**。目标三链、精确Schema、合成正反例、净增预算与运行验收矩阵见[互通详设](capability-interconnect-detail.md)。

后继源码、注册、迁移/路由、UI调用变更必须重新核source_file_manifest和完整集合差异；减少或改分类须给理由/后继，不允许仅把枚举分母改小后宣告无遗漏。
