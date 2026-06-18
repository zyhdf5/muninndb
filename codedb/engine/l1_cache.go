package engine

import (
	"encoding/hex"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// L1Cache 是最近访问 engram 的热缓存。
// 缓存键按 vault 前缀隔离，不同 vault 的 engram 不会互相污染。
// 使用 sync.Map 实现无锁并发读取（常见路径）。
type L1Cache struct {
	data    sync.Map     // string (vaultHex+":"+ULID) → *cacheEntry
	count   atomic.Int64 // 近似条目计数
	maxSize int          // 最大缓存条目数
}

// cacheKeyFor 返回 (vault, id) 对的复合缓存键。
// 格式: 16 字符 hex vault 前缀 + ":" + ULID 字符串 = 43 字符。
func cacheKeyFor(ws [8]byte, id ULID) string {
	return hex.EncodeToString(ws[:]) + ":" + id.String()
}

// cacheEntry 包装 engram 并附带访问跟踪。
type cacheEntry struct {
	eng        *Engram
	lastAccess atomic.Int64 // Unix 纳秒
}

// NewL1Cache 创建指定最大容量的 L1 缓存。
func NewL1Cache(maxSize int) *L1Cache {
	if maxSize <= 0 {
		maxSize = 10000 // 默认值
	}
	return &L1Cache{
		maxSize: maxSize,
	}
}

// Get 从缓存中检索 engram。
// vault 前缀包含在查找键中，确保不同 vault 的 engram 不会互相混淆。
func (c *L1Cache) Get(ws [8]byte, id ULID) (*Engram, bool) {
	val, ok := c.data.Load(cacheKeyFor(ws, id))
	if !ok {
		return nil, false
	}
	entry := val.(*cacheEntry)
	entry.lastAccess.Store(time.Now().UnixNano())
	return entry.eng, true
}

// Set 将 engram 存入给定 vault 前缀的缓存。
func (c *L1Cache) Set(ws [8]byte, id ULID, eng *Engram) {
	entry := &cacheEntry{eng: eng}
	entry.lastAccess.Store(time.Now().UnixNano())
	c.data.Store(cacheKeyFor(ws, id), entry)
	newCount := c.count.Add(1)
	if newCount > int64(c.maxSize) {
		c.evict()
	}
}

// Delete 从缓存中移除指定 vault 的 engram。
// 使用 LoadAndDelete 防止并发淘汰导致计数器漂移。
func (c *L1Cache) Delete(ws [8]byte, id ULID) {
	if _, loaded := c.data.LoadAndDelete(cacheKeyFor(ws, id)); loaded {
		c.count.Add(-1)
	}
}

// Len 返回缓存中的近似条目数。
func (c *L1Cache) Len() int {
	return int(c.count.Load())
}

// LastAccessNs 返回 (vault, engram) 对的最后访问时间（Unix 纳秒）。
// 不在缓存中时返回 0。
func (c *L1Cache) LastAccessNs(ws [8]byte, id ULID) int64 {
	val, ok := c.data.Load(cacheKeyFor(ws, id))
	if !ok {
		return 0
	}
	return val.(*cacheEntry).lastAccess.Load()
}

// DeleteByVault 删除指定 vault 工作空间前缀的所有缓存条目。
// 使用 LoadAndDelete 原子操作保证并发安全。
func (c *L1Cache) DeleteByVault(ws [8]byte) {
	prefix := hex.EncodeToString(ws[:]) + ":"
	c.data.Range(func(k, _ any) bool {
		if strings.HasPrefix(k.(string), prefix) {
			if _, loaded := c.data.LoadAndDelete(k); loaded {
				c.count.Add(-1)
			}
		}
		return true
	})
}

// evict 使用近似 LRU 策略淘汰一个条目。
// 采样 64 个随机条目，淘汰 lastAccess 最旧的。
func (c *L1Cache) evict() {
	const sampleSize = 64
	var oldest *cacheEntry
	var oldestID string
	var oldestTime int64 = time.Now().UnixNano()

	count := 0
	c.data.Range(func(key, val any) bool {
		if count >= sampleSize {
			return false
		}
		count++
		entry := val.(*cacheEntry)
		lastAccess := entry.lastAccess.Load()
		if lastAccess < oldestTime {
			oldestTime = lastAccess
			oldest = entry
			oldestID = key.(string)
		}
		return true
	})

	if oldest != nil && oldestID != "" {
		if _, loaded := c.data.LoadAndDelete(oldestID); loaded {
			c.count.Add(-1)
		}
	}
}
