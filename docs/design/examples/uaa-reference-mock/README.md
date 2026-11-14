# UAA双模式与治理feed离线参考

> 日期：2026-11-13。状态：设计参考，全部为本地合成数据。运行只使用Python标准库，不访问网络、公司UAA/MQ、主仓IAM或业务数据库；不是OIDC/SAML/SCIM服务，也不是公共UAA产品实现。
> 权威设计：[UAA治理与双模式](../../uaa-governance-integration.md)、[公共实现估算](../../../review/uaa-implementation-estimate.md)、[真实联调条件](../../../review/uaa-real-integration-conditions.md)。

## 文件与复跑

- `fixtures.json`：固定画像与57个独立预期；修改实现不能顺手放宽这些预期。
- `uaa_mock.py`：单应用、单来源、串行内存参考模型。
- `verify.py`：运行57条行为及16个去守卫变异；任何行为失败或变异未被抓住均非零退出。
- `test_contract.py`：标准库unittest入口，同一组固定预期另以逐例测试呈现，不算额外57条覆盖。
- [port-map.json](port-map.json)：项目候选JSON端口与未来协议adapter的映射；不是已注册HTTP路由。

从主仓根目录运行：

```bash
python3 -B docs/design/examples/uaa-reference-mock/verify.py
python3 -B docs/design/examples/uaa-reference-mock/test_contract.py
python3 -B docs/design/examples/uaa-reference-mock/verify.py --output .codex/audits/uaa-audit-supplement-20261113/mock-reviewed-green.json
```

当前57/57行为、16/16变异通过，网络与企业调用为0。此结果证明本目录的有限合同；不替代主仓acceptance、真实协议互操作、会话撤权或多实例验收。先前38/9绿是独立审查随后发现缺口的历史结果，不再作为当前充分性结论。

## 有限画像与合同

画像包含两个本地人员、显式外部subject映射、两个受管能力`ticket.list`/`ticket.create`和非受管`profile.self`。不自动按邮箱、姓名或工号合并身份，不给外部角色内置管理员语义。

独立模式不需要UAA配置，使用既有本地权限。企业合成模式只从中央投影取得受管能力，旧本地受管绑定不能复活中央已撤权权限；本地安全停用仍优先。非受管本地能力保留。失联不会自动切回独立模式或打开本地密码入口。接管、恢复和退出的`approved`、`caller`、`verified`、`set_control`等是测试夹具的可信控制输入，**不属于可由HTTP/MQ调用者自行填写并取得权限的接口**；真实实现必须从受鉴别主体和获批配置取得这些事实。

归一化feed仅有三类：账户状态、单用户完整角色集合、单角色完整资源集合。封闭envelope包含`event_id/application/source/kind/subject/revision/complete/payload`，按`kind+subject`比较来源版本；来源/应用和调用上下文分开检查。完整空集合撤权，不完整集合不能冒充删除快照；重复、旧版本和同版本冲突分开处理，未知引用等待依赖，不临时放权。一个用户的待补依赖会保守阻塞本合成来源的受管调用；不据此设计生产全局锁。

内存接收返回`committed_pending_effect`或`held_dependency`；只有`apply_policy`完成才把对应收据标记`effective`。未曾生效便被新版本替代的旧收据保持`superseded`；同内容新传输event ID跟随原收据。不把接收、broker ACK、数据库提交、权限生效或外部回执送达混为同一成功。账户停用在投影前即拒绝，角色/资源变更在投影未收敛时暂停受管调用；重放不增加策略revision。

时间是夹具逻辑时钟。F006已确认失联先暂停受管写、公司允许且可证明授权有效时保留获准只读的方向，不重问这一原则。`freshness_window=10`仅为合成时窗，实际公司窗口、允许动作和准入参数仍未批准，不是生产SLA。无业务变更消息不自动等于失联；有效窗口内的当前授权读可按画像继续，明确失联时受管写暂停，窗口过期读也暂停。动作的read/write取自可信固定`capability_metadata`；调用者漏填或伪造`write`不会改变分类。合成可信心跳还检查当前connector是否启用，只推进已验证时间，不恢复被撤掉的权限；这里没有实现真实来源水位证明。

恢复入口仅允许指定、未过期的模拟操作员检查/修复连接/对账，不能授业务角色；F006已确认此用途，实际账号、动作和期限仍需绑定。退出需获批退出方案、本地凭据和**明确获批的本地权限基线**，批准退出不等于恢复接管前的旧grant。`exit_plan`钉身份映射、当前来源policy revision、退出前本地权限基线及退出后的明确权限映射；可信审批digest绑定该计划。缺计划保持企业模式，计划被改拒绝；身份/来源revision/本地基线漂移须重新预览审批。成功后只应用计划中的权限，保留本地身份；重新授予旧能力也必须出现在新批准映射中。夹具没有执行真实密码迁移、所有者审批或影子差异预览。U33及U31退出后的重载只是JSON状态连续性，不是磁盘/PG崩溃持久性。

## 固定预期与先红后绿

U01–U05覆盖双模式及完整空集撤权；U06–U17覆盖完整性、重复乱序、来源/应用/调用者边界、未知依赖和资源目录；U18–U21覆盖立即收紧、策略重载和生效收据；U22–U27覆盖时间窗口、失联及旧token/grant不恢复当前已撤权能力；U28–U31覆盖受限恢复/获批退出；U32–U35覆盖去敏、状态重载、收据当前权限和无效执行身份；U36–U37覆盖被替代及别名收据；U38检查审计调用者与目标归属。U39–U46覆盖退出基线、审批绑定、当前基线变化与明确新授权；U47–U49覆盖不可信write标记与可信动作类型；U50覆盖禁用connector不能刷新授权；U51–U54覆盖非对象JSON拒绝且不改变desired/收据/revision；U55–U57覆盖数组/对象kind在成员检查前拒绝，不抛TypeError。

固定预期先于守卫实现。初始可运行的按到达顺序覆盖基线在35例中9过、26失败，共31个断言失败；不是编译或环境失败。自审追加收据窗口后，37例中U36/U37实际失败，共4个断言；追加审计归属后，38例中U38实际失败，共2个断言。各轮红结果与当时源码分别封存在本地ignored审计目录，不能把这些不同轮次相加当成一次测试或产品漏洞。

独立审查的36个手写probe随后发现6个失败，涉及退出恢复旧grant、遗漏write标记绕过失联写暂停、禁用connector仍刷新心跳及三类非对象JSON异常。原U31期待“批准退出+凭据即恢复旧create”本身违反获批授权迁移合同，已明确更正，**不能声称原预期一直正确**；原六文件和38/9绿完整归档。先更正U31并补U39–U54固定预期，旧模型在54例中39过、15失败，共26个断言红，再加守卫转绿。独立扩查又发现畸形kind类型；先补U55–U57，57例中3条实际TypeError红，再补类型守卫转绿。更正理由和每轮原红均在本地ignored审计目录保存。16个变异包含原9项以及恢复旧退出权限、跳过退出digest/当前基线、信任调用者write、忽略connector撤销、移除非对象守卫和移除kind类型守卫，全部被固定预期抓住。

## 日志与审计接点

模型只把固定字段投影到内存audit：schema版本、审计ID、合成request ID、动作/结果、模式、来源、策略revision、调用者、已注册目标引用和散列后的来源事件引用。未知字段与合成secret标记不进日志；调用者和被操作的账户/能力是不同概念。U38仅检查授权判定的调用者/目标字段；不声称所有治理feed、恢复和接管事件已完成生产归属与全链路关联。

实际接线仍需在access/error/audit logger之前去敏，并分别记录登录、接收、拒绝/冲突、提交、投影生效/失败、撤权、失联/恢复、接管/退出及配置审批。不要记录原始code/state/nonce/code_verifier/token/secret、完整claims或HTTP/MQ原始头体。本模型没有真实授权码回调、审计持久化/查询权限、异步outbox、不可篡改、留存与外发投递；生成的request ID只演示字段形态，不证明跨服务链路唯一性。统一日志/审计的正式范围另由拥有者合同和后续实现验收。

## 公共标准依据及支持限界

- [OIDC Core 1.0](https://openid.net/specs/openid-connect-core-1_0.html)的ID Token验证、UserInfo subject一致性和claim稳定性规定支持以`issuer+subject`映射身份；它不规定企业菜单/API权限同步。公司是否使用OIDC尚未确认，本目录没有ID Token/JWKS/nonce验证。
- [OAuth安全实践RFC9700 §2.1](https://www.rfc-editor.org/rfc/rfc9700.html#section-2.1)支持授权码+PKCE候选画像。它不能让测试布尔值成为真实认证；实际redirect/state/code重放/PKCE和issuer/audience边界需要真实协议测试。
- [SCIM schema RFC7643 §4.1.2](https://www.rfc-editor.org/rfc/rfc7643.html#section-4.1.2)未统一roles/entitlements词汇及语法。因此不能把SCIM当作公司角色→菜单/API治理的现成合同，不能假定SCIM id/externalId等于OIDC subject。
- [SCIM protocol RFC7644 §3.4.2.4](https://www.rfc-editor.org/rfc/rfc7644.html#section-3.4.2.4)明确分页不保留跨请求状态。`totalResults`或短页不足以证明权威完整快照。本模型`complete`是**合成生产者已经证明单个对象/集合完整性**的输入，未实现SCIM采集或分页证明；若未来选用SCIM，由adapter隔离它的HTTP方法及协议差异。

可复用[既有MCP SDK合成AS样例](../mcp-sdk-interop/README.md)的真实HTTP PKCE/授权拒绝测试方法，但该样例不是OIDC登录；[能力参考](../capability-reference/README.md)的receipt及[AI/SOC参考](../ai-soc-reference/README.md)的当前权限复核也不替代核心IAM。

## 正式实现与真实联调边界

研究和本样例已完成部分不重复计价；公共产品P25–39工程日仍是待实施的公共端口、真实IAM三源撤权、最小UI和运维接线，不因57例通过改成已实现。C可先冻结公开归一化语义，P保留可替换合成供给；真实R13–22仍需实际协议/消息/字段、测试主体/资源目录、连接和公司撤权时效等条件。S01的真实影子diff、S02在公司约束内的真实失联时窗/动作、S03实际恢复操作员/动作/期限须在有真实事实后批准；不把测试参数当这些实际清单已批准，F006确认的失联与恢复用途方向继续遵守。

未验证项还包括：真实登录/登出与本地JWT、三源授权图/组织继承、跨实例会话/缓存失效、来源水位与分页/对账完整性、schema升级、批量流控重试/死信、PG事务并发、网络故障、回调重放防护、完整退出迁移以及公司供应方兼容性。它们进入正式P/R验收，不要求为了当前设计演示读取公司私密资料。
