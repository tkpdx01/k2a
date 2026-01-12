# 简化版 Dockerfile - 纯 Go 构建，无 CGO 依赖
# 适用于 Zeabur 等云平台

FROM golang:1.24-alpine AS builder

# 安装基础依赖
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# 复制 go mod 文件并下载依赖
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 纯 Go 构建（sonic 会自动回退到纯 Go 实现）
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o kiro2api main.go

# 运行阶段
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

# 创建非 root 用户
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

WORKDIR /app

# 复制二进制文件和静态资源
COPY --from=builder /app/kiro2api .
COPY --from=builder /app/static ./static

# 创建数据目录
RUN mkdir -p /app/data && \
    chown -R appuser:appgroup /app

USER appuser

EXPOSE 8080

CMD ["./kiro2api"]
