package engine

import (
	"hash/fnv"
	"strings"
	"sync"

	"golang.org/x/text/unicode/norm"
)

// mergeGuardStripes 是合并守卫的条纹数。256 个条纹对于任意两个随机实体名称
// 产生约 0.4% 的伪共享概率，对于 MergeEntity 这种不频繁的管理操作而言可忽略。
const mergeGuardStripes = 256

// mergeGuard 用于串行化同时操作相同实体的并发 MergeEntity 调用。
//
// 为何独立于存储层的 entityLocks？
// 存储层在 UpsertEntityRecord 内部获取实体条纹锁以防止 TOCTOU 竞争。
// 如果 MergeEntity 在整个持续时间内持有同一条纹锁，然后在内部调用
// UpsertEntityRecord，就会发生死锁——sync.Mutex 不可重入。
// mergeGuard 使用独立的条纹数组，永远不会与存储层锁交互。
//
// 并发保证：
//   - MergeEntity(A→B) 和 MergeEntity(A→C) 被串行化，因为两者都获取 stripe(A)。
//     一个阻塞直到另一个完成，防止 A 的 engram 被分散到两个目标。
//   - MergeEntity(A→B) 和 MergeEntity(B→C) 被串行化，因为它们共享 stripe(B)。
//   - MergeEntity(A→B) 和 MergeEntity(A→B)（重复调用）以相同方式串行化。
//   - MergeEntity(A→B) 和 MergeEntity(B→A) 以相同的规范（升序）顺序获取
//     相同的两个条纹锁，因此不会死锁。
//   - 不相关的合并操作可以并发进行，除非碰巧共享条纹
//     （每个实体 1/256 的概率，可接受的伪共享）。
type mergeGuard struct {
	mu [mergeGuardStripes]sync.Mutex
}

// stripeIndex 返回实体名称对应的条纹索引。
// 使用与存储层 getEntityLock 相同的 NFKC 规范化 + 小写 + 裁剪管道。
// 一致的规范化确保仅在 Unicode 表示上不同的实体名称
// （例如 "café" 与 "cafe\u0301"）映射到相同的条纹。
func (g *mergeGuard) stripeIndex(name string) uint32 {
	normalized := strings.ToLower(strings.TrimSpace(norm.NFKC.String(name)))
	h := fnv.New32a()
	h.Write([]byte(normalized))
	return h.Sum32() % mergeGuardStripes
}

// Lock 按升序条纹索引获取 entityA 和 entityB 的条纹锁以防止死锁。
// 如果两个名称哈希到同一条纹，则只锁定一次。
func (g *mergeGuard) Lock(entityA, entityB string) {
	idxA := g.stripeIndex(entityA)
	idxB := g.stripeIndex(entityB)

	switch {
	case idxA == idxB:
		g.mu[idxA].Lock()
	case idxA < idxB:
		g.mu[idxA].Lock()
		g.mu[idxB].Lock()
	default:
		g.mu[idxB].Lock()
		g.mu[idxA].Lock()
	}
}

// Unlock 释放之前通过 Lock 获取的条纹锁。
// 必须使用与前一个 Lock 调用相同的参数。
func (g *mergeGuard) Unlock(entityA, entityB string) {
	idxA := g.stripeIndex(entityA)
	idxB := g.stripeIndex(entityB)
	g.mu[idxA].Unlock()
	if idxA != idxB {
		g.mu[idxB].Unlock()
	}
}
