package engine

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/codedb/engine/keys"
)

// WriteLastAccessEntry 更新 0x22 最后访问索引。
func (ps *PebbleStore) WriteLastAccessEntry(ctx context.Context, ws [8]byte, id ULID, prevMillis, newMillis int64) error {
	_ = ctx
	batch := ps.db.NewBatch()
	defer batch.Close()

	if prevMillis != 0 {
		if err := batch.Delete(keys.LastAccessIndexKey(ws, prevMillis, [16]byte(id)), nil); err != nil {
			return fmt.Errorf("删除旧最后访问索引失败: %w", err)
		}
	}
	if err := batch.Set(keys.LastAccessIndexKey(ws, newMillis, [16]byte(id)), nil, nil); err != nil {
		return fmt.Errorf("写入新最后访问索引失败: %w", err)
	}
	return batch.Commit(pebble.NoSync)
}

// ScanLastAccessDesc 按最近访问优先顺序扫描 0x22 索引。
func (ps *PebbleStore) ScanLastAccessDesc(ctx context.Context, ws [8]byte, fn func(id ULID, lastAccessMillis int64) error) error {
	prefix := keys.LastAccessIndexPrefix(ws)
	iter, err := PrefixIterator(ps.pebbleReader(ctx), prefix)
	if err != nil {
		return fmt.Errorf("创建最后访问扫描迭代器失败: %w", err)
	}
	defer iter.Close()

	for ok := iter.First(); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		k := iter.Key()
		if len(k) != 33 {
			continue
		}
		inverted := binary.BigEndian.Uint64(k[9:17])
		millis := int64(^inverted)
		var id ULID
		copy(id[:], k[17:33])
		if err := fn(id, millis); err != nil {
			return err
		}
	}
	return iter.Error()
}

// DeleteLastAccessEntry 删除 engram 的 0x22 索引键。
func (ps *PebbleStore) DeleteLastAccessEntry(ctx context.Context, ws [8]byte, id ULID, lastAccessMillis int64) error {
	_ = ctx
	if lastAccessMillis == 0 {
		return nil
	}
	return ps.db.Delete(keys.LastAccessIndexKey(ws, lastAccessMillis, [16]byte(id)), pebble.NoSync)
}
