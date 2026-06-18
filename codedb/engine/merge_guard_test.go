package engine

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeGuard_StripeIndex_NFKCNormalization(t *testing.T) {
	var g mergeGuard
	// "café" (precomposed) 与 "cafe\u0301" (decomposed) 应映射到同一条纹。
	idx1 := g.stripeIndex("café")
	idx2 := g.stripeIndex("cafe\u0301")
	assert.Equal(t, idx1, idx2, "NFKC 等价的名称应映射到相同条纹")
}

func TestMergeGuard_StripeIndex_CaseInsensitive(t *testing.T) {
	var g mergeGuard
	idx1 := g.stripeIndex("PostgreSQL")
	idx2 := g.stripeIndex("postgresql")
	assert.Equal(t, idx1, idx2, "大小写不同的名称应映射到相同条纹")
}

func TestMergeGuard_StripeIndex_TrimSpace(t *testing.T) {
	var g mergeGuard
	idx1 := g.stripeIndex("  Entity  ")
	idx2 := g.stripeIndex("entity")
	assert.Equal(t, idx1, idx2, "带有前后空格的名称应映射到相同条纹")
}

func TestMergeGuard_StripeIndex_Range(t *testing.T) {
	var g mergeGuard
	for i := 0; i < 1000; i++ {
		idx := g.stripeIndex(fmt.Sprintf("entity-%d", i))
		assert.Less(t, idx, uint32(mergeGuardStripes), "条纹索引应小于 %d", mergeGuardStripes)
	}
}

func TestMergeGuard_SameStripeLockOnce(t *testing.T) {
	var g mergeGuard
	// 寻找两个哈希到同一条纹的名称。
	stripeMap := make(map[uint32]string, mergeGuardStripes)
	var nameA, nameB string
	for i := 0; i < 10_000; i++ {
		name := fmt.Sprintf("collision-search-%d", i)
		idx := g.stripeIndex(name)
		if existing, ok := stripeMap[idx]; ok {
			nameA, nameB = existing, name
			break
		}
		stripeMap[idx] = name
	}
	require.NotEmpty(t, nameA, "应能在 10000 个名称中找到条纹碰撞")

	// 不应死锁。
	g.Lock(nameA, nameB)
	g.Unlock(nameA, nameB)
}

func TestMergeGuard_CanonicalOrderNeverDeadlocks(t *testing.T) {
	var g mergeGuard

	var nameA, nameB string
	for i := 0; i < 256; i++ {
		a := fmt.Sprintf("forward-%d", i)
		b := fmt.Sprintf("reverse-%d", i)
		if g.stripeIndex(a) != g.stripeIndex(b) {
			nameA, nameB = a, b
			break
		}
	}
	require.NotEmpty(t, nameA, "应能找到两个不同条纹的名称")

	done := make(chan struct{}, 2)
	go func() {
		g.Lock(nameA, nameB)
		g.Unlock(nameA, nameB)
		done <- struct{}{}
	}()
	go func() {
		g.Lock(nameB, nameA)
		g.Unlock(nameB, nameA)
		done <- struct{}{}
	}()

	<-done
	<-done
}

func TestMergeGuard_SerialisesConflictingMerges(t *testing.T) {
	var g mergeGuard

	const entityA = "shared-entity"
	const entityB = "target-b"
	const entityC = "target-c"

	counter := 0
	var wg sync.WaitGroup

	for _, target := range []string{entityB, entityC} {
		target := target
		wg.Add(1)
		go func() {
			defer wg.Done()
			g.Lock(entityA, target)
			defer g.Unlock(entityA, target)
			counter++
		}()
	}
	wg.Wait()

	assert.Equal(t, 2, counter, "两个合并操作都应完成")
}

func TestMergeGuard_ConcurrentUnrelatedDoNotBlock(t *testing.T) {
	var g mergeGuard

	type pair struct{ a, b string }
	var p1, p2 pair
	found := false
	for i := 0; i < 256 && !found; i++ {
		for j := i + 1; j < 256 && !found; j++ {
			a1 := fmt.Sprintf("p1-alpha-%d", i)
			b1 := fmt.Sprintf("p1-beta-%d", i)
			a2 := fmt.Sprintf("p2-alpha-%d", j)
			b2 := fmt.Sprintf("p2-beta-%d", j)
			s1a, s1b := g.stripeIndex(a1), g.stripeIndex(b1)
			s2a, s2b := g.stripeIndex(a2), g.stripeIndex(b2)
			if s1a != s2a && s1a != s2b && s1b != s2a && s1b != s2b {
				p1, p2 = pair{a1, b1}, pair{a2, b2}
				found = true
			}
		}
	}
	if !found {
		t.Skip("无法找到两组完全独立的实体对")
	}

	bothInCritical := make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	inCritical := 0

	var wg sync.WaitGroup
	for _, p := range []pair{p1, p2} {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			g.Lock(p.a, p.b)
			defer g.Unlock(p.a, p.b)

			mu.Lock()
			inCritical++
			if inCritical == 2 {
				once.Do(func() { close(bothInCritical) })
			}
			mu.Unlock()

			<-bothInCritical
		}()
	}
	wg.Wait()
}
