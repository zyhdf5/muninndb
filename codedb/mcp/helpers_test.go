package mcp

import (
	"testing"

	"github.com/scrypster/muninndb/codedb/engine"
)

func TestRelTypeMappings(t *testing.T) {
	// given: 16 个关系类型
	rels := []string{
		"supports", "contradicts", "depends_on", "supersedes", "relates_to", "is_part_of", "causes", "preceded_by",
		"followed_by", "created_by_person", "belongs_to_project", "references", "implements", "blocks", "resolves", "refines",
	}

	for _, rel := range rels {
		t.Run(rel, func(t *testing.T) {
			// when: 正反向映射
			code, ok := RelTypeFromString(rel)
			if !ok {
				t.Fatalf("expected known rel type: %s", rel)
			}
			back := RelTypeToString(code)
			// then: 映射可逆
			if back != rel {
				t.Fatalf("reverse mismatch: %s != %s", back, rel)
			}
		})
	}

	// given/when/then: 未知值处理
	if _, ok := RelTypeFromString("unknown"); ok {
		t.Fatal("unknown rel should not be found")
	}
	if RelTypeToString(0xFFFF) != "" {
		t.Fatal("unknown code should return empty string")
	}
}

func TestParseBasicArgs(t *testing.T) {
	args := map[string]any{
		"s":   "x",
		"i1":  12,
		"i2":  "34",
		"f1":  1.5,
		"f2":  "2.5",
		"b":   true,
		"arr": []any{"a", "", "b"},
	}

	// given/when/then: 字符串参数
	if got := parseStringArg(args, "s", "d"); got != "x" {
		t.Fatalf("parseStringArg mismatch: %s", got)
	}
	if got := parseStringArg(args, "missing", "d"); got != "d" {
		t.Fatalf("parseStringArg default mismatch: %s", got)
	}

	// given/when/then: 整型参数
	if got := parseIntArg(args, "i1", 0); got != 12 {
		t.Fatalf("parseIntArg int mismatch: %d", got)
	}
	if got := parseIntArg(args, "i2", 0); got != 34 {
		t.Fatalf("parseIntArg string mismatch: %d", got)
	}

	// given/when/then: 浮点参数
	if got := parseFloatArg(args, "f1", 0); got != 1.5 {
		t.Fatalf("parseFloatArg float mismatch: %f", got)
	}
	if got := parseFloatArg(args, "f2", 0); got != 2.5 {
		t.Fatalf("parseFloatArg string mismatch: %f", got)
	}

	// given/when/then: 布尔参数
	if !parseBoolArg(args, "b", false) {
		t.Fatalf("parseBoolArg mismatch")
	}

	// given/when/then: 字符串切片参数
	arr := parseStringSliceArg(args, "arr")
	if len(arr) != 2 || arr[0] != "a" || arr[1] != "b" {
		t.Fatalf("parseStringSliceArg mismatch: %#v", arr)
	}
}

func TestParseEmbeddingArg(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		// given: 合法 embedding 数组
		emb, err := parseEmbeddingArg(map[string]any{"embedding": []any{1.0, 2.0}})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		// then: 正常转为 float32
		if len(emb) != 2 || emb[0] != 1 || emb[1] != 2 {
			t.Fatalf("unexpected embedding: %#v", emb)
		}
	})

	t.Run("empty", func(t *testing.T) {
		// given: 空数组
		emb, err := parseEmbeddingArg(map[string]any{"embedding": []any{}})
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		// then: 空数组被视为 nil
		if emb != nil {
			t.Fatalf("expected nil embedding, got %#v", emb)
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		// given/when: 包含非数字元素
		_, err := parseEmbeddingArg(map[string]any{"embedding": []any{1.0, "bad"}})
		// then: 返回错误
		if err == nil {
			t.Fatal("expected parseEmbeddingArg error")
		}
	})
}

func TestApplyTypeArgs(t *testing.T) {
	t.Run("known type", func(t *testing.T) {
		// given: 已知类型
		req := &WriteRequest{}
		applyTypeArgs(req, map[string]any{"type": "decision"})
		// then: MemoryType/TypeLabel 正确
		if req.MemoryType != uint8(engine.TypeDecision) || req.TypeLabel != "decision" {
			t.Fatalf("unexpected req: %+v", req)
		}
	})

	t.Run("unknown type with explicit label", func(t *testing.T) {
		// given: 未知类型 + 显式 type_label
		req := &WriteRequest{}
		applyTypeArgs(req, map[string]any{"type": "my_type", "type_label": "explicit"})
		// then: 回退 fact 且优先使用 type_label
		if req.MemoryType != uint8(engine.TypeFact) || req.TypeLabel != "explicit" {
			t.Fatalf("unexpected req: %+v", req)
		}
	})
}

func TestApplyEnrichmentArgs(t *testing.T) {
	// given: 含实体、关系、摘要的参数
	req := &WriteRequest{}
	args := map[string]any{
		"summary": "sum",
		"entities": []any{
			map[string]any{"name": "Alice", "type": "person"},
			"bad",
		},
		"entity_relationships": []any{
			map[string]any{"from_entity": "Alice", "to_entity": "Bob", "rel_type": "uses", "weight": 2.0},
		},
	}

	// when: 应用富化参数
	malformed := applyEnrichmentArgs(req, args)

	// then: 正常值写入，异常项计数
	if req.Summary != "sum" || len(req.Entities) != 1 || len(req.EntityRelationships) != 1 {
		t.Fatalf("unexpected req: %+v", req)
	}
	if req.Entities[0].Name != "alice" {
		t.Fatalf("entity name should normalize lower-case: %+v", req.Entities[0])
	}
	if req.EntityRelationships[0].Weight != 1 {
		t.Fatalf("weight should clamp to 1, got %f", req.EntityRelationships[0].Weight)
	}
	if malformed != 1 {
		t.Fatalf("malformed count mismatch: %d", malformed)
	}
}

func TestLifecycleStateLabelAndValidationSets(t *testing.T) {
	// given/when/then: 所有生命周期状态标签
	if lifecycleStateLabel(engine.StatePlanning) != "planning" ||
		lifecycleStateLabel(engine.StateActive) != "active" ||
		lifecycleStateLabel(engine.StatePaused) != "paused" ||
		lifecycleStateLabel(engine.StateBlocked) != "blocked" ||
		lifecycleStateLabel(engine.StateCompleted) != "completed" ||
		lifecycleStateLabel(engine.StateCancelled) != "cancelled" ||
		lifecycleStateLabel(engine.StateArchived) != "archived" {
		t.Fatalf("lifecycle labels mismatch")
	}
	if lifecycleStateLabel(engine.StateSoftDeleted) != "unknown" {
		t.Fatalf("unknown label mismatch")
	}

	// given/when/then: 校验集合覆盖关键值
	if !validLifecycleStates["active"] || !validLifecycleStates["archived"] {
		t.Fatalf("validLifecycleStates missing required keys")
	}
	if !validEntityStates["merged"] || !validEntityStates["resolved"] {
		t.Fatalf("validEntityStates missing required keys")
	}
	if !validEntityTypes["person"] || !validEntityTypes["other"] {
		t.Fatalf("validEntityTypes missing required keys")
	}
}
