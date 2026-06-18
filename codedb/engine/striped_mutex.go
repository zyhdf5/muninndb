package engine

import (
	"hash/fnv"
	"sync"
)

const lockStripes = 256

// stripedMutex 是固定大小的互斥锁数组，用于锁分片。
// 替代无界的 sync.Map 锁池。内存使用恒定：lockStripes × sizeof(sync.Mutex) ≈ 6 KB。
//
// 不同键可能映射到同一条纹（假共享），但在 256 条纹下概率极低，
// 且安全 —— 竞争只会导致短暂等待，不会造成数据损坏。
type stripedMutex struct {
	mu [lockStripes]sync.Mutex
}

// For 使用 FNV-32a 哈希返回给定字节键对应的互斥锁。
func (s *stripedMutex) For(key []byte) *sync.Mutex {
	h := fnv.New32a()
	h.Write(key)
	return &s.mu[h.Sum32()%lockStripes]
}
