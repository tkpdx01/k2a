# kiro2api 改进计划

> 基于 kiro.rs、AIClient-2-API、KiroGate 的对比分析

---

## 一、改进优先级总览

| 优先级 | 功能 | 工作量 | 价值 | 参考项目 |
|:------:|------|:------:|:----:|----------|
| **P0** | Token 自动回写 | 中 | ⭐⭐⭐⭐⭐ | kiro.rs |
| **P0** | Admin API | 中 | ⭐⭐⭐⭐⭐ | kiro.rs |
| **P1** | 完整代理支持 | 低 | ⭐⭐⭐⭐ | kiro.rs, AIClient-2-API |
| **P1** | Web UI 管理界面 | 高 | ⭐⭐⭐⭐ | kiro.rs, KiroGate |
| **P2** | 多租户支持 | 中 | ⭐⭐⭐ | KiroGate |
| **P2** | Docker Compose 优化 | 低 | ⭐⭐⭐ | AIClient-2-API |

---

## 二、P0 - Token 自动回写

### 2.1 问题描述

当前 kiro2api 刷新 Token 后，新的 `refreshToken` 仅保存在内存中。如果服务重启，会丢失刷新后的 Token，导致需要重新配置。

### 2.2 kiro.rs 实现参考

```rust
// kiro.rs/src/kiro/token_manager.rs (第 793-825 行)
fn persist_credentials(&self) -> anyhow::Result<bool> {
    // 仅多凭据格式才回写
    if !self.is_multiple_format {
        return Ok(false);
    }

    let path = match &self.credentials_path {
        Some(p) => p,
        None => return Ok(false),
    };

    // 收集所有凭据
    let credentials: Vec<KiroCredentials> = {
        let entries = self.entries.lock();
        entries.iter().map(|e| e.credentials.clone()).collect()
    };

    // 序列化为 pretty JSON
    let json = serde_json::to_string_pretty(&credentials)?;
    std::fs::write(path, &json)?;
    Ok(true)
}
```

### 2.3 kiro2api 实现方案

#### 步骤 1: 修改 `auth/config.go`

```go
// AuthConfig 添加字段
type AuthConfig struct {
    // ... 现有字段

    // 内部使用，不序列化
    configPath    string `json:"-"` // 配置文件路径
    isMultiFormat bool   `json:"-"` // 是否为数组格式
    index         int    `json:"-"` // 在配置数组中的索引
}

// LoadAuthConfigs 修改返回值
func LoadAuthConfigs() ([]AuthConfig, string, bool, error) {
    // ... 现有逻辑

    // 返回配置路径和格式信息
    return configs, configPath, isMultiFormat, nil
}
```

#### 步骤 2: 修改 `auth/token_manager.go`

```go
// TokenManager 添加字段
type TokenManager struct {
    // ... 现有字段
    configPath    string
    isMultiFormat bool
    persistMutex  sync.Mutex
}

// PersistCredentials 将刷新后的 Token 回写到配置文件
func (tm *TokenManager) PersistCredentials() error {
    if !tm.isMultiFormat || tm.configPath == "" {
        return nil
    }

    tm.persistMutex.Lock()
    defer tm.persistMutex.Unlock()

    // 收集所有配置（包含最新的 Token）
    configs := make([]AuthConfig, len(tm.configs))
    for i := range tm.configs {
        configs[i] = tm.configs[i]

        // 从缓存中获取最新的 Token
        cacheKey := fmt.Sprintf(config.TokenCacheKeyFormat, i)
        if cached, ok := tm.tokenCache.Load(cacheKey); ok {
            if tokenInfo, ok := cached.(*TokenInfo); ok {
                configs[i].RefreshToken = tokenInfo.RefreshToken
                configs[i].AccessToken = tokenInfo.AccessToken
                configs[i].ExpiresAt = tokenInfo.ExpiresAt
            }
        }
    }

    // 序列化为 pretty JSON
    data, err := json.MarshalIndent(configs, "", "  ")
    if err != nil {
        return fmt.Errorf("序列化配置失败: %w", err)
    }

    // 原子写入（先写临时文件，再重命名）
    tmpPath := tm.configPath + ".tmp"
    if err := os.WriteFile(tmpPath, data, 0600); err != nil {
        return fmt.Errorf("写入临时文件失败: %w", err)
    }

    if err := os.Rename(tmpPath, tm.configPath); err != nil {
        os.Remove(tmpPath)
        return fmt.Errorf("重命名文件失败: %w", err)
    }

    logger.Info("Token 配置已回写", logger.String("path", tm.configPath))
    return nil
}
```

#### 步骤 3: 修改 `auth/refresh.go`

```go
// refreshSingleToken 刷新成功后触发回写
func (tm *TokenManager) refreshSingleToken(ctx context.Context, index int) error {
    // ... 现有刷新逻辑

    // 刷新成功后，异步回写配置
    go func() {
        if err := tm.PersistCredentials(); err != nil {
            logger.Warn("Token 回写失败", logger.Err(err))
        }
    }()

    return nil
}
```

### 2.4 配置文件格式

支持两种格式，仅数组格式支持回写：

```json
// 单凭据格式（不回写）
{
  "auth": "Social",
  "refreshToken": "xxx"
}

// 多凭据格式（支持回写）
[
  {
    "auth": "Social",
    "refreshToken": "xxx",
    "disabled": false
  },
  {
    "auth": "IdC",
    "refreshToken": "yyy",
    "clientId": "...",
    "clientSecret": "..."
  }
]
```

---

## 三、P0 - Admin API

### 3.1 API 设计

参考 kiro.rs 的 Admin API 设计：

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/admin/credentials` | 获取所有凭据状态 |
| GET | `/api/admin/credentials/:id` | 获取单个凭据详情 |
| POST | `/api/admin/credentials/:id/disabled` | 设置禁用状态 |
| POST | `/api/admin/credentials/:id/priority` | 设置优先级 |
| POST | `/api/admin/credentials/:id/reset` | 重置失败计数 |
| GET | `/api/admin/credentials/:id/balance` | 查询余额 |
| POST | `/api/admin/credentials` | 添加新凭据 |
| DELETE | `/api/admin/credentials/:id` | 删除凭据 |
| GET | `/api/admin/stats` | 获取统计信息 |

### 3.2 实现方案

#### 创建 `server/admin_handlers.go`

```go
package server

import (
    "net/http"
    "os"
    "strconv"

    "github.com/gin-gonic/gin"
    "kiro2api/auth"
    "kiro2api/logger"
)

// CredentialStatus 凭据状态响应
type CredentialStatus struct {
    ID           int       `json:"id"`
    Auth         string    `json:"auth"`
    Disabled     bool      `json:"disabled"`
    Priority     int       `json:"priority"`
    FailCount    int       `json:"failCount"`
    LastUsed     time.Time `json:"lastUsed,omitempty"`
    LastError    string    `json:"lastError,omitempty"`
    TokenExpires time.Time `json:"tokenExpires,omitempty"`
    HasProfile   bool      `json:"hasProfile"`
}

// GetAllCredentials 获取所有凭据状态
func GetAllCredentials(c *gin.Context) {
    tm := auth.GetTokenManager()
    statuses := tm.GetAllCredentialStatus()
    c.JSON(http.StatusOK, gin.H{
        "credentials": statuses,
        "total":       len(statuses),
    })
}

// SetCredentialDisabled 设置凭据禁用状态
func SetCredentialDisabled(c *gin.Context) {
    id, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
        return
    }

    var req struct {
        Disabled bool `json:"disabled"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    tm := auth.GetTokenManager()
    if err := tm.SetDisabled(id, req.Disabled); err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
        return
    }

    // 触发配置回写
    go tm.PersistCredentials()

    c.JSON(http.StatusOK, gin.H{"message": "success"})
}

// SetCredentialPriority 设置凭据优先级
func SetCredentialPriority(c *gin.Context) {
    id, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
        return
    }

    var req struct {
        Priority int `json:"priority"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    tm := auth.GetTokenManager()
    if err := tm.SetPriority(id, req.Priority); err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
        return
    }

    go tm.PersistCredentials()

    c.JSON(http.StatusOK, gin.H{"message": "success"})
}

// ResetCredentialFailCount 重置失败计数
func ResetCredentialFailCount(c *gin.Context) {
    id, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
        return
    }

    tm := auth.GetTokenManager()
    if err := tm.ResetFailCount(id); err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "success"})
}

// GetCredentialBalance 查询凭据余额
func GetCredentialBalance(c *gin.Context) {
    id, err := strconv.Atoi(c.Param("id"))
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
        return
    }

    tm := auth.GetTokenManager()
    balance, err := tm.GetBalance(id)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, balance)
}

// GetStats 获取统计信息
func GetStats(c *gin.Context) {
    tm := auth.GetTokenManager()
    stats := tm.GetStats()
    c.JSON(http.StatusOK, stats)
}
```

#### 创建 `server/admin_middleware.go`

```go
package server

import (
    "net/http"
    "os"

    "github.com/gin-gonic/gin"
)

// AdminAuthMiddleware Admin API 认证中间件
func AdminAuthMiddleware() gin.HandlerFunc {
    adminKey := os.Getenv("ADMIN_API_KEY")
    if adminKey == "" {
        // 未配置 Admin Key，禁用 Admin API
        return func(c *gin.Context) {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "error": "Admin API is disabled. Set ADMIN_API_KEY to enable.",
            })
        }
    }

    return func(c *gin.Context) {
        // 支持两种认证方式
        apiKey := c.GetHeader("X-Admin-Key")
        if apiKey == "" {
            apiKey = c.GetHeader("Authorization")
            if len(apiKey) > 7 && apiKey[:7] == "Bearer " {
                apiKey = apiKey[7:]
            }
        }

        if apiKey != adminKey {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error": "Invalid admin API key",
            })
            return
        }
        c.Next()
    }
}
```

#### 修改 `server/server.go` 注册路由

```go
func SetupRouter() *gin.Engine {
    r := gin.Default()

    // ... 现有路由

    // Admin API 路由组
    admin := r.Group("/api/admin")
    admin.Use(AdminAuthMiddleware())
    {
        admin.GET("/credentials", GetAllCredentials)
        admin.GET("/credentials/:id", GetCredentialDetail)
        admin.POST("/credentials/:id/disabled", SetCredentialDisabled)
        admin.POST("/credentials/:id/priority", SetCredentialPriority)
        admin.POST("/credentials/:id/reset", ResetCredentialFailCount)
        admin.GET("/credentials/:id/balance", GetCredentialBalance)
        admin.POST("/credentials", AddCredential)
        admin.DELETE("/credentials/:id", DeleteCredential)
        admin.GET("/stats", GetStats)
    }

    return r
}
```

### 3.3 环境变量配置

```bash
# Admin API 认证密钥（必须设置才能启用 Admin API）
ADMIN_API_KEY=your-secret-admin-key
```

---

## 四、P1 - 完整代理支持

### 4.1 当前状态

kiro2api 已有 `auth/proxy_pool.go`，但主要用于代理池管理，未集成到实际的 HTTP 请求中。

### 4.2 实现方案

#### 修改 `utils/client.go`

```go
package utils

import (
    "net/http"
    "net/url"
    "os"
    "time"

    "golang.org/x/net/proxy"
    "kiro2api/logger"
)

var SharedHTTPClient *http.Client

func init() {
    SharedHTTPClient = createHTTPClient()
}

func createHTTPClient() *http.Client {
    transport := &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
        TLSHandshakeTimeout: 10 * time.Second,
    }

    // 配置代理
    proxyURL := os.Getenv("PROXY_URL")
    if proxyURL != "" {
        if err := configureProxy(transport, proxyURL); err != nil {
            logger.Warn("代理配置失败，使用直连", logger.Err(err))
        } else {
            logger.Info("已配置代理", logger.String("proxy", proxyURL))
        }
    }

    return &http.Client{
        Transport: transport,
        Timeout:   5 * time.Minute,
    }
}

func configureProxy(transport *http.Transport, proxyURL string) error {
    parsed, err := url.Parse(proxyURL)
    if err != nil {
        return err
    }

    switch parsed.Scheme {
    case "http", "https":
        transport.Proxy = http.ProxyURL(parsed)

    case "socks5", "socks5h":
        // SOCKS5 代理
        auth := &proxy.Auth{}
        if parsed.User != nil {
            auth.User = parsed.User.Username()
            auth.Password, _ = parsed.User.Password()
        }

        dialer, err := proxy.SOCKS5("tcp", parsed.Host, auth, proxy.Direct)
        if err != nil {
            return err
        }
        transport.DialContext = dialer.(proxy.ContextDialer).DialContext

    default:
        return fmt.Errorf("不支持的代理协议: %s", parsed.Scheme)
    }

    return nil
}
```

### 4.3 环境变量配置

```bash
# HTTP/HTTPS 代理
PROXY_URL=http://127.0.0.1:7890

# SOCKS5 代理
PROXY_URL=socks5://127.0.0.1:1080

# 带认证的代理
PROXY_URL=http://user:pass@127.0.0.1:7890
```

---

## 五、P1 - Web UI 管理界面

### 5.1 方案选择

| 方案 | 优点 | 缺点 |
|------|------|------|
| **A: 内嵌 HTML** | 简单、单二进制 | 功能有限 |
| **B: Vue/React SPA** | 功能丰富 | 需要构建步骤 |
| **C: 外部独立服务** | 解耦 | 部署复杂 |

**推荐方案 A**：参考 KiroGate 的轻量级内嵌方案，使用 Go embed 嵌入静态文件。

### 5.2 目录结构

```
kiro2api/
├── admin-ui/
│   ├── index.html
│   ├── app.js
│   └── style.css
├── server/
│   └── admin_ui.go
```

### 5.3 核心功能

1. **凭据管理**
   - 列表展示所有凭据
   - 启用/禁用切换
   - 优先级调整
   - 删除凭据

2. **状态监控**
   - Token 过期时间
   - 失败计数
   - 最后使用时间

3. **余额查询**
   - 查询单个凭据余额
   - 批量刷新余额

4. **统计信息**
   - 请求总数
   - 成功率
   - 平均延迟

### 5.4 实现示例

#### `admin-ui/index.html`

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>kiro2api Admin</title>
    <link rel="stylesheet" href="style.css">
</head>
<body>
    <div id="app">
        <header>
            <h1>kiro2api 管理控制台</h1>
        </header>

        <main>
            <section id="credentials">
                <h2>凭据管理</h2>
                <table id="credentials-table">
                    <thead>
                        <tr>
                            <th>ID</th>
                            <th>类型</th>
                            <th>状态</th>
                            <th>优先级</th>
                            <th>失败次数</th>
                            <th>Token 过期</th>
                            <th>操作</th>
                        </tr>
                    </thead>
                    <tbody></tbody>
                </table>
            </section>

            <section id="stats">
                <h2>统计信息</h2>
                <div id="stats-content"></div>
            </section>
        </main>
    </div>

    <script src="app.js"></script>
</body>
</html>
```

#### `server/admin_ui.go`

```go
package server

import (
    "embed"
    "io/fs"
    "net/http"

    "github.com/gin-gonic/gin"
)

//go:embed admin-ui/*
var adminUIFS embed.FS

func RegisterAdminUI(r *gin.Engine) {
    // 获取子目录
    subFS, _ := fs.Sub(adminUIFS, "admin-ui")

    // 静态文件服务
    r.StaticFS("/admin", http.FS(subFS))

    // 重定向根路径
    r.GET("/admin", func(c *gin.Context) {
        c.Redirect(http.StatusMovedPermanently, "/admin/")
    })
}
```

---

## 六、P2 - 多租户支持

### 6.1 设计思路

参考 KiroGate 的组合模式，支持用户在 API Key 中携带自己的 RefreshToken：

```
格式: PROXY_API_KEY:USER_REFRESH_TOKEN
```

### 6.2 实现方案

#### 修改 `server/middleware.go`

```go
// AuthMiddleware 认证中间件
func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        apiKey := extractAPIKey(c)

        // 检查是否为组合格式
        if idx := strings.Index(apiKey, ":"); idx > 0 {
            proxyKey := apiKey[:idx]
            userToken := apiKey[idx+1:]

            // 验证代理密钥
            if !validateProxyKey(proxyKey) {
                c.AbortWithStatusJSON(401, gin.H{"error": "Invalid proxy key"})
                return
            }

            // 设置用户 Token 到上下文
            c.Set("userRefreshToken", userToken)
            c.Set("isMultiTenant", true)
        } else {
            // 简单模式
            if !validateProxyKey(apiKey) {
                c.AbortWithStatusJSON(401, gin.H{"error": "Invalid API key"})
                return
            }
            c.Set("isMultiTenant", false)
        }

        c.Next()
    }
}
```

---

## 七、实施计划

### 第一阶段 (P0)

1. **Token 自动回写** - 1-2 天
   - 修改配置加载逻辑
   - 实现 PersistCredentials 方法
   - 添加触发时机

2. **Admin API** - 2-3 天
   - 实现所有 API 端点
   - 添加认证中间件
   - 编写 API 文档

### 第二阶段 (P1)

3. **完整代理支持** - 0.5 天
   - 集成 SOCKS5 支持
   - 添加环境变量配置

4. **Web UI** - 3-5 天
   - 设计 UI 界面
   - 实现前端逻辑
   - 集成到 Go 二进制

### 第三阶段 (P2)

5. **多租户支持** - 1-2 天
   - 修改认证逻辑
   - 添加用户 Token 缓存

6. **Docker Compose 优化** - 0.5 天
   - 添加健康检查
   - 配置持久化卷

---

## 八、总结

kiro2api 作为高性能的 Go 实现，核心功能已经非常完善。通过以上改进，可以：

1. **提升可靠性** - Token 自动回写避免数据丢失
2. **增强可运维性** - Admin API 支持远程管理
3. **改善用户体验** - Web UI 提供可视化操作
4. **扩展适用场景** - 代理支持和多租户支持

建议按照优先级逐步实施，首先完成 P0 级别的功能，确保核心稳定性。

---

## 附录 A：kiro.rs 完整参考代码

### A.1 Token 管理器核心实现 (token_manager.rs)

以下是 kiro.rs 中 `MultiTokenManager` 的核心实现，包含凭据管理、故障转移、Token 回写等功能：

```rust
// kiro.rs/src/kiro/token_manager.rs

use anyhow::bail;
use chrono::{DateTime, Duration, Utc};
use parking_lot::Mutex;
use serde::Serialize;
use tokio::sync::Mutex as TokioMutex;
use std::path::PathBuf;

/// 每个凭据最大 API 调用失败次数
const MAX_FAILURES_PER_CREDENTIAL: u32 = 3;

/// 单个凭据条目的状态
struct CredentialEntry {
    /// 凭据唯一 ID
    id: u64,
    /// 凭据信息
    credentials: KiroCredentials,
    /// API 调用连续失败次数
    failure_count: u32,
    /// 是否已禁用
    disabled: bool,
    /// 禁用原因
    disabled_reason: Option<DisabledReason>,
}

/// 禁用原因
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum DisabledReason {
    /// Admin API 手动禁用
    Manual,
    /// 连续失败达到阈值后自动禁用
    TooManyFailures,
    /// 额度已用尽
    QuotaExceeded,
}

/// 凭据条目快照（用于 Admin API 读取）
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CredentialEntrySnapshot {
    pub id: u64,
    pub priority: u32,
    pub disabled: bool,
    pub failure_count: u32,
    pub auth_method: Option<String>,
    pub has_profile_arn: bool,
    pub expires_at: Option<String>,
}

/// 多凭据 Token 管理器
pub struct MultiTokenManager {
    config: Config,
    proxy: Option<ProxyConfig>,
    /// 凭据条目列表
    entries: Mutex<Vec<CredentialEntry>>,
    /// 当前活动凭据 ID
    current_id: Mutex<u64>,
    /// Token 刷新锁
    refresh_lock: TokioMutex<()>,
    /// 凭据文件路径（用于回写）
    credentials_path: Option<PathBuf>,
    /// 是否为多凭据格式
    is_multiple_format: bool,
}

impl MultiTokenManager {
    /// 创建多凭据 Token 管理器
    pub fn new(
        config: Config,
        credentials: Vec<KiroCredentials>,
        proxy: Option<ProxyConfig>,
        credentials_path: Option<PathBuf>,
        is_multiple_format: bool,
    ) -> anyhow::Result<Self> {
        // 为没有 ID 的凭据分配新 ID
        let max_existing_id = credentials.iter().filter_map(|c| c.id).max().unwrap_or(0);
        let mut next_id = max_existing_id + 1;
        let mut has_new_ids = false;

        let entries: Vec<CredentialEntry> = credentials
            .into_iter()
            .map(|mut cred| {
                let id = cred.id.unwrap_or_else(|| {
                    let id = next_id;
                    next_id += 1;
                    cred.id = Some(id);
                    has_new_ids = true;
                    id
                });
                CredentialEntry {
                    id,
                    credentials: cred,
                    failure_count: 0,
                    disabled: false,
                    disabled_reason: None,
                }
            })
            .collect();

        // 选择初始凭据：优先级最高的凭据
        let initial_id = entries
            .iter()
            .min_by_key(|e| e.credentials.priority)
            .map(|e| e.id)
            .unwrap_or(0);

        let manager = Self {
            config,
            proxy,
            entries: Mutex::new(entries),
            current_id: Mutex::new(initial_id),
            refresh_lock: TokioMutex::new(()),
            credentials_path,
            is_multiple_format,
        };

        // 如果有新分配的 ID，立即持久化
        if has_new_ids {
            if let Err(e) = manager.persist_credentials() {
                tracing::warn!("新分配 ID 后持久化失败: {}", e);
            }
        }

        Ok(manager)
    }

    /// 将凭据列表回写到源文件
    fn persist_credentials(&self) -> anyhow::Result<bool> {
        // 仅多凭据格式才回写
        if !self.is_multiple_format {
            return Ok(false);
        }

        let path = match &self.credentials_path {
            Some(p) => p,
            None => return Ok(false),
        };

        // 收集所有凭据
        let credentials: Vec<KiroCredentials> = {
            let entries = self.entries.lock();
            entries.iter().map(|e| e.credentials.clone()).collect()
        };

        // 序列化为 pretty JSON
        let json = serde_json::to_string_pretty(&credentials)?;

        // 写入文件
        if tokio::runtime::Handle::try_current().is_ok() {
            tokio::task::block_in_place(|| std::fs::write(path, &json))?;
        } else {
            std::fs::write(path, &json)?;
        }

        tracing::debug!("已回写凭据到文件: {:?}", path);
        Ok(true)
    }

    /// 报告 API 调用失败
    pub fn report_failure(&self, id: u64) -> bool {
        let mut entries = self.entries.lock();
        let mut current_id = self.current_id.lock();

        let entry = match entries.iter_mut().find(|e| e.id == id) {
            Some(e) => e,
            None => return entries.iter().any(|e| !e.disabled),
        };

        entry.failure_count += 1;

        if entry.failure_count >= MAX_FAILURES_PER_CREDENTIAL {
            entry.disabled = true;
            entry.disabled_reason = Some(DisabledReason::TooManyFailures);

            // 切换到下一个可用凭据
            if let Some(next) = entries
                .iter()
                .filter(|e| !e.disabled)
                .min_by_key(|e| e.credentials.priority)
            {
                *current_id = next.id;
            }
        }

        entries.iter().any(|e| !e.disabled)
    }

    /// 报告 API 调用成功
    pub fn report_success(&self, id: u64) {
        let mut entries = self.entries.lock();
        if let Some(entry) = entries.iter_mut().find(|e| e.id == id) {
            entry.failure_count = 0;
        }
    }

    // ========== Admin API 方法 ==========

    /// 设置凭据禁用状态
    pub fn set_disabled(&self, id: u64, disabled: bool) -> anyhow::Result<()> {
        {
            let mut entries = self.entries.lock();
            let entry = entries
                .iter_mut()
                .find(|e| e.id == id)
                .ok_or_else(|| anyhow::anyhow!("凭据不存在: {}", id))?;

            entry.disabled = disabled;
            if !disabled {
                entry.failure_count = 0;
                entry.disabled_reason = None;
            } else {
                entry.disabled_reason = Some(DisabledReason::Manual);
            }
        }
        self.persist_credentials()?;
        Ok(())
    }

    /// 设置凭据优先级
    pub fn set_priority(&self, id: u64, priority: u32) -> anyhow::Result<()> {
        {
            let mut entries = self.entries.lock();
            let entry = entries
                .iter_mut()
                .find(|e| e.id == id)
                .ok_or_else(|| anyhow::anyhow!("凭据不存在: {}", id))?;
            entry.credentials.priority = priority;
        }
        self.select_highest_priority();
        self.persist_credentials()?;
        Ok(())
    }

    /// 重置失败计数并重新启用
    pub fn reset_and_enable(&self, id: u64) -> anyhow::Result<()> {
        {
            let mut entries = self.entries.lock();
            let entry = entries
                .iter_mut()
                .find(|e| e.id == id)
                .ok_or_else(|| anyhow::anyhow!("凭据不存在: {}", id))?;
            entry.failure_count = 0;
            entry.disabled = false;
            entry.disabled_reason = None;
        }
        self.persist_credentials()?;
        Ok(())
    }

    /// 添加新凭据
    pub async fn add_credential(&self, new_cred: KiroCredentials) -> anyhow::Result<u64> {
        // 验证并刷新 Token
        let mut validated_cred = refresh_token(&new_cred, &self.config, self.proxy.as_ref()).await?;

        // 分配新 ID
        let new_id = {
            let entries = self.entries.lock();
            entries.iter().map(|e| e.id).max().unwrap_or(0) + 1
        };

        validated_cred.id = Some(new_id);

        {
            let mut entries = self.entries.lock();
            entries.push(CredentialEntry {
                id: new_id,
                credentials: validated_cred,
                failure_count: 0,
                disabled: false,
                disabled_reason: None,
            });
        }

        self.persist_credentials()?;
        Ok(new_id)
    }

    /// 删除凭据（必须先禁用）
    pub fn delete_credential(&self, id: u64) -> anyhow::Result<()> {
        {
            let mut entries = self.entries.lock();
            let entry = entries
                .iter()
                .find(|e| e.id == id)
                .ok_or_else(|| anyhow::anyhow!("凭据不存在: {}", id))?;

            if !entry.disabled {
                anyhow::bail!("只能删除已禁用的凭据");
            }

            entries.retain(|e| e.id != id);
        }

        self.select_highest_priority();
        self.persist_credentials()?;
        Ok(())
    }

    fn select_highest_priority(&self) {
        let entries = self.entries.lock();
        let mut current_id = self.current_id.lock();

        if let Some(best) = entries
            .iter()
            .filter(|e| !e.disabled)
            .min_by_key(|e| e.credentials.priority)
        {
            *current_id = best.id;
        }
    }
}
```

### A.2 Admin API 处理器 (handlers.rs)

```rust
// kiro.rs/src/admin/handlers.rs

use axum::{
    Json,
    extract::{Path, State},
    response::IntoResponse,
};

/// GET /api/admin/credentials - 获取所有凭据状态
pub async fn get_all_credentials(State(state): State<AdminState>) -> impl IntoResponse {
    let response = state.service.get_all_credentials();
    Json(response)
}

/// POST /api/admin/credentials/:id/disabled - 设置禁用状态
pub async fn set_credential_disabled(
    State(state): State<AdminState>,
    Path(id): Path<u64>,
    Json(payload): Json<SetDisabledRequest>,
) -> impl IntoResponse {
    match state.service.set_disabled(id, payload.disabled) {
        Ok(_) => {
            let action = if payload.disabled { "禁用" } else { "启用" };
            Json(SuccessResponse::new(format!("凭据 #{} 已{}", id, action))).into_response()
        }
        Err(e) => (e.status_code(), Json(e.into_response())).into_response(),
    }
}

/// POST /api/admin/credentials/:id/priority - 设置优先级
pub async fn set_credential_priority(
    State(state): State<AdminState>,
    Path(id): Path<u64>,
    Json(payload): Json<SetPriorityRequest>,
) -> impl IntoResponse {
    match state.service.set_priority(id, payload.priority) {
        Ok(_) => Json(SuccessResponse::new(format!(
            "凭据 #{} 优先级已设置为 {}", id, payload.priority
        ))).into_response(),
        Err(e) => (e.status_code(), Json(e.into_response())).into_response(),
    }
}

/// POST /api/admin/credentials/:id/reset - 重置失败计数
pub async fn reset_failure_count(
    State(state): State<AdminState>,
    Path(id): Path<u64>,
) -> impl IntoResponse {
    match state.service.reset_and_enable(id) {
        Ok(_) => Json(SuccessResponse::new(format!(
            "凭据 #{} 已重置并启用", id
        ))).into_response(),
        Err(e) => (e.status_code(), Json(e.into_response())).into_response(),
    }
}

/// GET /api/admin/credentials/:id/balance - 获取余额
pub async fn get_credential_balance(
    State(state): State<AdminState>,
    Path(id): Path<u64>,
) -> impl IntoResponse {
    match state.service.get_balance(id).await {
        Ok(response) => Json(response).into_response(),
        Err(e) => (e.status_code(), Json(e.into_response())).into_response(),
    }
}

/// POST /api/admin/credentials - 添加新凭据
pub async fn add_credential(
    State(state): State<AdminState>,
    Json(payload): Json<AddCredentialRequest>,
) -> impl IntoResponse {
    match state.service.add_credential(payload).await {
        Ok(response) => Json(response).into_response(),
        Err(e) => (e.status_code(), Json(e.into_response())).into_response(),
    }
}

/// DELETE /api/admin/credentials/:id - 删除凭据
pub async fn delete_credential(
    State(state): State<AdminState>,
    Path(id): Path<u64>,
) -> impl IntoResponse {
    match state.service.delete_credential(id) {
        Ok(_) => Json(SuccessResponse::new(format!("凭据 #{} 已删除", id))).into_response(),
        Err(e) => (e.status_code(), Json(e.into_response())).into_response(),
    }
}
```

### A.3 HTTP 客户端代理支持 (http_client.rs)

```rust
// kiro.rs/src/http_client.rs

use reqwest::{Client, Proxy};
use std::time::Duration;

/// 代理配置
#[derive(Debug, Clone, Default)]
pub struct ProxyConfig {
    /// 代理地址，支持 http/https/socks5
    pub url: String,
    /// 代理认证用户名
    pub username: Option<String>,
    /// 代理认证密码
    pub password: Option<String>,
}

impl ProxyConfig {
    pub fn new(url: impl Into<String>) -> Self {
        Self {
            url: url.into(),
            username: None,
            password: None,
        }
    }

    pub fn with_auth(mut self, username: impl Into<String>, password: impl Into<String>) -> Self {
        self.username = Some(username.into());
        self.password = Some(password.into());
        self
    }
}

/// 构建 HTTP Client
pub fn build_client(proxy: Option<&ProxyConfig>, timeout_secs: u64) -> anyhow::Result<Client> {
    let mut builder = Client::builder().timeout(Duration::from_secs(timeout_secs));

    if let Some(proxy_config) = proxy {
        let mut proxy = Proxy::all(&proxy_config.url)?;

        // 设置代理认证
        if let (Some(username), Some(password)) = (&proxy_config.username, &proxy_config.password) {
            proxy = proxy.basic_auth(username, password);
        }

        builder = builder.proxy(proxy);
        tracing::debug!("HTTP Client 使用代理: {}", proxy_config.url);
    }

    Ok(builder.build()?)
}
```

---

## 附录 B：KiroGate 多租户实现参考

### B.1 认证模式设计 (main.py)

KiroGate 支持两种认证模式：

```python
# KiroGate/main.py

def validate_configuration() -> None:
    """
    验证配置，支持两种认证模式：
    1. 简单模式：需要配置 REFRESH_TOKEN 或 KIRO_CREDS_FILE
    2. 组合模式：只需配置 PROXY_API_KEY，REFRESH_TOKEN 由用户在请求中传递
    """
    # PROXY_API_KEY 是必须的
    if not settings.proxy_api_key:
        errors.append("PROXY_API_KEY is required!")

    has_refresh_token = bool(settings.refresh_token)
    has_creds_file = bool(settings.kiro_creds_file)

    if has_refresh_token or has_creds_file:
        # 简单模式 + 多租户模式
        logger.info("Auth mode: Simple mode + Multi-tenant mode supported")
    else:
        # 仅多租户模式
        logger.info("Auth mode: Multi-tenant only (users must provide PROXY_API_KEY:REFRESH_TOKEN)")
```

### B.2 组合模式认证解析

```python
# 认证格式解析逻辑
def parse_api_key(api_key: str) -> tuple[str, Optional[str]]:
    """
    解析 API Key，支持两种格式：
    1. 简单格式: PROXY_API_KEY
    2. 组合格式: PROXY_API_KEY:REFRESH_TOKEN

    Returns:
        (proxy_key, user_refresh_token)
    """
    if ':' in api_key:
        colon_index = api_key.index(':')
        proxy_key = api_key[:colon_index]
        user_token = api_key[colon_index + 1:]
        return proxy_key, user_token
    return api_key, None
```

### B.3 用户 Token 缓存

```python
# KiroGate/kiro_gateway/auth_cache.py

from functools import lru_cache
from typing import Optional

class AuthCache:
    """用户认证缓存，最多缓存 100 个用户"""

    def __init__(self, max_size: int = 100):
        self._cache = {}
        self._max_size = max_size

    def get_auth_manager(self, refresh_token: str) -> Optional[KiroAuthManager]:
        """获取或创建用户的 AuthManager"""
        cache_key = hash(refresh_token)

        if cache_key in self._cache:
            return self._cache[cache_key]

        # 创建新的 AuthManager
        auth_manager = KiroAuthManager(refresh_token=refresh_token)

        # LRU 淘汰
        if len(self._cache) >= self._max_size:
            oldest_key = next(iter(self._cache))
            del self._cache[oldest_key]

        self._cache[cache_key] = auth_manager
        return auth_manager
```

---

## 附录 C：AIClient-2-API Web UI 参考

### C.1 目录结构

```
AIClient-2-API/static/
├── index.html          # 主页面
├── login.html          # 登录页面
├── app/                # 前端 JS 模块
│   ├── auth.js         # 认证逻辑
│   ├── config-manager.js
│   ├── provider-manager.js
│   ├── usage-manager.js
│   └── ...
└── components/         # UI 组件
    ├── header.html
    ├── sidebar.html
    └── ...
```

### C.2 核心功能模块

AIClient-2-API 的 Web UI 包含以下功能：

1. **Dashboard** - 系统概览、路由示例、客户端配置指南
2. **Configuration** - 实时参数修改、多提供商配置
3. **Provider Pools** - 监控活跃连接、健康统计、启用/禁用管理
4. **Config Files** - OAuth 凭证管理、文件操作
5. **Real-time Logs** - 实时日志显示、管理控制

### C.3 登录认证

```javascript
// AIClient-2-API/static/app/auth.js

const AUTH_CONFIG = {
    passwordFile: 'pwd',
    defaultPassword: 'admin123',
    sessionKey: 'aiclient2api_session'
};

async function login(password) {
    const response = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password })
    });

    if (response.ok) {
        const { token } = await response.json();
        sessionStorage.setItem(AUTH_CONFIG.sessionKey, token);
        return true;
    }
    return false;
}
```

---

## 附录 D：环境变量配置汇总

### kiro2api 现有配置

```bash
# 认证配置
KIRO_AUTH_TOKEN='[{"auth":"Social","refreshToken":"xxx"}]'
# 或文件路径
KIRO_AUTH_TOKEN=/path/to/auth_config.json

# API 认证
KIRO_CLIENT_TOKEN=your-api-key

# 服务配置
PORT=8080
LOG_LEVEL=debug
LOG_FORMAT=text
GIN_MODE=debug
```

### 建议新增配置

```bash
# Admin API 认证密钥
ADMIN_API_KEY=your-admin-secret-key

# 代理配置
PROXY_URL=http://127.0.0.1:7890
# 或 SOCKS5
PROXY_URL=socks5://127.0.0.1:1080

# 多租户模式（可选）
MULTI_TENANT_ENABLED=true
```
