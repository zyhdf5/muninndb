package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBFSTraverse_EmptySeeds(t *testing.T) {
	store := newMockStore()
	ws := store.ResolveVaultPrefix(testVault)

	results, err := BFSTraverse(context.Background(), store, ws, nil, 0.0, 3)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestBFSTraverse_SingleSeedNoEdges(t *testing.T) {
	store := newMockStore()
	ws := store.ResolveVaultPrefix(testVault)
	id := NewULID()
	store.addEngram(ws, makeEngram(id, "孤立节点"))

	results, err := BFSTraverse(context.Background(), store, ws, []ULID{id}, 0.0, 3)
	require.NoError(t, err)
	assert.Empty(t, results, "没有出边时结果应为空")
}

func TestBFSTraverse_LinearChain(t *testing.T) {
	store := newMockStore()
	ws := store.ResolveVaultPrefix(testVault)

	ids := multiIDGen(4) // A -> B -> C -> D
	for _, id := range ids {
		store.addEngram(ws, makeEngram(id, "node"))
	}
	for i := 0; i < 3; i++ {
		store.addAssociation(ws, ids[i], Association{
			TargetID: ids[i+1],
			Weight:   0.8,
			RelType:  RelSupports,
		})
	}

	results, err := BFSTraverse(context.Background(), store, ws, []ULID{ids[0]}, 0.0, 5)
	require.NoError(t, err)

	// 应发现 B、C、D（种子 A 不包含在结果中）。
	assert.Len(t, results, 3)

	// 验证跳数和分数衰减。
	for _, r := range results {
		assert.Greater(t, r.Score, 0.0)
		assert.Greater(t, r.HopDepth, 0)
	}
}

func TestBFSTraverse_DepthLimit(t *testing.T) {
	store := newMockStore()
	ws := store.ResolveVaultPrefix(testVault)

	ids := multiIDGen(5) // A -> B -> C -> D -> E
	for _, id := range ids {
		store.addEngram(ws, makeEngram(id, "node"))
	}
	for i := 0; i < 4; i++ {
		store.addAssociation(ws, ids[i], Association{
			TargetID: ids[i+1],
			Weight:   0.9,
			RelType:  RelSupports,
		})
	}

	// maxDepth=2: 应只发现 B 和 C。
	results, err := BFSTraverse(context.Background(), store, ws, []ULID{ids[0]}, 0.0, 2)
	require.NoError(t, err)
	assert.Len(t, results, 2, "深度限制为 2 时只应发现 2 个节点")
}

func TestBFSTraverse_HopPenaltyDecay(t *testing.T) {
	store := newMockStore()
	ws := store.ResolveVaultPrefix(testVault)

	ids := multiIDGen(3) // A -> B -> C
	for _, id := range ids {
		store.addEngram(ws, makeEngram(id, "node"))
	}
	store.addAssociation(ws, ids[0], Association{TargetID: ids[1], Weight: 1.0, RelType: RelSupports})
	store.addAssociation(ws, ids[1], Association{TargetID: ids[2], Weight: 1.0, RelType: RelSupports})

	results, err := BFSTraverse(context.Background(), store, ws, []ULID{ids[0]}, 0.0, 5)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// B 在 hop 1: score = 1.0 * 1.0 * 0.7^1 = 0.7
	// C 在 hop 2: score = 0.7 * 1.0 * 0.7^2 = 0.7 * 0.49 = 0.343
	assert.InDelta(t, 0.7, results[0].Score, 0.001)
	assert.InDelta(t, 0.7*0.7*0.7, results[1].Score, 0.001)
}

func TestBFSTraverse_Branching(t *testing.T) {
	store := newMockStore()
	ws := store.ResolveVaultPrefix(testVault)

	// A -> B, A -> C（分叉）
	ids := multiIDGen(3)
	for _, id := range ids {
		store.addEngram(ws, makeEngram(id, "node"))
	}
	store.addAssociation(ws, ids[0], Association{TargetID: ids[1], Weight: 0.8, RelType: RelSupports})
	store.addAssociation(ws, ids[0], Association{TargetID: ids[2], Weight: 0.6, RelType: RelRelatesTo})

	results, err := BFSTraverse(context.Background(), store, ws, []ULID{ids[0]}, 0.0, 3)
	require.NoError(t, err)
	assert.Len(t, results, 2, "分叉后应发现两个子节点")
}

func TestBFSTraverse_NoDuplicateVisits(t *testing.T) {
	store := newMockStore()
	ws := store.ResolveVaultPrefix(testVault)

	// 环形: A -> B -> C -> A
	ids := multiIDGen(3)
	for _, id := range ids {
		store.addEngram(ws, makeEngram(id, "node"))
	}
	store.addAssociation(ws, ids[0], Association{TargetID: ids[1], Weight: 0.9, RelType: RelSupports})
	store.addAssociation(ws, ids[1], Association{TargetID: ids[2], Weight: 0.9, RelType: RelSupports})
	store.addAssociation(ws, ids[2], Association{TargetID: ids[0], Weight: 0.9, RelType: RelSupports})

	results, err := BFSTraverse(context.Background(), store, ws, []ULID{ids[0]}, 0.0, 5)
	require.NoError(t, err)
	assert.Len(t, results, 2, "环形图中不应重复访问种子节点")
}

func TestBFSTraverse_LowWeightPruning(t *testing.T) {
	store := newMockStore()
	ws := store.ResolveVaultPrefix(testVault)

	ids := multiIDGen(2)
	for _, id := range ids {
		store.addEngram(ws, makeEngram(id, "node"))
	}
	// 极低权重的关联，传播分数将低于 bfsMinHopScore (0.05)。
	store.addAssociation(ws, ids[0], Association{TargetID: ids[1], Weight: 0.01, RelType: RelRelatesTo})

	results, err := BFSTraverse(context.Background(), store, ws, []ULID{ids[0]}, 0.0, 3)
	require.NoError(t, err)
	assert.Empty(t, results, "传播分数低于阈值的边应被裁剪")
}

func TestBFSTraverse_ContextCancellation(t *testing.T) {
	store := newMockStore()
	ws := store.ResolveVaultPrefix(testVault)

	id := NewULIDWithTime(time.Now())
	store.addEngram(ws, makeEngram(id, "node"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// 取消后 BFS 应正常返回（GetAssociations 不会报错，只是返回空）。
	results, err := BFSTraverse(ctx, store, ws, []ULID{id}, 0.0, 3)
	require.NoError(t, err)
	assert.Empty(t, results)
}
