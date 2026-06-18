package engine

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/pebble"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/scrypster/muninndb/codedb/engine/keys"
)

// 编译期接口断言：PebbleStore 必须实现 EngineStore。
var _ EngineStore = (*PebbleStore)(nil)

// PebbleStoreConfig 是 PebbleStore 的创建时配置。
type PebbleStoreConfig struct {
	CacheSize int // L1 缓存最大条目数。0 表示不缓存。
}

// PebbleStore 是基于 Pebble 的 EngineStore 具体实现。
type PebbleStore struct {
	db    *pebble.DB
	cache *L1Cache

	vaultCounters sync.Map // [8]byte → *vaultCounter

	// assocCache: [24]byte (wsPrefix[8]+engramID[16]) → *assocCacheEntry
	// 缓存前向关联列表，避免对热门 engram 重复扫描 Pebble SSTable。
	// 写入关联时失效。限 500,000 条，2 秒 TTL。
	assocCache *expirable.LRU[[24]byte, *assocCacheEntry]

	// metaCache: [16]byte (engramID) → *EngramMeta
	// 热路径元数据读缓存。GetMetadata 首次读取 Pebble 后填充。
	// UpdateMetadata/WriteEngram 时失效。限 100,000 条。
	metaCache *lru.Cache[[16]byte, *EngramMeta]

	// vaultPrefixCache: vault 名称 → [8]byte 工作空间前缀
	// 消除 ResolveVaultPrefix 的 Pebble.Get。限 10,000 条。
	vaultPrefixCache *lru.Cache[string, [8]byte]

	// vaultNameWritten: [8]byte → struct{} — 跟踪已持久化名称的 vault，
	// 消除 WriteVaultName 的存在性检查。
	vaultNameWritten sync.Map

	// recentActiveCache: [8]byte → *recentActiveCacheEntry
	// 按 vault 缓存 RecentActive 结果，100ms TTL。
	recentActiveCache sync.Map

	closeOnce sync.Once

	// entityLocks 和 coOccurrenceLocks 使用固定大小条纹互斥锁数组
	// （256 × sizeof(sync.Mutex) ≈ 6 KB），替代无界 sync.Map 锁池。
	entityLocks       stripedMutex // UpsertEntityRecord 的 TOCTOU 保护
	coOccurrenceLocks stripedMutex // IncrementEntityCoOccurrence 的 TOCTOU 保护
}

// assocCacheEntry 缓存的关联列表。
// TTL 由 expirable.LRU 管理（2 秒），无需逐条过期字段。
type assocCacheEntry struct {
	assocs []Association
}

const assocCacheTTL = 2 * time.Second

// recentActiveCacheEntry 是 RecentActive 的 TTL 缓存条目。
type recentActiveCacheEntry struct {
	ids     []ULID
	expires int64 // Unix 纳秒
}

// vaultCounter 跟踪 vault 的 engram 计数。
// sync.Once 确保每个 vault 每进程生命周期只从 Pebble 种子一次。
type vaultCounter struct {
	once  sync.Once
	count atomic.Int64
}

// getOrInitCounter 返回 vault 计数器，首次访问时从 Pebble 初始化。
func (ps *PebbleStore) getOrInitCounter(ctx context.Context, wsPrefix [8]byte) *vaultCounter {
	if v, ok := ps.vaultCounters.Load(wsPrefix); ok {
		return v.(*vaultCounter)
	}
	vc := &vaultCounter{}
	actual, _ := ps.vaultCounters.LoadOrStore(wsPrefix, vc)
	loaded := actual.(*vaultCounter)
	loaded.once.Do(func() {
		countKey := keys.VaultCountKey(wsPrefix)
		val, err := Get(ps.db, countKey)
		if err == nil && len(val) == 8 {
			n := int64(binary.BigEndian.Uint64(val))
			loaded.count.Store(n)
			return
		}
		// 回退到 vault 扫描（首次启动时一次性开销）
		n, _ := ps.countEngramsForVault(ctx, wsPrefix)
		loaded.count.Store(n)
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, uint64(n))
		_ = ps.db.Set(countKey, buf, pebble.NoSync)
	})
	return loaded
}

// countEngramsForVault 扫描 0x01 前缀统计单个 vault 的 engram 数量。
func (ps *PebbleStore) countEngramsForVault(ctx context.Context, wsPrefix [8]byte) (int64, error) {
	lower := keys.EngramKey(wsPrefix, [16]byte{})
	upperWS := wsPrefix
	for i := 7; i >= 0; i-- {
		upperWS[i]++
		if upperWS[i] != 0 {
			break
		}
	}
	upper := make([]byte, 1+8)
	upper[0] = 0x01
	copy(upper[1:9], upperWS[:])
	iter, err := ps.db.NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return 0, err
	}
	defer iter.Close()
	var count int64
	for valid := iter.First(); valid; valid = iter.Next() {
		if len(iter.Key()) >= 25 {
			count++
		}
	}
	if err := iter.Error(); err != nil {
		return 0, fmt.Errorf("统计 engram 扫描: %w", err)
	}
	return count, nil
}

// GetVaultCount 返回 vault 当前的 engram 计数。
func (ps *PebbleStore) GetVaultCount(ctx context.Context, wsPrefix [8]byte) int64 {
	return ps.getOrInitCounter(ctx, wsPrefix).count.Load()
}

// NewPebbleStore 创建包装 Pebble 数据库和 L1 缓存的 PebbleStore。
func NewPebbleStore(db *pebble.DB, cfg PebbleStoreConfig) *PebbleStore {
	metaCache, _ := lru.New[[16]byte, *EngramMeta](100_000)
	vaultPrefixCache, _ := lru.New[string, [8]byte](10_000)
	assocCache := expirable.NewLRU[[24]byte, *assocCacheEntry](500_000, nil, assocCacheTTL)
	return &PebbleStore{
		db:               db,
		cache:            NewL1Cache(cfg.CacheSize),
		metaCache:        metaCache,
		vaultPrefixCache: vaultPrefixCache,
		assocCache:       assocCache,
	}
}

// CacheLen 返回 L1 缓存中的条目数。
func (ps *PebbleStore) CacheLen() int {
	return ps.cache.Len()
}

// VaultPrefix 计算 vault 名称的 8 字节 SipHash 前缀。
func (ps *PebbleStore) VaultPrefix(vault string) [8]byte {
	return keys.VaultPrefix(vault)
}

// DiskSize 返回所有 Pebble 数据库文件的磁盘总大小。
func (ps *PebbleStore) DiskSize() int64 {
	return int64(ps.db.Metrics().DiskSpaceUsage())
}

// PebbleMetrics 返回原始 Pebble 指标，用于可观测性和诊断。
func (ps *PebbleStore) PebbleMetrics() *pebble.Metrics {
	return ps.db.Metrics()
}

// Checkpoint 在 destDir 创建 Pebble 检查点（磁盘一致性快照）。
func (ps *PebbleStore) Checkpoint(destDir string) error {
	return ps.db.Checkpoint(destDir)
}

// Close 刷新所有挂起写入并关闭 Pebble 数据库。幂等。
func (ps *PebbleStore) Close() error {
	var closeErr error
	ps.closeOnce.Do(func() {
		closeErr = ps.db.Close()
	})
	return closeErr
}

// persistVaultCount 将 vault 计数器持久化到 Pebble（NoSync）。
func (ps *PebbleStore) persistVaultCount(wsPrefix [8]byte, count int64) {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(count))
	_ = ps.db.Set(keys.VaultCountKey(wsPrefix), buf, pebble.NoSync)
}
