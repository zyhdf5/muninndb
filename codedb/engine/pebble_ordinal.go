package engine

import (
	"context"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/codedb/engine/keys"
)

// WriteOrdinal 写入 parent-child 的序号。
func (ps *PebbleStore) WriteOrdinal(ctx context.Context, wsPrefix [8]byte, parentID, childID ULID, ordinal int32) error {
	_ = ctx
	if ordinal < 0 {
		return fmt.Errorf("序号不能为负数: %d", ordinal)
	}
	var val [4]byte
	binary.BigEndian.PutUint32(val[:], uint32(ordinal))
	batch := ps.db.NewBatch()
	defer batch.Close()
	if err := batch.Set(keys.OrdinalKey(wsPrefix, [16]byte(parentID), [16]byte(childID)), val[:], nil); err != nil {
		return err
	}
	return batch.Commit(pebble.NoSync)
}

// ReadOrdinal 读取 parent-child 的序号。
func (ps *PebbleStore) ReadOrdinal(ctx context.Context, wsPrefix [8]byte, parentID, childID ULID) (int32, bool, error) {
	val, err := Get(ps.pebbleReader(ctx), keys.OrdinalKey(wsPrefix, [16]byte(parentID), [16]byte(childID)))
	if err != nil {
		return 0, false, err
	}
	if len(val) < 4 {
		return 0, false, nil
	}
	return int32(binary.BigEndian.Uint32(val[:4])), true, nil
}

// DeleteOrdinal 删除 parent-child 的序号键。
func (ps *PebbleStore) DeleteOrdinal(ctx context.Context, wsPrefix [8]byte, parentID, childID ULID) error {
	_ = ctx
	batch := ps.db.NewBatch()
	defer batch.Close()
	if err := batch.Delete(keys.OrdinalKey(wsPrefix, [16]byte(parentID), [16]byte(childID)), nil); err != nil {
		return err
	}
	return batch.Commit(pebble.NoSync)
}

// DeleteEngramOrdinal 删除 engram 关联的序号键。
func (ps *PebbleStore) DeleteEngramOrdinal(ctx context.Context, wsPrefix [8]byte, parentID, childID ULID) error {
	return ps.DeleteOrdinal(ctx, wsPrefix, parentID, childID)
}

// ListChildOrdinals 返回 parent 下全部子节点序号并按序排序。
func (ps *PebbleStore) ListChildOrdinals(ctx context.Context, wsPrefix [8]byte, parentID ULID) ([]OrdinalEntry, error) {
	prefix := keys.OrdinalPrefixForParent(wsPrefix, [16]byte(parentID))
	iter, err := PrefixIterator(ps.pebbleReader(ctx), prefix)
	if err != nil {
		return nil, fmt.Errorf("创建序号扫描迭代器失败: %w", err)
	}
	defer iter.Close()

	out := make([]OrdinalEntry, 0, 16)
	for ok := iter.First(); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		k := iter.Key()
		v := iter.Value()
		if len(k) < 41 || len(v) < 4 {
			continue
		}
		var child ULID
		copy(child[:], k[25:41])
		out = append(out, OrdinalEntry{ChildID: child, Ordinal: int32(binary.BigEndian.Uint32(v[:4]))})
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ordinal < out[j].Ordinal })
	return out, nil
}
