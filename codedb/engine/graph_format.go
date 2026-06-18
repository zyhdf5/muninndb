package engine

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
)

// ────────────────────────────────────────────────────────────────────
// JSON-LD 格式化
// ────────────────────────────────────────────────────────────────────

// FormatGraphJSONLD 将图序列化为 JSON-LD 格式字符串。
// 节点和边按照 Muninn 本体（ontology）的命名空间进行标注。
func FormatGraphJSONLD(g *ExportGraph) (string, error) {
	graph := make([]map[string]any, 0, len(g.Nodes)+len(g.Edges))

	for _, node := range g.Nodes {
		n := map[string]any{
			"@type": "muninn:Entity",
			"@id":   "muninn:entity/" + node.ID,
			"name":  node.ID,
		}
		if node.Type != "" {
			n["muninn:entityType"] = node.Type
		}
		graph = append(graph, n)
	}

	for _, edge := range g.Edges {
		e := map[string]any{
			"@type":          "muninn:Relationship",
			"@id":            fmt.Sprintf("muninn:rel/%s/%s/%s", edge.From, edge.RelType, edge.To),
			"muninn:from":    "muninn:entity/" + edge.From,
			"muninn:to":      "muninn:entity/" + edge.To,
			"muninn:relType": edge.RelType,
			"muninn:weight":  edge.Weight,
		}
		graph = append(graph, e)
	}

	doc := map[string]any{
		"@context": map[string]any{
			"@vocab": "https://schema.org/",
			"muninn": "https://muninndb.io/ontology#",
		},
		"@graph": graph,
	}

	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("格式化 json-ld: %w", err)
	}
	return string(b), nil
}

// ────────────────────────────────────────────────────────────────────
// GraphML 格式化
// ────────────────────────────────────────────────────────────────────

// graphMLDoc 是 GraphML 序列化的顶层 XML 文档。
type graphMLDoc struct {
	XMLName xml.Name     `xml:"graphml"`
	Xmlns   string       `xml:"xmlns,attr"`
	Keys    []graphMLKey `xml:"key"`
	Graph   graphMLGraph `xml:"graph"`
}

// graphMLKey 定义 GraphML 的属性键。
type graphMLKey struct {
	ID       string `xml:"id,attr"`
	For      string `xml:"for,attr"`
	AttrName string `xml:"attr.name,attr"`
	AttrType string `xml:"attr.type,attr"`
}

// graphMLGraph 是 GraphML 的图容器，包含节点和边。
type graphMLGraph struct {
	ID          string        `xml:"id,attr"`
	EdgeDefault string        `xml:"edgedefault,attr"`
	Nodes       []graphMLNode `xml:"node"`
	Edges       []graphMLEdge `xml:"edge"`
}

// graphMLNode 是 GraphML 的节点元素。
type graphMLNode struct {
	ID   string        `xml:"id,attr"`
	Data []graphMLData `xml:"data"`
}

// graphMLEdge 是 GraphML 的边元素。
type graphMLEdge struct {
	ID     string        `xml:"id,attr"`
	Source string        `xml:"source,attr"`
	Target string        `xml:"target,attr"`
	Data   []graphMLData `xml:"data"`
}

// graphMLData 是 GraphML 的属性值元素。
type graphMLData struct {
	Key   string `xml:"key,attr"`
	Value string `xml:",chardata"`
}

// FormatGraphGraphML 将图序列化为 GraphML XML 格式字符串。
// 输出包含 XML 声明头，节点包含 type 属性，边包含 weight 和 reltype 属性。
func FormatGraphGraphML(g *ExportGraph) (string, error) {
	doc := graphMLDoc{
		Xmlns: "http://graphml.graphdrawing.org/graphml",
		Keys: []graphMLKey{
			{ID: "type", For: "node", AttrName: "type", AttrType: "string"},
			{ID: "weight", For: "edge", AttrName: "weight", AttrType: "double"},
			{ID: "reltype", For: "edge", AttrName: "reltype", AttrType: "string"},
		},
		Graph: graphMLGraph{
			ID:          "G",
			EdgeDefault: "directed",
		},
	}

	for _, node := range g.Nodes {
		n := graphMLNode{ID: node.ID}
		if node.Type != "" {
			n.Data = append(n.Data, graphMLData{Key: "type", Value: node.Type})
		}
		doc.Graph.Nodes = append(doc.Graph.Nodes, n)
	}

	for i, edge := range g.Edges {
		e := graphMLEdge{
			ID:     fmt.Sprintf("e%d", i),
			Source: edge.From,
			Target: edge.To,
			Data: []graphMLData{
				{Key: "weight", Value: fmt.Sprintf("%g", edge.Weight)},
				{Key: "reltype", Value: edge.RelType},
			},
		}
		doc.Graph.Edges = append(doc.Graph.Edges, e)
	}

	b, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("格式化 graphml: %w", err)
	}
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteByte('\n')
	sb.Write(b)
	return sb.String(), nil
}
