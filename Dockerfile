# 预编译版 Dockerfile - 使用静态编译的二进制文件
# 无需安装任何依赖，构建速度极快

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# 复制预编译的二进制文件和静态资源
COPY dist/kiro2api-linux-amd64 /app/kiro2api
COPY static /app/static

# distroless 镜像默认使用 nonroot 用户 (uid 65532)

EXPOSE 8080

ENTRYPOINT ["/app/kiro2api"]
