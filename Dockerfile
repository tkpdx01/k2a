# 预编译版 Dockerfile - 直接使用仓库中的二进制文件
# 构建速度极快，适合 Zeabur 等云平台

FROM debian:bookworm-slim

# 安装运行时依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

# 创建非 root 用户
RUN groupadd -g 1001 appgroup && \
    useradd -u 1001 -g appgroup -m appuser

WORKDIR /app

# 复制预编译的二进制文件和静态资源
COPY dist/kiro2api-linux-amd64 ./kiro2api
COPY static ./static

# 创建数据目录并设置权限
RUN mkdir -p /app/data && \
    chmod +x /app/kiro2api && \
    chown -R appuser:appgroup /app

USER appuser

EXPOSE 8080

CMD ["./kiro2api"]
