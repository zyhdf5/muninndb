package engine

// ────────────────────────────────────────────────────────────────────
// 图导出数据结构
// ────────────────────────────────────────────────────────────────────

// GraphNode 表示导出图中的命名实体节点。
type GraphNode struct {
	ID   string // 实体名称（唯一标识）
	Type string // 实体类型（可选，来自实体注册表）
}

// GraphEdge 表示导出图中实体间的类型化关系边。
type GraphEdge struct {
	From    string  // 源实体名称
	To      string  // 目标实体名称
	RelType string  // 关系类型字符串
	Weight  float32 // 关系权重
}

// ExportGraph 保存 vault 的完整图结构：节点（实体）和边（关系）。
type ExportGraph struct {
	Nodes []GraphNode // 所有实体节点
	Edges []GraphEdge // 所有关系边
}
