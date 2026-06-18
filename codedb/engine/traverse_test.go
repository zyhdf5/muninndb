package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraverse_InvalidStartID(t *testing.T) {
	eng, _ := setupTestEngine()

	_, _, err := eng.Traverse(context.Background(), testVault, "invalid-id", 2, 20, false)
	assert.Error(t, err, "无效的起始 ID 应返回错误")
}

func TestTraverse_BasicBFS(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	ids := multiIDGen(3) // A -> B -> C
	for _, id := range ids {
		store.addEngram(ws, makeEngram(id, "node"))
	}
	store.addAssociation(ws, ids[0], Association{TargetID: ids[1], Weight: 0.8, RelType: RelSupports})
	store.addAssociation(ws, ids[1], Association{TargetID: ids[2], Weight: 0.6, RelType: RelRelatesTo})

	nodes, edges, err := eng.Traverse(context.Background(), testVault, ids[0].String(), 3, 20, false)
	require.NoError(t, err)

	assert.Len(t, nodes, 3, "应包含 A、B、C")
	assert.Len(t, edges, 2, "应包含 A->B 和 B->C 两条边")
}

func TestTraverse_MaxHopsLimit(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	ids := multiIDGen(4) // A -> B -> C -> D
	for _, id := range ids {
		store.addEngram(ws, makeEngram(id, "node"))
	}
	for i := 0; i < 3; i++ {
		store.addAssociation(ws, ids[i], Association{TargetID: ids[i+1], Weight: 0.8, RelType: RelSupports})
	}

	// maxHops=1: 只能到 B。
	nodes, _, err := eng.Traverse(context.Background(), testVault, ids[0].String(), 1, 20, false)
	require.NoError(t, err)

	// A (hop 0) + B (hop 1) = 2 nodes
	assert.LessOrEqual(t, len(nodes), 2)
}

func TestTraverse_MaxNodesLimit(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	ids := multiIDGen(10)
	for _, id := range ids {
		store.addEngram(ws, makeEngram(id, "node"))
	}
	// 星形拓扑: ids[0] -> ids[1..9]
	for i := 1; i < 10; i++ {
		store.addAssociation(ws, ids[0], Association{TargetID: ids[i], Weight: 0.8, RelType: RelSupports})
	}

	nodes, _, err := eng.Traverse(context.Background(), testVault, ids[0].String(), 3, 5, false)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(nodes), 5, "节点数应受 maxNodes 限制")
}

func TestTraverse_SoftDeletedSkipped(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	ids := multiIDGen(3)
	store.addEngram(ws, makeEngram(ids[0], "start"))
	store.addEngram(ws, makeEngramWithState(ids[1], "deleted", StateSoftDeleted))
	store.addEngram(ws, makeEngram(ids[2], "end"))

	store.addAssociation(ws, ids[0], Association{TargetID: ids[1], Weight: 0.8, RelType: RelSupports})
	store.addAssociation(ws, ids[0], Association{TargetID: ids[2], Weight: 0.6, RelType: RelRelatesTo})

	nodes, _, err := eng.Traverse(context.Background(), testVault, ids[0].String(), 3, 20, false)
	require.NoError(t, err)

	for _, n := range nodes {
		assert.NotEqual(t, ids[1], n.ID, "软删除的节点不应出现在结果中")
	}
}

func TestTraverse_FollowEntities(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	ids := multiIDGen(3)
	store.addEngram(ws, makeEngram(ids[0], "A"))
	store.addEngram(ws, makeEngram(ids[1], "B"))
	store.addEngram(ws, makeEngram(ids[2], "C"))

	// A -> B 通过直接关联。
	store.addAssociation(ws, ids[0], Association{TargetID: ids[1], Weight: 0.8, RelType: RelSupports})

	// A 和 C 通过共享实体 "PostgreSQL" 连接（无直接关联）。
	store.addEntityEngramLink("PostgreSQL", ws, ids[0])
	store.addEntityEngramLink("PostgreSQL", ws, ids[2])

	// followEntities=false: 只发现 A 和 B。
	nodes1, _, err := eng.Traverse(context.Background(), testVault, ids[0].String(), 3, 20, false)
	require.NoError(t, err)

	foundC := false
	for _, n := range nodes1 {
		if n.ID == ids[2] {
			foundC = true
		}
	}
	assert.False(t, foundC, "followEntities=false 时不应通过实体链接发现 C")

	// followEntities=true: 应额外发现 C。
	nodes2, edges2, err := eng.Traverse(context.Background(), testVault, ids[0].String(), 3, 20, true)
	require.NoError(t, err)

	foundC = false
	for _, n := range nodes2 {
		if n.ID == ids[2] {
			foundC = true
		}
	}
	assert.True(t, foundC, "followEntities=true 时应通过共享实体发现 C")

	// 验证实体跳边的权重为 entityHopWeight。
	for _, edge := range edges2 {
		if edge.To == ids[2] {
			assert.Equal(t, float32(entityHopWeight), edge.Weight, "实体跳边权重应为 entityHopWeight")
		}
	}
}

func TestTraverse_NoCycle(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	// A -> B -> A（环）
	ids := multiIDGen(2)
	store.addEngram(ws, makeEngram(ids[0], "A"))
	store.addEngram(ws, makeEngram(ids[1], "B"))

	store.addAssociation(ws, ids[0], Association{TargetID: ids[1], Weight: 0.8, RelType: RelSupports})
	store.addAssociation(ws, ids[1], Association{TargetID: ids[0], Weight: 0.8, RelType: RelSupports})

	nodes, _, err := eng.Traverse(context.Background(), testVault, ids[0].String(), 5, 20, false)
	require.NoError(t, err)

	// 每个节点只应出现一次。
	idSet := make(map[ULID]int)
	for _, n := range nodes {
		idSet[n.ID]++
	}
	for id, count := range idSet {
		assert.Equal(t, 1, count, "节点 %s 不应重复出现", id)
	}
}
