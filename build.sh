#!/bin/bash
# 本地编译脚本 - 编译 Linux amd64 版本

set -e

VERSION=${VERSION:-"latest"}
OUTPUT_DIR="dist"

# 清理并创建输出目录
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

echo "🔨 Building kiro2api for linux/amd64..."

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.Version=$VERSION" \
    -o "$OUTPUT_DIR/kiro2api-linux-amd64" \
    main.go

# 复制静态资源
echo "📦 Copying static files..."
cp -r static "$OUTPUT_DIR/"

# 显示结果
echo ""
echo "✅ Build complete!"
ls -lh "$OUTPUT_DIR"/kiro2api-*
echo ""
echo "📁 Output directory: $OUTPUT_DIR/"
echo ""
echo "Next step: docker build -f Dockerfile.prebuilt -t kiro2api ."
