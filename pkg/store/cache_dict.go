package store

import (
	"fmt"
	"sync"
	"time"
)

// CacheDict 临时缓存字典（内存实现）
type CacheDict struct {
	data map[string]*cacheItem
	mu   sync.RWMutex
}

type cacheItem struct {
	value      interface{}
	expiration time.Time
}

// NewCacheDict 创建新的缓存字典
func NewCacheDict() *CacheDict {
	dict := &CacheDict{
		data: make(map[string]*cacheItem),
	}
	
	// 启动清理goroutine
	go dict.cleanup()
	
	return dict
}

// Set 设置缓存（带过期时间）
func (c *CacheDict) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.data[key] = &cacheItem{
		value:      value,
		expiration: time.Now().Add(ttl),
	}
}

// Get 获取缓存
func (c *CacheDict) Get(key string, dest interface{}) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	item, ok := c.data[key]
	if !ok {
		return fmt.Errorf("键不存在: %s", key)
	}
	
	// 检查是否过期
	if time.Now().After(item.expiration) {
		return fmt.Errorf("缓存已过期: %s", key)
	}
	
	// 直接赋值（假设dest是指针类型）
	switch v := dest.(type) {
	case *map[string]interface{}:
		if data, ok := item.value.(map[string]interface{}); ok {
			*v = data
		} else {
			return fmt.Errorf("类型不匹配")
		}
	default:
		return fmt.Errorf("不支持的目标类型")
	}
	
	return nil
}

// Delete 删除缓存
func (c *CacheDict) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	delete(c.data, key)
}

// cleanup 定期清理过期缓存
func (c *CacheDict) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, item := range c.data {
			if now.After(item.expiration) {
				delete(c.data, key)
			}
		}
		c.mu.Unlock()
	}
}

// 全局绑定缓存实例
var sharedBindingCache *CacheDict

// GetBindingCache 获取共享的绑定缓存实例
func GetBindingCache() *CacheDict {
	if sharedBindingCache == nil {
		sharedBindingCache = NewCacheDict()
	}
	return sharedBindingCache
}
