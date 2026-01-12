# 预编译版 Dockerfile - 使用静态编译的二进制文件
# 无需安装任何依赖，构建速度极快

FROM busybox:1.36-glibc

WORKDIR /app

# 复制预编译的二进制文件和静态资源
COPY dist/kiro2api-linux-amd64 /app/kiro2api
COPY static /app/static

# 创建数据目录并设置权限
RUN mkdir -p /app/data && \
    chmod +x /app/kiro2api

EXPOSE 8080

CMD ["/app/kiro2api"]
