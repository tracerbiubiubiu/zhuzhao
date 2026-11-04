# 生态全面审计 B03：结果补档与测试环境重建（2026-11-04）

## 范围与授权

所有者本轮询问审计结果记录、实现是否被修改，并授权“清理已有环境，重新拉起一套最新的环境”。本批补齐 B01/B02 正式报告，重建范围限定为 Compose 项目 `zhuzhao_test` 的旧容器、专属数据卷及网络。独立 dev/其他项目容器和宿主服务保留。该授权不包含修复受审实现、提交或推送。

批号 `ECO-20261104-B03`，属于阶段 0 环境准备，不计作其他模块审计完成。B01/B02 的历史观测保留原时点事实，不因环境更新而覆盖。

## 基线与产物来源

五仓提交与 B01/B02 相同，完整 hash、源码归档 SHA-256 和隔离源码路径见 [sources.json](evidence/ECO-20261104-b03/sources.json)。镜像及运行容器分别标记完整 revision：

- app：zhuzhao `5c03581d5999139a3e8e5ead5972c026868aa181`。
- taskrunner：`1dcc83185f24b8dc2bed7bba6786272a5fdfb19f`。
- activelist：`af0b350da650517df2c9d15cca4892cdcd51be88`。
- ui：`b20594cc744a61e2bcb41535b0ea668a711aa4df`。
- utils 仓 HEAD 为 `5a3900fa...`，三个 Go 服务实际消费仍为其声明的 `v0.4.2`；新部署二进制 buildinfo 已逐个确认。

镜像 ID 与标签见 [新镜像来源](evidence/ECO-20261104-b03/new-image-provenance.json)，实际运行容器见 [重建后清单](evidence/ECO-20261104-b03/post-rebuild-inventory.json)。运行中三个 Go 二进制另存 SHA-256；UI 容器内 259 个本次构建产物逐件与构建输出匹配，零差异，见 [部署产物校验](evidence/ECO-20261104-b03/deployed-artifact-verification.json)。

## 实际执行与构建差异

先准备四个应用镜像并核对提交标签，再删除旧 test 栈，避免构建失败时留下部分不可用环境。第一次原样构建遇到 pnpm 下载连接超时，以及 taskrunner Dockerfile 外置 syntax frontend 的镜像认证失败；日志保留，不记成产品功能缺陷。

本批采用以下环境绕行，均未改受审仓文件：

1. 三个 Go 镜像通过本机已配置的系统 HTTP 代理构建。taskrunner 临时构建配方只移除第一行外置 `# syntax=docker/dockerfile:1` 下载声明，使用 Docker 内置 frontend，其余构建指令不变；[配方 diff](evidence/ECO-20261104-b03/taskrunner-build-recipe.diff)保留。
2. UI 的原 pnpm 12.3.4 原生安装器持续停滞，原 Docker 全流程未通过。改在固定 UI 源码快照中用宿主 Node **22.14.0**、已安装的 Vite **8.3.4** 构建；已核对安装锁与仓库锁的 `lockfileVersion/importers/packages/snapshots/settings/overrides` 语义全部一致，未安装或升级依赖、未改锁文件。构建 exit 0，产物按源 Dockerfile 的 nginx 1.27-alpine、模板、环境默认值及静态目录布局封装。本次运行环境不能用来声称原 Docker Node 20 构建路径通过。构建保留大 chunk 与无效动态导入警告，未在本批定级为产品缺陷。
3. 原 Compose 指定的 migrate v4.17.0 镜像拉取缓慢，镜像源替代也未成功。停止该拉取，使用本机已有 migrate；CLI `-version` 显示 dev，但 `go version -m` 实核其模块为 **v4.19.1**。为此仅在环境覆盖中将 test 主 PG 发布到 Docker 动态分配的回环端口 **127.0.0.1:55509**，执行固定快照同一组 SQL。迁移工具版本与原 Compose 的差异如实保留，不将此视为 v4.17.0 容器迁移路径验收。

原样失败、代理重试及 UI 构建日志均见本批 [证据清单](evidence/ECO-20261104-b03/manifest.json)。这些是本机环境可用性的处置，不是提交给产品的实现修复。

## 清理、启动与验证

清理前核实：旧测试栈 8 个容器，另有 7 个独立容器须保留，测试数据卷与其他项目交叉共享数为 0。[清理前清单](evidence/ECO-20261104-b03/pre-rebuild-inventory.json)包含旧容器 ID、卷和归属。

依次执行：

1. 对 `zhuzhao_test` 执行 `down --volumes`，删除旧测试容器和专属卷。
2. 启动全新 PG×3、Redis，等待依赖健康。
3. 使用上述 host migrate 执行全部 up SQL，成功到 81。
4. 启动四个固定镜像应用，主应用 `/health/ready` 返回 200。
5. 在独立 UI 源码副本运行原有 `e2e/global-setup.ts`，API_BASE 显式钉到 `http://127.0.0.1:48081`；admin 收敛为台账终态，operator/viewer 预设与 S1 账号按原脚本建立。E000001 仅通过项目允许的 globalSetup 闭环改密，未在 dev 环境测试该账号。
6. 运行一个独立 API/网关冒烟用例，exit 0，1 passed，retries=0。配置没有 dev webServer 或浏览器启动。

冒烟实际结果：

- profile、menus、permissions、ticket-types：HTTP 200 / code 0。
- `/api/v1/runs`：HTTP 200 / code 0，taskrunner 代理链通。
- `/al/api/v1/admin/types`：HTTP 200 / code 0，activelist 代理链通。
- 匿名 profile：401；viewer 创建用户请求：403。
- SPA HTML 与其引用的脚本资源：200。

[冒烟日志](evidence/ECO-20261104-b03/environment-smoke.log)、[命令与结果](evidence/ECO-20261104-b03/environment-smoke-result.json)、[精确独立用例](evidence/ECO-20261104-b03/environment.spec.ts.txt)及 [配置](evidence/ECO-20261104-b03/audit.playwright.config.ts.txt)均已保存。没有运行全量浏览器 E2E 或 make acceptance。

收尾实核 `schema_migrations=81|f`（version 81、dirty=false）；新测试栈 8 容器运行，旧测试容器 ID 已全部替换。其他 7 个容器的 ID 与 StartedAt 全部未变，即未被本批重启。详见 [重建后清单](evidence/ECO-20261104-b03/post-rebuild-inventory.json)。

## 当前入口、限制与接续

测试 UI：<http://127.0.0.1:48081>；app 直连：<http://127.0.0.1:33334>。配置继续保留测试栈默认 `storage.enabled=false`，S5 上传/下载及完整浏览器体验尚不能据本批验证通过；完整多组织角色矩阵仍待准备。

环境源码/覆盖/镜像记录保留在 `/Users/bujibuji/.codex/tmp/zhuzhao-audit/ECO-20261104-environment`。本次实际使用该目录中固定快照的 Compose 文件和 `compose.audit.json`，不是可随源仓工作区漂移的构建。查询当前状态：

```sh
audit_stack=/Users/bujibuji/.codex/tmp/zhuzhao-audit/ECO-20261104-environment
docker compose -p zhuzhao_test \
  -f "$audit_stack/sources/zhuzhao/deployments/docker-compose.test.yaml" \
  -f "$audit_stack/compose.audit.json" ps
```

日常重启现有镜像使用 `up -d --no-build`；全新卷仍须先执行迁移，不依赖 app 与 migrate 的并发启动。保留上述环境目录及镜像，避免挂载配置失效；重建新版需另钉提交并更新产物来源。

**B02 的 P0 仍为待修**：本批没有实现修复，环境冒烟绿不代表撤权问题关闭，也不代表系统没有其他错误。整体审计仍处于阶段 0/1 与阶段 2 的认证定向初核。

源码工作树核对见 [完整性回执](evidence/ECO-20261104-b03/source-checkout-integrity.json)：五仓受审实现零 diff，本会话更改仅为审计文档/证据。收尾另见 UI `.gitignore` 新增 `.VSCodeCounter` 在途修改，归属未确认，已保留并排除于固定提交构建，不宣称此刻所有兄弟仓均干净；见 [收尾状态](evidence/ECO-20261104-b03/final-worktree-state.json)。B01/B02 正式报告本批已补档，证据仍为本地材料；未 commit/push。业务健康台账继续由所有者承接。
