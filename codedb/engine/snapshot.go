package engine

import (
	"context"

	"github.com/cockroachdb/pebble"
)

type snapshotCtxKey struct{}

// ContextWithSnapshot 返回携带 Pebble 快照的 context。
// PebbleStore 的读方法（GetEngrams, GetMetadata, GetAssociations,
// RecentActive 等）会使用此快照进行 Pebble 读取（而非实时 DB），
// 从而在整个激活管道中提供时间点一致性。
func ContextWithSnapshot(ctx context.Context, snap *pebble.Snapshot) context.Context {
	return context.WithValue(ctx, snapshotCtxKey{}, snap)
}

// pebbleReader 从 ctx 中提取快照（如果存在），否则返回实时 DB。
func (ps *PebbleStore) pebbleReader(ctx context.Context) pebble.Reader {
	if snap, ok := ctx.Value(snapshotCtxKey{}).(*pebble.Snapshot); ok && snap != nil {
		return snap
	}
	return ps.db
}

// NewSnapshot 创建 Pebble 快照用于时间点一致性读取。
// 调用者使用完毕后必须关闭返回的快照。
func (ps *PebbleStore) NewSnapshot() *pebble.Snapshot {
	return ps.db.NewSnapshot()
}
