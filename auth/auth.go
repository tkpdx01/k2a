package auth

import (
	"fmt"
	"kiro2api/logger"
	"kiro2api/types"
)

// AuthService 认证服务（推荐使用依赖注入方式）
type AuthService struct {
	tokenManager *TokenManager
	configs      []AuthConfig
}

// NewAuthService 创建新的认证服务（推荐使用此方法而不是全局函数）
// 即使没有配置也会返回有效的服务实例，允许用户通过 Web UI 添加配置
func NewAuthService() (*AuthService, error) {
	logger.Info("创建AuthService实例")

	// 加载配置
	configs, err := loadConfigs()
	if err != nil {
		logger.Warn("加载配置失败，服务将以无配置模式启动", logger.Err(err))
		configs = []AuthConfig{}
	}

	if len(configs) == 0 {
		logger.Warn("未找到有效的认证配置",
			logger.String("提示", "请通过 /admin 页面添加 Token 或设置 KIRO_AUTH_TOKEN 环境变量"))
	}

	// 创建token管理器（即使没有配置也创建）
	tokenManager := NewTokenManager(configs)

	// 预热第一个可用token（如果有配置）
	if len(configs) > 0 {
		_, warmupErr := tokenManager.getBestToken()
		if warmupErr != nil {
			logger.Warn("token预热失败", logger.Err(warmupErr))
		}
	}

	logger.Info("AuthService创建完成", logger.Int("config_count", len(configs)))

	return &AuthService{
		tokenManager: tokenManager,
		configs:      configs,
	}, nil
}

// GetToken 获取可用的token
func (as *AuthService) GetToken() (types.TokenInfo, error) {
	if as.tokenManager == nil {
		return types.TokenInfo{}, fmt.Errorf("token管理器未初始化")
	}
	return as.tokenManager.getBestToken()
}

// GetTokenWithFingerprint 获取token及其对应的指纹
func (as *AuthService) GetTokenWithFingerprint() (types.TokenInfo, *Fingerprint, error) {
	if as.tokenManager == nil {
		return types.TokenInfo{}, nil, fmt.Errorf("token管理器未初始化")
	}
	return as.tokenManager.GetTokenWithFingerprint()
}

// MarkTokenFailed 标记当前token请求失败
func (as *AuthService) MarkTokenFailed() {
	if as.tokenManager == nil {
		return
	}
	tokenKey := as.tokenManager.GetCurrentTokenKey()
	if tokenKey != "" {
		as.tokenManager.MarkTokenFailed(tokenKey)
	}
}

// GetTokenManager 获取底层的TokenManager（用于高级操作）
func (as *AuthService) GetTokenManager() *TokenManager {
	return as.tokenManager
}

// GetConfigs 获取认证配置
func (as *AuthService) GetConfigs() []AuthConfig {
	return as.configs
}
