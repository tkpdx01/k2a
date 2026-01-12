package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// StoreData JSON 存储的数据结构
type StoreData struct {
	Admin    AdminConfig   `json:"admin"`
	Tokens   []TokenConfig `json:"tokens"`
	Sessions []Session     `json:"sessions,omitempty"`
}

// AdminConfig 管理员配置
type AdminConfig struct {
	PasswordHash string `json:"password_hash"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

// TokenConfig Token 配置（扩展自 AuthConfig）
type TokenConfig struct {
	ID           string `json:"id"`
	Name         string `json:"name,omitempty"`
	AuthType     string `json:"auth"`
	RefreshToken string `json:"refreshToken"`
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	Disabled     bool   `json:"disabled,omitempty"`
	// 运行时状态（可选持久化）
	UserEmail      string `json:"userEmail,omitempty"`
	RemainingUsage int    `json:"remainingUsage,omitempty"`
	LastUsed       string `json:"lastUsed,omitempty"`
	LastError      string `json:"lastError,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
}

// Session 会话
type Session struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
}

// Store JSON 文件存储
type Store struct {
	mu       sync.RWMutex
	filePath string
	data     *StoreData
}

var (
	globalStore *Store
	storeOnce   sync.Once
)

// InitStore 初始化存储
func InitStore(filePath string) error {
	var initErr error
	storeOnce.Do(func() {
		globalStore = &Store{
			filePath: filePath,
		}
		initErr = globalStore.load()
	})
	return initErr
}

// GetStore 获取全局存储实例
func GetStore() *Store {
	return globalStore
}

// load 从文件加载数据
func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 确保目录存在
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 读取文件
	data, err := os.ReadFile(s.filePath)
	if os.IsNotExist(err) {
		// 文件不存在，创建默认数据
		s.data = s.createDefaultData()
		return s.saveUnsafe()
	}
	if err != nil {
		return fmt.Errorf("读取文件失败: %w", err)
	}

	// 解析 JSON
	s.data = &StoreData{}
	if err := json.Unmarshal(data, s.data); err != nil {
		return fmt.Errorf("解析 JSON 失败: %w", err)
	}

	// 确保有默认管理员密码
	if s.data.Admin.PasswordHash == "" {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		s.data.Admin.PasswordHash = string(hash)
		s.data.Admin.UpdatedAt = time.Now().Format(time.RFC3339)
		_ = s.saveUnsafe()
	}

	return nil
}

// createDefaultData 创建默认数据
func (s *Store) createDefaultData() *StoreData {
	hash, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	return &StoreData{
		Admin: AdminConfig{
			PasswordHash: string(hash),
			UpdatedAt:    time.Now().Format(time.RFC3339),
		},
		Tokens:   []TokenConfig{},
		Sessions: []Session{},
	}
}

// saveUnsafe 保存数据（不加锁，调用者需持有锁）
func (s *Store) saveUnsafe() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 JSON 失败: %w", err)
	}

	// 原子写入：先写临时文件，再重命名
	tmpFile := s.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}

	if err := os.Rename(tmpFile, s.filePath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("重命名文件失败: %w", err)
	}

	return nil
}

// Save 保存数据
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveUnsafe()
}

// === 管理员认证 ===

// VerifyAdminPassword 验证管理员密码
func (s *Store) VerifyAdminPassword(password string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	err := bcrypt.CompareHashAndPassword([]byte(s.data.Admin.PasswordHash), []byte(password))
	return err == nil
}

// UpdateAdminPassword 更新管理员密码
func (s *Store) UpdateAdminPassword(newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("密码加密失败: %w", err)
	}

	s.data.Admin.PasswordHash = string(hash)
	s.data.Admin.UpdatedAt = time.Now().Format(time.RFC3339)

	return s.saveUnsafe()
}

// === 会话管理 ===

// generateSessionToken 生成会话 token
func generateSessionToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	hash := sha256.Sum256(bytes)
	return hex.EncodeToString(hash[:])
}

// CreateSession 创建会话
func (s *Store) CreateSession(duration time.Duration) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	token := generateSessionToken()
	session := Session{
		Token:     token,
		ExpiresAt: time.Now().Add(duration).Format(time.RFC3339),
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	s.data.Sessions = append(s.data.Sessions, session)
	s.saveUnsafe()

	return token
}

// ValidateSession 验证会话
func (s *Store) ValidateSession(token string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, session := range s.data.Sessions {
		if session.Token == token {
			expiresAt, err := time.Parse(time.RFC3339, session.ExpiresAt)
			if err != nil {
				continue
			}
			return time.Now().Before(expiresAt)
		}
	}
	return false
}

// DeleteSession 删除会话
func (s *Store) DeleteSession(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, session := range s.data.Sessions {
		if session.Token == token {
			s.data.Sessions = append(s.data.Sessions[:i], s.data.Sessions[i+1:]...)
			s.saveUnsafe()
			return
		}
	}
}

// CleanExpiredSessions 清理过期会话
func (s *Store) CleanExpiredSessions() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	cleaned := 0
	validSessions := []Session{}

	for _, session := range s.data.Sessions {
		expiresAt, err := time.Parse(time.RFC3339, session.ExpiresAt)
		if err != nil || now.After(expiresAt) {
			cleaned++
			continue
		}
		validSessions = append(validSessions, session)
	}

	if cleaned > 0 {
		s.data.Sessions = validSessions
		s.saveUnsafe()
	}

	return cleaned
}

// === Token 管理 ===

// generateTokenID 生成 Token ID
func generateTokenID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// GetAllTokens 获取所有 Token
func (s *Store) GetAllTokens() []TokenConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 返回副本
	tokens := make([]TokenConfig, len(s.data.Tokens))
	copy(tokens, s.data.Tokens)
	return tokens
}

// GetToken 根据 ID 获取 Token
func (s *Store) GetToken(id string) (*TokenConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, token := range s.data.Tokens {
		if token.ID == id {
			t := token // 复制
			return &t, true
		}
	}
	return nil, false
}

// AddToken 添加 Token
func (s *Store) AddToken(token TokenConfig) (*TokenConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 生成 ID
	if token.ID == "" {
		token.ID = generateTokenID()
	}

	// 设置时间戳
	now := time.Now().Format(time.RFC3339)
	token.CreatedAt = now
	token.UpdatedAt = now

	// 默认认证类型
	if token.AuthType == "" {
		token.AuthType = "Social"
	}

	s.data.Tokens = append(s.data.Tokens, token)

	if err := s.saveUnsafe(); err != nil {
		return nil, err
	}

	return &token, nil
}

// UpdateToken 更新 Token
func (s *Store) UpdateToken(id string, updates TokenConfig) (*TokenConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, token := range s.data.Tokens {
		if token.ID == id {
			// 保留原有字段，更新指定字段
			if updates.Name != "" {
				s.data.Tokens[i].Name = updates.Name
			}
			if updates.AuthType != "" {
				s.data.Tokens[i].AuthType = updates.AuthType
			}
			if updates.RefreshToken != "" {
				s.data.Tokens[i].RefreshToken = updates.RefreshToken
			}
			if updates.ClientID != "" {
				s.data.Tokens[i].ClientID = updates.ClientID
			}
			if updates.ClientSecret != "" {
				s.data.Tokens[i].ClientSecret = updates.ClientSecret
			}
			s.data.Tokens[i].Disabled = updates.Disabled
			s.data.Tokens[i].UpdatedAt = time.Now().Format(time.RFC3339)

			if err := s.saveUnsafe(); err != nil {
				return nil, err
			}

			t := s.data.Tokens[i]
			return &t, nil
		}
	}

	return nil, fmt.Errorf("Token 不存在: %s", id)
}

// DeleteToken 删除 Token
func (s *Store) DeleteToken(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, token := range s.data.Tokens {
		if token.ID == id {
			s.data.Tokens = append(s.data.Tokens[:i], s.data.Tokens[i+1:]...)
			return s.saveUnsafe()
		}
	}

	return fmt.Errorf("Token 不存在: %s", id)
}

// ToggleToken 切换 Token 启用/禁用状态
func (s *Store) ToggleToken(id string) (*TokenConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, token := range s.data.Tokens {
		if token.ID == id {
			s.data.Tokens[i].Disabled = !s.data.Tokens[i].Disabled
			s.data.Tokens[i].UpdatedAt = time.Now().Format(time.RFC3339)

			if err := s.saveUnsafe(); err != nil {
				return nil, err
			}

			t := s.data.Tokens[i]
			return &t, nil
		}
	}

	return nil, fmt.Errorf("Token 不存在: %s", id)
}

// BatchAddTokens 批量添加 Token
func (s *Store) BatchAddTokens(tokens []TokenConfig) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Format(time.RFC3339)
	added := 0

	for _, token := range tokens {
		if token.RefreshToken == "" {
			continue // 跳过无效 token
		}

		if token.ID == "" {
			token.ID = generateTokenID()
		}
		if token.AuthType == "" {
			token.AuthType = "Social"
		}
		token.CreatedAt = now
		token.UpdatedAt = now

		s.data.Tokens = append(s.data.Tokens, token)
		added++
	}

	if added > 0 {
		if err := s.saveUnsafe(); err != nil {
			return 0, err
		}
	}

	return added, nil
}

// UpdateTokenStatus 更新 Token 运行时状态
func (s *Store) UpdateTokenStatus(id string, userEmail string, remainingUsage int, lastError string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, token := range s.data.Tokens {
		if token.ID == id {
			if userEmail != "" {
				s.data.Tokens[i].UserEmail = userEmail
			}
			if remainingUsage >= 0 {
				s.data.Tokens[i].RemainingUsage = remainingUsage
			}
			s.data.Tokens[i].LastUsed = time.Now().Format(time.RFC3339)
			s.data.Tokens[i].LastError = lastError
			s.saveUnsafe()
			return
		}
	}
}

// GetEnabledTokens 获取所有启用的 Token
func (s *Store) GetEnabledTokens() []TokenConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tokens []TokenConfig
	for _, token := range s.data.Tokens {
		if !token.Disabled {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

// GetTokenStats 获取 Token 统计信息
func (s *Store) GetTokenStats() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := map[string]int{
		"total":    len(s.data.Tokens),
		"enabled":  0,
		"disabled": 0,
		"social":   0,
		"idc":      0,
	}

	for _, token := range s.data.Tokens {
		if token.Disabled {
			stats["disabled"]++
		} else {
			stats["enabled"]++
		}
		if token.AuthType == "Social" {
			stats["social"]++
		} else if token.AuthType == "IdC" {
			stats["idc"]++
		}
	}

	return stats
}

// === 导出/导入功能 ===

// ExportData 导出数据结构（用于导出配置）
type ExportData struct {
	Version   string        `json:"version"`
	ExportAt  string        `json:"exportAt"`
	Tokens    []TokenConfig `json:"tokens"`
	TokensCount int         `json:"tokensCount"`
}

// ExportConfig 导出配置（不包含敏感的会话信息和密码哈希）
func (s *Store) ExportConfig() *ExportData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 复制 tokens
	tokens := make([]TokenConfig, len(s.data.Tokens))
	copy(tokens, s.data.Tokens)

	return &ExportData{
		Version:     "1.0",
		ExportAt:    time.Now().Format(time.RFC3339),
		Tokens:      tokens,
		TokensCount: len(tokens),
	}
}

// ImportData 导入数据结构
type ImportData struct {
	Version string        `json:"version"`
	Tokens  []TokenConfig `json:"tokens"`
}

// ImportResult 导入结果
type ImportResult struct {
	TokensAdded   int      `json:"tokensAdded"`
	TokensSkipped int      `json:"tokensSkipped"`
	TokensUpdated int      `json:"tokensUpdated"`
	Errors        []string `json:"errors,omitempty"`
}

// ImportConfig 导入配置
// mode: "merge" - 合并（跳过已存在的）, "replace" - 替换所有, "update" - 更新已存在的
func (s *Store) ImportConfig(data *ImportData, mode string) *ImportResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := &ImportResult{
		Errors: []string{},
	}

	now := time.Now().Format(time.RFC3339)

	switch mode {
	case "replace":
		// 替换模式：清空现有 tokens，导入新的
		s.data.Tokens = []TokenConfig{}
		for _, token := range data.Tokens {
			if token.RefreshToken == "" {
				result.Errors = append(result.Errors, fmt.Sprintf("跳过无效 token: %s", token.Name))
				result.TokensSkipped++
				continue
			}
			if token.ID == "" {
				token.ID = generateTokenID()
			}
			if token.AuthType == "" {
				token.AuthType = "Social"
			}
			token.CreatedAt = now
			token.UpdatedAt = now
			s.data.Tokens = append(s.data.Tokens, token)
			result.TokensAdded++
		}

	case "update":
		// 更新模式：更新已存在的，添加新的
		existingMap := make(map[string]int) // refreshToken -> index
		for i, t := range s.data.Tokens {
			existingMap[t.RefreshToken] = i
		}

		for _, token := range data.Tokens {
			if token.RefreshToken == "" {
				result.Errors = append(result.Errors, fmt.Sprintf("跳过无效 token: %s", token.Name))
				result.TokensSkipped++
				continue
			}

			if idx, exists := existingMap[token.RefreshToken]; exists {
				// 更新已存在的
				if token.Name != "" {
					s.data.Tokens[idx].Name = token.Name
				}
				if token.AuthType != "" {
					s.data.Tokens[idx].AuthType = token.AuthType
				}
				if token.ClientID != "" {
					s.data.Tokens[idx].ClientID = token.ClientID
				}
				if token.ClientSecret != "" {
					s.data.Tokens[idx].ClientSecret = token.ClientSecret
				}
				s.data.Tokens[idx].Disabled = token.Disabled
				s.data.Tokens[idx].UpdatedAt = now
				result.TokensUpdated++
			} else {
				// 添加新的
				if token.ID == "" {
					token.ID = generateTokenID()
				}
				if token.AuthType == "" {
					token.AuthType = "Social"
				}
				token.CreatedAt = now
				token.UpdatedAt = now
				s.data.Tokens = append(s.data.Tokens, token)
				result.TokensAdded++
			}
		}

	default: // "merge"
		// 合并模式：跳过已存在的（基于 refreshToken）
		existingTokens := make(map[string]bool)
		for _, t := range s.data.Tokens {
			existingTokens[t.RefreshToken] = true
		}

		for _, token := range data.Tokens {
			if token.RefreshToken == "" {
				result.Errors = append(result.Errors, fmt.Sprintf("跳过无效 token: %s", token.Name))
				result.TokensSkipped++
				continue
			}

			if existingTokens[token.RefreshToken] {
				result.TokensSkipped++
				continue
			}

			if token.ID == "" {
				token.ID = generateTokenID()
			}
			if token.AuthType == "" {
				token.AuthType = "Social"
			}
			token.CreatedAt = now
			token.UpdatedAt = now
			s.data.Tokens = append(s.data.Tokens, token)
			result.TokensAdded++
		}
	}

	s.saveUnsafe()
	return result
}

// ClearAllTokens 清空所有 Token
func (s *Store) ClearAllTokens() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := len(s.data.Tokens)
	s.data.Tokens = []TokenConfig{}
	s.saveUnsafe()
	return count
}
