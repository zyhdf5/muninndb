package engine

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/oklog/ulid/v2"
	"github.com/scrypster/muninndb/codedb/engine/erf"
	"github.com/scrypster/muninndb/codedb/engine/keys"
)

// recentActiveCacheTTL 表示 RecentActive 结果在内存中的缓存时间。
const recentActiveCacheTTL = 1 * time.Second

// RecentActive 返回指定 vault 中相关性最高的前 topK 个 engram ID。
func (ps *PebbleStore) RecentActive(ctx context.Context, wsPrefix [8]byte, topK int) ([]ULID, error) {
	if topK <= 0 {
		return nil, nil
	}

	now := time.Now().UnixNano()
	if v, ok := ps.recentActiveCache.Load(wsPrefix); ok {
		entry := v.(*recentActiveCacheEntry)
		if now < entry.expires {
			if len(entry.ids) > topK {
				return entry.ids[:topK], nil
			}
			return entry.ids, nil
		}
	}

	lower := make([]byte, 1+8+1+16)
	lower[0] = 0x10
	copy(lower[1:9], wsPrefix[:])

	upper := make([]byte, 1+8+1+16)
	upper[0] = 0x10
	copy(upper[1:9], wsPrefix[:])
	upper[9] = 0xFF
	for i := 10; i < len(upper); i++ {
		upper[i] = 0xFF
	}

	iter, err := ps.pebbleReader(ctx).NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, fmt.Errorf("创建相关性桶迭代器失败: %w", err)
	}
	defer iter.Close()

	ids := make([]ULID, 0, topK)
	seen := make(map[ULID]struct{}, topK)
	for ok := iter.First(); ok && len(ids) < topK; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		k := iter.Key()
		if len(k) < 26 {
			continue
		}
		var id ULID
		copy(id[:], k[10:26])
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("扫描相关性桶失败: %w", err)
	}

	ps.recentActiveCache.Store(wsPrefix, &recentActiveCacheEntry{
		ids:     append([]ULID(nil), ids...),
		expires: time.Now().Add(recentActiveCacheTTL).UnixNano(),
	})
	return ids, nil
}

// ListByState 使用状态二级索引返回指定状态的 engram ID。
func (ps *PebbleStore) ListByState(ctx context.Context, wsPrefix [8]byte, state LifecycleState, limit int) ([]ULID, error) {
	if limit <= 0 {
		limit = 50
	}
	lower := keys.StateIndexKey(wsPrefix, uint8(state), [16]byte{})
	upper := keys.StateIndexKey(wsPrefix, uint8(state)+1, [16]byte{})

	iter, err := ps.pebbleReader(ctx).NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, fmt.Errorf("创建状态索引迭代器失败: %w", err)
	}
	defer iter.Close()

	ids := make([]ULID, 0, limit)
	for ok := iter.First(); ok && len(ids) < limit; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		k := iter.Key()
		if len(k) < 26 {
			continue
		}
		var id ULID
		copy(id[:], k[10:26])
		ids = append(ids, id)
	}
	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("扫描状态索引失败: %w", err)
	}
	return ids, nil
}

// ulidMinFromTime 构造某个时间点的最小 ULID。
func ulidMinFromTime(t time.Time) [16]byte {
	ms := ulid.Timestamp(t)
	var id ulid.ULID
	binary.BigEndian.PutUint32(id[0:4], uint32(ms>>16))
	binary.BigEndian.PutUint16(id[4:6], uint16(ms))
	return [16]byte(id)
}

// ulidMaxFromTime 构造某个时间点的最大 ULID。
func ulidMaxFromTime(t time.Time) [16]byte {
	ms := ulid.Timestamp(t)
	var id ulid.ULID
	binary.BigEndian.PutUint32(id[0:4], uint32(ms>>16))
	binary.BigEndian.PutUint16(id[4:6], uint16(ms))
	for i := 6; i < 16; i++ {
		id[i] = 0xFF
	}
	return [16]byte(id)
}

// EngramsByCreatedSince 返回 since 之后创建的 engram（按时间升序）。
func (ps *PebbleStore) EngramsByCreatedSince(ctx context.Context, wsPrefix [8]byte, since time.Time, offset, limit int) ([]*Engram, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	lower := keys.EngramKey(wsPrefix, ulidMinFromTime(since))
	upperWS, err := keys.IncrementWSPrefix(wsPrefix)
	if err != nil {
		return nil, fmt.Errorf("计算 vault 上界失败: %w", err)
	}
	upper := make([]byte, 1+8)
	upper[0] = 0x01
	copy(upper[1:9], upperWS[:])

	iter, err := ps.pebbleReader(ctx).NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, fmt.Errorf("创建时间范围迭代器失败: %w", err)
	}
	defer iter.Close()

	out := make([]*Engram, 0, limit)
	skipped := 0
	for ok := iter.First(); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		k := iter.Key()
		if len(k) < 25 {
			continue
		}
		if skipped < offset {
			skipped++
			continue
		}
		meta, concept, err := erf.DecodeMetaConcept(iter.Value())
		if err != nil {
			continue
		}
		var id ULID
		copy(id[:], k[9:25])
		out = append(out, &Engram{ID: id, Concept: concept, CreatedAt: meta.CreatedAt})
		if len(out) >= limit {
			break
		}
	}
	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("扫描时间范围失败: %w", err)
	}
	return out, nil
}

// CountEngramsByDay 统计指定时间区间内每天的 engram 数量（UTC）。
func (ps *PebbleStore) CountEngramsByDay(ctx context.Context, wsPrefix [8]byte, since, until time.Time) (map[string]int64, error) {
	lower := keys.EngramKey(wsPrefix, ulidMinFromTime(since))
	upper := append(keys.EngramKey(wsPrefix, ulidMaxFromTime(until)), 0x00)

	iter, err := ps.pebbleReader(ctx).NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, fmt.Errorf("创建按天统计迭代器失败: %w", err)
	}
	defer iter.Close()

	counts := make(map[string]int64)
	for ok := iter.First(); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		k := iter.Key()
		if len(k) < 25 {
			continue
		}
		ms := uint64(binary.BigEndian.Uint32(k[9:13]))<<16 | uint64(binary.BigEndian.Uint16(k[13:15]))
		t := time.Unix(int64(ms/1000), int64(ms%1000)*1e6).UTC()
		counts[t.Format("2006-01-02")]++
	}
	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("扫描按天统计失败: %w", err)
	}
	return counts, nil
}

// EngramIDsByCreatedRange 返回 [since, until] 区间内的 engram ID。
func (ps *PebbleStore) EngramIDsByCreatedRange(ctx context.Context, wsPrefix [8]byte, since, until time.Time) ([]ULID, error) {
	minID := ulidMinFromTime(since)
	maxID := ulidMaxFromTime(until)
	lower := keys.EngramKey(wsPrefix, minID)

	maxPlusOne := maxID
	for i := 15; i >= 0; i-- {
		maxPlusOne[i]++
		if maxPlusOne[i] != 0 {
			break
		}
	}
	upper := keys.EngramKey(wsPrefix, maxPlusOne)

	iter, err := ps.pebbleReader(ctx).NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, fmt.Errorf("创建创建时间范围迭代器失败: %w", err)
	}
	defer iter.Close()

	ids := make([]ULID, 0, 128)
	for ok := iter.First(); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		k := iter.Key()
		if len(k) < 25 {
			continue
		}
		var id ULID
		copy(id[:], k[9:25])
		ids = append(ids, id)
	}
	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("扫描创建时间范围失败: %w", err)
	}
	return ids, nil
}

// LowestRelevanceIDs 返回相关性最低的前 n 个 engram ID。
func (ps *PebbleStore) LowestRelevanceIDs(ctx context.Context, wsPrefix [8]byte, n int) ([]ULID, error) {
	if n <= 0 {
		return nil, nil
	}

	lower := make([]byte, 1+8+1+16)
	lower[0] = 0x10
	copy(lower[1:9], wsPrefix[:])

	upper := make([]byte, 1+8+1+16)
	upper[0] = 0x10
	copy(upper[1:9], wsPrefix[:])
	upper[9] = 0xFF
	for i := 10; i < len(upper); i++ {
		upper[i] = 0xFF
	}

	iter, err := ps.pebbleReader(ctx).NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, fmt.Errorf("创建低相关性迭代器失败: %w", err)
	}
	defer iter.Close()

	ids := make([]ULID, 0, n)
	seen := make(map[ULID]struct{}, n)
	for ok := iter.Last(); ok && len(ids) < n; ok = iter.Prev() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		k := iter.Key()
		if len(k) < 26 {
			continue
		}
		var id ULID
		copy(id[:], k[10:26])
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("扫描低相关性桶失败: %w", err)
	}
	return ids, nil
}
