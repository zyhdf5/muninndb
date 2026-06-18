package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportGraph_EmptyVault(t *testing.T) {
	eng, _ := setupTestEngine()

	g, err := eng.ExportGraph(context.Background(), "empty-vault", false)
	require.NoError(t, err)
	require.NotNil(t, g)
	assert.Empty(t, g.Nodes)
	assert.Empty(t, g.Edges)
}

func TestExportGraph_SingleRelationship(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	store.addRelationship(ws, RelationshipRecord{
		FromEntity: "PostgreSQL",
		ToEntity:   "Redis",
		RelType:    "caches_with",
		Weight:     0.8,
	})

	g, err := eng.ExportGraph(context.Background(), testVault, false)
	require.NoError(t, err)
	require.Len(t, g.Nodes, 2)
	require.Len(t, g.Edges, 1)

	assert.Equal(t, "caches_with", g.Edges[0].RelType)
	assert.Equal(t, float32(0.8), g.Edges[0].Weight)
}

func TestExportGraph_DeduplicatesEdges(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	// 同一 (From, To, RelType) 三元组，不同权重。
	store.addRelationship(ws, RelationshipRecord{
		FromEntity: "A", ToEntity: "B", RelType: "uses", Weight: 0.3,
	})
	store.addRelationship(ws, RelationshipRecord{
		FromEntity: "A", ToEntity: "B", RelType: "uses", Weight: 0.9,
	})

	g, err := eng.ExportGraph(context.Background(), testVault, false)
	require.NoError(t, err)
	require.Len(t, g.Edges, 1, "相同三元组应只保留一条边")
	assert.Equal(t, float32(0.9), g.Edges[0].Weight, "应保留最高权重")
}

func TestExportGraph_IncludeEngrams_EnrichesType(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	store.addRelationship(ws, RelationshipRecord{
		FromEntity: "PostgreSQL", ToEntity: "Redis", RelType: "uses", Weight: 0.8,
	})
	store.addEntityRecord(EntityRecord{Name: "PostgreSQL", Type: "database", State: "active"})
	store.addEntityRecord(EntityRecord{Name: "Redis", Type: "cache", State: "active"})

	g, err := eng.ExportGraph(context.Background(), testVault, true)
	require.NoError(t, err)

	typeMap := make(map[string]string)
	for _, n := range g.Nodes {
		typeMap[n.ID] = n.Type
	}
	assert.Equal(t, "database", typeMap["PostgreSQL"])
	assert.Equal(t, "cache", typeMap["Redis"])
}

func TestExportGraph_IncludeEngrams_MissingRecord(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	store.addRelationship(ws, RelationshipRecord{
		FromEntity: "Unknown", ToEntity: "Known", RelType: "uses", Weight: 0.5,
	})
	store.addEntityRecord(EntityRecord{Name: "Known", Type: "service"})

	g, err := eng.ExportGraph(context.Background(), testVault, true)
	require.NoError(t, err)
	assert.Len(t, g.Nodes, 2, "即使实体记录不存在也应包含节点")
}

func TestExportGraph_DeterministicOrder(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	store.addRelationship(ws, RelationshipRecord{FromEntity: "C", ToEntity: "A", RelType: "uses", Weight: 0.5})
	store.addRelationship(ws, RelationshipRecord{FromEntity: "A", ToEntity: "B", RelType: "depends_on", Weight: 0.6})
	store.addRelationship(ws, RelationshipRecord{FromEntity: "B", ToEntity: "C", RelType: "manages", Weight: 0.7})

	g1, err := eng.ExportGraph(context.Background(), testVault, false)
	require.NoError(t, err)
	g2, err := eng.ExportGraph(context.Background(), testVault, false)
	require.NoError(t, err)

	require.Equal(t, len(g1.Nodes), len(g2.Nodes))
	require.Equal(t, len(g1.Edges), len(g2.Edges))

	for i := range g1.Nodes {
		assert.Equal(t, g1.Nodes[i].ID, g2.Nodes[i].ID)
	}
	for i := range g1.Edges {
		assert.Equal(t, g1.Edges[i].From, g2.Edges[i].From)
		assert.Equal(t, g1.Edges[i].To, g2.Edges[i].To)
	}

	// 验证节点按 ID 升序排列。
	for i := 1; i < len(g1.Nodes); i++ {
		assert.Less(t, g1.Nodes[i-1].ID, g1.Nodes[i].ID)
	}
}

func TestExportGraph_ContextCancellation(t *testing.T) {
	eng, store := setupTestEngine()
	ws := store.ResolveVaultPrefix(testVault)

	store.addRelationship(ws, RelationshipRecord{
		FromEntity: "A", ToEntity: "B", RelType: "uses", Weight: 0.5,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// 已取消的 context 应导致提前返回。
	_, err := eng.ExportGraph(ctx, testVault, true)
	assert.Error(t, err)
}
