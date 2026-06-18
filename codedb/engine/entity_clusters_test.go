package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetEntityClusters_Empty(t *testing.T) {
	eng, _ := setupTestEngine()

	clusters, err := eng.GetEntityClusters(context.Background(), testVault, 2, 20)
	require.NoError(t, err)
	assert.Empty(t, clusters)
}

func TestGetEntityClusters_SortDescending(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	store.addEntityCluster(ws, "A", "B", 3)
	store.addEntityCluster(ws, "C", "D", 10)
	store.addEntityCluster(ws, "E", "F", 5)

	clusters, err := eng.GetEntityClusters(context.Background(), testVault, 1, 20)
	require.NoError(t, err)
	require.Len(t, clusters, 3)

	assert.Equal(t, 10, clusters[0].Count, "最高共现次数应排在前面")
	assert.Equal(t, 5, clusters[1].Count)
	assert.Equal(t, 3, clusters[2].Count)
}

func TestGetEntityClusters_MinCountFilter(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	store.addEntityCluster(ws, "A", "B", 1)
	store.addEntityCluster(ws, "C", "D", 5)

	clusters, err := eng.GetEntityClusters(context.Background(), testVault, 3, 20)
	require.NoError(t, err)
	require.Len(t, clusters, 1)
	assert.Equal(t, "C", clusters[0].EntityA)
}

func TestGetEntityClusters_TopNCap(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	for i := 0; i < 10; i++ {
		store.addEntityCluster(ws, "A", string(rune('B'+i)), i+2)
	}

	clusters, err := eng.GetEntityClusters(context.Background(), testVault, 1, 3)
	require.NoError(t, err)
	assert.Len(t, clusters, 3, "topN 应限制返回数量")
}

func TestGetEntityClusters_TopNZero(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	for i := 0; i < 5; i++ {
		store.addEntityCluster(ws, "A", string(rune('B'+i)), i+2)
	}

	clusters, err := eng.GetEntityClusters(context.Background(), testVault, 1, 0)
	require.NoError(t, err)
	assert.Len(t, clusters, 5, "topN=0 时不应限制")
}
