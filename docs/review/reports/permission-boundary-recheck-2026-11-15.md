# 权限与关键边界第二轮复验

日期：2026-11-15。主仓main `aa745173`，UI `69fbad99`、TR `b0906b83`；规划对照为`d1219aa3`。本轮沿前两份[权限专项](authorization-design-implementation-review-2026-11-15.md)、[核心交互专项](core-interaction-architecture-review-2026-11-15.md)继续查同类入口，**仅补审计与设计，未修产品、未合main**。

结论：新增确认**3组主要问题＋2项影响较窄的资源边界**。组织停用、组织赋角、审计维护动作和C03六类风险仍开放，没有因本轮普通回归通过而关闭。已将不成立、既有计划覆盖和不同信任级的候选排除，不能用审查次数保证未来零缺陷。

## 1. RECHECK-01 · P1 · 角色编辑把派生来源固化为直接授权

源码：`internal/service/user_service.go`的`GetUserRoleIDs`调用`RoleRepo.ListRoleIDsByUserID`，返回直接＋组织＋继承展开；同文件`SetRoles`却整体替换`user_roles`直接绑定。UI `src/views/system/user/index.vue`的`openRoles/submitRoles`将GET结果原样作为编辑值提交。

独占PG18.6＋真实PAT/JWT/Casbin/UserHandler链证明：用户原直接角色priority60，组织授priority20的user:update能力，但其直接角色档位不足以管理priority50目标，接口403。内置admin打开角色编辑并原样POST GET的role_ids，200后该组织角色成了直接角色，用户修改目标200且实际落库；通过正常RemoveMember移出组织后仍200。未进行该保存的同配置对照移出后403。**不是未授权用户直接冒充管理员，是管理页面的读写语义误导授权来源并改变后续撤权/档位。**

另一个同源负面效果：禁用但仍有直接绑定的角色不在effective集合，原样保存会把其绑定删掉。正常授权展示与“直接授予编辑”是两个视图，不能用同一个展开查询兼任。

最小改法：`GET /users/:id/roles`按文档本义返回直接绑定ID；有效三源仍供鉴权/菜单，不能为修本问题改回直接角色鉴权。已绑定停用角色作为失效绑定保留/标识，不把“停用不生效”误成“解除绑定”；删除角色另按既有软删政策。保留失效绑定不允许操作者新增超出自身可授档位的角色，写侧约束不放宽。用户管理的直接角色可授档位语义继续独立。C04需真值区分assignment与effective，不因AUTH-03修菜单同源就让编辑也复用展开IDs。

回归：直接/组织/parent三来源分别原样保存；去组织/断继承后只保留合法独立来源；停用角色绑定往返不丢；授权展示正确、无新直接grant，管理员显式新增/删除直接角色仍按当前权限/档位。

## 2. RECHECK-02 · P2 · 授权编辑未绑定加载成功与当前目标

源码：同一UI文件`openRoles/submitRoles`、`openOrgs/submitOrgs`及两个弹窗footer。

直接编译真实SFC script并调用其函数，Vue/TypeScript用当前安装版本、Node26.13.1，API/消息框为受控替身；5场景各8轮确认：

- 角色读取成功的对照保留选中值；读取失败被catch后或仍pending时，保存仍会提交空role_ids，可能意外清空直接角色。
- 先打开甲、再打开乙，乙先返回而甲后返回，组织选择会被甲覆盖；保存发给乙的user_id却带甲的org_ids/primary。
- 甲加载成功、乙加载失败时，乙保留甲的组织选择并可保存。

真实模板未给角色保存按钮绑定加载/错误禁用，submit也未核当前成功快照；组织加载缺请求世代和目标绑定。**本探针不是浏览器E2E，也未向真实后端写这些UI请求**；它证明真实组件函数的数据流，上一节/下一节另验证真实后端命令。

最小改法：打开目标即隔离旧草稿，加载结果绑定user_id＋请求世代；pending/error未形成当前成功快照时按钮及submit共同拒绝。失败显示可重试状态，不能当空集合；迟到响应不能覆盖新目标。加载成功后用户明确清空仍允许按原命令提交，不引入额外审批。C04/U06同批补真实浏览器延迟/失败拦截用例，不能只改按钮外观。

## 3. RECHECK-03 · P2 · 组织归属原样保存重置既有任职属性

源码：`internal/repository/org_repo.go`的`SetUserOrgsTx`先删全部user_orgs再仅插user_id/org_id/is_primary；`UserService.GetUserOrgs`和现有UI编辑只携带组织集合与主组织。

真实GET→POST相同org_ids和primary均200，但原`org_member_role=admin,ticket_scope=group`变成`member,assigned`。这是旧“整体覆盖组织ID集合”的设计在新增成员角色/scope等字段后没有续接，**不是外人突破权限，也不能把原DELETE实现描述为违反当时Phase1稿**。实际影响是无意撤去职责/数据范围；source/expires_at/joined_at等属性亦不能由此隐式重建，后续按字段补回归，本轮未动态证明所有属性组合。

最小改法：同一事务对集合做差量：保留项的is_primary按既有primary_org_id输入语义更新，不借差量改缺省/null含义；保留现有任职角色/scope/来源/时效；新增项按已有默认，移除项走现有owner双轨清理与撤权。成员角色/scope修改继续用既有专用命令，不借org_ids编辑改这些属性。与C04守卫及D06来源写主统一，不新造另一套组织授权平台。

## 4. 两项窄资源边界

### RECHECK-04 · 条件性P2 · 头像旧键退役未验合法归属

`UserService.UpdateProfile`允许写任意avatar；`internal/repository/storage_avatar.go`的`StorageRepo.BindAvatar`验新对象本人归属，却仅凭旧key/无users引用退役旧对象；`internal/service/storage/service.go`的`BindAvatar`随后物理删除该旧键。

独占PG及真实profile/storage handler/service/repo、认证主体夹具和内存S3连续8轮证明：**已知**他人confirmed且尚未绑定的avatar key时，先放进自己profile，再绑定自己的新头像，可退役他人暂存头像，原上传者后续绑定404。所有已绑定他人头像对照均保留，外人直接bind也404。未证明枚举UUID、删除活跃他人头像或读取秘密内容。

最小改法：旧对象退役须当前用户合法归属且无其它有效引用；内部对象键的profile写入与正式bind共享验证，保留合法旧外链兼容。C02/C04的资源引用规则补此头像路径，不只覆盖工单附件。

### RECHECK-05 · P3 · 附件删除在关联验证前透露内嵌图用途

`storage.Service.DeleteTicketAttachment`按全局key取元数据，先返回richtext专用400，后验证ticket_id关联。A票有update权时，传无权B的已绑定内嵌图key得400；不存在或B普通附件得404。8轮一致；需要已知key，**没有读取/删除B内容**。

最小改法：先确认`bucket＋ticket_id＋key`属于当前受权票，再返回用途专用提示；非法关联同形404，合法A内嵌图仍保留原400语义。随C02收口，不升级成新权限框架。

## 5. 排除、已知项和正确控制

- **人工重试假设被排除**：Go overlay曾用fake Inspector把一次旧attempt2任务当attempt1重投，复现SkipRetry后pending；但固定Asynq0.26.0的`internal/rdb/inspect.go`之`runTaskCmd`只移动到pending，不清重试计数，探针缺真实delivery context。故不计新生产缺陷；留反证，不以8次“确认”掩盖错误前提。Store.ResetPendingIfTerminal的旧注释误称计数重置，触碰时应校正。既有C03问题不因此关闭。
- **AL Import尾部EOF**、custom_data file/richtext Update绑定已分别在R06/C02排期，不重复计新遗漏。
- **CLI任意callback目标**需要宿主命令及数据库/队列凭据，按受信运维工具能力评估；不能报普通HTTP用户SSRF。新严格core profile仍须按已定目录/完成合同准入。
- **虚拟组admin删组**的静态宽判断在普通路径受成员保护阻断，本轮未证明可利用，不报新增漏洞。
- 角色priority/isSystem、用户直接角色管理档位、附件下载先验对象权限/再处理ETag、外人暂存及正式B对象不能跨票绑定等对照符合现行政策。全局字典/类型/模板及非敏感共享任务不误报IDOR。缺lookup/batch/relation-delete产品端点不当作漏鉴权路由。

## 6. 计划与证据

前3组挂C04/D06/U06及首版本地S1/薄UI修复；两窄资源边界挂C02/C04。正式使用前优先修角色来源和授权编辑数据流，普通全量门禁不能替代这些反例。实施前逐项与原包扣重复估，68–104日仍候选，不认定全部新修复零成本，也不因本审计增加顶层阶段。无需所有者现在决定新权限引擎、消息broker或授权审批体系。

原始证据在canonical main ignored `.codex/audits/second-recheck-20261115/`：`iam/probe.stdout`及冻结Go探针、root源码比对/持久样本；`ui-editor-probe-8-rounds.json`、真实SFC编译探针及源码hash；`resources/observed.log`、`expected-safe-red.log`、源manifest和清理收据；`retry-probe-disposition.md`保存反证。确认型probe的PASS是重现现状，资源安全预期另保留有效红，不当作产品已修。

IAM使用完整81迁移、专属PG和真实PAT中间件/handler；资源探针使用独占PG、注入认证主体/票授权口和内存S3；UI为真实script＋替身API，没有浏览器/后端实写声明。root以可核实日志/源码/持久样本为准，并用固定依赖源码反证错误候选，不把代理进度或未经验证判断当结论；独立复审仅列实际完成范围。各专属PG/匿名卷已精确回收，未操作共享账号/业务数据。

本轮独占栈通过lint（含Swagger）、race单元/集成、cover、migrate-up、完整acceptance：253通过/0失败/0跳过，81对迁移往返零漂移，默认标签覆盖率20.2%。这些是未修复产品的普通基线；专项反例另存，不关闭问题。UI未改产品，本轮未跑其全量E2E/远端CI，实际浏览器用例留修复出口。规划19/31项测试、原DAG及9份文件335处本地链接/锚点检查通过，价格/阶段分母不变。

基线临时9容器及项目卷/网络精确回收，两个独占探针PG及匿名卷已清理；16个既有共享容器的ID/镜像/状态/启动时间/重启数/挂载/端口均不变。五仓canonical main保持干净，本次只有本地设计文档提交，不推送/合main/部署；没有新增真实权限、路由或迁移。
