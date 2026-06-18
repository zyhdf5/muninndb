package engine

import (
	"bytes"
	"fmt"

	"github.com/cockroachdb/pebble"
)

// Options 是打开 Pebble 数据库的配置项。
type Options struct {
	MaxOpenFiles          int    // 最大打开文件数
	MemTableSize          uint64 // MemTable 大小
	L0CompactionThreshold int    // L0 压缩阈值
	L0StopWritesThreshold int    // L0 停写阈值
	LBaseMaxBytes         int64  // LBase 最大字节数
	DisableWAL            bool   // 禁用 WAL
}

// DefaultOptions 返回适用于 MuninnDB 的 Pebble 默认配置。
func DefaultOptions() *Options {
	return &Options{
		MaxOpenFiles:          1000,
		MemTableSize:          64 * 1024 * 1024, // 64MB
		L0CompactionThreshold: 4,
		L0StopWritesThreshold: 12,
		LBaseMaxBytes:         256 * 1024 * 1024, // 256MB
		DisableWAL:            false,
	}
}

// OpenPebble 在指定路径打开 Pebble 数据库。
func OpenPebble(path string, opts *Options) (*pebble.DB, error) {
	if opts == nil {
		opts = DefaultOptions()
	}
	pebbleOpts := &pebble.Options{
		MaxOpenFiles:  opts.MaxOpenFiles,
		MemTableSize:  opts.MemTableSize,
		LBaseMaxBytes: opts.LBaseMaxBytes,
		DisableWAL:    opts.DisableWAL,
	}
	pebbleOpts.L0CompactionThreshold = opts.L0CompactionThreshold
	pebbleOpts.L0StopWritesThreshold = opts.L0StopWritesThreshold
	return pebble.Open(path, pebbleOpts)
}

// BatchSet 向批次添加一条 Set 操作。
func BatchSet(batch *pebble.Batch, key, value []byte) {
	batch.Set(key, value, nil)
}

// BatchDelete 向批次添加一条 Delete 操作。
func BatchDelete(batch *pebble.Batch, key []byte) {
	batch.Delete(key, nil)
}

// PrefixIterator 创建一个以指定前缀为边界的迭代器。
func PrefixIterator(db pebble.Reader, prefix []byte) (*pebble.Iterator, error) {
	upper := make([]byte, len(prefix))
	copy(upper, prefix)
	for i := len(upper) - 1; i >= 0; i-- {
		if upper[i] < 0xFF {
			upper[i]++
			break
		}
	}
	// 如果所有字节都是 0xFF，追加 0x00 使上界严格大于前缀
	if bytes.Equal(upper, prefix) {
		upper = append(upper, 0x00)
	}
	iter, err := db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: upper,
	})
	if err != nil {
		return nil, fmt.Errorf("创建前缀迭代器: %w", err)
	}
	return iter, nil
}

// Get 从数据库中读取单个键。
// 键不存在时返回 (nil, nil)。
func Get(r pebble.Reader, key []byte) ([]byte, error) {
	val, closer, err := r.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	defer closer.Close()
	result := make([]byte, len(val))
	copy(result, val)
	return result, nil
}

// MultiGet 从数据库中批量读取多个键。
// 不存在的键在结果切片中对应位置为 nil。
func MultiGet(r pebble.Reader, keys [][]byte) ([][]byte, error) {
	results := make([][]byte, len(keys))
	for i, key := range keys {
		val, closer, err := r.Get(key)
		if err != nil {
			if err == pebble.ErrNotFound {
				results[i] = nil
				continue
			}
			return nil, err
		}
		result := make([]byte, len(val))
		copy(result, val)
		results[i] = result
		closer.Close()
	}
	return results, nil
}
