package mcp

import (
	"context"
	"errors"

	"github.com/scrypster/muninndb/codedb/engine"
)

type mockEngine struct {
	WriteFn               func(context.Context, string, *WriteRequest) (*WriteResult, error)
	WriteBatchFn          func(context.Context, string, []*WriteRequest) ([]*WriteResult, []error)
	ActivateFn            func(context.Context, string, *ActivateRequest) (*ActivateResponse, error)
	ReadFn                func(context.Context, string, string) (*Memory, error)
	ForgetFn              func(context.Context, string, string) error
	LinkFn                func(context.Context, string, string, string, uint16, float32) error
	EvolveFn              func(context.Context, string, string, string, string) error
	ConsolidateFn         func(context.Context, string, []string, string) (string, error)
	GetContradictionsFn   func(context.Context, string) ([]ContradictionPair, error)
	GetStatusFn           func(context.Context, string) (*VaultStatus, error)
	GetSessionFn          func(context.Context, string, string) (*SessionSummary, error)
	DecideFn              func(context.Context, string, string, string, []string, []string) (string, error)
	RestoreFn             func(context.Context, string, string) error
	TraverseFn            func(context.Context, string, string, int, int, []string, bool) (*TraverseResult, error)
	ExplainFn             func(context.Context, string, string, []string, []float32) (any, error)
	SetStateFn            func(context.Context, string, string, string, string) error
	ListDeletedFn         func(context.Context, string, int) ([]DeletedEngram, error)
	RetryEnrichFn         func(context.Context, string, string) error
	GetGuideFn            func(context.Context, string) (string, error)
	WhereLeftOffFn        func(context.Context, string, int) ([]WhereLeftOffEntry, error)
	RememberTreeFn        func(context.Context, string, TreeNodeInput) (*RememberTreeResult, error)
	RecallTreeFn          func(context.Context, string, string, int, int, bool) (*TreeNodeOutput, error)
	AddChildFn            func(context.Context, string, string, string, string, string, []string, *int, []float32) (string, error)
	FindByEntityFn        func(context.Context, string, string, int) ([]*engine.Engram, error)
	SetEntityStateFn      func(context.Context, string, []EntityStateOp) ([]error, error)
	GetEntityClustersFn   func(context.Context, string, int, int) (*EntityClusterResult, error)
	ExportGraphFn         func(context.Context, string, string, bool) (string, error)
	FindSimilarEntitiesFn func(context.Context, string, float64, int) ([]SimilarEntityPair, error)
	MergeEntityFn         func(context.Context, string, string, string, bool) (*MergeEntityResult, error)
	GetEntityTimelineFn   func(context.Context, string, string, int) (any, error)
	ReplayEnrichmentFn    func(context.Context, string, []string, int, bool) (*ReplayEnrichmentResult, error)
	GetProvenanceFn       func(context.Context, string, string) ([]ProvenanceEntry, error)
	RecordFeedbackFn      func(context.Context, string, string, bool) error
	GetEntityFn           func(context.Context, string, string, int) (*EntityAggregate, error)
	ListEntitiesFn        func(context.Context, string, string, int) ([]EntitySummary, error)
	ResolvePlasticityFn   func(context.Context, string) (*ResolvedPlasticity, error)

	LastWriteReq            *WriteRequest
	LastWriteBatchReqs      []*WriteRequest
	LastActivateReq         *ActivateRequest
	LastReadID              string
	LastForgetID            string
	LastLinkSourceID        string
	LastLinkTargetID        string
	LastLinkRelType         uint16
	LastLinkWeight          float32
	LastEvolveID            string
	LastConsolidateIDs      []string
	LastConsolidateContent  string
	LastSessionSince        string
	LastTraverseStartID     string
	LastTraverseMaxHops     int
	LastTraverseMaxNodes    int
	LastTraverseRelTypes    []string
	LastTraverseFollowEnt   bool
	LastExplainID           string
	LastExplainQuery        []string
	LastExplainEmbedding    []float32
	LastSetStateID          string
	LastSetState            string
	LastSetStateReason      string
	LastListDeletedLimit    int
	LastWhereLeftOffLimit   int
	LastRememberTreeRoot    TreeNodeInput
	LastRecallTreeRootID    string
	LastRecallTreeDepth     int
	LastRecallTreeLimit     int
	LastRecallTreeInclude   bool
	LastAddChildParentID    string
	LastAddChildConcept     string
	LastAddChildContent     string
	LastAddChildType        string
	LastAddChildTags        []string
	LastAddChildOrdinal     *int
	LastAddChildEmbedding   []float32
	LastFindByEntityName    string
	LastFindByEntityLimit   int
	LastEntityStateOps      []EntityStateOp
	LastEntityClustersMin   int
	LastEntityClustersTopN  int
	LastExportFormat        string
	LastExportIncludeEngram bool
	LastSimilarThreshold    float64
	LastSimilarTopN         int
	LastMergeEntityA        string
	LastMergeEntityB        string
	LastMergeDryRun         bool
	LastEntityTimelineName  string
	LastEntityTimelineLimit int
	LastReplayStages        []string
	LastReplayLimit         int
	LastReplayDryRun        bool
	LastProvenanceID        string
	LastFeedbackID          string
	LastFeedbackUseful      bool
	LastEntityName          string
	LastEntityLimit         int
	LastEntitiesState       string
	LastEntitiesLimit       int
}

func (m *mockEngine) Write(ctx context.Context, vault string, req *WriteRequest) (*WriteResult, error) {
	m.LastWriteReq = req
	if m.WriteFn != nil {
		return m.WriteFn(ctx, vault, req)
	}
	return &WriteResult{ID: "w-default", Concept: req.Concept}, nil
}

func (m *mockEngine) WriteBatch(ctx context.Context, vault string, reqs []*WriteRequest) ([]*WriteResult, []error) {
	m.LastWriteBatchReqs = reqs
	if m.WriteBatchFn != nil {
		return m.WriteBatchFn(ctx, vault, reqs)
	}
	res := make([]*WriteResult, len(reqs))
	errList := make([]error, len(reqs))
	for i := range reqs {
		res[i] = &WriteResult{ID: "wb-default", Concept: reqs[i].Concept}
	}
	return res, errList
}

func (m *mockEngine) Activate(ctx context.Context, vault string, req *ActivateRequest) (*ActivateResponse, error) {
	m.LastActivateReq = req
	if m.ActivateFn != nil {
		return m.ActivateFn(ctx, vault, req)
	}
	return &ActivateResponse{}, nil
}

func (m *mockEngine) Read(ctx context.Context, vault, id string) (*Memory, error) {
	m.LastReadID = id
	if m.ReadFn != nil {
		return m.ReadFn(ctx, vault, id)
	}
	return &Memory{ID: id, Concept: "default", Content: "default"}, nil
}

func (m *mockEngine) Forget(ctx context.Context, vault, id string) error {
	m.LastForgetID = id
	if m.ForgetFn != nil {
		return m.ForgetFn(ctx, vault, id)
	}
	return nil
}

func (m *mockEngine) Link(ctx context.Context, vault, sourceID, targetID string, relType uint16, weight float32) error {
	m.LastLinkSourceID = sourceID
	m.LastLinkTargetID = targetID
	m.LastLinkRelType = relType
	m.LastLinkWeight = weight
	if m.LinkFn != nil {
		return m.LinkFn(ctx, vault, sourceID, targetID, relType, weight)
	}
	return nil
}

func (m *mockEngine) Evolve(ctx context.Context, vault, id, newContent, reason string) error {
	m.LastEvolveID = id
	if m.EvolveFn != nil {
		return m.EvolveFn(ctx, vault, id, newContent, reason)
	}
	return nil
}

func (m *mockEngine) Consolidate(ctx context.Context, vault string, ids []string, mergedContent string) (string, error) {
	m.LastConsolidateIDs = ids
	m.LastConsolidateContent = mergedContent
	if m.ConsolidateFn != nil {
		return m.ConsolidateFn(ctx, vault, ids, mergedContent)
	}
	return "merged-default", nil
}

func (m *mockEngine) GetContradictions(ctx context.Context, vault string) ([]ContradictionPair, error) {
	if m.GetContradictionsFn != nil {
		return m.GetContradictionsFn(ctx, vault)
	}
	return nil, nil
}

func (m *mockEngine) GetStatus(ctx context.Context, vault string) (*VaultStatus, error) {
	if m.GetStatusFn != nil {
		return m.GetStatusFn(ctx, vault)
	}
	return &VaultStatus{}, nil
}

func (m *mockEngine) GetSession(ctx context.Context, vault, since string) (*SessionSummary, error) {
	m.LastSessionSince = since
	if m.GetSessionFn != nil {
		return m.GetSessionFn(ctx, vault, since)
	}
	return &SessionSummary{}, nil
}

func (m *mockEngine) Decide(ctx context.Context, vault, decision, rationale string, alternatives, evidenceIDs []string) (string, error) {
	if m.DecideFn != nil {
		return m.DecideFn(ctx, vault, decision, rationale, alternatives, evidenceIDs)
	}
	return "decide-default", nil
}

func (m *mockEngine) Restore(ctx context.Context, vault, id string) error {
	if m.RestoreFn != nil {
		return m.RestoreFn(ctx, vault, id)
	}
	return nil
}

func (m *mockEngine) Traverse(ctx context.Context, vault, startID string, maxHops, maxNodes int, relTypes []string, followEntities bool) (*TraverseResult, error) {
	m.LastTraverseStartID = startID
	m.LastTraverseMaxHops = maxHops
	m.LastTraverseMaxNodes = maxNodes
	m.LastTraverseRelTypes = relTypes
	m.LastTraverseFollowEnt = followEntities
	if m.TraverseFn != nil {
		return m.TraverseFn(ctx, vault, startID, maxHops, maxNodes, relTypes, followEntities)
	}
	return &TraverseResult{}, nil
}

func (m *mockEngine) Explain(ctx context.Context, vault, engramID string, query []string, embedding []float32) (any, error) {
	m.LastExplainID = engramID
	m.LastExplainQuery = query
	m.LastExplainEmbedding = embedding
	if m.ExplainFn != nil {
		return m.ExplainFn(ctx, vault, engramID, query, embedding)
	}
	return map[string]any{"ok": true}, nil
}

func (m *mockEngine) SetState(ctx context.Context, vault, id, state, reason string) error {
	m.LastSetStateID = id
	m.LastSetState = state
	m.LastSetStateReason = reason
	if m.SetStateFn != nil {
		return m.SetStateFn(ctx, vault, id, state, reason)
	}
	return nil
}

func (m *mockEngine) ListDeleted(ctx context.Context, vault string, limit int) ([]DeletedEngram, error) {
	m.LastListDeletedLimit = limit
	if m.ListDeletedFn != nil {
		return m.ListDeletedFn(ctx, vault, limit)
	}
	return nil, nil
}

func (m *mockEngine) RetryEnrich(ctx context.Context, vault, id string) error {
	if m.RetryEnrichFn != nil {
		return m.RetryEnrichFn(ctx, vault, id)
	}
	return nil
}

func (m *mockEngine) GetGuide(ctx context.Context, vault string) (string, error) {
	if m.GetGuideFn != nil {
		return m.GetGuideFn(ctx, vault)
	}
	return "", errors.New("no guide")
}

func (m *mockEngine) WhereLeftOff(ctx context.Context, vault string, limit int) ([]WhereLeftOffEntry, error) {
	m.LastWhereLeftOffLimit = limit
	if m.WhereLeftOffFn != nil {
		return m.WhereLeftOffFn(ctx, vault, limit)
	}
	return nil, nil
}

func (m *mockEngine) RememberTree(ctx context.Context, vault string, root TreeNodeInput) (*RememberTreeResult, error) {
	m.LastRememberTreeRoot = root
	if m.RememberTreeFn != nil {
		return m.RememberTreeFn(ctx, vault, root)
	}
	return &RememberTreeResult{RootID: "root-default", NodeMap: map[string]string{"root": "root-default"}}, nil
}

func (m *mockEngine) RecallTree(ctx context.Context, vault, rootID string, maxDepth, limit int, includeCompleted bool) (*TreeNodeOutput, error) {
	m.LastRecallTreeRootID = rootID
	m.LastRecallTreeDepth = maxDepth
	m.LastRecallTreeLimit = limit
	m.LastRecallTreeInclude = includeCompleted
	if m.RecallTreeFn != nil {
		return m.RecallTreeFn(ctx, vault, rootID, maxDepth, limit, includeCompleted)
	}
	return &TreeNodeOutput{ID: rootID, Concept: "root"}, nil
}

func (m *mockEngine) AddChild(ctx context.Context, vault, parentID, concept, content, typ string, tags []string, ordinal *int, embedding []float32) (string, error) {
	m.LastAddChildParentID = parentID
	m.LastAddChildConcept = concept
	m.LastAddChildContent = content
	m.LastAddChildType = typ
	m.LastAddChildTags = tags
	m.LastAddChildOrdinal = ordinal
	m.LastAddChildEmbedding = embedding
	if m.AddChildFn != nil {
		return m.AddChildFn(ctx, vault, parentID, concept, content, typ, tags, ordinal, embedding)
	}
	return "child-default", nil
}

func (m *mockEngine) FindByEntity(ctx context.Context, vault, entityName string, limit int) ([]*engine.Engram, error) {
	m.LastFindByEntityName = entityName
	m.LastFindByEntityLimit = limit
	if m.FindByEntityFn != nil {
		return m.FindByEntityFn(ctx, vault, entityName, limit)
	}
	return nil, nil
}

func (m *mockEngine) SetEntityState(ctx context.Context, vault string, ops []EntityStateOp) ([]error, error) {
	m.LastEntityStateOps = ops
	if m.SetEntityStateFn != nil {
		return m.SetEntityStateFn(ctx, vault, ops)
	}
	return make([]error, len(ops)), nil
}

func (m *mockEngine) GetEntityClusters(ctx context.Context, vault string, minCount, topN int) (*EntityClusterResult, error) {
	m.LastEntityClustersMin = minCount
	m.LastEntityClustersTopN = topN
	if m.GetEntityClustersFn != nil {
		return m.GetEntityClustersFn(ctx, vault, minCount, topN)
	}
	return &EntityClusterResult{}, nil
}

func (m *mockEngine) ExportGraph(ctx context.Context, vault, format string, includeEngrams bool) (string, error) {
	m.LastExportFormat = format
	m.LastExportIncludeEngram = includeEngrams
	if m.ExportGraphFn != nil {
		return m.ExportGraphFn(ctx, vault, format, includeEngrams)
	}
	return "{}", nil
}

func (m *mockEngine) FindSimilarEntities(ctx context.Context, vault string, threshold float64, topN int) ([]SimilarEntityPair, error) {
	m.LastSimilarThreshold = threshold
	m.LastSimilarTopN = topN
	if m.FindSimilarEntitiesFn != nil {
		return m.FindSimilarEntitiesFn(ctx, vault, threshold, topN)
	}
	return nil, nil
}

func (m *mockEngine) MergeEntity(ctx context.Context, vault, entityA, entityB string, dryRun bool) (*MergeEntityResult, error) {
	m.LastMergeEntityA = entityA
	m.LastMergeEntityB = entityB
	m.LastMergeDryRun = dryRun
	if m.MergeEntityFn != nil {
		return m.MergeEntityFn(ctx, vault, entityA, entityB, dryRun)
	}
	return &MergeEntityResult{EntityA: entityA, EntityB: entityB, DryRun: dryRun}, nil
}

func (m *mockEngine) GetEntityTimeline(ctx context.Context, vault, entityName string, limit int) (any, error) {
	m.LastEntityTimelineName = entityName
	m.LastEntityTimelineLimit = limit
	if m.GetEntityTimelineFn != nil {
		return m.GetEntityTimelineFn(ctx, vault, entityName, limit)
	}
	return map[string]any{"entity": entityName, "limit": limit}, nil
}

func (m *mockEngine) ReplayEnrichment(ctx context.Context, vault string, stages []string, limit int, dryRun bool) (*ReplayEnrichmentResult, error) {
	m.LastReplayStages = stages
	m.LastReplayLimit = limit
	m.LastReplayDryRun = dryRun
	if m.ReplayEnrichmentFn != nil {
		return m.ReplayEnrichmentFn(ctx, vault, stages, limit, dryRun)
	}
	return &ReplayEnrichmentResult{}, nil
}

func (m *mockEngine) GetProvenance(ctx context.Context, vault, id string) ([]ProvenanceEntry, error) {
	m.LastProvenanceID = id
	if m.GetProvenanceFn != nil {
		return m.GetProvenanceFn(ctx, vault, id)
	}
	return nil, nil
}

func (m *mockEngine) RecordFeedback(ctx context.Context, vault, engramID string, useful bool) error {
	m.LastFeedbackID = engramID
	m.LastFeedbackUseful = useful
	if m.RecordFeedbackFn != nil {
		return m.RecordFeedbackFn(ctx, vault, engramID, useful)
	}
	return nil
}

func (m *mockEngine) GetEntity(ctx context.Context, vault, name string, limit int) (*EntityAggregate, error) {
	m.LastEntityName = name
	m.LastEntityLimit = limit
	if m.GetEntityFn != nil {
		return m.GetEntityFn(ctx, vault, name, limit)
	}
	return &EntityAggregate{Entity: EntitySummary{Name: name}}, nil
}

func (m *mockEngine) ListEntities(ctx context.Context, vault, state string, limit int) ([]EntitySummary, error) {
	m.LastEntitiesState = state
	m.LastEntitiesLimit = limit
	if m.ListEntitiesFn != nil {
		return m.ListEntitiesFn(ctx, vault, state, limit)
	}
	return nil, nil
}

func (m *mockEngine) ResolvePlasticity(ctx context.Context, vault string) (*ResolvedPlasticity, error) {
	if m.ResolvePlasticityFn != nil {
		return m.ResolvePlasticityFn(ctx, vault)
	}
	return &ResolvedPlasticity{BehaviorMode: "autonomous", InlineEnrichment: "sync"}, nil
}
