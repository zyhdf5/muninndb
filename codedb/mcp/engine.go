package mcp

import (
	"context"
	"time"

	"github.com/scrypster/muninndb/codedb/engine"
)

// WriteRequest 表示写入参数。
type WriteRequest struct {
	Content             string                     `json:"content"`
	Concept             string                     `json:"concept,omitempty"`
	Tags                []string                   `json:"tags,omitempty"`
	Confidence          float32                    `json:"confidence,omitempty"`
	CreatedAt           *time.Time                 `json:"created_at,omitempty"`
	MemoryType          uint8                      `json:"memory_type,omitempty"`
	TypeLabel           string                     `json:"type_label,omitempty"`
	Summary             string                     `json:"summary,omitempty"`
	Entities            []InlineEntity             `json:"entities,omitempty"`
	Relationships       []InlineRelationship       `json:"relationships,omitempty"`
	EntityRelationships []InlineEntityRelationship `json:"entity_relationships,omitempty"`
	Embedding           []float32                  `json:"embedding,omitempty"`
	OpID                string                     `json:"op_id,omitempty"`
}

// ActivateRequest 表示召回参数。
type ActivateRequest struct {
	Context    []string  `json:"context"`
	Threshold  float32   `json:"threshold,omitempty"`
	MaxResults int       `json:"max_results,omitempty"`
	Profile    string    `json:"profile,omitempty"`
	MaxHops    int       `json:"max_hops,omitempty"`
	Weights    *Weights  `json:"weights,omitempty"`
	Filters    []Filter  `json:"filters,omitempty"`
	Embedding  []float32 `json:"embedding,omitempty"`
	Mode       string    `json:"mode,omitempty"`
}

// ActivateResponse 表示召回结果。
type ActivateResponse struct {
	Memories   []Memory `json:"memories"`
	TotalFound int      `json:"total_found"`
}

// EngineInterface 定义 mcp 包依赖的引擎能力。
type EngineInterface interface {
	Write(ctx context.Context, vault string, req *WriteRequest) (*WriteResult, error)
	WriteBatch(ctx context.Context, vault string, reqs []*WriteRequest) ([]*WriteResult, []error)
	Activate(ctx context.Context, vault string, req *ActivateRequest) (*ActivateResponse, error)
	Read(ctx context.Context, vault, id string) (*Memory, error)
	Forget(ctx context.Context, vault, id string) error
	Link(ctx context.Context, vault, sourceID, targetID string, relType uint16, weight float32) error
	Evolve(ctx context.Context, vault, id, newContent, reason string) error
	Consolidate(ctx context.Context, vault string, ids []string, mergedContent string) (string, error)
	GetContradictions(ctx context.Context, vault string) ([]ContradictionPair, error)
	GetStatus(ctx context.Context, vault string) (*VaultStatus, error)
	GetSession(ctx context.Context, vault, since string) (*SessionSummary, error)
	Decide(ctx context.Context, vault, decision, rationale string, alternatives, evidenceIDs []string) (string, error)
	Restore(ctx context.Context, vault, id string) error
	Traverse(ctx context.Context, vault, startID string, maxHops, maxNodes int, relTypes []string, followEntities bool) (*TraverseResult, error)
	Explain(ctx context.Context, vault, engramID string, query []string, embedding []float32) (any, error)
	SetState(ctx context.Context, vault, id, state, reason string) error
	ListDeleted(ctx context.Context, vault string, limit int) ([]DeletedEngram, error)
	RetryEnrich(ctx context.Context, vault, id string) error
	GetGuide(ctx context.Context, vault string) (string, error)
	WhereLeftOff(ctx context.Context, vault string, limit int) ([]WhereLeftOffEntry, error)
	RememberTree(ctx context.Context, vault string, root TreeNodeInput) (*RememberTreeResult, error)
	RecallTree(ctx context.Context, vault, rootID string, maxDepth, limit int, includeCompleted bool) (*TreeNodeOutput, error)
	AddChild(ctx context.Context, vault, parentID, concept, content, typ string, tags []string, ordinal *int, embedding []float32) (string, error)
	FindByEntity(ctx context.Context, vault, entityName string, limit int) ([]*engine.Engram, error)
	SetEntityState(ctx context.Context, vault string, ops []EntityStateOp) ([]error, error)
	GetEntityClusters(ctx context.Context, vault string, minCount, topN int) (*EntityClusterResult, error)
	ExportGraph(ctx context.Context, vault, format string, includeEngrams bool) (string, error)
	FindSimilarEntities(ctx context.Context, vault string, threshold float64, topN int) ([]SimilarEntityPair, error)
	MergeEntity(ctx context.Context, vault, entityA, entityB string, dryRun bool) (*MergeEntityResult, error)
	GetEntityTimeline(ctx context.Context, vault, entityName string, limit int) (any, error)
	ReplayEnrichment(ctx context.Context, vault string, stages []string, limit int, dryRun bool) (*ReplayEnrichmentResult, error)
	GetProvenance(ctx context.Context, vault, id string) ([]ProvenanceEntry, error)
	RecordFeedback(ctx context.Context, vault, engramID string, useful bool) error
	GetEntity(ctx context.Context, vault, name string, limit int) (*EntityAggregate, error)
	ListEntities(ctx context.Context, vault, state string, limit int) ([]EntitySummary, error)
	ResolvePlasticity(ctx context.Context, vault string) (*ResolvedPlasticity, error)
}
