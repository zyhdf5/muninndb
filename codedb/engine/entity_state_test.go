package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetEntityState_Success(t *testing.T) {
	eng, store := setupTestEngine()

	store.addEntityRecord(EntityRecord{
		Name:       "PostgreSQL",
		Type:       "database",
		State:      "active",
		Confidence: 0.9,
	})

	err := eng.SetEntityState(context.Background(), "PostgreSQL", "deprecated", "", "")
	require.NoError(t, err)

	rec, err := store.GetEntityRecord(context.Background(), "PostgreSQL")
	require.NoError(t, err)
	assert.Equal(t, "deprecated", rec.State)
	assert.Equal(t, "database", rec.Type, "未提供 entityType 时应保留原有类型")
	assert.Equal(t, float32(0.9), rec.Confidence, "应保留原有置信度")
}

func TestSetEntityState_WithTypeOverride(t *testing.T) {
	eng, store := setupTestEngine()

	store.addEntityRecord(EntityRecord{
		Name:  "Redis",
		Type:  "cache",
		State: "active",
	})

	err := eng.SetEntityState(context.Background(), "Redis", "active", "", "database")
	require.NoError(t, err)

	rec, err := store.GetEntityRecord(context.Background(), "Redis")
	require.NoError(t, err)
	assert.Equal(t, "database", rec.Type, "提供 entityType 时应更新类型")
}

func TestSetEntityState_NotFound(t *testing.T) {
	eng, _ := setupTestEngine()

	err := eng.SetEntityState(context.Background(), "不存在的实体", "deprecated", "", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "不存在")
}

func TestSetEntityState_EmptyName(t *testing.T) {
	eng, _ := setupTestEngine()

	err := eng.SetEntityState(context.Background(), "", "active", "", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "不能为空")
}

func TestSetEntityState_Merged(t *testing.T) {
	eng, store := setupTestEngine()

	store.addEntityRecord(EntityRecord{Name: "OldName", Type: "service", State: "active"})

	err := eng.SetEntityState(context.Background(), "OldName", "merged", "NewName", "")
	require.NoError(t, err)

	rec, err := store.GetEntityRecord(context.Background(), "OldName")
	require.NoError(t, err)
	assert.Equal(t, "merged", rec.State)
	assert.Equal(t, "NewName", rec.MergedInto)
}

func TestSetEntityStateBatch_Success(t *testing.T) {
	eng, store := setupTestEngine()

	store.addEntityRecord(EntityRecord{Name: "A", State: "active"})
	store.addEntityRecord(EntityRecord{Name: "B", State: "active"})

	ops := []EntityStateOp{
		{EntityName: "A", State: "deprecated"},
		{EntityName: "B", State: "resolved"},
	}

	errs := eng.SetEntityStateBatch(context.Background(), ops)
	require.Len(t, errs, 2)
	assert.NoError(t, errs[0])
	assert.NoError(t, errs[1])

	recA, _ := store.GetEntityRecord(context.Background(), "A")
	recB, _ := store.GetEntityRecord(context.Background(), "B")
	assert.Equal(t, "deprecated", recA.State)
	assert.Equal(t, "resolved", recB.State)
}

func TestSetEntityStateBatch_PartialFailure(t *testing.T) {
	eng, store := setupTestEngine()

	store.addEntityRecord(EntityRecord{Name: "Exists", State: "active"})

	ops := []EntityStateOp{
		{EntityName: "Exists", State: "deprecated"},
		{EntityName: "Missing", State: "active"},
	}

	errs := eng.SetEntityStateBatch(context.Background(), ops)
	require.Len(t, errs, 2)
	assert.NoError(t, errs[0], "存在的实体应成功")
	assert.Error(t, errs[1], "不存在的实体应失败")
}

func TestSetEntityStateBatch_ContextCancellation(t *testing.T) {
	eng, store := setupTestEngine()

	store.addEntityRecord(EntityRecord{Name: "A", State: "active"})
	store.addEntityRecord(EntityRecord{Name: "B", State: "active"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ops := []EntityStateOp{
		{EntityName: "A", State: "deprecated"},
		{EntityName: "B", State: "deprecated"},
	}

	errs := eng.SetEntityStateBatch(ctx, ops)
	require.Len(t, errs, 2)

	// 两个操作都应因 context 取消而失败。
	for _, e := range errs {
		assert.Error(t, e)
	}
}
