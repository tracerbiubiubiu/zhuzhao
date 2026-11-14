# 真实前端首组基线与近期排期校准

日期：2026-11-12。性质：FD07首组真实页面观察，**不是全站重设计完成或验收**。未改Vue、接口、依赖、数据库迁移或产品配置。当前UI源码固定为`1fc697d5043358797302037e27ff26c3d9545cb6`；主仓为`061c56af`加本轮冻结的规划文档。源码运行前后逐文件一致，三后端ELF与容器内副本逐hash一致。

## 实际覆盖与证据质量

最终attempt-004有24张合格截图：登录、管理员首页、工单列表、情报空态、分派规则、普通无home菜单用户点击Logo后落点，分别覆盖亮/暗以及1366×768、1920×1080。它们是六种代表状态，不是六种角色，也不是27页全量覆盖。逐张验证最终路由和目标DOM标记；正常业务页必须实际命中，普通Logo案例则记录其错误落点。

实际流量经过私有zhuzhao/AL/TR/PG/Redis/RustFS，未stub网络响应；数据与账号全合成，仅使用专属管理员/普通用户，不登录或修改E000001，种子号指纹保持。主题经产品`vea-app-v2.isDark`持久化流程初始化，逐图核Pinia状态、html class及实际颜色，未强塞CSS模拟主题。

这是当前Vite开发模式和本机缓存Chromium的观察。UI源码、package/lock及实际版本有绑定；node_modules复用已有安装目录，未证明全依赖逐字节冻结。字体加载完成后取图，但computed字体栈不等于识别了每个汉字的最终fallback字形。不能把它描述成已部署生产页面、干净安装复现或全客户端认证。

本地原始收据在`.codex/audits/next-schedule-20261112/frontend-baseline/`：`baseline-summary.json`给24图hash/角色/主题/视口/尺寸，`attempt-004/visual-result.json`给DOM/请求/console，`console-receipts.json`及`representative-contrast.json`给错误和色对。原始图/日志不入Git；仓内正文给可审结论，异机须重建同类合成环境，不能假定本地文件随仓可得。

## 实际发现与承接

| 发现 | 证据和边界 | 当前安排 |
|---|---|---|
| 普通无home菜单用户落404 | 该用户可打开工单列表，登录默认`/home`和点击Logo均显示页面不存在；亮暗/两视口均复现 | U01/P6.0安全入口，保留无权限用户的受权落点，不为修导航赠工单读取权 |
| 亮色弱文案/表头对比不足 | 规则说明12px/16px行高，`#9ca3af`对白底约2.5388:1；代表表头14px、600字重，`#7a879c`对白底约3.6375:1 | U02统一语义tokens，U07验代表色对及状态；不是全站颜色结论 |
| **BL-FE-003规则查询初始化错误** | 四个规则页记录各一次caught console ReferenceError，表格仍显示1条；迟到数据/刷新重排影响未验 | U02/P6.1开工优先修复，U06迁移前回归；U02从3–5复估4–6日，增加1日用于有界修复/异步返回回归 |
| 主操作与长技术说明不一致 | 工单“新建工单”在表格左侧，规则“新建规则”在卡头右侧；规则说明直接暴露priority/keyword/target_user/target_org | U02共用操作/帮助合同，U03和U06按页迁移；详细技术解释进入帮助，不挤占默认卡头 |
| 字体声明与壳层密度需统一 | 代表外壳/说明/表头computed栈为Times，EP控件为Arial；登录有独立显式栈。1366样本Logo/顶栏高60px，规则卡从y=122起高596px | U02明确字体/字号/行高和壳层；U06渐进消费已确认的单侧栏+身份顶栏。实际中文fallback未识别，不据此断言汉字一定混用字体 |

色对用DOM前景和最近不透明底色计算，排除图片、部分alpha等不能直接判断的项；没有取截图抗锯齿像素。相同代表说明/表头暗色约6.0449/5.9855:1，不意味着整个暗色模式合格。普通文本4.5:1、大字3:1的参考及例外见[W3C说明](https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html)；这里的12/14px代表项按普通文本评价，未做全站WCAG符合性声明。

## BL-FE-003的实际代码与验证出口

[rules/index.vue](../../../../zhuzhao-ui/src/views/ticket/rules/index.vue)的setup先执行`useTableFit(ruleTable, () => rulesQuery.data.value)`，之后才声明`rulesQuery`；[useTableFit.ts](../../../../zhuzhao-ui/src/common/hooks/src/useTableFit.ts)的`useTableFit`同步注册`watch(watchSource, scheduleRelayout)`，首次求getter进入变量暂时性死区。

实际日志为`[errorHandler] watcher getter: ReferenceError: Cannot access 'rulesQuery' before initialization`。原始JSON Pointer为`/consoles/11`、`/31`、`/51`、`/71`，分别对应四个规则页。这里只证明初始化错误和当前仍能显示一条规则，不虚构数据损坏或整页崩溃；后续异步数据watch是否漏重排应由修复回归验证。

修复先保证依赖初始化后才注册观察，保持现有表格/查询职责；不借机重写全站hook。验收覆盖冷加载、真实异步返回/刷新、表实例/卸载与无console error；不只用源码行顺序检查代替运行行为。该项已新增到来源总账，尚未修复，不以本报告销账。

## 首次失败、错误与清理

- attempt-001：合成长标题216字符超过真实接口200限制，合法返回400；将夹具限定180，没有拍图，不是产品缺陷。
- attempt-002：仓外临时`.ts`配置被按CJS加载原ESM配置，Vite未就绪；改ignored wrapper为`.mjs`，没有改UI配置。
- attempt-003：拍出24图但情报误用了组件目录式路径`/intel/query`，四图落404。整组不作为最终合格基线；保留原图，并从真实菜单`intel_query.path=/intel`取路由、补目标标记检查后重取。
- attempt-004：24个目标标记/路由通过，104个API响应无HTTP/业务码错误，无uncaught pageerror或request failure。**仍有4条caught console error**，不能称前端零错误。76条warning分为44条router next弃用和32条el-link underline布尔API弃用；未解析组件警告为0，弃用债随G07A/U06核兼容。

四轮私有容器/卷/网络及Vite/浏览器进程全部清理，UI端口可重新绑定；原六共享容器完整指纹未变，五仓受控源与种子号指纹均未变。先前主仓七项目标通过的证据单独保留，它不替代本组视觉/console观察，也不说明这4条错误已修复。

## 剩余范围与排期含义

首组让U01导航、U02 tokens/初始化和U07证据要求有实际依据，FD07仍为部分完成。情报仅看初始空态，未查真实Provider、历史tab内容/回填重查/结果缓存与失败。未注入错误、未做键盘焦点/缩放/屏幕阅读器/移动端、未遍历全部角色/类型/27路由或真实大数据量。

U05完整历史旅程继续按确认方案验，U03–U06其余页面在改造前逐组补基线。U06的4–7日仍待更广范围复估；不能以24图覆盖到新页面、所有交互或全站预算。新版[执行排期](../execution-schedule.md)吸收本轮发现，T0/投入仍未绑定，没有以本次走查批准新UI或业务实现。
