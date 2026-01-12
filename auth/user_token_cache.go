package auth

import (
	"kiro2api/logger"
	"kiro2api/types"
	"sync"
	"time"
)

// UserTokenCache 用户 Token 缓存（多租户模式）
// 最多缓存 maxSize 个用户的 Token，使用 LRU 淘汰策略
type UserTokenCache struct {
	mu       sync.RWMutex
	cache    map[string]*userTokenEntry
	order    []string // LRU 顺序
	maxSize  int
}

type userTokenEntry struct {
	token     types.TokenInfo
	createdAt time.Time
}

var (
	globalUserTokenCache *UserTokenCache
	userTokenCacheOnce   sync.Once
)

// GetUserTokenCache 获取全局用户 Token 缓存
func GetUserTokenCache() *UserTokenCache {
	userTokenCacheOnce.Do(func() {
		globalUserTokenCache = &UserTokenCache{
			cache:   make(map[string]*userTokenEntry),
			order:   make([]string, 0, 100),
			maxSize: 100,
		}
	})
	return globalUserTokenCache
}

// GetOrRefresh 获取用户 Token，如果不存在或已过期则刷新
func (c *UserTokenCache) GetOrRefresh(refreshToken string) (types.TokenInfo, error) {
	cacheKey := hashRefreshToken(refreshToken)

	c.mu.RLock()
	entry, exists := c.cache[cacheKey]
	c.mu.RUnlock()

	// 检查缓存是否有效
	if exists && !entry.token.IsExpired() {
		// 更新 LRU 顺序
		c.touchKey(cacheKey)
		logger.Debug("使用缓存的用户 Token")
		return entry.token, nil
	}

	// 刷新 Token
	logger.Debug("刷新用户 Token")
	token, err := refreshSocialToken(refreshToken)
	if err != nil {
		return types.TokenInfo{}, err
	}

	// 存入缓存
	c.mu.Lock()
	defer c.mu.Unlock()

	// LRU 淘汰
	if len(c.cache) >= c.maxSize && !exists {
		c.evictOldest()
	}

	c.cache[cacheKey] = &userTokenEntry{
		token:     token,
		createdAt: time.Now(),
	}

	// 更新 LRU 顺序
	if !exists {
		c.order = append(c.order, cacheKey)
	}

	return token, nil
}

// touchKey 更新 LRU 顺序（将 key 移到末尾）
func (c *UserTokenCache) touchKey(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, k := range c.order {
		if k == key {
			// 移到末尾
			c.order = append(c.order[:i], c.order[i+1:]...)
			c.order = append(c.order, key)
			break
		}
	}
}

// evictOldest 淘汰最旧的条目（调用前需持有写锁）
func (c *UserTokenCache) evictOldest() {
	if len(c.order) == 0 {
		return
	}

	oldestKey := c.order[0]
	c.order = c.order[1:]
	delete(c.cache, oldestKey)
	logger.Debug("淘汰最旧的用户 Token 缓存")
}

// hashRefreshToken 对 RefreshToken 进行哈希（用作缓存 key）
func hashRefreshToken(token string) string {
	// 使用前 32 字符作为 key（足够唯一且不暴露完整 token）
	if len(token) > 32 {
		return token[:32]
	}
	return token
}

// Size 返回缓存大小
func (c *UserTokenCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}
