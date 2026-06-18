package mcp

import "fmt"

// RecallMode 表示召回模式预设。
type RecallMode struct {
	MaxHops            int
	Threshold          float32
	SemanticSimilarity float32
	FullTextRelevance  float32
	Recency            float32
	DisableACTR        bool
}

var recallModePresets = map[string]RecallMode{
	"semantic": {
		SemanticSimilarity: 0.8,
		FullTextRelevance:  0.2,
		MaxHops:            0,
		Threshold:          0.3,
		DisableACTR:        true,
	},
	"recent": {
		Recency:            0.7,
		SemanticSimilarity: 0.3,
		MaxHops:            1,
		Threshold:          0.2,
	},
	"balanced": {},
	"deep": {
		MaxHops:   4,
		Threshold: 0.1,
	},
}

// LookupMode 按名称查找召回模式。
func LookupMode(name string) (RecallMode, error) {
	m, ok := recallModePresets[name]
	if !ok {
		return RecallMode{}, fmt.Errorf("unknown recall mode %q: valid modes are semantic, recent, balanced, deep", name)
	}
	return m, nil
}
