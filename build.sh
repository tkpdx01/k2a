#!/bin/bash
# 本地编译脚本 - 支持多平台交叉编译

set -e

VERSION=${VERSION:-"latest"}
OUTPUT_DIR="dist"

# 清理并创建输出目录
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

echo "🔨 Building kiro2api..."

# 默认编译 Linux amd64（Docker 常用）
build_linux_amd64() {
    echo "  → linux/amd64"
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
        -ldflags="-s -w -X main.Version=$VERSION" \
        -o "$OUTPUT_DIR/kiro2api-linux-amd64" \
        main.go
}

# Linux arm64（Apple Silicon Docker / ARM 服务器）
build_linux_arm64() {
    echo "  → linux/arm64"
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
        -ldflags="-s -w -X main.Version=$VERSION" \
        -o "$OUTPUT_DIR/kiro2api-linux-arm64" \
        main.go
}

# macOS（本地开发）
build_darwin() {
    echo "  → darwin/arm64"
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
        -ldflags="-s -w -X main.Version=$VERSION" \
        -o "$OUTPUT_DIR/kiro2api-darwin-arm64" \
        main.go
}

# 根据参数选择编译目标
case "${1:-all}" in
    linux)
        build_linux_amd64
        ;;
    linux-arm)
        build_linux_arm64
        ;;
    darwin)
        build_darwin
        ;;
    all)
        build_linux_amd64
        build_linux_arm64
        build_darwin
        ;;
    docker)
        # 只编译 Docker 需要的版本
        build_linux_amd64
        build_linux_arm64
        ;;
    *)
        echo "Usage: $0 [linux|linux-arm|darwin|docker|all]"
        exit 1
        ;;
esac

# 复制静态资源
echo "📦 Copying static files..."
cp -r static "$OUTPUT_DIR/"

# 显示结果
echo ""
echo "✅ Build complete!"
ls -lh "$OUTPUT_DIR"/kiro2api-* 2>/dev/null || true
echo ""
echo "📁 Output directory: $OUTPUT_DIR/"
