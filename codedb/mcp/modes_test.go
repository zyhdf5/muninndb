package mcp

import "testing"

func TestLookupMode_AllPresets(t *testing.T) {
	// given: 四种内置 mode
	tests := []struct {
		name string
		exp  RecallMode
	}{
		{name: "semantic", exp: RecallMode{SemanticSimilarity: 0.8, FullTextRelevance: 0.2, MaxHops: 0, Threshold: 0.3, DisableACTR: true}},
		{name: "recent", exp: RecallMode{Recency: 0.7, SemanticSimilarity: 0.3, MaxHops: 1, Threshold: 0.2}},
		{name: "balanced", exp: RecallMode{}},
		{name: "deep", exp: RecallMode{MaxHops: 4, Threshold: 0.1}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// when: 查找 mode
			got, err := LookupMode(tc.name)
			if err != nil {
				t.Fatalf("LookupMode error: %v", err)
			}
			// then: 与预期配置一致
			if got != tc.exp {
				t.Fatalf("preset mismatch: got=%+v exp=%+v", got, tc.exp)
			}
		})
	}
}

func TestLookupMode_Unknown(t *testing.T) {
	// given/when: 查询未知 mode
	_, err := LookupMode("nope")
	// then: 返回错误
	if err == nil {
		t.Fatal("expected error for unknown mode")
	}
}
