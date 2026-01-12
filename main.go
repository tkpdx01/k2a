package main

import (
	"os"
	"strings"

	"kiro2api/auth"
	"kiro2api/logger"
	"kiro2api/server"

	"github.com/joho/godotenv"
)

func main() {
	// 自动加载.env文件
	if err := godotenv.Load(); err != nil {
		logger.Info("未找到.env文件，使用环境变量")
	}

	// 重新初始化logger以使用.env文件中的配置
	logger.Reinitialize()

	// 显示当前日志级别设置（仅在DEBUG级别时显示详细信息）
	// 注意：移除重复的系统字段，这些信息已包含在日志结构中
	logger.Debug("日志系统初始化完成",
		logger.String("config_level", os.Getenv("LOG_LEVEL")),
		logger.String("config_file", os.Getenv("LOG_FILE")))

	// 初始化管理存储
	dataDir := os.Getenv("K2A_DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}
	if err := server.InitAdminStore(dataDir); err != nil {
		logger.Warn("管理存储初始化失败，Web管理功能不可用", logger.Err(err))
	} else {
		logger.Info("管理存储初始化成功", logger.String("data_dir", dataDir))
	}

	// 初始化代理池（如果配置了代理）
	initProxyPool()

	// 🚀 创建AuthService实例（使用依赖注入）
	logger.Info("正在创建AuthService...")
	authService, err := auth.NewAuthService()
	if err != nil {
		// 这种情况不应该发生，因为 NewAuthService 现在总是返回有效实例
		logger.Error("AuthService创建失败", logger.Err(err))
		logger.Error("请检查token配置后重新启动服务器")
		os.Exit(1)
	}

	port := "8080" // 默认端口
	if len(os.Args) > 1 {
		port = os.Args[1]
	}
	// 从环境变量获取端口，覆盖命令行参数
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
	}

	// 从环境变量获取客户端认证token（必需，无默认值）
	clientToken := os.Getenv("KIRO_CLIENT_TOKEN")
	if clientToken == "" {
		logger.Error("致命错误: 未设置 KIRO_CLIENT_TOKEN 环境变量")
		logger.Error("请在 .env 文件中设置强密码，例如: KIRO_CLIENT_TOKEN=your-secure-random-password")
		logger.Error("安全提示: 请使用至少32字符的随机字符串")
		os.Exit(1)
	}

	server.StartServer(port, clientToken, authService)
}


// initProxyPool 初始化代理池
func initProxyPool() {
	proxyList := os.Getenv("PROXY_POOL")
	if proxyList == "" {
		logger.Debug("未配置代理池")
		return
	}

	// 解析代理列表（逗号分隔）
	var proxies []string
	for _, proxy := range splitAndTrim(proxyList, ",") {
		if proxy != "" {
			proxies = append(proxies, proxy)
		}
	}

	if len(proxies) == 0 {
		return
	}

	cfg := auth.DefaultProxyPoolConfig()
	cfg.Proxies = proxies

	auth.InitProxyPool(cfg)
	logger.Info("代理池初始化完成", logger.Int("proxy_count", len(proxies)))
}

// splitAndTrim 分割字符串并去除空白
func splitAndTrim(s string, sep string) []string {
	var result []string
	for _, part := range strings.Split(s, sep) {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
