# 开发指南

## 1. 先确认工具与数据归属

Go 版本下限以根 `go.mod` 为准。当前编译基线为 Go 1.27.4；本机系统 Go 可能较旧，运行命令前明确选择 SDK。生成器另外需要 Python 3，固定工具安装到 ignored `.codex/tools/`，不写入业务依赖。

```sh
make hooks
go version
make tools-install
```

已有服务先按 [runbook §7](../ops/runbook.md) 查占用与会话台账。**2026-11-13所有者确认：本项目现有开发栈和数据卷均为可丢弃测试环境，可重建，不要求为这些测试数据先做保留/迁移。**另开 HTTP 端口仍不等于数据隔离；重建时核准项目、Compose、卷和在途任务，避免干扰其他项目或仍在执行的测试。该授权不扩展到生产数据、私人文件或无关Docker资源。

## 2. 准备依赖与显式迁移

以下命令从**主仓根目录**执行，用于已确认归属的本地开发栈：

```sh
make docker-dev-up
make migrate-up
make dev
```

预期是 PG/Redis 健康、迁移成功且非 dirty，然后应用监听默认 33333。端口占用时 `make dev` 拒启并显示归属。它不自动启动 UI、activelist 或 taskrunner；四进程联调步骤、密钥对值和 Compose 区别见 [部署指南](../ops/deployment.md)。

需要从空测试数据开始时可运行`make docker-dev-reset`：删除当前主仓开发Compose声明的PG/Redis卷并重新迁移；它不等于清理AL、TR、对象存储或旧版本遗留的所有卷。完整重建应先按各仓配置列清项目资源，显式逐项处理，不执行全局Docker prune。此处说明授权和范围，本轮README更新不需要先删除现有栈。

直连探针为 `/health/live` 和 `/health/ready`。ready 只证明应用就绪条件，不证明所有业务与上游链路通过。UI 开发使用其仓 `pnpm dev`，保持规定 mode。多账号测试用 runbook §4 专属账号，E000001 不用于人工改密实验。

## 3. 修改与生成

先读 [工程公约](../standards.md) 及对应模块文档。迁移须 up/down 成对，编号查 [迁移地图](../review/11-project-control.md)；权限、配置与接口变化同步控制台。

```sh
make wire
make swag
make guard
```

Wire / Swagger 使用固定版本工具，覆盖路径仍会核模块、版本及校验和。Swagger 三份生成物留在 `docs/` 根，供 Go 导入和 UI codegen 使用。

## 4. 交付前验证

先执行 `make quality-plan`（可用 `BASE=<提交或引用>` 指定基线），审阅完整 `base..HEAD` 与暂存/工作树/未跟踪候选，再执行 `make quality-check`。三档均保留差异与文档链接检查，提交规范另由钩子和独立CI检查：

- `document`：说明、设计、报告等受准入规则约束的 Markdown/静态图片，含不改变执行义务且无运行消费者的提交元数据格式说明；无需 Go、PG、Redis、acceptance 或业务 E2E。
- `reference`：登记的规划 JSON/生成工具、候选设计 Schema、隔离示例或原型；另跑对应语义、样例、独立 SDK 或离线浏览器检查，不宣称产品实现通过。
- `ordinary`：代码、运行配置、依赖、迁移、正式 API、产品测试、CI/检查器/豁免和具有执行效力的协作规则，以及未知文件。混合取最高档，保留其中的参考检查；代码继续测试先行。

分类权威是 `scripts/quality-gate.py` 的明确注册规则。不是所有 `docs/**` 或 `.md` 都免业务验证：运行时/测试读取的文档、生成 Swagger、`docs/docs.go`、规则文件、可执行或符号链接均不能伪装成普通说明。改名同时检查旧路径。CI 检查事件基线到实际 checkout 的全部差异，本地还收集在途文件，不修改索引。

提交元数据的格式、工具名称和示例只维护在[贡献指南](../contributing.md#提交元数据与ai署名)，受保护原则归[工程公约§12-9](../standards.md#12-工程与协作纪律)。单独说明修改保留提交规范、差异/文档链接检查及人工语义复核；与控制台混改保留规划recipe。说明页无权改变测试、授权、职责、准入或豁免，内容越界时必须同步受保护权威并走普通档。现有分类器不从任意文案推断规则效力；`AGENTS.md`、`standards.md`及开发/测试指南等受保护文件仍按原GOVERNANCE规则处理。本次职责拆分首次按普通档验证，后续独立说明修改才按实际候选取档。

消费识别是保守的静态辅助，不是完整依赖分析。新增生产/测试读取文档的路径，尤其动态拼接或生产脚本读取，必须同步登记 `_consumed_paths` 的受控集合并评审；登记规则变化本身走普通档。源码字面量、相对URL及`go:embed`覆盖常见情况，不能据此假定任意动态消费都能自动发现。

`quality-check`执行轻量和针对检查；遇普通档会明确退出并要求在独占环境运行以下原有命令，避免自动连接默认共享栈。该退出不能记为完整验收通过。原命令保留；集成环境就绪后运行并记录准确结果：

```sh
make lint
make test-unit
make test-integration
make test-cover
make acceptance
```

集成测试需要 Docker 可创建 PostgreSQL 等夹具；acceptance 是四档 API 与迁移往返门禁，必须按 [验收隔离规则](../ops/runbook.md#71-验收连接与临时文件隔离2026-11-04) 准备专属环境与实际连接，不能指向共享开发栈后并发修改角色。新迁移还需 `make migrate-up`；生成相关变更核生成物新鲜度。

通过后记录精确候选、SDK/依赖、运行产物及结果。失败或未运行如实登记，不用旧报告替当前证明。提交按显式路径，推送另遵循窗口核对与单一执行者纪律。

CI 始终创建 `quality-gate`，以 `always()` 汇总分类、提交规则、文档、参考及所有原产品 job 的实际结果；应跑未跑、结果缺失、取消或失败均拒绝。无关的 helper 镜像只在真实 helper/锁/门禁输入变化时重建，离线 MCP 示例的独立 `go.mod` 不触发产品/helper 构建。远端 required checks 属另行 G02 配置，不从 workflow 文件推断已经启用。

针对检查工具在 `scripts/reference-tools/` 独立锁定 Ajv（沿用既有 schema 验证器）及 Playwright（离线原型实际交互）；不加入 Go 产品依赖。需要时 `npm ci --ignore-scripts --prefix scripts/reference-tools`，设置 `AJV_MODULE`、`QUALITY_PLAYWRIGHT_MODULE` 为相应模块绝对路径；原型另安装该版本 Chromium。仅规划数据检查无需安装 Node/浏览器。未知或缺工具不得降档跳过。
