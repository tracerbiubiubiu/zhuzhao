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

```sh
make lint
make test-unit
make test-integration
make test-cover
make acceptance
```

集成测试需要 Docker 可创建 PostgreSQL 等夹具；acceptance 是四档 API 与迁移往返门禁，必须按 [验收隔离规则](../ops/runbook.md#71-验收连接与临时文件隔离2026-11-04) 准备专属环境与实际连接，不能指向共享开发栈后并发修改角色。新迁移还需 `make migrate-up`；生成相关变更核生成物新鲜度。

通过后记录精确候选、SDK/依赖、运行产物及结果。失败或未运行如实登记，不用旧报告替当前证明。提交按显式路径，推送另遵循窗口核对与单一执行者纪律。
