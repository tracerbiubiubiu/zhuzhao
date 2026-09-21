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
| 构建 | Vite | ✅ 拍板 | dev 经 proxy 转发 `/api` 到本地后端；生产同源，无运行时配置需求 |
| 组件库 | Element Plus（+ `@element-plus/icons-vue`） | ✅ 拍板 | 菜单表 `icon` 列的种子值（`system`/`task`/`al`…）建集中映射表解析 |
| 底座模板 | **vue-element-plus-admin v3.1.0** 种子拷入（备胎 pure-admin-thin） | ✅ 定案（2026-09-21） | 四候选代码级评估，见 §9.1 |
| 路由 | vue-router 4 | ✅ 拍板 | 动态注册模式见 §3.2 |
| 客户端状态 | Pinia | ✅ 拍板 | **只放 session/菜单/UI 态**，见 §4 分界 |
| 服务端状态 | `@tanstack/vue-query` | 📌 本文档新增建议 | 管理端 90% 状态是服务端数据；缓存/重取/失效声明式，避免全量 Pinia 化的脏数据地狱 |
| HTTP | axios | 📌 本文档新增 | 拦截器链成熟（§3.3）；不引二次封装的请求库 |
| 动态表单 | form-create（渲染 `@form-create/element-ui`；设计器 `@form-create/designer`） | ✅ 拍板（12-frontend §3.1 / 00 §2.1） | 仅工单自定义字段用；普通表单走 el-form + rules |
| 时间 | dayjs | 📌 本文档新增 | 后端时间戳格式以 swag 为准，集中一处格式化 |
| 原子 CSS | UnoCSS | ⏸ 暂不启用 | Element Plus 样式体系起步足够；出现高频布局痛点再开（开放问题） |
| 质量门禁 | vue-tsc + ESLint + Prettier / Vitest / Playwright | 📌 本文档新增 | 见 §7；与 Go 门禁分仓各跑 |

**明确不做**：i18n（单语内部系统）、SSR/Nuxt（管理端无 SEO/首屏诉求）、微前端（§2.1 五拍板①）、移动端/响应式（12-frontend §6 既有口径）。

## 2. 目录结构（单仓单 SPA）

```
zhuzhao-ui/
  src/
    api/<域>/<资源>/{index.ts, types.ts}   # 按域分文件；类型与请求同域共存
    views/<域>/<功能>/index.vue            # 命名从 07-menu 契约：动态路由 component 字符串直映射
                                           # ⚠ 12-frontend §4 旧稿为 src/pages/，启动批改为 views 并回写该文档
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

## 3. 应用壳层四件（模板不可替代部分，全部自写）

### 3.1 启动引导与路由守卫（时序）

```
应用启动
 └─ router.beforeEach 守卫（每次导航）：
    1. 白名单页（/login）→ 直接放行
    2. 无 AT → 重定向 /login（带 redirect 回跳参数）
    3. session 未加载（首跳/刷新）→ 并行拉 GET /user/menus + GET /user/permissions
       → 存 Pinia → 按菜单树 addRoute（§3.2）
    4. 注册 404 catch-all（必须在全部 addRoute 之后，仅一次）
    5. next()；目标路由不存在（component 解析失败/无权限）→ 落 404 页
```

要点：菜单/权限码**每会话拉一次**，登出即清 Pinia（不落 localStorage，防登出残留）；「已加载」标志防守卫重入死循环。

### 3.2 动态路由注册（07-menu 契约的落地核心）

- **component 字符串解析**：`import.meta.glob('../views/**/*.vue')` 建 `Map<string, Component>`，菜单节点 `component: "system/user/index"` → `views/system/user/index.vue`。零构建步骤介入；glob 在构建期静态展开，lazy chunk 按页面自然切分。
- **注册范围**：只取 `menu_type IN (1,2)` 且 `visible=true` 的节点（后端 `GET /user/menus` 已过滤，前端不重复判）；目录（type 1）作为父级布局路由，页面（type 2）挂 `component`。
- **404 时序红线**：catch-all `{path: '/:pathMatch(.*)*'}` **只能在动态路由全部 addRoute 完成后注册**——先注册会导致刷新/直达 URL 误跳 404（RuoYi 系最高频死链坑，见 00 号业界核验）。
- **契约纪律**：新页面前端组件、菜单种子迁移、后端路由**同一 PR**（07-menu 推荐流程）；前端不得运行时要求后端加菜单行（菜单词表已拍板只读化，00 §2.4）。

### 3.3 请求层（axios 实例 + 拦截器链）

统一信封（已核 `zhuzhao-utils/response`）：成功 `{code:0, message, data, request_id}`，失败 `{code≠0, message, data:null, request_id}`；分页 `data:{list, total, page, page_size}`；登录返回 `{access_token, refresh_token}`（`internal/model/token.go`）。

| 拦截器 | 行为 |
|--------|------|
| 请求 | 附 `Authorization: Bearer <AT>`；透传/生成 `X-Request-ID` |
| 响应-成功 | `code===0` → 直接返回 `data`（调用方拿到的就是业务数据）；否则进错误分支 |
| 响应-401 | **单飞刷新**：并发 401 只发一次 `POST /auth/refresh`，其余请求挂起等待后用新 AT 重放；刷新失败（RT 无效/重放/改密——后端 20003/20004/20014/20015 家族）→ 清 session → 重定向 /login。防刷新风暴是 RT 轮换机制的前端配合面 |
| 响应-错误 | toast 展示后端 `message`（后端已做文案人性化，**前端不维护码→文案映射**）；`request_id` 一并展示供报障对日志；仅「需特殊行为的码」进白名单表（`common/constants/errorBehavior.ts`，如强制登出类） |

**Token 存储抽象**：`common/request/tokenStorage.ts` 定义 get/set/clear 接口，现态实现 = 内存 + localStorage（AT/RT）；B7 cookie 会话演进（phase2 D2-23 方向）时**只换实现**，调用面零改动。localStorage 方案运行期须配合 XSS 基线（§6）。

### 3.4 权限三件套（12-frontend §3.5 的实现规格）

- `route:{path}` 码：路由级兜底——守卫在 addRoute 后对目标页校验 `route:` 码集合（菜单树本身已过滤，此层防御 URL 手输）；
- `button:{code}` 码：`v-permission` 指令（无权限**移除 DOM**，非 `display:none`——防 DevTools 放显）+ `<AuthButton :capability>` 组件（置灰态，用于「能看不能点」场景）；
- `usePermission(ANY/ALL)` composable：复杂显隐逻辑（多码组合）走函数式，模板里只留指令；
- 码来源：`GET /user/permissions`（admin/superadmin 由后端全量展开，B4-4 语义前端无感知）。

## 4. 状态管理分界（最易做错的一件）

| 类别 | 归属 | 例 |
|------|------|-----|
| 客户端状态 | **Pinia** | session（用户/角色）、菜单树+权限码、布局 UI 态（侧栏折叠） |
| 服务端状态 | **vue-query** | 一切列表/详情/树数据（用户、角色、工单、审计、死信…） |

- **query key 约定**：`[域, 资源, 参数]`（如 `['system','users',{page,keyword}]`），写操作成功后按 key 前缀失效（`invalidateQueries({queryKey:['system','users']})`）；
- **禁止**把接口数据手工搬进 Pinia（历史上这类模式必然产出「改了 A 页 B 页不刷新」的脏数据）；
- 分页参数与后端 `page/page_size` 字段直通（已核 `PageData`），ProTable 封装内做映射，页面层零感知。

## 5. 页面范式与范例页（约定的活文档）

前端启动批**先产三个范例页再批量铺页面**（建议挂在 system 域，页面最典型）：

| 范例 | 承载约定 | 规格 |
|------|----------|------|
| 列表页（用户管理） | ProTable 封装 | 搜索区（el-form 内联）+ 表格 + 分页 + 操作列权限槽位（`v-permission`）；加载/空/错误三态由 vue-query 统一供给（loading=skeleton、error=重试卡片）；写操作（新建/编辑/删改确认）走页面内弹窗 |
| 表单页（角色编辑） | el-form + rules | async-validator 规则集中定义；服务端字段错误回填到对应表单项；提交防重（按钮 loading） |
| 树管理页（菜单/组织） | el-tree + 表单联动 | 左树右表布局；节点 CRUD 带子节点/占用预检提示（对接后端 ErrMenuHasChildren 等语义） |

ProTable 是页面一致性的最大杠杆：**所有列表页禁止手搓 el-table + 分页拼装**，一律经封装（封装内统一处理 `PageData` 解包、排序参数、列权限、操作列宽度）。自建封装的参考实现 = Geeker-Admin 的 ProTable（MIT，组件 + useTable 约 1100 行，自治低耦合）——**只借鉴设计，代码按 `PageData` 契约自写**，不引依赖。工单域动态表单走 form-create 渲染器（12-frontend §3.1），不进 ProTable 范畴。

## 6. 会话与安全

- **登录**：`POST /auth/login`（`employee_no`+`password`，已核 acceptance 脚本调用形态）；TokenPair 存 TokenStorage（§3.3）；
- **登出**：`POST /auth/logout`（自服务白名单端点）→ 清 Pinia + TokenStorage + vue-query 缓存（`queryClient.clear()`）→ /login；
- **XSS 基线**：全站禁 `v-html`（例外须评审并 sanitize）；AT/RT 存 localStorage 的风险与 B7 cookie 演进绑定，TokenStorage 抽象保证一步迁移；
- **错误边界**：`app.config.errorHandler` 全局兜底（上报 console + 友好页）；路由级 403/404 独立页面；
- **接口粒度不设防前端**：前端显隐只是体验层，真实鉴权在后端三层（L1 Casbin 为准）——前端权限码仅用于渲染决策，不得视为安全边界（与「按钮码不走 Casbin」的既有口径一致，07-menu）。

## 7. 工程化与门禁（「前端验收口径待定」的建议答案）

| 环节 | 工具 | 门禁口径 |
|------|------|----------|
| 静态检查 | `vue-tsc --noEmit` + ESLint + Prettier | 等价 Go 侧 `make lint`，CI 必跑 |
| 单元测试 | Vitest | 盖 composables（usePermission/request 单飞刷新/tokenStorage）与纯函数（动态路由解析/菜单树转换）；组件测试只盖 AuthButton 等关键件 |
| E2E | Playwright | 冒烟集：登录→菜单渲染→列表页翻页→按钮权限（对应 12-frontend §5 FE3）；依赖 docker compose 起后端栈 |
| 验收基线 | 12-frontend §5 FE1–FE4 | FE1 动态表单 / FE2 管理页无 SQL 全流程 / FE3 viewer 双级不可见 / FE4 审批链路 |
| 构建 | `pnpm build` → dist | 产物交付=nginx 静态容器 + 反代 API（00 §2.1 五拍板⑤推荐形态）；CI 分仓：zhuzhao-ui 自己的流水线跑前端门禁，Go 四仓门禁不掺和 |

## 8. 里程碑切分（建议，量级对齐 00 号「季度级」标定）

| Wave | 内容 | 出口标准 |
|------|------|----------|
| W1 壳层 | 底座模板裁剪 + 壳层四件自写 + 登录/登出 + 菜单树渲染 + 动态路由 + 三范例页 | 范例页过 FE3（viewer 双级不可见）；E2E 冒烟绿 |
| W2 system 域 | 用户/角色（含 AssignMenus 勾选树）/菜单/组织四组页面 | 管理全流程无 SQL（FE2 同口径）；菜单/角色变更后无需重新登录即可生效（每请求读库语义） |
| W3 ticket 域 | 工单发起（模板卡片+form-create 动态表单）/列表/详情/评论，类型/字段/模板管理三件套（12-frontend §2/§3.2 规格） | FE1 + FE2 |
| W4 al/task/audit 域 | 名单页（走 `/al/api/v1` 反代）、任务管理/死信、审计日志查询 | 各域页面经对应权限码可见性验证；项目内页面清单与菜单种子对账一致 |

## 9. 开放问题（启动批拍板）

1. ~~底座模板最终选型~~ **✅ 已定案（2026-09-21 四候选代码级评估），全文见下节 §9.1**；
2. UnoCSS 启用与否（建议默认不启用，见 §1）；
3. 多标签页登出/改权同步：BroadcastChannel 监听登出事件强制各标签失效（低成本，建议 W1 顺带）；401 被动失效已由请求层覆盖；
4. 暗色主题（Element Plus CSS vars 原生支持，随手项）；
5. Playwright 冒烟的 compose 环境依赖口径（复用标准环境还是独立 stub，随 FE 验收口径一起定）。

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

1. `npx degit kailong321200875/vue-element-plus-admin` 种子拷入 zhuzhao-ui 工作区（degit 不带上游历史，仓史从 zhuzhao-ui 自己首笔提交开始）；
2. 首笔提交注明基线（模板版本 + **上游 commit hash 锚点**），LICENSE 保留上游版权行（MIT 义务），README 注明来源；
3. `pnpm install` 全量锁版本 → 跑通 build/lint/vue-tsc = **栈兼容一次性验证**；失败即同机制换 pure-admin-thin 种子重来（损失半天）；
4. 换壳按评估工单执行：删 `apps/docs`、mock、演示视图（Dashboard/Level）与非必要组件；改 `api/login`（真实端点）、`request`（补 401 单飞刷新 + request_id）、user store（token 字段）、Login 页；增 `v-permission` 指令（模板无此件，约 30 行）与 `/user/permissions` 对接；路由/权限核心 `permission.ts` + `routerHelper.ts` 基本免改。

**不 fork 的理由**：W2 起壳层即按 zhuzhao 契约重写，与上游方向立即分叉，合并收益趋零而同步负担永续；且 zhuzhao-ui 仓已存在，fork 无法改名顶替。后续需要上游修复时，对着 hash 锚点在 GitHub 做 diff 按需手工摘取即可。

## 10. 与既有文档的关系

| 文档 | 关系 |
|------|------|
| phase1/07-menu | 动态路由契约 SSOT，本文 §3.2 是其实现规格；`views` 命名从它 |
| phase3/12-frontend | 工单域功能规格 SSOT；其 §4 工程结构 `src/pages/` 与本文冲突，**启动批改 §4 为 views 并以本文 §2 为准**；§3.5 权限三件套由本文 §3.4 实现化 |
| phase4/00 §2.1 | 五拍板=本文的上位决策；前端条目量级（季度级）不变，本文 §8 是其切分 |
| phase2/01（D2-23/B7） | cookie 会话演进方向，本文 TokenStorage 抽象为其预留 |
