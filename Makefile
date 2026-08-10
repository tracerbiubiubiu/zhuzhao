.PHONY: build run dev tidy wire migrate-up migrate-down docker-up docker-down swag

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
	cd internal/app && wire

migrate-up:
	migrate -path migrations -database "postgres://zhuzhao:zhuzhao_dev@localhost:5432/zhuzhao?sslmode=disable" up

migrate-down:
	migrate -path migrations -database "postgres://zhuzhao:zhuzhao_dev@localhost:5432/zhuzhao?sslmode=disable" down

docker-up:
	cd deployments && docker-compose up -d

docker-down:
	cd deployments && docker-compose down

swag:
	swag init -g cmd/server/main.go -o docs
