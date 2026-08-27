.PHONY: build run dev tidy wire migrate-up migrate-down docker-dev-up docker-dev-down docker-dev-reset docker-up docker-down swag lint test test-unit test-integration test-cover benchmark

APP_NAME=zhuzhao
BUILD_DIR=bin

build:
	go build -o $(BUILD_DIR)/$(APP_NAME) ./cmd/server

run: build
	./$(BUILD_DIR)/$(APP_NAME)

dev:
	go run ./cmd/server

tidy:
	go mod tidy

wire:
	go run github.com/google/wire/cmd/wire ./internal/app/

lint:
	go vet ./...
	@test -z "$$(gofmt -l . 2>&1 | grep -v '^vendor/' | tee /dev/stderr)" || (echo "ERROR: gofmt drift detected, run 'gofmt -w .'" && exit 1)

migrate-up:
	migrate -path migrations -database "postgres://zhuzhao:zhuzhao_dev@localhost:5432/zhuzhao?sslmode=disable" up

migrate-down:
	migrate -path migrations -database "postgres://zhuzhao:zhuzhao_dev@localhost:5432/zhuzhao?sslmode=disable" down

# 本地开发：只起 PG + Redis
docker-dev-up:
	cd deployments && docker-compose -f docker-compose.dev.yaml up -d

docker-dev-down:
	cd deployments && docker-compose -f docker-compose.dev.yaml down

# 清空 PG/Redis 数据卷并重建（会删除所有本地 dev 数据，随后自动 migrate-up）
docker-dev-reset:
	cd deployments && docker-compose -f docker-compose.dev.yaml down -v
	cd deployments && docker-compose -f docker-compose.dev.yaml up -d
	@echo "Waiting for postgres..."
	@sleep 5
	$(MAKE) migrate-up

# 完整部署：应用 + PG + Redis
docker-up:
	cd deployments && docker-compose -f docker-compose.yaml up -d --build

docker-down:
	cd deployments && docker-compose -f docker-compose.yaml down

swag:
	swag init -g cmd/server/main.go -o docs

test: test-unit

test-unit:
	go test -v -race -count=1 ./internal/...

test-integration:
	go test -v -race -count=1 -tags=integration ./internal/...

test-cover:
	go test -race -coverprofile=coverage.out ./internal/...
	go tool cover -html=coverage.out -o coverage.html

benchmark:
	go test -bench=. -benchmem ./internal/...
