package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindByEntity_EmptyName(t *testing.T) {
	eng, _ := setupTestEngine()

	_, err := eng.FindByEntity(context.Background(), testVault, "", 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "不能为空")
}

func TestFindByEntity_Success(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	ids := multiIDGen(3)
	for _, id := range ids {
		store.addEngram(ws, makeEngram(id, "test-eng"))
		store.addEntityEngramLink("PostgreSQL", ws, id)
	}

	results, err := eng.FindByEntity(context.Background(), testVault, "PostgreSQL", 10)
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestFindByEntity_SoftDeletedSkipped(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	id1 := NewULID()
	id2 := NewULID()
	store.addEngram(ws, makeEngram(id1, "active"))
	store.addEngram(ws, makeEngramWithState(id2, "deleted", StateSoftDeleted))
	store.addEntityEngramLink("Entity", ws, id1)
	store.addEntityEngramLink("Entity", ws, id2)

	results, err := eng.FindByEntity(context.Background(), testVault, "Entity", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1, "软删除的 engram 应被跳过")
}

func TestFindByEntity_LimitApplied(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	for i := 0; i < 10; i++ {
		id := NewULID()
		store.addEngram(ws, makeEngram(id, "eng"))
		store.addEntityEngramLink("Entity", ws, id)
	}

	results, err := eng.FindByEntity(context.Background(), testVault, "Entity", 3)
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestFindByEntity_DefaultLimit(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	for i := 0; i < 25; i++ {
		id := NewULID()
		store.addEngram(ws, makeEngram(id, "eng"))
		store.addEntityEngramLink("Entity", ws, id)
	}

	results, err := eng.FindByEntity(context.Background(), testVault, "Entity", 0)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(results), 20, "默认 limit 应为 20")
}

func TestFindByEntity_MaxLimitClamped(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	for i := 0; i < 60; i++ {
		id := NewULID()
		store.addEngram(ws, makeEngram(id, "eng"))
		store.addEntityEngramLink("Entity", ws, id)
	}

	results, err := eng.FindByEntity(context.Background(), testVault, "Entity", 100)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(results), 50, "limit 应被钳制为 50")
}

func TestFindByEntity_DifferentVaultFiltered(t *testing.T) {
	eng, store := setupTestEngine()
	ws1 := store.ResolveVaultPrefix("vault-1")
	ws2 := store.ResolveVaultPrefix("vault-2")

	id1 := NewULID()
	id2 := NewULID()
	store.addEngram(ws1, makeEngram(id1, "v1-eng"))
	store.addEngram(ws2, makeEngram(id2, "v2-eng"))
	store.addEntityEngramLink("Shared", ws1, id1)
	store.addEntityEngramLink("Shared", ws2, id2)

	results, err := eng.FindByEntity(context.Background(), "vault-1", "Shared", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1, "应只返回对应 vault 的 engram")
}
