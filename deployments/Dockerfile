# 多阶段构建：编译 Go 二进制 + 最小运行镜像
# 使用方法：docker build -t zhuzhao:latest -f deployments/Dockerfile .

# === Stage 1: build ===
FROM golang:1.26-alpine AS builder

# 国内镜像源加速（apk + Go proxy）
RUN sed -i 's#https://dl-cdn.alpinelinux.org#https://mirrors.aliyun.com#g' /etc/apk/repositories && \
    apk add --no-cache git && \
    go env -w GOPROXY=https://goproxy.cn,direct

WORKDIR /build

# 先拷依赖文件，利用 Docker 层缓存
COPY go.mod go.sum ./
RUN go mod download

# 拷源码
COPY . .

# 编译（CGO 禁用，纯静态二进制）
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /zhuzhao ./cmd/server

# === Stage 2: runtime ===
FROM alpine:3.20

RUN sed -i 's#https://dl-cdn.alpinelinux.org#https://mirrors.aliyun.com#g' /etc/apk/repositories && \
    apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 appuser

WORKDIR /app

COPY --from=builder /zhuzhao /app/zhuzhao
COPY configs/ /app/configs/
COPY migrations/ /app/migrations/

USER appuser

EXPOSE 33333

ENTRYPOINT ["/app/zhuzhao"]
