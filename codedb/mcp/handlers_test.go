package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/scrypster/muninndb/codedb/engine"
)

const testVault = "v-test"

func mustErr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleRemember(t *testing.T) {
	ctx := context.Background()

	t.Run("success with clamping and enrichment", func(t *testing.T) {
		// given: 包含所有关键参数
		eng := &mockEngine{}
		long := strings.Repeat("x", 520)
		args := map[string]any{
			"content":    long,
			"concept":    "c1",
			"confidence": 9.9,
			"created_at": "2026-03-31T10:00:00Z",
			"tags":       []any{"a", "b"},
			"type":       "decision",
			"summary":    "s",
			"entities": []any{
				map[string]any{"name": "Alice", "type": "person"},
				"bad",
			},
			"embedding": []any{1.0, 2.0},
		}

		// when: 调用 remember
		out, err := HandleRemember(ctx, eng, testVault, args)
		mustNoErr(t, err)

		// then: 请求参数被规范化并返回提示
		wr := eng.LastWriteReq
		if wr == nil || wr.Confidence != 1 || wr.CreatedAt == nil || wr.TypeLabel != "decision" {
			t.Fatalf("unexpected write request: %+v", wr)
		}
		if len(wr.Entities) != 1 || len(wr.Embedding) != 2 {
			t.Fatalf("unexpected enrichment args: %+v", wr)
		}
		res := out.(*WriteResult)
		if res.Hint == "" || len(res.Warnings) == 0 {
			t.Fatalf("expected hint/warnings in response: %+v", res)
		}
	})

	t.Run("error content required", func(t *testing.T) {
		// given/when: content 缺失
		_, err := HandleRemember(ctx, &mockEngine{}, testVault, map[string]any{})
		// then: 返回参数错误
		mustErr(t, err)
	})
}

func TestHandleRememberBatch(t *testing.T) {
	ctx := context.Background()
	t.Run("success partial failures", func(t *testing.T) {
		// given: 两条写入，第二条引擎失败
		eng := &mockEngine{WriteBatchFn: func(_ context.Context, _ string, reqs []*WriteRequest) ([]*WriteResult, []error) {
			return []*WriteResult{{ID: "1", Concept: reqs[0].Concept}, nil}, []error{nil, errors.New("boom")}
		}}
		args := map[string]any{"memories": []any{
			map[string]any{"content": "a", "concept": "c1", "entities": []any{"bad"}},
			map[string]any{"content": "b", "concept": "c2"},
		}}

		// when
		out, err := HandleRememberBatch(ctx, eng, testVault, args)
		mustNoErr(t, err)

		// then
		m := out.(map[string]any)
		items := m["results"].([]map[string]any)
		if len(items) != 2 || items[0]["status"] != "ok" || items[1]["status"] != "error" {
			t.Fatalf("unexpected batch results: %#v", items)
		}
	})

	t.Run("error exceeds max 50", func(t *testing.T) {
		// given: 51 条
		mems := make([]any, 51)
		for i := range mems {
			mems[i] = map[string]any{"content": "x"}
		}
		// when
		_, err := HandleRememberBatch(ctx, &mockEngine{}, testVault, map[string]any{"memories": mems})
		// then
		mustErr(t, err)
	})
}

func TestHandleRecall(t *testing.T) {
	ctx := context.Background()
	t.Run("success mode and filters", func(t *testing.T) {
		// given
		eng := &mockEngine{ActivateFn: func(_ context.Context, _ string, req *ActivateRequest) (*ActivateResponse, error) {
			if req.Mode != "recent" || req.MaxHops != 1 || req.MaxResults != 100 {
				return nil, errors.New("mode preset or limit normalize mismatch")
			}
			if len(req.Filters) != 2 || len(req.Embedding) != 2 {
				return nil, errors.New("filters/embedding mismatch")
			}
			return &ActivateResponse{Memories: []Memory{{ID: "m1"}}, TotalFound: 1}, nil
		}}
		args := map[string]any{
			"context":   []any{"topic"},
			"mode":      "recent",
			"limit":     999,
			"since":     "2026-03-30T00:00:00Z",
			"before":    "2026-03-31T00:00:00Z",
			"embedding": []any{1.0, 2.0},
		}

		// when
		out, err := HandleRecall(ctx, eng, testVault, args)
		mustNoErr(t, err)
		m := out.(map[string]any)
		if m["total"].(int) != 1 {
			t.Fatalf("unexpected total: %#v", m)
		}
	})

	t.Run("error missing context", func(t *testing.T) {
		// given/when
		_, err := HandleRecall(ctx, &mockEngine{}, testVault, map[string]any{})
		// then
		mustErr(t, err)
	})
}

func TestHandleRead(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		// given
		eng := &mockEngine{ReadFn: func(_ context.Context, _ string, id string) (*Memory, error) {
			return &Memory{ID: id, Concept: "c", Content: "x"}, nil
		}}
		// when
		out, err := HandleRead(ctx, eng, testVault, map[string]any{"id": "id1"})
		mustNoErr(t, err)
		// then
		if out.(*Memory).ID != "id1" {
			t.Fatalf("unexpected read result: %+v", out)
		}
	})
	t.Run("error missing id", func(t *testing.T) {
		_, err := HandleRead(ctx, &mockEngine{}, testVault, map[string]any{})
		mustErr(t, err)
	})
}

func TestHandleForget(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		eng := &mockEngine{}
		out, err := HandleForget(ctx, eng, testVault, map[string]any{"id": "id1"})
		mustNoErr(t, err)
		if !out.(map[string]any)["ok"].(bool) {
			t.Fatal("expected ok=true")
		}
	})
	t.Run("error missing id", func(t *testing.T) {
		_, err := HandleForget(ctx, &mockEngine{}, testVault, map[string]any{})
		mustErr(t, err)
	})
}

func TestHandleLink(t *testing.T) {
	ctx := context.Background()
	t.Run("success relation fallback and weight clamp", func(t *testing.T) {
		eng := &mockEngine{}
		_, err := HandleLink(ctx, eng, testVault, map[string]any{
			"source_id": "s1", "target_id": "t1", "relation": "unknown", "weight": 9.9,
		})
		mustNoErr(t, err)
		if eng.LastLinkRelType != relTypeMap["relates_to"] || eng.LastLinkWeight != 1 {
			t.Fatalf("unexpected link args: rel=%d w=%f", eng.LastLinkRelType, eng.LastLinkWeight)
		}
	})
	t.Run("error required fields", func(t *testing.T) {
		_, err := HandleLink(ctx, &mockEngine{}, testVault, map[string]any{"source_id": "s1"})
		mustErr(t, err)
	})
}

func TestHandleContradictions(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		eng := &mockEngine{GetContradictionsFn: func(_ context.Context, _ string) ([]ContradictionPair, error) {
			return []ContradictionPair{{ConceptA: "a", ConceptB: "b"}}, nil
		}}
		out, err := HandleContradictions(ctx, eng, testVault, nil)
		mustNoErr(t, err)
		if len(out.(map[string]any)["contradictions"].([]ContradictionPair)) != 1 {
			t.Fatal("expected one contradiction")
		}
	})
	t.Run("error from engine", func(t *testing.T) {
		eng := &mockEngine{GetContradictionsFn: func(_ context.Context, _ string) ([]ContradictionPair, error) { return nil, errors.New("boom") }}
		_, err := HandleContradictions(ctx, eng, testVault, nil)
		mustErr(t, err)
	})
}

func TestHandleStatus(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		out, err := HandleStatus(ctx, &mockEngine{GetStatusFn: func(_ context.Context, _ string) (*VaultStatus, error) {
			return &VaultStatus{EngramCount: 3, VaultCount: 1}, nil
		}}, testVault, nil)
		mustNoErr(t, err)
		if out.(*VaultStatus).EngramCount != 3 {
			t.Fatal("unexpected status")
		}
	})
	t.Run("error", func(t *testing.T) {
		_, err := HandleStatus(ctx, &mockEngine{GetStatusFn: func(_ context.Context, _ string) (*VaultStatus, error) {
			return nil, errors.New("boom")
		}}, testVault, nil)
		mustErr(t, err)
	})
}

func TestHandleEvolve(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		out, err := HandleEvolve(ctx, &mockEngine{}, testVault, map[string]any{"id": "i", "new_content": "n", "reason": "r"})
		mustNoErr(t, err)
		if !out.(map[string]any)["updated"].(bool) {
			t.Fatal("expected updated=true")
		}
	})
	t.Run("error missing params", func(t *testing.T) {
		_, err := HandleEvolve(ctx, &mockEngine{}, testVault, map[string]any{"id": "i"})
		mustErr(t, err)
	})
}

func TestHandleConsolidate(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		eng := &mockEngine{ConsolidateFn: func(_ context.Context, _ string, ids []string, _ string) (string, error) {
			if len(ids) != 2 {
				return "", errors.New("bad ids")
			}
			return "merged1", nil
		}}
		out, err := HandleConsolidate(ctx, eng, testVault, map[string]any{"ids": []any{"a", "b"}, "merged_content": "mc"})
		mustNoErr(t, err)
		if out.(map[string]any)["id"].(string) != "merged1" {
			t.Fatal("unexpected consolidate id")
		}
	})
	t.Run("error too few ids", func(t *testing.T) {
		_, err := HandleConsolidate(ctx, &mockEngine{}, testVault, map[string]any{"ids": []any{"a"}, "merged_content": "mc"})
		mustErr(t, err)
	})
}

func TestHandleSession(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		out, err := HandleSession(ctx, &mockEngine{}, testVault, map[string]any{"since": "2026-03-31T00:00:00Z"})
		mustNoErr(t, err)
		if out == nil {
			t.Fatal("expected non-nil summary")
		}
	})
	t.Run("error invalid since", func(t *testing.T) {
		_, err := HandleSession(ctx, &mockEngine{}, testVault, map[string]any{"since": "bad"})
		mustErr(t, err)
	})
}

func TestHandleDecide(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		out, err := HandleDecide(ctx, &mockEngine{}, testVault, map[string]any{"decision": "d", "rationale": "r", "alternatives": []any{"a"}})
		mustNoErr(t, err)
		if out.(DecideResult).ID == "" {
			t.Fatal("expected id")
		}
	})
	t.Run("error required", func(t *testing.T) {
		_, err := HandleDecide(ctx, &mockEngine{}, testVault, map[string]any{"decision": "d"})
		mustErr(t, err)
	})
}

func TestHandleRestore(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		out, err := HandleRestore(ctx, &mockEngine{}, testVault, map[string]any{"id": "i"})
		mustNoErr(t, err)
		if !out.(map[string]any)["restored"].(bool) {
			t.Fatal("expected restored=true")
		}
	})
	t.Run("error id required", func(t *testing.T) {
		_, err := HandleRestore(ctx, &mockEngine{}, testVault, map[string]any{})
		mustErr(t, err)
	})
}

func TestHandleTraverse(t *testing.T) {
	ctx := context.Background()
	t.Run("success defaults and limits", func(t *testing.T) {
		eng := &mockEngine{}
		_, err := HandleTraverse(ctx, eng, testVault, map[string]any{"start_id": "s", "max_hops": 99, "max_nodes": 999, "follow_entities": true})
		mustNoErr(t, err)
		if eng.LastTraverseMaxHops != 5 || eng.LastTraverseMaxNodes != 100 || !eng.LastTraverseFollowEnt {
			t.Fatalf("unexpected traverse args: hops=%d nodes=%d follow=%v", eng.LastTraverseMaxHops, eng.LastTraverseMaxNodes, eng.LastTraverseFollowEnt)
		}
	})
	t.Run("error required", func(t *testing.T) {
		_, err := HandleTraverse(ctx, &mockEngine{}, testVault, map[string]any{})
		mustErr(t, err)
	})
}

func TestHandleExplain(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		eng := &mockEngine{}
		out, err := HandleExplain(ctx, eng, testVault, map[string]any{"engram_id": "e1", "query": []any{"q1"}, "embedding": []any{1.0}})
		mustNoErr(t, err)
		if out.(map[string]any)["ok"] != true || eng.LastExplainID != "e1" || len(eng.LastExplainEmbedding) != 1 {
			t.Fatalf("unexpected explain path: out=%#v eng=%+v", out, eng)
		}
	})
	t.Run("error query required", func(t *testing.T) {
		_, err := HandleExplain(ctx, &mockEngine{}, testVault, map[string]any{"engram_id": "e1"})
		mustErr(t, err)
	})
}

func TestHandleState(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		eng := &mockEngine{}
		out, err := HandleState(ctx, eng, testVault, map[string]any{"id": "i", "state": "active", "reason": "r"})
		mustNoErr(t, err)
		if out.(map[string]any)["state"].(string) != "active" || eng.LastSetStateReason != "r" {
			t.Fatalf("unexpected state result: %#v", out)
		}
	})
	t.Run("error invalid state", func(t *testing.T) {
		_, err := HandleState(ctx, &mockEngine{}, testVault, map[string]any{"id": "i", "state": "bad"})
		mustErr(t, err)
	})
}

func TestHandleListDeleted(t *testing.T) {
	ctx := context.Background()
	t.Run("success nil list becomes empty", func(t *testing.T) {
		eng := &mockEngine{}
		out, err := HandleListDeleted(ctx, eng, testVault, map[string]any{"limit": -9})
		mustNoErr(t, err)
		m := out.(map[string]any)
		if m["count"].(int) != 0 || eng.LastListDeletedLimit != 0 {
			t.Fatalf("unexpected list_deleted: %#v limit=%d", m, eng.LastListDeletedLimit)
		}
	})
	t.Run("error from engine", func(t *testing.T) {
		eng := &mockEngine{ListDeletedFn: func(_ context.Context, _ string, _ int) ([]DeletedEngram, error) { return nil, errors.New("boom") }}
		_, err := HandleListDeleted(ctx, eng, testVault, nil)
		mustErr(t, err)
	})
}

func TestHandleRetryEnrich(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		out, err := HandleRetryEnrich(ctx, &mockEngine{}, testVault, map[string]any{"id": "i"})
		mustNoErr(t, err)
		if !out.(map[string]any)["queued"].(bool) {
			t.Fatal("expected queued=true")
		}
	})
	t.Run("error id required", func(t *testing.T) {
		_, err := HandleRetryEnrich(ctx, &mockEngine{}, testVault, map[string]any{})
		mustErr(t, err)
	})
}

func TestHandleGuide(t *testing.T) {
	ctx := context.Background()
	t.Run("success from engine guide", func(t *testing.T) {
		eng := &mockEngine{GetGuideFn: func(_ context.Context, _ string) (string, error) { return "g1", nil }}
		out, err := HandleGuide(ctx, eng, testVault, nil)
		mustNoErr(t, err)
		if out.(map[string]any)["content"] == nil {
			t.Fatalf("unexpected guide output: %#v", out)
		}
	})
	t.Run("fallback and error path", func(t *testing.T) {
		eng := &mockEngine{
			GetGuideFn: func(_ context.Context, _ string) (string, error) { return "", errors.New("no") },
			ResolvePlasticityFn: func(_ context.Context, _ string) (*ResolvedPlasticity, error) {
				return nil, errors.New("boom")
			},
		}
		_, err := HandleGuide(ctx, eng, testVault, nil)
		mustErr(t, err)
	})
}

func TestHandleWhereLeftOff(t *testing.T) {
	ctx := context.Background()
	t.Run("success clamp", func(t *testing.T) {
		eng := &mockEngine{}
		out, err := HandleWhereLeftOff(ctx, eng, testVault, map[string]any{"limit": 999})
		mustNoErr(t, err)
		if eng.LastWhereLeftOffLimit != 50 || out.(map[string]any)["count"].(int) != 0 {
			t.Fatalf("unexpected where_left_off: limit=%d out=%#v", eng.LastWhereLeftOffLimit, out)
		}
	})
	t.Run("error", func(t *testing.T) {
		eng := &mockEngine{WhereLeftOffFn: func(_ context.Context, _ string, _ int) ([]WhereLeftOffEntry, error) { return nil, errors.New("boom") }}
		_, err := HandleWhereLeftOff(ctx, eng, testVault, nil)
		mustErr(t, err)
	})
}

func TestHandleRememberTree(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		eng := &mockEngine{}
		out, err := HandleRememberTree(ctx, eng, testVault, map[string]any{"root": map[string]any{"concept": "root", "content": "c"}})
		mustNoErr(t, err)
		if out.(*RememberTreeResult).RootID == "" || eng.LastRememberTreeRoot.Concept != "root" {
			t.Fatalf("unexpected remember_tree output: %+v", out)
		}
	})
	t.Run("error root required", func(t *testing.T) {
		_, err := HandleRememberTree(ctx, &mockEngine{}, testVault, map[string]any{})
		mustErr(t, err)
	})
}

func TestHandleRecallTree(t *testing.T) {
	ctx := context.Background()
	t.Run("success bounds", func(t *testing.T) {
		eng := &mockEngine{}
		out, err := HandleRecallTree(ctx, eng, testVault, map[string]any{"root_id": "r1", "max_depth": 99, "limit": 9999, "include_completed": false})
		mustNoErr(t, err)
		if eng.LastRecallTreeDepth != 50 || eng.LastRecallTreeLimit != 1000 || eng.LastRecallTreeInclude {
			t.Fatalf("unexpected recall_tree args: %+v", eng)
		}
		if out.(*TreeNodeOutput).ID != "r1" {
			t.Fatal("unexpected tree output")
		}
	})
	t.Run("error root_id required", func(t *testing.T) {
		_, err := HandleRecallTree(ctx, &mockEngine{}, testVault, map[string]any{})
		mustErr(t, err)
	})
}

func TestHandleAddChild(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		eng := &mockEngine{}
		out, err := HandleAddChild(ctx, eng, testVault, map[string]any{
			"parent_id": "p", "concept": "c", "content": "x", "ordinal": 2, "embedding": []any{1.0},
		})
		mustNoErr(t, err)
		if out.(map[string]any)["child_id"].(string) == "" || eng.LastAddChildOrdinal == nil || *eng.LastAddChildOrdinal != 2 {
			t.Fatalf("unexpected add_child output: %#v eng=%+v", out, eng)
		}
	})
	t.Run("error required fields", func(t *testing.T) {
		_, err := HandleAddChild(ctx, &mockEngine{}, testVault, map[string]any{"parent_id": "p"})
		mustErr(t, err)
	})
}

func TestHandleFindByEntity(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		id := engine.NewULIDWithTime(time.Now())
		eng := &mockEngine{FindByEntityFn: func(_ context.Context, _ string, _ string, _ int) ([]*engine.Engram, error) {
			return []*engine.Engram{{ID: id, Concept: "c", Summary: "s", State: engine.StateActive}}, nil
		}}
		out, err := HandleFindByEntity(ctx, eng, testVault, map[string]any{"entity_name": "alice", "limit": 0})
		mustNoErr(t, err)
		if out.(map[string]any)["count"].(int) != 1 || eng.LastFindByEntityLimit != 1 {
			t.Fatalf("unexpected find_by_entity output: %#v", out)
		}
	})
	t.Run("error entity_name required", func(t *testing.T) {
		_, err := HandleFindByEntity(ctx, &mockEngine{}, testVault, map[string]any{})
		mustErr(t, err)
	})
}

func TestHandleEntityState(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		eng := &mockEngine{}
		out, err := HandleEntityState(ctx, eng, testVault, map[string]any{"entity_name": "alice", "state": "active", "type": "person"})
		mustNoErr(t, err)
		if out.(map[string]any)["state"].(string) != "active" || len(eng.LastEntityStateOps) != 1 {
			t.Fatalf("unexpected entity_state output: %#v", out)
		}
	})
	t.Run("error merged without merged_into", func(t *testing.T) {
		_, err := HandleEntityState(ctx, &mockEngine{}, testVault, map[string]any{"entity_name": "alice", "state": "merged"})
		mustErr(t, err)
	})
}

func TestHandleEntityStateBatch(t *testing.T) {
	ctx := context.Background()
	t.Run("success partial errors", func(t *testing.T) {
		eng := &mockEngine{SetEntityStateFn: func(_ context.Context, _ string, ops []EntityStateOp) ([]error, error) {
			return []error{nil, errors.New("bad")}, nil
		}}
		args := map[string]any{"operations": []any{
			map[string]any{"entity_name": "a", "state": "active"},
			map[string]any{"entity_name": "b", "state": "deprecated"},
		}}
		out, err := HandleEntityStateBatch(ctx, eng, testVault, args)
		mustNoErr(t, err)
		items := out.(map[string]any)["results"].([]map[string]any)
		if items[0]["status"] != "ok" || items[1]["status"] != "error" {
			t.Fatalf("unexpected batch status: %#v", items)
		}
	})
	t.Run("error exceeds max 50", func(t *testing.T) {
		ops := make([]any, 51)
		for i := range ops {
			ops[i] = map[string]any{"entity_name": "e", "state": "active"}
		}
		_, err := HandleEntityStateBatch(ctx, &mockEngine{}, testVault, map[string]any{"operations": ops})
		mustErr(t, err)
	})
}

func TestHandleEntityClusters(t *testing.T) {
	ctx := context.Background()
	t.Run("success normalize", func(t *testing.T) {
		eng := &mockEngine{}
		_, err := HandleEntityClusters(ctx, eng, testVault, map[string]any{"min_count": 0, "top_n": 0})
		mustNoErr(t, err)
		if eng.LastEntityClustersMin != 1 || eng.LastEntityClustersTopN != 1 {
			t.Fatalf("unexpected entity_clusters args: min=%d top=%d", eng.LastEntityClustersMin, eng.LastEntityClustersTopN)
		}
	})
	t.Run("error", func(t *testing.T) {
		eng := &mockEngine{GetEntityClustersFn: func(_ context.Context, _ string, _, _ int) (*EntityClusterResult, error) {
			return nil, errors.New("boom")
		}}
		_, err := HandleEntityClusters(ctx, eng, testVault, nil)
		mustErr(t, err)
	})
}

func TestHandleExportGraph(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		eng := &mockEngine{ExportGraphFn: func(_ context.Context, _ string, format string, include bool) (string, error) {
			if format != "graphml" || !include {
				return "", errors.New("bad args")
			}
			return "<graphml/>", nil
		}}
		out, err := HandleExportGraph(ctx, eng, testVault, map[string]any{"format": "graphml", "include_engrams": true})
		mustNoErr(t, err)
		if out.(map[string]any)["format"].(string) != "graphml" {
			t.Fatalf("unexpected export output: %#v", out)
		}
	})
	t.Run("error invalid format", func(t *testing.T) {
		_, err := HandleExportGraph(ctx, &mockEngine{}, testVault, map[string]any{"format": "xml"})
		mustErr(t, err)
	})
}

func TestHandleSimilarEntities(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		eng := &mockEngine{FindSimilarEntitiesFn: func(_ context.Context, _ string, _ float64, _ int) ([]SimilarEntityPair, error) {
			return []SimilarEntityPair{{EntityA: "a", EntityB: "b", Similarity: 0.9}}, nil
		}}
		out, err := HandleSimilarEntities(ctx, eng, testVault, map[string]any{"threshold": 0.9, "top_n": 2})
		mustNoErr(t, err)
		if out.(map[string]any)["count"].(int) != 1 {
			t.Fatalf("unexpected similar output: %#v", out)
		}
	})
	t.Run("error threshold out of range", func(t *testing.T) {
		_, err := HandleSimilarEntities(ctx, &mockEngine{}, testVault, map[string]any{"threshold": 1.5})
		mustErr(t, err)
	})
}

func TestHandleMergeEntity(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		eng := &mockEngine{MergeEntityFn: func(_ context.Context, _ string, a, b string, dry bool) (*MergeEntityResult, error) {
			return &MergeEntityResult{EntityA: a, EntityB: b, EngramsRelinked: 2, DryRun: dry}, nil
		}}
		out, err := HandleMergeEntity(ctx, eng, testVault, map[string]any{"entity_a": "a", "entity_b": "b", "dry_run": true})
		mustNoErr(t, err)
		if out.(map[string]any)["dry_run"].(bool) != true {
			t.Fatalf("unexpected merge output: %#v", out)
		}
	})
	t.Run("error required", func(t *testing.T) {
		_, err := HandleMergeEntity(ctx, &mockEngine{}, testVault, map[string]any{"entity_a": "a"})
		mustErr(t, err)
	})
}

func TestHandleEntityTimeline(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		eng := &mockEngine{}
		out, err := HandleEntityTimeline(ctx, eng, testVault, map[string]any{"entity_name": "a", "limit": 999})
		mustNoErr(t, err)
		if eng.LastEntityTimelineLimit != 50 || out.(map[string]any)["entity"].(string) != "a" {
			t.Fatalf("unexpected entity_timeline output: %#v", out)
		}
	})
	t.Run("error required", func(t *testing.T) {
		_, err := HandleEntityTimeline(ctx, &mockEngine{}, testVault, map[string]any{})
		mustErr(t, err)
	})
}

func TestHandleReplayEnrichment(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		eng := &mockEngine{ReplayEnrichmentFn: func(_ context.Context, _ string, stages []string, limit int, dry bool) (*ReplayEnrichmentResult, error) {
			if limit != 200 || !dry {
				return nil, errors.New("unexpected args")
			}
			return &ReplayEnrichmentResult{Processed: 1, StagesRun: stages, DryRun: dry}, nil
		}}
		out, err := HandleReplayEnrichment(ctx, eng, testVault, map[string]any{"stages": []any{"entities"}, "limit": 999, "dry_run": true})
		mustNoErr(t, err)
		if out.(*ReplayEnrichmentResult).Processed != 1 {
			t.Fatalf("unexpected replay output: %+v", out)
		}
	})
	t.Run("error from engine", func(t *testing.T) {
		eng := &mockEngine{ReplayEnrichmentFn: func(_ context.Context, _ string, _ []string, _ int, _ bool) (*ReplayEnrichmentResult, error) {
			return nil, errors.New("boom")
		}}
		_, err := HandleReplayEnrichment(ctx, eng, testVault, nil)
		mustErr(t, err)
	})
}

func TestHandleProvenance(t *testing.T) {
	ctx := context.Background()
	t.Run("success nil entries normalize", func(t *testing.T) {
		out, err := HandleProvenance(ctx, &mockEngine{}, testVault, map[string]any{"id": "i"})
		mustNoErr(t, err)
		if out.(*ProvenanceResult).Entries == nil {
			t.Fatal("entries should be non-nil empty slice")
		}
	})
	t.Run("error id required", func(t *testing.T) {
		_, err := HandleProvenance(ctx, &mockEngine{}, testVault, nil)
		mustErr(t, err)
	})
}

func TestHandleFeedback(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		eng := &mockEngine{}
		out, err := HandleFeedback(ctx, eng, testVault, map[string]any{"engram_id": "e1", "useful": true})
		mustNoErr(t, err)
		if out.(map[string]any)["useful"].(bool) != true || !eng.LastFeedbackUseful {
			t.Fatalf("unexpected feedback output: %#v", out)
		}
	})
	t.Run("error engram_id required", func(t *testing.T) {
		_, err := HandleFeedback(ctx, &mockEngine{}, testVault, nil)
		mustErr(t, err)
	})
}

func TestHandleEntity(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		eng := &mockEngine{}
		out, err := HandleEntity(ctx, eng, testVault, map[string]any{"name": "alice", "limit": 1})
		mustNoErr(t, err)
		if out.(*EntityAggregate).Entity.Name != "alice" || eng.LastEntityLimit != 1 {
			t.Fatalf("unexpected entity output: %+v", out)
		}
	})
	t.Run("error name required", func(t *testing.T) {
		_, err := HandleEntity(ctx, &mockEngine{}, testVault, nil)
		mustErr(t, err)
	})
}

func TestHandleEntities(t *testing.T) {
	ctx := context.Background()
	t.Run("success default and custom limit", func(t *testing.T) {
		eng := &mockEngine{ListEntitiesFn: func(_ context.Context, _ string, _ string, limit int) ([]EntitySummary, error) {
			return []EntitySummary{{Name: "e", MentionCount: int32(limit)}}, nil
		}}
		out, err := HandleEntities(ctx, eng, testVault, map[string]any{"state": "active", "limit": 12})
		mustNoErr(t, err)
		if out.(map[string]any)["count"].(int) != 1 || eng.LastEntitiesLimit != 12 {
			t.Fatalf("unexpected entities output: %#v", out)
		}
	})
	t.Run("error from engine", func(t *testing.T) {
		eng := &mockEngine{ListEntitiesFn: func(_ context.Context, _ string, _ string, _ int) ([]EntitySummary, error) {
			return nil, errors.New("boom")
		}}
		_, err := HandleEntities(ctx, eng, testVault, nil)
		mustErr(t, err)
	})
}
