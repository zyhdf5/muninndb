package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyEntityBoost_EmptyResults(t *testing.T) {
	eng, _ := setupTestEngine()
	ws := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}

	result := eng.ApplyEntityBoost(context.Background(), ws, nil)
	assert.Empty(t, result)
}

func TestApplyEntityBoost_BoostExisting(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	ids := multiIDGen(2)
	engA := makeEngram(ids[0], "A")
	engB := makeEngram(ids[1], "B")

	store.addEngram(ws, engA)
	store.addEngram(ws, engB)

	// A 和 B 共享实体 "PostgreSQL"。
	store.addEntityEngramLink("PostgreSQL", ws, ids[0])
	store.addEntityEngramLink("PostgreSQL", ws, ids[1])

	// B 已在结果中。
	initialResults := []ScoredEngram{
		{Engram: engA, Score: 0.8},
		{Engram: engB, Score: 0.3},
	}

	boosted := eng.ApplyEntityBoost(context.Background(), ws, initialResults)

	// B 应获得 entityBoostFactor 的加成。
	scoreMap := make(map[ULID]float64)
	for _, r := range boosted {
		scoreMap[r.Engram.ID] = r.Score
	}

	assert.Greater(t, scoreMap[ids[1]], 0.3, "B 应获得 entity boost 加成")
	assert.InDelta(t, 0.3+entityBoostFactor, scoreMap[ids[1]], 0.001)
}

func TestApplyEntityBoost_AddNew(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	ids := multiIDGen(2)
	engA := makeEngram(ids[0], "A")
	engB := makeEngram(ids[1], "B")

	store.addEngram(ws, engA)
	store.addEngram(ws, engB)

	// A 和 B 共享实体，但 B 不在初始结果中。
	store.addEntityEngramLink("PostgreSQL", ws, ids[0])
	store.addEntityEngramLink("PostgreSQL", ws, ids[1])

	initialResults := []ScoredEngram{
		{Engram: engA, Score: 0.8},
	}

	boosted := eng.ApplyEntityBoost(context.Background(), ws, initialResults)

	scoreMap := make(map[ULID]float64)
	for _, r := range boosted {
		scoreMap[r.Engram.ID] = r.Score
	}

	require.Contains(t, scoreMap, ids[1], "B 应被 entity boost 新增到结果中")
	assert.InDelta(t, entityBoostFactor, scoreMap[ids[1]], 0.001)
}

func TestApplyEntityBoost_SkipSoftDeletedAndArchived(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	ids := multiIDGen(3)
	engA := makeEngram(ids[0], "A")
	engDel := makeEngramWithState(ids[1], "deleted", StateSoftDeleted)
	engArch := makeEngramWithState(ids[2], "archived", StateArchived)

	store.addEngram(ws, engA)
	store.addEngram(ws, engDel)
	store.addEngram(ws, engArch)

	store.addEntityEngramLink("Entity", ws, ids[0])
	store.addEntityEngramLink("Entity", ws, ids[1])
	store.addEntityEngramLink("Entity", ws, ids[2])

	initialResults := []ScoredEngram{
		{Engram: engA, Score: 0.8},
	}

	boosted := eng.ApplyEntityBoost(context.Background(), ws, initialResults)

	for _, r := range boosted {
		assert.NotEqual(t, StateSoftDeleted, r.Engram.State, "软删除的 engram 不应出现")
		assert.NotEqual(t, StateArchived, r.Engram.State, "已归档的 engram 不应出现")
	}
}

func TestApplyEntityBoost_ReSort(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	ids := multiIDGen(3)
	engA := makeEngram(ids[0], "A")
	engB := makeEngram(ids[1], "B")
	engC := makeEngram(ids[2], "C")

	store.addEngram(ws, engA)
	store.addEngram(ws, engB)
	store.addEngram(ws, engC)

	store.addEntityEngramLink("Entity", ws, ids[0])
	store.addEntityEngramLink("Entity", ws, ids[2])

	initialResults := []ScoredEngram{
		{Engram: engA, Score: 0.5},
		{Engram: engB, Score: 0.9},
	}

	boosted := eng.ApplyEntityBoost(context.Background(), ws, initialResults)

	// 验证结果按分数降序排列。
	for i := 1; i < len(boosted); i++ {
		assert.GreaterOrEqual(t, boosted[i-1].Score, boosted[i].Score, "结果应按分数降序排列")
	}
}

func TestApplyEntityBoost_SeedLimit(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	// 创建超过 entityBoostTopN (5) 个种子。
	ids := multiIDGen(8)
	var initialResults []ScoredEngram
	for i, id := range ids {
		eng := makeEngram(id, "seed")
		store.addEngram(ws, eng)
		store.addEntityEngramLink("SharedEntity", ws, id)
		initialResults = append(initialResults, ScoredEngram{Engram: eng, Score: float64(8 - i)})
	}

	// 不应 panic。
	boosted := eng.ApplyEntityBoost(context.Background(), ws, initialResults)
	assert.NotEmpty(t, boosted)
}
