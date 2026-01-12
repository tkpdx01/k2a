package auth

import (
	"encoding/json"
	"fmt"
	"os"

	"kiro2api/logger"
	"kiro2api/store"
)

// AuthConfig 简化的认证配置
type AuthConfig struct {
	AuthType     string `json:"auth"`
	RefreshToken string `json:"refreshToken"`
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	Disabled     bool   `json:"disabled,omitempty"`
}

// 认证方法常量
const (
	AuthMethodSocial = "Social"
	AuthMethodIdC    = "IdC"
)

// loadConfigs 从环境变量或 store 加载配置
func loadConfigs() ([]AuthConfig, error) {
	var allConfigs []AuthConfig

	// 1. 尝试从 store 加载（Web 管理添加的 Token）
	if s := store.GetStore(); s != nil {
		storeTokens := s.GetEnabledTokens()
		for _, t := range storeTokens {
			config := AuthConfig{
				AuthType:     t.AuthType,
				RefreshToken: t.RefreshToken,
				ClientID:     t.ClientID,
				ClientSecret: t.ClientSecret,
				Disabled:     t.Disabled,
			}
			allConfigs = append(allConfigs, config)
		}
		if len(storeTokens) > 0 {
			logger.Info("从管理存储加载认证配置", logger.Int("数量", len(storeTokens)))
		}
	}

	// 2. 从环境变量加载（向后兼容）
	envConfigs, err := loadConfigsFromEnv()
	if err == nil && len(envConfigs) > 0 {
		allConfigs = append(allConfigs, envConfigs...)
		logger.Info("从环境变量加载认证配置", logger.Int("数量", len(envConfigs)))
	}

	// 3. 检查是否有有效配置
	if len(allConfigs) == 0 {
		return nil, fmt.Errorf("未找到有效的认证配置\n" +
			"请通过以下方式之一配置:\n" +
			"1. 访问 /admin 页面添加 Token\n" +
			"2. 设置环境变量 KIRO_AUTH_TOKEN")
	}

	logger.Info("认证配置加载完成", logger.Int("总数", len(allConfigs)))
	return allConfigs, nil
}

// loadConfigsFromEnv 从环境变量加载配置
func loadConfigsFromEnv() ([]AuthConfig, error) {
	// 检测并警告弃用的环境变量
	deprecatedVars := []string{
		"REFRESH_TOKEN",
		"AWS_REFRESHTOKEN",
		"IDC_REFRESH_TOKEN",
		"BULK_REFRESH_TOKENS",
	}

	for _, envVar := range deprecatedVars {
		if os.Getenv(envVar) != "" {
			logger.Warn("检测到已弃用的环境变量",
				logger.String("变量名", envVar),
				logger.String("迁移说明", "请迁移到KIRO_AUTH_TOKEN的JSON格式"))
			logger.Warn("迁移示例",
				logger.String("新格式", `KIRO_AUTH_TOKEN='[{"auth":"Social","refreshToken":"your_token"}]'`))
		}
	}

	// 只支持KIRO_AUTH_TOKEN的JSON格式（支持文件路径或JSON字符串）
	jsonData := os.Getenv("KIRO_AUTH_TOKEN")
	if jsonData == "" {
		return nil, nil // 环境变量未设置，返回空
	}

	// 优先尝试从文件加载，失败后再作为JSON字符串处理
	var configData string
	if fileInfo, err := os.Stat(jsonData); err == nil && !fileInfo.IsDir() {
		// 是文件，读取文件内容
		content, err := os.ReadFile(jsonData)
		if err != nil {
			return nil, fmt.Errorf("读取配置文件失败: %w\n配置文件路径: %s", err, jsonData)
		}
		configData = string(content)
		logger.Info("从文件加载认证配置", logger.String("文件路径", jsonData))
	} else {
		// 不是文件或文件不存在，作为JSON字符串处理
		configData = jsonData
		logger.Debug("从环境变量加载JSON配置")
	}

	// 解析JSON配置
	configs, err := parseJSONConfig(configData)
	if err != nil {
		return nil, fmt.Errorf("解析KIRO_AUTH_TOKEN失败: %w", err)
	}

	return processConfigs(configs), nil
}

// GetConfigs 公开的配置获取函数，供其他包调用
func GetConfigs() ([]AuthConfig, error) {
	return loadConfigs()
}

// parseJSONConfig 解析JSON配置字符串
func parseJSONConfig(jsonData string) ([]AuthConfig, error) {
	var configs []AuthConfig

	// 尝试解析为数组
	if err := json.Unmarshal([]byte(jsonData), &configs); err != nil {
		// 尝试解析为单个对象
		var single AuthConfig
		if err := json.Unmarshal([]byte(jsonData), &single); err != nil {
			return nil, fmt.Errorf("JSON格式无效: %w", err)
		}
		configs = []AuthConfig{single}
	}

	return configs, nil
}

// processConfigs 处理和验证配置
func processConfigs(configs []AuthConfig) []AuthConfig {
	var validConfigs []AuthConfig

	for i, config := range configs {
		// 验证必要字段
		if config.RefreshToken == "" {
			continue
		}

		// 设置默认认证类型
		if config.AuthType == "" {
			config.AuthType = AuthMethodSocial
		}

		// 验证IdC认证的必要字段
		if config.AuthType == AuthMethodIdC {
			if config.ClientID == "" || config.ClientSecret == "" {
				continue
			}
		}

		// 跳过禁用的配置
		if config.Disabled {
			continue
		}

		validConfigs = append(validConfigs, config)
		_ = i // 避免未使用变量警告
	}

	return validConfigs
}
