package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetEntityAggregate_NotFound(t *testing.T) {
	eng, _ := setupTestEngine()

	result, err := eng.GetEntityAggregate(context.Background(), testVault, "不存在的实体", 10)
	require.NoError(t, err)
	assert.Nil(t, result, "不存在的实体应返回 nil")
}

func TestGetEntityAggregate_FullFlow(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	ids := multiIDGen(3)
	store.addEntityRecord(EntityRecord{
		Name:         "PostgreSQL",
		Type:         "database",
		State:        "active",
		MentionCount: 3,
		FirstSeen:    time.Now().UnixNano(),
	})

	store.addEngram(ws, makeEngram(ids[0], "pg-primary"))
	store.addEngram(ws, makeEngram(ids[1], "pg-replica"))
	store.addEngram(ws, makeEngramWithState(ids[2], "pg-deleted", StateSoftDeleted))

	for _, id := range ids {
		store.addEntityEngramLink("PostgreSQL", ws, id)
	}

	store.addRelationship(ws, RelationshipRecord{
		FromEntity: "PostgreSQL", ToEntity: "Redis", RelType: "caches_with", Weight: 0.8,
	})
	// entityRelationships 已在 addRelationship 中维护。

	store.addEntityCluster(ws, "PostgreSQL", "Redis", 5)

	result, err := eng.GetEntityAggregate(context.Background(), testVault, "PostgreSQL", 20)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "PostgreSQL", result.Record.Name)
	assert.Len(t, result.Engrams, 2, "软删除的 engram 应被过滤")
	assert.Len(t, result.Relations, 1)
	assert.Len(t, result.CoOccurring, 1)
	assert.Equal(t, "Redis", result.CoOccurring[0].Name)
}

func TestGetEntityAggregate_DefaultLimit(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	store.addEntityRecord(EntityRecord{Name: "E", Type: "test", State: "active"})

	// 添加 25 个 engram，默认 limit 为 20。
	for i := 0; i < 25; i++ {
		id := NewULID()
		store.addEngram(ws, makeEngram(id, "engram"))
		store.addEntityEngramLink("E", ws, id)
	}

	result, err := eng.GetEntityAggregate(context.Background(), testVault, "E", 0)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.LessOrEqual(t, len(result.Engrams), 20, "默认 limit 应为 20")
}

func TestGetEntityAggregate_DifferentVaultFiltered(t *testing.T) {
	eng, store := setupTestEngine()
	ws1 := store.ResolveVaultPrefix("vault-1")
	ws2 := store.ResolveVaultPrefix("vault-2")

	id1 := NewULID()
	id2 := NewULID()

	store.addEntityRecord(EntityRecord{Name: "SharedEntity", Type: "test", State: "active"})

	store.addEngram(ws1, makeEngram(id1, "vault-1-eng"))
	store.addEngram(ws2, makeEngram(id2, "vault-2-eng"))
	store.addEntityEngramLink("SharedEntity", ws1, id1)
	store.addEntityEngramLink("SharedEntity", ws2, id2)

	result, err := eng.GetEntityAggregate(context.Background(), "vault-1", "SharedEntity", 20)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Engrams, 1, "应只返回 vault-1 中的 engram")
}

func TestListEntities_SortByMentionCount(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	entities := []EntityRecord{
		{Name: "A", MentionCount: 1, State: "active"},
		{Name: "B", MentionCount: 10, State: "active"},
		{Name: "C", MentionCount: 5, State: "active"},
	}
	for _, rec := range entities {
		store.addEntityRecord(rec)
		store.addEntityEngramLink(rec.Name, ws, NewULID())
	}

	result, err := eng.ListEntities(context.Background(), testVault, 10, "")
	require.NoError(t, err)
	require.Len(t, result, 3)

	assert.Equal(t, "B", result[0].Name, "提及次数最多的应排在前面")
	assert.Equal(t, "C", result[1].Name)
	assert.Equal(t, "A", result[2].Name)
}

func TestListEntities_StateFilter(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	store.addEntityRecord(EntityRecord{Name: "Active1", State: "active", MentionCount: 5})
	store.addEntityRecord(EntityRecord{Name: "Deprecated1", State: "deprecated", MentionCount: 3})
	store.addEntityEngramLink("Active1", ws, NewULID())
	store.addEntityEngramLink("Deprecated1", ws, NewULID())

	result, err := eng.ListEntities(context.Background(), testVault, 10, "active")
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "Active1", result[0].Name)
}

func TestListEntities_LimitApplied(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	for i := 0; i < 10; i++ {
		name := string(rune('A' + i))
		store.addEntityRecord(EntityRecord{Name: name, State: "active", MentionCount: int32(10 - i)})
		store.addEntityEngramLink(name, ws, NewULID())
	}

	result, err := eng.ListEntities(context.Background(), testVault, 3, "")
	require.NoError(t, err)
	assert.Len(t, result, 3)
}

func TestListEntities_DefaultLimit(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	for i := 0; i < 60; i++ {
		name := string(rune('A'+i/26)) + string(rune('a'+i%26))
		store.addEntityRecord(EntityRecord{Name: name, State: "active", MentionCount: 1})
		store.addEntityEngramLink(name, ws, NewULID())
	}

	result, err := eng.ListEntities(context.Background(), testVault, 0, "")
	require.NoError(t, err)
	assert.LessOrEqual(t, len(result), 50, "默认 limit 应为 50")
}
