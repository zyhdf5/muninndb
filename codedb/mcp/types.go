package mcp

import (
	"time"
)

// ToolDefinition 描述一个 MCP 工具定义。
type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

// WriteResult 表示写入结果。
type WriteResult struct {
	ID       string   `json:"id"`
	Concept  string   `json:"concept,omitempty"`
	Hint     string   `json:"hint,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// ReadEntity 表示读取时返回的实体。
type ReadEntity struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// ReadEntityRel 表示读取时返回的实体关系。
type ReadEntityRel struct {
	FromEntity string  `json:"from_entity"`
	ToEntity   string  `json:"to_entity"`
	RelType    string  `json:"rel_type"`
	Weight     float32 `json:"weight,omitempty"`
}

// Memory 是统一的记忆输出结构。
type Memory struct {
	ID                  string          `json:"id"`
	Concept             string          `json:"concept"`
	Content             string          `json:"content"`
	Summary             string          `json:"summary,omitempty"`
	Score               float64         `json:"score,omitempty"`
	VectorScore         float64         `json:"vector_score,omitempty"`
	Confidence          float32         `json:"confidence,omitempty"`
	Why                 string          `json:"why,omitempty"`
	CreatedAt           time.Time       `json:"created_at,omitempty"`
	LastAccess          time.Time       `json:"last_access,omitempty"`
	AccessCount         uint32          `json:"access_count,omitempty"`
	Relevance           float32         `json:"relevance,omitempty"`
	SourceType          string          `json:"source_type,omitempty"`
	Tags                []string        `json:"tags,omitempty"`
	State               string          `json:"state,omitempty"`
	Entities            []ReadEntity    `json:"entities,omitempty"`
	EntityRelationships []ReadEntityRel `json:"entity_relationships,omitempty"`
}

// TraverseNode 表示图遍历节点。
type TraverseNode struct {
	ID      string `json:"id"`
	Concept string `json:"concept"`
	HopDist int    `json:"hop_dist"`
	Summary string `json:"summary,omitempty"`
}

// TraverseEdge 表示图遍历边。
type TraverseEdge struct {
	FromID  string  `json:"from_id"`
	ToID    string  `json:"to_id"`
	RelType string  `json:"rel_type,omitempty"`
	Weight  float32 `json:"weight"`
}

// TraverseResult 表示图遍历结果。
type TraverseResult struct {
	Nodes          []TraverseNode `json:"nodes"`
	Edges          []TraverseEdge `json:"edges"`
	TotalReachable int            `json:"total_reachable,omitempty"`
	QueryMs        float64        `json:"query_ms,omitempty"`
}

// EntitySummary 表示实体摘要。
type EntitySummary struct {
	Name         string `json:"name"`
	Type         string `json:"type,omitempty"`
	State        string `json:"state,omitempty"`
	MergedInto   string `json:"merged_into,omitempty"`
	MentionCount int32  `json:"mention_count,omitempty"`
}

// EntityRelRecord 表示实体关系记录。
type EntityRelRecord struct {
	FromEntity string  `json:"from_entity"`
	ToEntity   string  `json:"to_entity"`
	RelType    string  `json:"rel_type"`
	Weight     float32 `json:"weight,omitempty"`
}

// CoOccurringEntity 表示共现实体。
type CoOccurringEntity struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// EntityAggregate 表示实体聚合视图。
type EntityAggregate struct {
	Entity        EntitySummary       `json:"entity"`
	Engrams       []Memory            `json:"engrams"`
	Relationships []EntityRelRecord   `json:"relationships"`
	CoOccurring   []CoOccurringEntity `json:"co_occurring"`
}

// TreeNodeInput 表示树写入节点。
type TreeNodeInput struct {
	Concept  string          `json:"concept"`
	Content  string          `json:"content"`
	Type     string          `json:"type,omitempty"`
	Tags     []string        `json:"tags,omitempty"`
	Children []TreeNodeInput `json:"children,omitempty"`
}

// TreeNodeOutput 表示树读取节点。
type TreeNodeOutput struct {
	ID       string           `json:"id"`
	Concept  string           `json:"concept"`
	Content  string           `json:"content,omitempty"`
	Summary  string           `json:"summary,omitempty"`
	State    string           `json:"state,omitempty"`
	Children []TreeNodeOutput `json:"children,omitempty"`
}

// RememberTreeResult 表示树写入结果。
type RememberTreeResult struct {
	RootID  string            `json:"root_id"`
	NodeMap map[string]string `json:"node_map"`
}

// VaultStatus 表示仓状态。
type VaultStatus struct {
	EngramCount int64 `json:"engram_count"`
	VaultCount  int   `json:"vault_count"`
}

// SessionEntry 表示会话单条写入。
type SessionEntry struct {
	ID        string    `json:"id"`
	Concept   string    `json:"concept"`
	CreatedAt time.Time `json:"created_at"`
}

// SessionSummary 表示会话摘要。
type SessionSummary struct {
	Since       time.Time      `json:"since,omitempty"`
	Writes      []SessionEntry `json:"writes,omitempty"`
	Activations int            `json:"activations,omitempty"`
}

// ContradictionPair 表示矛盾对。
type ContradictionPair struct {
	IDa        string    `json:"id_a,omitempty"`
	ConceptA   string    `json:"concept_a,omitempty"`
	IDb        string    `json:"id_b,omitempty"`
	ConceptB   string    `json:"concept_b,omitempty"`
	DetectedAt time.Time `json:"detected_at,omitempty"`
}

// DeletedEngram 表示可恢复删除项。
type DeletedEngram struct {
	ID               string    `json:"id"`
	Concept          string    `json:"concept,omitempty"`
	DeletedAt        time.Time `json:"deleted_at,omitempty"`
	RecoverableUntil time.Time `json:"recoverable_until,omitempty"`
	Tags             []string  `json:"tags,omitempty"`
}

// ProvenanceEntry 表示审计轨迹条目。
type ProvenanceEntry struct {
	Timestamp string `json:"timestamp"`
	Source    string `json:"source"`
	AgentID   string `json:"agent_id,omitempty"`
	Operation string `json:"operation"`
	Note      string `json:"note,omitempty"`
}

// ProvenanceResult 表示审计轨迹结果。
type ProvenanceResult struct {
	ID      string            `json:"id"`
	Entries []ProvenanceEntry `json:"entries"`
}

// WhereLeftOffEntry 表示会话续接条目。
type WhereLeftOffEntry struct {
	ID         string    `json:"id"`
	Concept    string    `json:"concept"`
	Summary    string    `json:"summary,omitempty"`
	State      string    `json:"state,omitempty"`
	LastAccess time.Time `json:"last_access"`
}

// EntityClusterPair 表示实体共现对。
type EntityClusterPair struct {
	EntityA string `json:"entity_a"`
	EntityB string `json:"entity_b"`
	Count   int    `json:"count"`
}

// EntityClusterResult 表示实体共现结果。
type EntityClusterResult struct {
	Pairs []EntityClusterPair `json:"pairs"`
	Count int                 `json:"count"`
}

// DecideResult 表示决策写入结果。
type DecideResult struct {
	ID string `json:"id"`
}

// ExplainComponents 表示评分分解。
type ExplainComponents struct {
	FullTextRelevance  float64 `json:"full_text_relevance,omitempty"`
	SemanticSimilarity float64 `json:"semantic_similarity,omitempty"`
	DecayFactor        float64 `json:"decay_factor,omitempty"`
	HebbianBoost       float64 `json:"hebbian_boost,omitempty"`
	AccessFrequency    float64 `json:"access_frequency,omitempty"`
	Confidence         float64 `json:"confidence,omitempty"`
}

// ExplainResult 表示可解释评分结果。
type ExplainResult struct {
	EngramID    string            `json:"engram_id"`
	Concept     string            `json:"concept,omitempty"`
	FinalScore  float64           `json:"final_score,omitempty"`
	Components  ExplainComponents `json:"components,omitempty"`
	FTSMatches  []string          `json:"fts_matches,omitempty"`
	AssocPath   []string          `json:"assoc_path,omitempty"`
	WouldReturn bool              `json:"would_return,omitempty"`
	Threshold   float64           `json:"threshold,omitempty"`
}

// ResolvedPlasticity 表示解析后的认知配置。
type ResolvedPlasticity struct {
	BehaviorMode         string  `json:"behavior_mode"`
	BehaviorInstructions string  `json:"behavior_instructions"`
	HebbianEnabled       bool    `json:"hebbian_enabled"`
	PredictiveActivation bool    `json:"predictive_activation"`
	TemporalEnabled      bool    `json:"temporal_enabled"`
	HopDepth             int     `json:"hop_depth"`
	InlineEnrichment     string  `json:"inline_enrichment"`
	MaxEngrams           int     `json:"max_engrams"`
	RetentionDays        float64 `json:"retention_days"`
}

// InlineEntity 表示写入时内联实体。
type InlineEntity struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// InlineRelationship 表示写入时内联关联。
type InlineRelationship struct {
	TargetID string  `json:"target_id"`
	Relation string  `json:"relation"`
	Weight   float32 `json:"weight"`
}

// InlineEntityRelationship 表示写入时内联实体关系。
type InlineEntityRelationship struct {
	FromEntity string  `json:"from_entity"`
	ToEntity   string  `json:"to_entity"`
	RelType    string  `json:"rel_type"`
	Weight     float32 `json:"weight"`
}

// Weights 表示召回权重提示。
type Weights struct {
	SemanticSimilarity float32 `json:"semantic_similarity,omitempty"`
	FullTextRelevance  float32 `json:"full_text_relevance,omitempty"`
	Recency            float32 `json:"recency,omitempty"`
	DisableACTR        bool    `json:"disable_actr,omitempty"`
}

// Filter 表示查询过滤器。
type Filter struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value any    `json:"value"`
}

// EntityStateOp 表示实体状态变更操作。
type EntityStateOp struct {
	EntityName string `json:"entity_name"`
	State      string `json:"state"`
	MergedInto string `json:"merged_into,omitempty"`
	EntityType string `json:"type,omitempty"`
}

// SimilarEntityPair 表示相似实体对。
type SimilarEntityPair struct {
	EntityA    string  `json:"entity_a"`
	EntityB    string  `json:"entity_b"`
	Similarity float64 `json:"similarity"`
}

// MergeEntityResult 表示实体合并结果。
type MergeEntityResult struct {
	EntityA         string `json:"entity_a"`
	EntityB         string `json:"entity_b"`
	EngramsRelinked int    `json:"engrams_relinked"`
	DryRun          bool   `json:"dry_run"`
}

// ReplayEnrichmentResult 表示重放富化结果。
type ReplayEnrichmentResult struct {
	Processed int      `json:"processed"`
	Skipped   int      `json:"skipped"`
	Failed    int      `json:"failed"`
	Remaining int      `json:"remaining"`
	StagesRun []string `json:"stages_run"`
	DryRun    bool     `json:"dry_run"`
}
