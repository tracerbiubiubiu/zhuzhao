# 01 · 前端工程设计（zhuzhao-ui 初版）

> **文档定位**：zhuzhao-ui 的**工程设计文档**——架构、壳层、状态分界、页面范式、工程化与里程碑。功能规格不在本文：工单域（BK-18 前端/动态表单/审批页）以 [phase3/12-frontend](../phase3/12-frontend.md) 为规格 SSOT；动态路由与菜单契约以 [phase1/07-menu](../phase1/07-menu.md) 为准；工程架构五拍板（仓格局/底座/范例页/views/部署）以 [00 号 §2.1](./00-planning-inventory.md) 为准，本文是其展开。
>
> **状态基准**：2026-09-21。所有后端契约字段均经代码核验（`zhuzhao-utils/response`、`internal/model/token.go`、`internal/service/menu_service.go`、000002/22/24/25 菜单种子）。
>
> **设计哲学**（与后端同构）：抄模式不抄架构——底座模板只取重件，壳层按 zhuzhao 三层鉴权契约自写；约定靠「范例页 + 共享封装」承载，不靠代码生成器；源码组织与部署打包解耦。

---

## 1. 技术栈与选型基线

| 层 | 选型 | 状态 | 说明 |
|----|------|------|------|
| 框架 | Vue 3 + `<script setup>` + TypeScript strict | ✅ 拍板（12-frontend §1） | 全程 Composition API，不混用 Options API |
| 构建 | Vite | ✅ 拍板 | dev 经 proxy 转发 `/api` **与 `/al`** 到本地后端（`/al/api/v1/*` 是网关反代根级挂载，经 zhuzhao 转发 activelist——漏配则 W5 名单页 dev 期 404）；生产同源，无运行时配置需求。dev 环境依赖：**activelist 容器在线（本仓 compose 不含它，跨三仓编排脚本见 02 §2-W2 首日项——十批 P1-4 修正口径）**；`configs/config.yaml` 已配 `/al` 上游无额外配置；上游不可达=502+10008 |
| 组件库 | Element Plus（+ `@element-plus/icons-vue`） | ✅ 拍板 | 菜单表 `icon` 列的种子值（`home`/`settings`/`user`/`role`/`menu`/`org`/`ticket`/`task`/`al`/`audit-log`…）建集中映射表解析；⚠ 种子中 `settings`（system 目录）与 `setting`（ticket_type_manage）**近似键并存**，键值照抄种子勿「纠正」拼写 |
| 底座模板 | **vue-element-plus-admin v3.1.0** 种子拷入（备胎 pure-admin-thin） | ✅ 定案（2026-09-21） | 四候选代码级评估，见 §9.1 |
| 路由 | vue-router 4 | ✅ 拍板 | 动态注册模式见 §3.2 |
| 客户端状态 | Pinia | ✅ 拍板 | **只放 session/菜单/UI 态**，见 §4 分界 |
| 服务端状态 | `@tanstack/vue-query` | ✅ 定案（2026-09-21 随计划终确） | 管理端 90% 状态是服务端数据；缓存/重取/失效声明式，避免全量 Pinia 化的脏数据地狱 |
| HTTP | axios | ✅ 定案（2026-09-21 随计划终确） | 拦截器链成熟（§3.3）；不引二次封装的请求库 |
| 动态表单 | form-create（渲染 `@form-create/element-ui`；设计器 `@form-create/designer`） | ✅ 拍板（12-frontend §3.1 / 00 §2.1） | 仅工单自定义字段用；普通表单走 el-form + rules |
| 时间 | dayjs | ✅ 定案（2026-09-21 随计划终确） | 后端时间戳格式以 swag 为准，集中一处格式化 |
| 原子 CSS | UnoCSS | ⏸ 暂不启用 | Element Plus 样式体系起步足够；出现高频布局痛点再开（开放问题） |
| 质量门禁 | vue-tsc + ESLint + Prettier / Vitest / Playwright | ✅ 定案（2026-09-21 随计划终确） | 见 §7；与 Go 门禁分仓各跑 |

**明确不做**：SSR/Nuxt（管理端无 SEO/首屏诉求）、微前端（§2.1 五拍板①）、移动端/响应式（12-frontend §6 既有口径）、Storybook/视觉回归测试/浏览器兼容矩阵（内部管理系统过重，02 §5.3 处置 #11）。**i18n 不在「不做」列**——升格为可选项：P4-W3（system 域）收口后视进度接入（默认做，[02 §5.4](./02-implementation-plan.md)）。

## 2. 目录结构（单仓单 SPA）

```
zhuzhao-ui/
  src/
    api/<域>/<资源>/{index.ts, types.ts}   # 按域分文件；类型与请求同域共存
    views/<域>/<功能>/index.vue            # 命名从 07-menu 契约：动态路由 component 字符串直映射
                                           # ⚠ 12-frontend §4 旧稿为 src/pages/，P4-W2 首日改为 views 并回写该文档
    components/                            # 跨域通用组件（ProTable/AuthButton/…）
    common/
      auth/{permission.ts, directives/}    # 权限三件套（§3.4）
      request/                             # axios 实例 + 拦截器链（§3.3）
      constants/{errorBehavior.ts}         # 需特殊行为的错误码白名单（§3.3）
    stores/{session.ts, menu.ts, ui.ts}    # Pinia：仅客户端状态（§4）
    router/{guard.ts, dynamicRoutes.ts}    # 守卫时序 + 动态注册（§3.1/3.2）
    layouts/{AdminLayout.vue}              # 侧栏（菜单树渲染）+ 顶栏 + 内容区
  tests/{unit/, e2e/}                      # Vitest / Playwright
```

**域清单与后端对齐**（`<域>` 取值即菜单种子的顶层目录）：`system`（用户/角色/菜单/组织）、`ticket`（工单）、`task`（任务管理，API 走 zhuzhao 代理端点）、`al`（名单，API 走网关反代 `/al/api/v1/*`）、`audit`（审计日志）、`login`/`profile`（自服务）。**五个后端仓都不放前端代码**；`al`/`task` 页面消费的是 zhuzhao 控制台的代理/反代端点，权限判定集中一份。

**ID 序列化公约**：全仓 int64 主键/外键 JSON 序列化为**字符串**（`json:"...,string"`——`id`/`parent_id`/`tenant_id` 等；根节点 `parent_id` 为 `null`）。手写壳层代码（动态路由注册、AssignMenus 勾选树）一律按字符串处理；codegen 生成类型自带 `string` 无需特判。

## 3. 应用壳层四件（模板不可替代部分，全部自写）

### 3.1 启动引导与路由守卫（时序）

```
应用启动
 └─ router.beforeEach 守卫（每次导航）：
    1. 白名单页（/login、改密页）→ 直接放行
    2. 无 AT → 重定向 /login（带 redirect 回跳参数——**须校验为站内相对路径（以 / 开头且非 //），防开放重定向**）
    3. session 未加载（首跳/刷新）→ 并行拉 GET /user/profile + GET /user/menus + GET /user/permissions
       → 存 Pinia → 按菜单树 addRoute（§3.2）
    4. must_change_password=true（登录响应与 /user/profile 均回带该标记）→ 强制重定向改密页。
       后端已双保险：AT claims 带标记时除 /auth/password/update 外全部业务端点 403+20007
       （internal/middleware/jwt.go），前端 gate 只是体验层
    5. 注册 404 catch-all（必须在全部 addRoute 之后，仅一次）
    6. next()；目标路由不存在（component 解析失败/无权限）→ 落 404 页
```

要点：profile/menus/permissions **三件并行**——profile 供顶栏用户信息与改密标记（刷新后强制改密流不失效）；菜单/权限码**每会话拉一次**，登出即清 Pinia（不落 localStorage，防登出残留）；「已加载」标志防守卫重入死循环。

### 3.2 动态路由注册（07-menu 契约的落地核心）

- **component 字符串解析**：`import.meta.glob('../views/**/*.vue')` 建 `Map<string, Component>`，菜单节点 `component: "system/user/index"` → `views/system/user/index.vue`。零构建步骤介入；glob 在构建期静态展开，lazy chunk 按页面自然切分。
- **注册范围**：只取 `menu_type IN (1,2)` 且 `visible=true` 的节点（后端 `GET /user/menus` 已过滤，前端不重复判）；目录（type 1）作为父级布局路由，页面（type 2）挂 `component`。
- **404 时序红线**：catch-all `{path: '/:pathMatch(.*)*'}` **只能在动态路由全部 addRoute 完成后注册**——先注册会导致刷新/直达 URL 误跳 404（RuoYi 系最高频死链坑，见 00 号业界核验）。
- **契约纪律**：新页面前端组件、菜单种子迁移、后端路由**同一 PR**（07-menu 推荐流程）；前端不得运行时要求后端加菜单行（菜单词表已拍板只读化，00 §2.4）。
- **种子实况四特例 + 路由三层（routerHelper 必须覆盖，解析单测用真实种子值；①–④ 为种子形态特例、⑤ 为配套机制——十一批计数勘误）**：
  ① **父子同 path**：`/tickets` 目录（ticket_manage）与 ticket_list 页面同 path，`/tasks` 同理——naive 嵌套注册得 `/tickets/tickets`，须父 redirect/子绝对路径规则（RuoYi 系标准坑）；
  ② **无父顶层页面**：audit_log 为 type=2 且 parent=null（000025），需顶层页面注册分支；
  ③ **目录带非空 component**：home 为 type=1 且 component='home'（000002）——约定「type=1 且 component 非空按页面渲染」，形态定死 **`views/home.vue`**（对齐 07-menu 契约 `src/views/{component}.vue`：单段 component=文件，多段如 `system/user/index` 才走目录/index；glob 两种键形态天然并存，按 component 字符串精确查表）；W2 配工作台页，登录后 redirect 第一个可见菜单=/home 顺理成章；
  ④ **菜单外静态补充路由**：工单发起/详情（W4）、**「我的组织」自服务页（W3——委托入口对非 admin 委托者唯一可达，02 §2-W3 拍板）**等页不在菜单词表（只读化后不能加行），前端静态注册隐藏路由（挂父布局下），入口按钮由 `button:ticket:create/read` 码控制；静态路由无 `route:` 码，守卫放行（真实边界在后端，§6）。**W5「页面清单与菜单种子对账」须豁免静态路由清单**，防对账误报；
  ⑤ **常量路由组（constantRoutes）**：/login、403、404、强制改密页、redirect 中转——应用启动即注册、不依赖菜单树（菜单种子无这些节点），与菜单动态路由、④ 静态补充路由三层并存。

### 3.3 请求层（axios 实例 + 拦截器链）

统一信封（已核 `zhuzhao-utils/response`）：成功 `{code:0, message, data, request_id}`，失败 `{code≠0, message, data:null, request_id}`；分页 `data:{list, total, page, page_size}`；登录返回 `{access_token, refresh_token, expires_in, must_change_password}`（`internal/model/token.go`）。**自服务端点响应形状**：`GET /user/menus` → `data:{menus:[...]}`、`GET /user/permissions` → `data:{permissions:[...]}`、`GET /user/profile` → `data:User`；`POST /auth/refresh` → TokenPair（同登录形状）。codegen 产出类型自带正确形状，此处为契约描述完整性。

| 拦截器 | 行为 |
|--------|------|
| 请求 | 附 `Authorization: Bearer <AT>`；透传/生成 `X-Request-ID`——格式 **`req-`+32 位小写 hex**（D2-24，不符后端静默丢弃重生成，UUID 直生成无效） |
| 响应-成功 | 后端 `code≠0` **恒伴非 2xx HTTP**（`response.go` 的 `Fail` 恒显式状态码；`code=0` 恒 200）——axios 成功回调只会收到 `code=0`，直接返回 `data`（调用方拿到的就是业务数据）；分流按 HTTP 状态做，不按信封 code |
| 响应-401 | **先分码（B2-1）**：`20002`=AT 过期→可静默刷新；`20003`=签名错/typ 混淆/黑名单→**不刷新**，直接清 session 跳 /login。**单飞刷新**（仅 20002 触发）：并发 401 只发一次 `POST /auth/refresh`，其余请求挂起等待后用新 AT 重放；刷新失败（实测码族 **20004/20014/20015**——RT 无效/改密纪元/重放）→ 清 session → 重定向 /login。**HTTP 5xx（Redis 抖动 503+10008，fail-closed 可重试）不清会话**——拒绝挂起请求并提示重试，防一次抖动把全员登出。防刷新风暴是 RT 轮换机制的前端配合面 |
| 响应-错误 | toast 展示后端 `message`（后端已做文案人性化，**前端不维护码→文案映射**）；`request_id` 一并展示供报障对日志；仅「需特殊行为的码」进白名单表（`common/constants/errorBehavior.ts`）：`20007`（403 强制改密——跳改密页**不清会话**）、`20006`（账号锁定，HTTP 429——勿入 401 刷新/跳转分支）、强制登出类 |

**流式旁路（al 导出/导入）**：activelist `GET /al/api/v1/data/:typeName/export` 文件本体是**裸 JSON 数组、不走统一信封**（activelist 实现拍板：信封包文件体破坏流式与导出/导入对称性；流中途错误响应头已发、只能截断 body）。经 al 适配层走 `responseType:'blob'` 直下分支，不经信封拦截器；导入 body=同构 JSON 数组同理。

**Token 存储抽象**：`common/request/tokenStorage.ts` 定义 get/set/clear 接口，现态实现 = 内存 + localStorage（AT/RT + **`device_id`**——浏览器级 UUID，后端白名单 `[a-zA-Z0-9_-]{1,64}`（D2-22，UUID 天然合规），**登录/登出/改密三处同源（刷新不需要——设备身份在 RT claims 内，RefreshRequest 无此字段）**；后端注释明确要求前端生成并每次登录携带，缺省归 "default" 单槽会多浏览器互踢 20004/20015，登出漏传则吊销 "default" 空槽而真实 RT 槽残留，**改密轮换 RT 时漏传会把新 RT 写进 "default" 错槽 → 互踢**）；同浏览器多标签共享同一 device_id（配合单飞刷新），不同浏览器各自独立槽位。B7 cookie 会话演进（phase2 D2-23 方向）时**只换实现**，调用面零改动。localStorage 方案运行期须配合 XSS 基线（§6）。

**API 类型生成链（02 §5.3 处置 #3，P4-W2 壳层件）**：前端类型不手写——后端 `make swag` 产出 OpenAPI 后经 openapi-typescript（或 orval）生成 TS 类型进 `src/api/__generated__/`，链路 = `make swag && pnpm codegen`。⚠ swag 产物为 Swagger 2.0——**2026-09-21 拍板：目标格式 OpenAPI 3.0**，链路中加转换步（swagger2openapi 类工具做 2.0→3.0 机械转换），具体工具随 P4-W2 首日锁版本一并定。契约漂移（后端改字段形状）在**前端编译期报错**，不靠人记（对症「OKPage 形状变化断言要改」类历史教训）。纪律：生成物不手改、随后端契约批同批再生成。

### 3.4 权限三件套（12-frontend §3.5 的实现规格）

- `route:{path}` 码：路由级兜底——守卫在 addRoute 后对目标页校验 `route:` 码集合（菜单树本身已过滤，此层防御 URL 手输）；
- `button:{permission}` 码（**= menus.permission 字段值拼接，如 `button:ticket:create`——非 menus.code（如 ticket_create_btn），照 code 拼会全部不渲染且酷似「正常无权限」，十一批高危勘误**；menu_service.go「button:"+m.Permission」）：`v-permission` 指令（无权限**移除 DOM**，非 `display:none`——防 DevTools 放显）+ `<AuthButton :capability>` 组件（置灰态，用于「能看不能点」场景）；
- `usePermission(ANY/ALL)` composable：复杂显隐逻辑（多码组合）走函数式，模板里只留指令；
- 码来源：`GET /user/permissions`（admin/superadmin 由后端全量展开，B4-4 语义前端无感知）。

## 4. 状态管理分界（最易做错的一件）

| 类别 | 归属 | 例 |
|------|------|-----|
| 客户端状态 | **Pinia** | session（用户基本信息+权限码；**无角色**——`GET /user/profile` 返回的 User 结构体无角色字段，角色名无处可取）、菜单树、布局 UI 态（侧栏折叠） |
| 服务端状态 | **vue-query** | 一切列表/详情/树数据（用户、角色、工单、审计、死信…） |

- **query key 约定**：`[域, 资源, 参数]`（如 `['system','users',{page,keyword}]`），写操作成功后按 key 前缀失效（`invalidateQueries({queryKey:['system','users']})`）；
- **禁止**把接口数据手工搬进 Pinia（历史上这类模式必然产出「改了 A 页 B 页不刷新」的脏数据）；
- 分页参数与后端 `page/page_size` 字段直通（已核 `PageData`），ProTable 封装内做映射，页面层零感知。

## 5. 页面范式与范例页（约定的活文档）

前端启动批**先产三个范例页再批量铺页面**（建议挂在 system 域，页面最典型；**边界：范例页=约定载体（精简版），W3 铺全量功能面时以 W3 范围为准扩充——十批边界注记**）：

| 范例 | 承载约定 | 规格 |
|------|----------|------|
| 列表页（用户管理） | ProTable 封装 | 搜索区（el-form 内联；**以各端点实际过滤参数为准——部分列表暂无 keyword 模糊搜索，有则接、无则显式留空，不得假搜索**）+ 表格 + 分页 + 操作列权限槽位（`v-permission`）；加载/空/错误三态由 vue-query 统一供给（loading=skeleton、error=重试卡片）；写操作（新建/编辑/删改确认）走页面内弹窗 |
| 表单页（角色编辑） | el-form + rules | async-validator 规则集中定义；服务端字段错误回填到对应表单项；提交防重（按钮 loading） |
| 树管理页（组织，承载 CRUD 范例语义） | el-tree + 表单联动 | 左树右表布局；节点 CRUD/move/占用预检提示（子节点/成员占用）。**菜单页为只读树**（P4-W1 只读化后无写接口，ErrMenuHasChildren 随 **000031** 清理——十二批勘误，死码源于词表只读化非 BK-18），CRUD 范例语义由组织树承载 |

ProTable 是页面一致性的最大杠杆：**所有列表页禁止手搓 el-table + 分页拼装**，一律经封装（封装内统一处理 `PageData` 解包、排序参数、列权限、操作列宽度）。自建封装的参考实现 = Geeker-Admin 的 ProTable（MIT，组件 + useTable 约 1100 行，自治低耦合）——**只借鉴设计，代码按 `PageData` 契约自写**，不引依赖。工单域动态表单走 form-create 渲染器（12-frontend §3.1），不进 ProTable 范畴。

**双分页模式（02 §5.3 处置 #2）**：封装必须同时支持两种分页——**offset 模式**（`page/page_size`，zhuzhao 全部端点）与 **cursor 模式**（keyset 游标，activelist `/al/api/v1` 端点：请求 `after_created_at`(RFC3339)+`after_id` 成对+`page_size`，响应 `{list, page_size, next_cursor}`，**无 total**——历史实锤坑：cursor 含时区 `+` 过网关必须 urlencode，须在 al 适配层内统一解决，不外溢到页面层）。⚠ 封装接口预留时勿只预留参数形状：cursor 模式分页器是**上一页/下一页形态**（无页码跳转、无总数），UI 形态一并预留；**无 total 形态适用面=al 域（cursor）+ taskrunner 死信（page 翻页但 {list,page_size} 无总数，十三批扩面）**。页面层经数据源抽象无感切换，避免 W5 现场返工。

## 6. 会话与安全

- **登录**：`POST /auth/login`（`employee_no`+`password`+**`device_id`**——浏览器级 UUID 必传，语义见 §3.3 TokenStorage）；响应含 `must_change_password`（首登强制改密，守卫分支见 §3.1；**改密成功响应即新 TokenPair——改密即轮换**，须整体替换本地 token，旧 AT 已进黑名单；改密请求须带同一 `device_id`——UpdatePassword 轮换 RT 落槽用，漏传落 "default" 错槽互踢）；TokenPair 存 TokenStorage（§3.3）；**登录表单预留验证码插槽**（P4-7 在穿插池后位，届时只接插槽不返工表单布局）；
- **登出**：`POST /auth/logout` **携带与登录相同的 `device_id`**（漏传=吊销 "default" 空槽，真实 RT 槽残留）→ 清 Pinia + TokenStorage + vue-query 缓存（`queryClient.clear()`）→ /login；
- **请求方法**：全仓仅 GET/POST（standards §3-1）。BK-18 五端点的历史 PUT/DELETE **已拍板整改（2026-09-21：单仓涉及直接改，不豁免）**——随 P4-W1 改 POST zhuzhao 风格并删 standards §3-5 豁免条；整改合入后前端按 GET/POST 消费即可，请求封装仍勿写死方法白名单（防御性）；
- **CSRF 面**：现态纯 Bearer 头（浏览器不自动附带）**无 CSRF 面**——B7 cookie 会话演进时才需评估（SameSite/CSRF token 随 B7 拍板）；
- **XSS 基线**：全站禁 `v-html`（例外须评审并 sanitize）；AT/RT 存 localStorage 的风险与 B7 cookie 演进绑定，TokenStorage 抽象保证一步迁移；
- **错误边界**：`app.config.errorHandler` 全局兜底（上报 console + 友好页）；路由级 403/404 独立页面；**错误上报服务（Sentry/GlitchTip 类）触发驱动**（02 §5.3 处置 #10：对外可访问或用户成规模再接，errorHandler 是接入门槛最低的挂点）；
- **接口粒度不设防前端**：前端显隐只是体验层，真实鉴权在后端三层（L1 Casbin 为准）——前端权限码仅用于渲染决策，不得视为安全边界（与「按钮码不走 Casbin」的既有口径一致，07-menu）。

## 7. 工程化与门禁（「前端验收口径待定」的建议答案）

| 环节 | 工具 | 门禁口径 |
|------|------|----------|
| 静态检查 | `vue-tsc --noEmit` + ESLint + Prettier | 等价 Go 侧 `make lint`，CI 必跑 |
| 供应链 | `pnpm audit --prod` | 对称 Go 侧 govulncheck 决策（02 §5.3 处置 #5）——npm 供应链是前端最大风险面，随 lint 同批跑；renovate 可选（仓多再上） |
| 单元测试 | Vitest | 盖 composables（usePermission/request 单飞刷新/tokenStorage）与纯函数（动态路由解析/菜单树转换）；组件测试只盖 AuthButton 等关键件 |
| E2E | Playwright | 冒烟集：登录→菜单渲染→列表页翻页→按钮权限（对应 12-frontend §5 FE3）；**✅ 拍板（2026-09-21）：打标准三栈 compose，不用 stub**——菜单可见性/权限码必须打真后端，stub 抓不到契约类 bug（role_menus 缺口活例）；测试账号基座三档（admin=既有 E000001——**凭据幂等闭环：首跑经强制改密流程改到固定测试密码（存 E2E env），后续跑复用该密码，否则改密即轮换会破坏二次运行**；operator/viewer=E2E setup 幂等建号（H1：不进迁移）+ 运行时 POST /roles/menus 分配） |
| 验收基线 | 12-frontend §5 FE1–FE3（FE4 审批链路随主轴⑤翻案批复活时验收，02 §5.1 已同步修订） | FE1 动态表单 / **FE2 管理全流程无 SQL（口径=02 §5.1：用户/角色/菜单/组织/工单类型配置）** / **FE3 viewer 业务只读可见/管理面+审计不可见**（B 案后口径，替换原「双级不可见」） |
| 构建 | `pnpm build` → dist | 产物交付=nginx 静态容器 + 反代 API（00 §2.1 五拍板⑤推荐形态）；CI 分仓：zhuzhao-ui 自己的流水线跑前端门禁，Go 四仓门禁不掺和 |

**仓引导（02 §5.3 处置 #4，P4-W2 首日）**：zhuzhao-ui 落 AGENTS.md（前端版协作协议：门禁四件 + 提交规范 + 变更评审三节的前端适配——摘要/影响面/验证证据）、README（定位 + 设计文档指回本仓 docs/phase4/01）、LICENSE 已在、node/pnpm 版本锁定文件（`.nvmrc`/`package.json` engines）——四个 Go 仓都有协作协议，新仓不裸奔。**分支纪律（所有者 2026-09-21 指示）**：不直接在 main 开发——main 只收合入；建设期每 Wave 开短命分支（`p4-w2-shell`…），全量前端门禁绿后合入即删（对齐 standards §12.7 单主干精神，前端无 CI 故门禁本地跑、合入前必绿）；种子拷入也在首个 wave 分支上进行。

## 8. 里程碑切分（编号与 02 号 §2 对齐；2026-09-21 验证修订：原 W1–W4 改 P4-W2–P4-W5，消除跨文档错位）

| Wave | 内容 | 出口标准 |
|------|------|----------|
| P4-W2 壳层 | 底座模板裁剪 + 壳层四件自写 + 登录/登出 + **首登强制改密流（20007 gate + profile/改密页，§3.1/§6）** + **个人中心页（profile 自服务：资料展示/编辑 + 自愿改密入口——与强制改密页共用表单组件，十一批显式化）** + 菜单树渲染 + 动态路由 + 三范例页 + **首页工作台（简单仪表盘——2026-09-21 拍板：待办/已办等卡片。首版=可见工单状态统计+最近工单列表，零后端改动；「处理人=我」维度需后端补 `assignee=me` 查询参数，随 P4-W4 工单域批交付后点亮待办/已办卡）** | 范例页过 FE3（viewer：业务只读可见/管理面+审计不可见——**viewer 账号由 E2E setup 幂等创建（H1：不进迁移）+ 运行时分配绑定**）；强制改密流过 E2E；E2E 冒烟绿 |
| P4-W3 system 域 | 用户/角色（含 AssignMenus 勾选树——**el-tree 必须 `check-strictly=true`：级联勾选会让「只勾页面、不勾按钮」的只读授权选不出来，B 案词表拆分的 UI 前提**；int64 string 公约）/菜单（**只读树**+角色分配入口）/组织两面（02 §2-W3 拍板）：管理面（admin 专属）+「我的组织」自服务静态页（§3.2④；owner/admin 见委托控件；⚠ `ticket_visibility` 仅 update 表单） | 管理全流程无 SQL（FE2 同口径）；**API 权限即时生效（写后刷内存语义，rbac reloadPolicy）+ 侧边栏/动态路由下次登录刷新**（原「每请求读库/无需重登」措辞不准，已修订） |
| P4-W4 ticket 域 | 工单发起（form-create 动态表单，**菜单外静态路由** §3.2④）/列表/详情（静态路由）/评论/备注/关联（写操作挂对应按钮码——`ticket:relation` 随 B 案补种）+ 处理动作=close/assign/update/delete（**后端无 approve 端点**，审批随翻案批）+ 类型/字段/模板管理三件套（历史 PUT/DELETE 已随 W1 整改为 POST） | FE1 + FE2；新增路由的菜单种子同 PR 对账（07-menu 流程） |
| P4-W5 al/task/audit 域 | 名单页（`/al/api/v1` 反代：types/data 两页，cursor 分页；**含导出（blob 流式旁路）/导入入口**）/任务中心（tasks/runs/死信/jobs **页内 Tab**；死信只读列表，重试走 `POST /tasks/retry`，无独立重投端点——取消/重试按钮挂 `task:operate` 码，B 案补种）/审计日志查询 | 各域页面经对应权限码可见性验证；页面清单与菜单种子对账一致（**豁免静态路由清单** §3.2④） |

## 9. 开放问题（启动批拍板）

1. ~~底座模板最终选型~~ **✅ 已定案（2026-09-21 四候选代码级评估），全文见下节 §9.1**；
2. UnoCSS 启用与否（建议默认不启用，见 §1）；
3. 多标签页登出/改权同步：BroadcastChannel 监听登出事件强制各标签失效（低成本，建议 P4-W2 顺带）；401 被动失效已由请求层覆盖；
4. ~~暗色主题~~ **✅ 拍板（2026-09-21 所有者要求支持明暗切换）**：Element Plus 原生 dark CSS vars（`html.dark`）+ 顶栏切换开关 + localStorage 持久化偏好，P4-W2 壳层顺带；
6. 模板自带 tab 页签系统（多标签工作区）去留：保留=多页状态/去留=单页+面包屑更简——W2 拿到种子实看后定（二十二批登记，倾向去留皆可、与 B 案按钮授权无耦合）；
7. ~~Playwright 冒烟环境口径~~ **✅ 拍板（2026-09-21）：复用标准三栈 compose，不用 stub**——stub 的 mock 数据抓不到后端契约类 bug（role_menus 绑定缺口即活例）。

### 9.1 底座模板选型定案（2026-09-21，四候选代码级评估）

**结论：首选 vue-element-plus-admin v3.1.0；备胎 pure-admin-thin；Geeker-Admin 仅作 ProTable 参考实现；soybean-admin-element-plus 排除。采纳机制 = degit 种子拷入 zhuzhao-ui，不 fork。**

评估前提：Element Plus 已拍板为硬筛（antd/naive 主线直接排除）；评估轴 = 权限层可拔度（最高权重）/请求封装可改造度/演示页占比/整洁度/维护健康/壳层重件。**一个总体发现：四候选全部已是「后端下发路由 + `import.meta.glob` 组件映射 + addRoute」模式——本文 §3.2 的动态路由架构即业界事实标准。**

| 候选 | 可拔度 | 请求封装 | 演示占比 | 整洁度 | 维护健康 | 壳层重件 | 换壳工时 | 关键事实（代码核验） |
|------|--------|----------|----------|--------|----------|----------|----------|----------------------|
| **vue-element-plus-admin v3.1.0（首选）** | 5 | 4 | 5 | 4 | 4 | 4 | **3–5 人日** | 2026-09-01 仍在提交、release-please 高频发版；v3（2026-08 重写）全库仅 69 src 文件=极简壳；信封 `code=0` 与 zhuzhao 同构；`permission.ts`→generateRoutes→addRoute 与本契约几乎一致；MIT、0 个 .js 源文件。风险：Vite 8/vue-router 5/Pinia 4 极新（插件兼容小概率踩坑）+ v3 刚发布资料少 |
| **pure-admin-thin（备胎）** | 5 | 4 | 5 | 4 | 3 | 4 | 5–8 人日 | v6.2.0（2025-10-30）后上游停更 ~11 个月；404 尾注册（`addPathMatch`）与 i18n 移除做得最彻底、与契约最同构；views 仅 8 页；风险：`@pureadmin/utils` 自研轮子渗透深（清理成本）、刷新为本地 expires 预判须替换 |
| Geeker-Admin（不作底座） | 5 | 4 | 2 | 4 | 3 | **5** | 5–8 人日 | 72 vue 中 ~55 演示页裁剪量大；信封 `{code:200,msg}`、无 refresh token；tag/CHANGELOG 停 2023、单人维护。**价值点：ProTable + useTable 约 1100 行（MIT、自治低耦合）= §5 自建列表封装的参考实现** |
| soybean-admin-element-plus（排除） | 4 | 4 | 2 | 4 | 3 | 5 | 8–11 人日 | i18n 深耦合（78 文件引用 `$t`、生成路由自带 57 个 i18nKey）；rolldown-vite 非官方 Vite；EP 变体（末次 2026-06-22）维护力度明显弱于主线（2026-09-07） |

**采纳机制（P4-W2 第一日）**：

1. `npx degit kailong321200875/vue-element-plus-admin` 种子拷入 zhuzhao-ui 工作区（degit 不带上游历史，仓史从 zhuzhao-ui 自己首笔提交开始）；⚠ 上游是 **pnpm monorepo**（`apps/admin` + `packages/{request,hooks,…}`）——拷入后首日先定结构：**拍平为单应用**（apps/admin 提根、packages 内联为 src/common 子目录，推荐——zhuzhao-ui 无多包诉求）或保留 workspace（后续多包才值得），随首日锁版本验证一并定；
2. 首笔提交注明基线（模板版本 + **上游 commit hash 锚点**），LICENSE 保留上游版权行（MIT 义务），README 注明来源；
3. `pnpm install` 全量锁版本 → 跑通 build/lint/vue-tsc = **栈兼容一次性验证**；失败即同机制换 pure-admin-thin 种子重来（损失半天）；
4. 换壳按评估工单执行：删 `apps/docs`、mock、演示视图（Dashboard/Level）与非必要组件；改 `api/login`（真实端点）、`request`（补 401 单飞刷新 + request_id）、user store（token 字段）、Login 页；增 `v-permission` 指令（模板无此件，约 30 行）与 `/user/permissions` 对接；路由/权限核心 `permission.ts` + `routerHelper.ts` 基本免改。

**不 fork 的理由**：P4-W2 起壳层即按 zhuzhao 契约重写，与上游方向立即分叉，合并收益趋零而同步负担永续；且 zhuzhao-ui 仓已存在，fork 无法改名顶替。后续需要上游修复时，对着 hash 锚点在 GitHub 做 diff 按需手工摘取即可。

## 10. 与既有文档的关系

| 文档 | 关系 |
|------|------|
| phase1/07-menu | 动态路由契约 SSOT，本文 §3.2 是其实现规格；`views` 命名从它 |
| phase3/12-frontend | 工单域功能规格 SSOT；其 §4 工程结构 `src/pages/` 与本文冲突，**启动批改 §4 为 views 并以本文 §2 为准**；§3.5 权限三件套由本文 §3.4 实现化 |
| phase4/00 §2.1 | 五拍板=本文的上位决策；前端条目量级（季度级）不变，本文 §8 是其切分 |
| phase2/01（D2-23/B7） | cookie 会话演进方向，本文 TokenStorage 抽象为其预留 |
| phase4/03（场景矩阵） | 用户场景走查与测试覆盖的 SSOT——本文页面的交互约束（禁用态/无 total/危险确认等）以 [03 号](./03-scenarios-and-tests.md) 场景表为准 |

## 11. 复审落档（2026-09-21，四批评审收敛）

> 会话流程/契约细节/内部矛盾/动态路由四批评审 + 计划层复查，全部经代码核验后落位：**正文就地修**（§1/§2/§3/§4/§5/§6/§7/§8），本表只留追溯。波次编号错位（原 C1）已在同日终确批修复，不重录。

| 批 | 发现（核验锚点） | 落点 |
|----|------------------|------|
| A 会话 | 首登强制改密：登录响应带 `must_change_password`（token.go:8）、种子 admin 即 true（000002 F-10）、AT claims 拦截 403+20007（jwt.go:93）、改密即轮换返回新 TokenPair（auth_handler.go:129） | §3.1/§3.3/§6/§8-W2 |
| A 会话 | device_id 三件同源：缺省归 "default" 单槽多浏览器互踢（auth_service.go:23）、登出漏传吊空槽（token.go:39） | §3.3/§6 |
| B 契约 | 刷新失败码族实为 20004/20014/20015 + Redis 抖动 503+10008 不清会话（auth_service.go Refresh）；X-Request-ID 格式 `req-`+32hex（D2-24）；int64 全仓 string 序列化公约；`/user/menus`·`/user/permissions` 包装对象 + User 无角色字段；20006 锁定=429 | §3.3/§2/§4/§6 |
| C 矛盾 | FE4 无 Wave 认领（本 doc §7 + 02 §5.1 同修）；§5 树管理 CRUD 与菜单只读化冲突（02 §2 P4-W1/00 §2.4）；「变更无需重登生效」半成立（rbac reloadPolicy 写后刷内存，导航仍每会话一次） | §7/§5/§8-W3 + 02 §5.1 |
| D 路由 | 父子同 path（/tickets·/tasks）、顶层页 audit_log、目录带组件 home、菜单外静态路由机制；icon 种子实际值（settings/setting 并存）；code≠0 恒非 2xx 按 HTTP 状态分流；dev 依赖 activelist 容器在线（**表述已随 P1-4 修正：本仓栈不含它，跨三仓编排见 02 §2-W2 首日项**） | §3.2/§1/§3.3 |
| 计划层 | 前端部署件（nginx+trusted_proxies+/al location）、穿插池前端联动面、00 §2.4 漏排四行——归 02 号处置（其 §5.5） | 02 号 §2/§3/§5.5 |
| 五批复审 | **role_menus 绑定缺口（后端 P0）**：000018/22/24/25 五页面菜单零绑定 + GetUserMenus INNER JOIN 无 admin 旁路（GetUserPermissions 有 B4-4=不对称，admin「有码无路」）→ 修复归 02 §2 P4-W1（~~000030 同批~~ **000031，十四批勘误对齐 02 §5.5 #7**）；401 分码 20002（过期可刷新）/20003（无效跳登录，B2-1）；al 导出裸 JSON 数组流旁路（activelist data.go 实现拍板）；device_id 白名单 `[a-zA-Z0-9_-]{1,64}`（D2-22）；constantRoutes 常量路由组 | §3.2⑤/§3.3 + 02 §2 P4-W1 |
| 六~十一批 | 计划层为主（B 案六点定案/组织两面拆分/BK-18 整改/catalogExempt/assignee=me 后端承载/compose 口径修正/缓冲算术/部署原子性/死码两引用点/迁移拆分两连号）——处置留痕统一见 **02 §5.5 #10–14**；契约级修正（device_id 三处/home.vue/3PUT+2DELETE 拆分/自服务端点形状/搜索区注记/**button:{permission} 口径勘误**）已就地落 §3/§5/§6 | 02 §5.5 #10–14 + 正文 |
