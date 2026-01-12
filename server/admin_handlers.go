package server

import (
	"encoding/json"
	"io"
	"kiro2api/logger"
	"kiro2api/store"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	sessionCookieName = "k2a_session"
	sessionDuration   = 24 * time.Hour
)

// === 认证中间件 ===

// AdminAuthMiddleware 管理员认证中间件
func AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 cookie 获取 session token
		token, err := c.Cookie(sessionCookieName)
		if err != nil || token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			c.Abort()
			return
		}

		// 验证 session
		s := store.GetStore()
		if s == nil || !s.ValidateSession(token) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "会话已过期"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// === 认证 API ===

// handleAdminLogin 管理员登录
func handleAdminLogin(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	s := store.GetStore()
	if s == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "存储未初始化"})
		return
	}

	if !s.VerifyAdminPassword(req.Password) {
		logger.Warn("管理员登录失败", logger.String("ip", c.ClientIP()))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "密码错误"})
		return
	}

	// 创建会话
	token := s.CreateSession(sessionDuration)

	// 设置 cookie
	c.SetCookie(sessionCookieName, token, int(sessionDuration.Seconds()), "/", "", false, true)

	logger.Info("管理员登录成功", logger.String("ip", c.ClientIP()))
	c.JSON(http.StatusOK, gin.H{"message": "登录成功"})
}

// handleAdminLogout 管理员登出
func handleAdminLogout(c *gin.Context) {
	token, _ := c.Cookie(sessionCookieName)
	if token != "" {
		s := store.GetStore()
		if s != nil {
			s.DeleteSession(token)
		}
	}

	c.SetCookie(sessionCookieName, "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "已登出"})
}

// handleAdminChangePassword 修改管理员密码
func handleAdminChangePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	if len(req.NewPassword) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "新密码至少6位"})
		return
	}

	s := store.GetStore()
	if s == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "存储未初始化"})
		return
	}

	if !s.VerifyAdminPassword(req.OldPassword) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "原密码错误"})
		return
	}

	if err := s.UpdateAdminPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "修改密码失败"})
		return
	}

	logger.Info("管理员密码已修改", logger.String("ip", c.ClientIP()))
	c.JSON(http.StatusOK, gin.H{"message": "密码修改成功"})
}

// handleAdminStatus 检查登录状态
func handleAdminStatus(c *gin.Context) {
	token, err := c.Cookie(sessionCookieName)
	if err != nil || token == "" {
		c.JSON(http.StatusOK, gin.H{"logged_in": false})
		return
	}

	s := store.GetStore()
	if s == nil || !s.ValidateSession(token) {
		c.JSON(http.StatusOK, gin.H{"logged_in": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{"logged_in": true})
}

// === Token 管理 API ===

// handleListTokens 获取 Token 列表
func handleListTokens(c *gin.Context) {
	s := store.GetStore()
	if s == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "存储未初始化"})
		return
	}

	tokens := s.GetAllTokens()
	stats := s.GetTokenStats()

	// 隐藏敏感信息
	for i := range tokens {
		tokens[i].RefreshToken = maskToken(tokens[i].RefreshToken)
		tokens[i].ClientSecret = maskToken(tokens[i].ClientSecret)
	}

	c.JSON(http.StatusOK, gin.H{
		"tokens": tokens,
		"stats":  stats,
	})
}

// handleGetToken 获取单个 Token
func handleGetToken(c *gin.Context) {
	id := c.Param("id")

	s := store.GetStore()
	if s == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "存储未初始化"})
		return
	}

	token, found := s.GetToken(id)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "Token 不存在"})
		return
	}

	// 隐藏敏感信息
	token.RefreshToken = maskToken(token.RefreshToken)
	token.ClientSecret = maskToken(token.ClientSecret)

	c.JSON(http.StatusOK, token)
}

// handleAddToken 添加 Token
func handleAddToken(c *gin.Context) {
	var req store.TokenConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	if req.RefreshToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refreshToken 不能为空"})
		return
	}

	s := store.GetStore()
	if s == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "存储未初始化"})
		return
	}

	token, err := s.AddToken(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logger.Info("添加 Token", logger.String("id", token.ID), logger.String("ip", c.ClientIP()))
	c.JSON(http.StatusCreated, token)
}

// handleUpdateToken 更新 Token
func handleUpdateToken(c *gin.Context) {
	id := c.Param("id")

	var req store.TokenConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	s := store.GetStore()
	if s == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "存储未初始化"})
		return
	}

	token, err := s.UpdateToken(id, req)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	logger.Info("更新 Token", logger.String("id", id), logger.String("ip", c.ClientIP()))
	c.JSON(http.StatusOK, token)
}

// handleDeleteToken 删除 Token
func handleDeleteToken(c *gin.Context) {
	id := c.Param("id")

	s := store.GetStore()
	if s == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "存储未初始化"})
		return
	}

	if err := s.DeleteToken(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	logger.Info("删除 Token", logger.String("id", id), logger.String("ip", c.ClientIP()))
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// handleToggleToken 切换 Token 状态
func handleToggleToken(c *gin.Context) {
	id := c.Param("id")

	s := store.GetStore()
	if s == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "存储未初始化"})
		return
	}

	token, err := s.ToggleToken(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	status := "启用"
	if token.Disabled {
		status = "禁用"
	}
	logger.Info("切换 Token 状态", logger.String("id", id), logger.String("status", status), logger.String("ip", c.ClientIP()))
	c.JSON(http.StatusOK, token)
}

// handleBatchAddTokens 批量添加 Token
func handleBatchAddTokens(c *gin.Context) {
	var req struct {
		Tokens []store.TokenConfig `json:"tokens"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	s := store.GetStore()
	if s == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "存储未初始化"})
		return
	}

	added, err := s.BatchAddTokens(req.Tokens)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logger.Info("批量添加 Token", logger.Int("count", added), logger.String("ip", c.ClientIP()))
	c.JSON(http.StatusOK, gin.H{
		"message": "批量添加成功",
		"added":   added,
	})
}

// handleUploadTokenFile 上传 Token 配置文件
func handleUploadTokenFile(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请上传文件"})
		return
	}

	// 限制文件大小 (1MB)
	if file.Size > 1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件过大，最大 1MB"})
		return
	}

	// 打开文件
	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "打开文件失败"})
		return
	}
	defer f.Close()

	// 读取内容
	content, err := io.ReadAll(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件失败"})
		return
	}

	// 解析 JSON
	var tokens []store.TokenConfig
	if err := json.Unmarshal(content, &tokens); err != nil {
		// 尝试解析单个 token
		var single store.TokenConfig
		if err2 := json.Unmarshal(content, &single); err2 != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "JSON 格式错误"})
			return
		}
		tokens = []store.TokenConfig{single}
	}

	s := store.GetStore()
	if s == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "存储未初始化"})
		return
	}

	added, err := s.BatchAddTokens(tokens)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logger.Info("上传 Token 文件", logger.Int("count", added), logger.String("filename", file.Filename), logger.String("ip", c.ClientIP()))
	c.JSON(http.StatusOK, gin.H{
		"message":  "上传成功",
		"added":    added,
		"filename": file.Filename,
	})
}

// === 导出/导入 API ===

// handleExportConfig 导出配置
func handleExportConfig(c *gin.Context) {
	s := store.GetStore()
	if s == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "存储未初始化"})
		return
	}

	exportData := s.ExportConfig()

	// 检查是否需要下载文件
	if c.Query("download") == "true" {
		c.Header("Content-Disposition", "attachment; filename=k2a_config_export.json")
		c.Header("Content-Type", "application/json")
	}

	logger.Info("导出配置", logger.Int("tokens_count", exportData.TokensCount), logger.String("ip", c.ClientIP()))
	c.JSON(http.StatusOK, exportData)
}

// handleImportConfig 导入配置
func handleImportConfig(c *gin.Context) {
	s := store.GetStore()
	if s == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "存储未初始化"})
		return
	}

	// 获取导入模式
	mode := c.DefaultQuery("mode", "merge")
	if mode != "merge" && mode != "replace" && mode != "update" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的导入模式，支持: merge, replace, update"})
		return
	}

	// 检查是否是文件上传
	file, err := c.FormFile("file")
	var content []byte

	if err == nil {
		// 文件上传模式
		if file.Size > 1024*1024 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "文件过大，最大 1MB"})
			return
		}

		f, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "打开文件失败"})
			return
		}
		defer f.Close()

		content, err = io.ReadAll(f)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件失败"})
			return
		}
	} else {
		// JSON body 模式
		content, err = io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "读取请求体失败"})
			return
		}
	}

	// 解析导入数据
	var importData store.ImportData
	if err := json.Unmarshal(content, &importData); err != nil {
		// 尝试解析为 token 数组（兼容旧格式）
		var tokens []store.TokenConfig
		if err2 := json.Unmarshal(content, &tokens); err2 != nil {
			// 尝试解析单个 token
			var single store.TokenConfig
			if err3 := json.Unmarshal(content, &single); err3 != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "JSON 格式错误"})
				return
			}
			tokens = []store.TokenConfig{single}
		}
		importData.Tokens = tokens
	}

	// 执行导入
	result := s.ImportConfig(&importData, mode)

	logger.Info("导入配置",
		logger.String("mode", mode),
		logger.Int("added", result.TokensAdded),
		logger.Int("updated", result.TokensUpdated),
		logger.Int("skipped", result.TokensSkipped),
		logger.String("ip", c.ClientIP()))

	c.JSON(http.StatusOK, gin.H{
		"message": "导入完成",
		"mode":    mode,
		"result":  result,
	})
}

// handleClearAllTokens 清空所有 Token
func handleClearAllTokens(c *gin.Context) {
	s := store.GetStore()
	if s == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "存储未初始化"})
		return
	}

	// 需要确认参数
	confirm := c.Query("confirm")
	if confirm != "yes" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请添加 ?confirm=yes 参数确认清空操作"})
		return
	}

	count := s.ClearAllTokens()

	logger.Warn("清空所有 Token", logger.Int("count", count), logger.String("ip", c.ClientIP()))
	c.JSON(http.StatusOK, gin.H{
		"message": "已清空所有 Token",
		"deleted": count,
	})
}

// === 辅助函数 ===

// maskToken 隐藏 token 敏感信息
func maskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "****" + token[len(token)-4:]
}

// RegisterAdminRoutes 注册管理路由
func RegisterAdminRoutes(r *gin.Engine) {
	// 公开路由
	r.POST("/api/admin/login", handleAdminLogin)
	r.GET("/api/admin/status", handleAdminStatus)

	// 需要认证的路由
	admin := r.Group("/api/admin")
	admin.Use(AdminAuthMiddleware())
	{
		admin.POST("/logout", handleAdminLogout)
		admin.POST("/change-password", handleAdminChangePassword)

		// Token 管理
		admin.GET("/tokens", handleListTokens)
		admin.GET("/tokens/:id", handleGetToken)
		admin.POST("/tokens", handleAddToken)
		admin.PUT("/tokens/:id", handleUpdateToken)
		admin.DELETE("/tokens/:id", handleDeleteToken)
		admin.POST("/tokens/:id/toggle", handleToggleToken)
		admin.POST("/tokens/batch", handleBatchAddTokens)
		admin.POST("/tokens/upload", handleUploadTokenFile)

		// 导出/导入
		admin.GET("/export", handleExportConfig)
		admin.POST("/import", handleImportConfig)
		admin.DELETE("/tokens/clear", handleClearAllTokens)
	}
}

// InitAdminStore 初始化管理存储
func InitAdminStore(dataDir string) error {
	filePath := dataDir + "/admin_data.json"

	// 检查是否有环境变量指定路径
	if envPath := os.Getenv("K2A_ADMIN_DATA"); envPath != "" {
		filePath = envPath
	}

	return store.InitStore(filePath)
}

