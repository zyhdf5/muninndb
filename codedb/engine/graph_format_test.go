package engine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatGraphJSONLD_EmptyGraph(t *testing.T) {
	g := &ExportGraph{}
	out, err := FormatGraphJSONLD(g)
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &doc))

	ctx, ok := doc["@context"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://schema.org/", ctx["@vocab"])
	assert.Equal(t, "https://muninndb.io/ontology#", ctx["muninn"])

	graph, ok := doc["@graph"].([]any)
	require.True(t, ok)
	assert.Empty(t, graph)
}

func TestFormatGraphJSONLD_NodesOnly(t *testing.T) {
	g := &ExportGraph{
		Nodes: []GraphNode{
			{ID: "PostgreSQL", Type: "database"},
			{ID: "Redis", Type: ""},
		},
	}
	out, err := FormatGraphJSONLD(g)
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &doc))

	graph := doc["@graph"].([]any)
	assert.Len(t, graph, 2)

	// 第一个节点应包含 entityType。
	node0 := graph[0].(map[string]any)
	assert.Equal(t, "muninn:Entity", node0["@type"])
	assert.Equal(t, "database", node0["muninn:entityType"])

	// 第二个节点不应包含 entityType（Type 为空）。
	node1 := graph[1].(map[string]any)
	_, hasType := node1["muninn:entityType"]
	assert.False(t, hasType, "空 Type 不应出现 muninn:entityType 字段")
}

func TestFormatGraphJSONLD_FullGraph(t *testing.T) {
	g := &ExportGraph{
		Nodes: []GraphNode{
			{ID: "A", Type: "service"},
			{ID: "B", Type: "database"},
		},
		Edges: []GraphEdge{
			{From: "A", To: "B", RelType: "uses", Weight: 0.9},
		},
	}
	out, err := FormatGraphJSONLD(g)
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &doc))

	graph := doc["@graph"].([]any)
	assert.Len(t, graph, 3) // 2 nodes + 1 edge

	edge := graph[2].(map[string]any)
	assert.Equal(t, "muninn:Relationship", edge["@type"])
	assert.Equal(t, "muninn:entity/A", edge["muninn:from"])
	assert.Equal(t, "muninn:entity/B", edge["muninn:to"])
	assert.Equal(t, "uses", edge["muninn:relType"])
}

func TestFormatGraphGraphML_EmptyGraph(t *testing.T) {
	g := &ExportGraph{}
	out, err := FormatGraphGraphML(g)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(out, `<?xml version="1.0" encoding="UTF-8"?>`))
	assert.Contains(t, out, "<graphml")
	assert.Contains(t, out, `edgedefault="directed"`)
}

func TestFormatGraphGraphML_FullGraph(t *testing.T) {
	g := &ExportGraph{
		Nodes: []GraphNode{
			{ID: "PostgreSQL", Type: "database"},
			{ID: "Redis", Type: ""},
		},
		Edges: []GraphEdge{
			{From: "PostgreSQL", To: "Redis", RelType: "caches_with", Weight: 0.75},
		},
	}
	out, err := FormatGraphGraphML(g)
	require.NoError(t, err)

	assert.Contains(t, out, `id="PostgreSQL"`)
	assert.Contains(t, out, `id="Redis"`)
	assert.Contains(t, out, `source="PostgreSQL"`)
	assert.Contains(t, out, `target="Redis"`)
	assert.Contains(t, out, "caches_with")
	assert.Contains(t, out, "0.75")

	// 带有 Type 的节点应有 data 子元素。
	assert.Contains(t, out, `<data key="type">database</data>`)
}

func TestFormatGraphGraphML_SpecialChars(t *testing.T) {
	g := &ExportGraph{
		Nodes: []GraphNode{
			{ID: "A&B", Type: "type<1>"},
		},
	}
	out, err := FormatGraphGraphML(g)
	require.NoError(t, err)

	// XML encoding 应正确转义特殊字符。
	assert.Contains(t, out, "A&amp;B")
	assert.Contains(t, out, "type&lt;1&gt;")
}

func TestFormatGraphGraphML_MultipleEdges(t *testing.T) {
	g := &ExportGraph{
		Nodes: []GraphNode{{ID: "A"}, {ID: "B"}, {ID: "C"}},
		Edges: []GraphEdge{
			{From: "A", To: "B", RelType: "uses", Weight: 0.5},
			{From: "B", To: "C", RelType: "depends_on", Weight: 0.8},
		},
	}
	out, err := FormatGraphGraphML(g)
	require.NoError(t, err)

	assert.Contains(t, out, `id="e0"`)
	assert.Contains(t, out, `id="e1"`)
}
