.PHONY: build run dev tidy wire tools-install migrate-up migrate-down docker-dev-up docker-dev-down docker-dev-reset docker-up docker-down swag lint commit-lint hooks test test-unit test-integration test-cover benchmark acceptance acceptance-2a acceptance-2b acceptance-2c snapshot snapshot-diff guard test-dev-stack

APP_NAME=zhuzhao
BUILD_DIR=bin
# 生成工具与业务依赖隔离；默认缓存已被 Git/Docker 忽略。GO 可显式选择已验证 SDK。
GO ?= go
GO_TOOL_CACHE ?= $(CURDIR)/.codex/tools
GO_TOOL = GO="$(GO)" GO_TOOL_CACHE="$(GO_TOOL_CACHE)" python3 scripts/go-tool.py

tools-install:
	$(GO_TOOL) install wire swag

build:
	go build -o $(BUILD_DIR)/$(APP_NAME) ./cmd/server

run: build
	./$(BUILD_DIR)/$(APP_NAME)

# 已有监听时拒启并显示归属；换APP_PORT或由所有者处理原进程。
# 2026-10-14：taskrunner 三件套补 dev 默认——并行会话/裸 make dev 启动不再丢
# 任务代理（此前无 TASKRUNNER_* env 的重启会让任务中心 502）；显式 env 仍优先
TASKRUNNER_BASE_URL ?= http://127.0.0.1:18080
TASKRUNNER_SK ?= dev-tr-e2e-sk
INTERNAL_JOBS_SK ?= dev-tr-e2e-sk
# 并行实例先查docs/ops/port-allocation.md与runbook §7：33334现归test，空闲端口可显式APP_PORT覆盖。
APP_PORT ?= 33333
export APP_PORT
# /al 上游 env 覆盖（2026-11-02 双拓扑治理）：宿主 dev 默认回环（activelist 宿主直跑），
# compose/其余拓扑 make dev GATEWAY_AL_TARGET=http://activelist:18081 覆盖
GATEWAY_AL_TARGET ?= http://127.0.0.1:18081
export GATEWAY_AL_TARGET
dev:
	@if command -v lsof >/dev/null 2>&1; then \
		listeners=$$(lsof -nP -iTCP:"$(APP_PORT)" -sTCP:LISTEN 2>/dev/null || true); \
		if [ -n "$$listeners" ]; then \
			printf 'ERROR: APP_PORT=%s 已被监听，请先核进程归属或换APP_PORT；未停止原进程。\n%s\n' "$(APP_PORT)" "$$listeners" >&2; exit 1; \
		fi; \
	fi
	@set -a; [ -f ../taskrunner/.env ] && . ../taskrunner/.env; set +a; \
	INTERNAL_JOBS_SK=$${TASKRUNNER_SELF_SK:-dev-internal-jobs-sk} \
	TASKRUNNER_SK=$${TASKRUNNER_CALLER_ZHUZHAO_SK:-dev-taskrunner-sk} \
	TASKRUNNER_BASE_URL=$${TASKRUNNER_BASE_URL:-http://127.0.0.1:18080} \
	GATEWAY_AL_TARGET=$(GATEWAY_AL_TARGET) go run ./cmd/server

tidy:
	go mod tidy

# W0b（2026-09-25）wire 已转正：手码装配升 provider（provideGateway/provideRateLimitConfig/
# provideJobsCallbackService）+BK-22 对账移 NewApp——make wire 可安全再生。
wire:
	$(GO_TOOL) wire ./internal/app/

lint:
	go vet ./...
	python3 scripts/check-gofmt.py
	@# swagger 新鲜度（红窗第三/四例同族收口 2026-10-22——struct/注释改动必随批
	@# make swag，CI 新鲜度守卫前移到本地）：重生成后 docs/ 有 diff 即失败；
	@# 失败时工作树已含重生成产物，review 后随批提交即可。
	@$(MAKE) --no-print-directory swag >/dev/null 2>&1 || (echo "ERROR: make swag failed" && exit 1)
	@git diff --quiet -- docs/docs.go docs/swagger.json docs/swagger.yaml \
		|| (echo "ERROR: swagger drift detected — run 'make swag' and commit regenerated docs/（与 CI 新鲜度守卫同款）" && exit 1)

# §12-9 发现链第 1 道：push 决策点必跑（默认待推送窗口；make commit-lint RANGE=HEAD~20 扫指定范围）
commit-lint:
	scripts/commit-lint.sh $(RANGE)

hooks:
	git config core.hooksPath .githooks
	@echo "✅ 并行提交守卫已装（pre-commit 在途可见+宽面提示 / commit-msg 跨主题 amend 拦截——standards §12-10；新环境开工第一步）"

# EC2：DSN 外置（可用环境变量或命令行覆盖：make migrate-up MIGRATE_DSN=...）
MIGRATE_DSN ?= postgres://zhuzhao:zhuzhao_dev@localhost:5432/zhuzhao?sslmode=disable

migrate-up:
	migrate -path migrations -database "$(MIGRATE_DSN)" up

migrate-down:
	migrate -path migrations -database "$(MIGRATE_DSN)" down

# 本地开发：显式project保留既有开发卷，仅起PG/Redis。
ZHUZHAO_DEV_PROJECT ?= zhuzhao-dev
docker-dev-up:
	docker compose -p "$(ZHUZHAO_DEV_PROJECT)" -f deployments/docker-compose.dev.yaml up -d --wait postgres redis

docker-dev-down:
	docker compose -p "$(ZHUZHAO_DEV_PROJECT)" -f deployments/docker-compose.dev.yaml down

# 清空 PG/Redis 数据卷并重建（会删除所有本地 dev 数据，随后自动 migrate-up）
docker-dev-reset:
	docker compose -p "$(ZHUZHAO_DEV_PROJECT)" -f deployments/docker-compose.dev.yaml down -v
	docker compose -p "$(ZHUZHAO_DEV_PROJECT)" -f deployments/docker-compose.dev.yaml up -d --wait postgres redis
	@echo "Waiting for postgres..."
	@sleep 5
	$(MAKE) migrate-up

# 完整部署：应用 + PG + Redis
docker-up:
	cd deployments && docker-compose -f docker-compose.yaml up -d --build

docker-down:
	cd deployments && docker-compose -f docker-compose.yaml down

swag:
	$(GO_TOOL) swag init -g cmd/server/main.go -o docs --parseDependency --parseInternal

test: test-unit

test-unit:
	go test -v -race -count=1 ./internal/...

test-integration:
	go test -v -race -count=1 -p 1 -tags=integration ./internal/...

test-cover:
	go test -race -coverprofile=coverage.out ./internal/...
	go tool cover -html=coverage.out -o coverage.html

benchmark:
	go test -bench=. -benchmem ./internal/...

# 验收脚本（F-24/F-25：00 §5.5 本地回归纪律入口；容器名由脚本自动探测）
# 四档链式全量门禁（经 2c 脚本链式：phase1 → 2a → 2b → 2c，对齐 AGENTS.md 门禁定义）
acceptance:
	bash scripts/migration-roundtrip-check.sh --check
	bash scripts/acceptance-phase2c.sh
	bash scripts/migration-roundtrip-check.sh

acceptance-2a:
	bash scripts/acceptance-phase2a.sh

acceptance-2b:
	bash scripts/acceptance-phase2b.sh

acceptance-2c:
	bash scripts/acceptance-phase2c.sh

# 项目现状快照：一条命令看清全貌（规模/分层/表/路由/权限/测试/技术债/文档一致性）
snapshot:
	bash scripts/snapshot.sh

# 只显示与上次快照的差异（AI 迭代后先看这个，30 秒掌握“改了什么”）
snapshot-diff:
	bash scripts/snapshot.sh --diff

# 迁移往返数据 diff（依赖开发 PG 容器；28 批 SYS-3：挂入 acceptance 链尾强制执行）
guard-roundtrip:
	bash scripts/migration-roundtrip-check.sh

# 架构守护：分层依赖 / 命名 / 技术债门禁（防止 AI 迭代悄悄破坏架构）
guard:
	go test -count=1 -run 'TestArchitecture|TestGuard' ./internal/...

# 离线黑盒：全部外部命令为桩，不访问Docker/共享进程。
test-dev-stack:
	python3 scripts/test-dev-stack.py
