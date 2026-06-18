package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetEntityTimeline_NotFound(t *testing.T) {
	eng, _ := setupTestEngine()

	_, err := eng.GetEntityTimeline(context.Background(), testVault, "不存在的实体", 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "未找到")
}

func TestGetEntityTimeline_EmptyName(t *testing.T) {
	eng, _ := setupTestEngine()

	_, err := eng.GetEntityTimeline(context.Background(), testVault, "", 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "不能为空")
}

func TestGetEntityTimeline_SortAscending(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	firstSeen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	store.addEntityRecord(EntityRecord{
		Name:         "PostgreSQL",
		Type:         "database",
		State:        "active",
		MentionCount: 3,
		FirstSeen:    firstSeen.UnixNano(),
	})

	t3 := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)

	ids := multiIDGen(3)
	store.addEngram(ws, makeEngramFull(ids[0], "entry-3", "摘要3", StateActive, t3))
	store.addEngram(ws, makeEngramFull(ids[1], "entry-1", "摘要1", StateActive, t1))
	store.addEngram(ws, makeEngramFull(ids[2], "entry-2", "摘要2", StateActive, t2))

	for _, id := range ids {
		store.addEntityEngramLink("PostgreSQL", ws, id)
	}

	timeline, err := eng.GetEntityTimeline(context.Background(), testVault, "PostgreSQL", 10)
	require.NoError(t, err)
	require.NotNil(t, timeline)

	assert.Equal(t, "PostgreSQL", timeline.Entity)
	assert.Equal(t, 3, timeline.MentionCount)
	assert.Equal(t, 3, timeline.Count)
	require.Len(t, timeline.Entries, 3)

	// 验证按创建时间升序排列。
	for i := 1; i < len(timeline.Entries); i++ {
		assert.True(t,
			timeline.Entries[i-1].CreatedAt.Before(timeline.Entries[i].CreatedAt) ||
				timeline.Entries[i-1].CreatedAt.Equal(timeline.Entries[i].CreatedAt),
			"条目应按创建时间升序排列")
	}
}

func TestGetEntityTimeline_LimitClamping(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	store.addEntityRecord(EntityRecord{
		Name:         "Test",
		State:        "active",
		MentionCount: 1,
		FirstSeen:    time.Now().UnixNano(),
	})

	id := NewULID()
	store.addEngram(ws, makeEngram(id, "test"))
	store.addEntityEngramLink("Test", ws, id)

	// limit = 0 → 默认 10。
	tl, err := eng.GetEntityTimeline(context.Background(), testVault, "Test", 0)
	require.NoError(t, err)
	assert.LessOrEqual(t, tl.Count, 10)

	// limit = 100 → 钳制为 50。
	tl, err = eng.GetEntityTimeline(context.Background(), testVault, "Test", 100)
	require.NoError(t, err)
	assert.LessOrEqual(t, tl.Count, 50)
}

func TestGetEntityTimeline_SoftDeletedSkipped(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	store.addEntityRecord(EntityRecord{
		Name:         "E",
		State:        "active",
		MentionCount: 2,
		FirstSeen:    time.Now().UnixNano(),
	})

	id1 := NewULID()
	id2 := NewULID()
	store.addEngram(ws, makeEngram(id1, "active-eng"))
	store.addEngram(ws, makeEngramWithState(id2, "deleted-eng", StateSoftDeleted))
	store.addEntityEngramLink("E", ws, id1)
	store.addEntityEngramLink("E", ws, id2)

	tl, err := eng.GetEntityTimeline(context.Background(), testVault, "E", 10)
	require.NoError(t, err)
	assert.Equal(t, 1, tl.Count, "软删除的 engram 应被跳过")
}
