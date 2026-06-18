package mcp

import (
	"strings"
	"testing"
)

func TestGenerateGuide_BehaviorModes(t *testing.T) {
	stats := EngineStats{EngramCount: 12, VaultCount: 3}

	t.Run("autonomous", func(t *testing.T) {
		// given: autonomous 模式
		g := GenerateGuide("v1", ResolvedPlasticity{BehaviorMode: "autonomous", InlineEnrichment: "sync", HebbianEnabled: true}, stats)
		// then: 包含自主模式建议与统计
		if !strings.Contains(g, "主动记录关键信息") || !strings.Contains(g, "记忆数量: 12") {
			t.Fatalf("guide content mismatch: %s", g)
		}
	})

	t.Run("prompted", func(t *testing.T) {
		// given: prompted 模式
		g := GenerateGuide("v1", ResolvedPlasticity{BehaviorMode: "prompted", InlineEnrichment: "disabled"}, stats)
		// then: 包含按需写入文案
		if !strings.Contains(g, "仅在用户明确要求时写入记忆") {
			t.Fatalf("prompted text missing: %s", g)
		}
	})

	t.Run("selective", func(t *testing.T) {
		// given: selective 模式
		g := GenerateGuide("v1", ResolvedPlasticity{BehaviorMode: "selective", InlineEnrichment: "inline"}, stats)
		// then: 包含选择性策略与富化建议
		if !strings.Contains(g, "自动记录决策") || !strings.Contains(g, "优先补充 type 与 summary") {
			t.Fatalf("selective text missing: %s", g)
		}
	})

	t.Run("custom", func(t *testing.T) {
		// given: custom 模式说明
		g := GenerateGuide("v1", ResolvedPlasticity{BehaviorMode: "custom", BehaviorInstructions: "请只记录架构决策", InlineEnrichment: "sync"}, stats)
		// then: 应使用自定义说明
		if !strings.Contains(g, "请只记录架构决策") {
			t.Fatalf("custom instructions missing: %s", g)
		}
	})

	t.Run("custom empty fallback", func(t *testing.T) {
		// given: custom 但无说明
		g := GenerateGuide("v1", ResolvedPlasticity{BehaviorMode: "custom", InlineEnrichment: "sync"}, stats)
		// then: 回退提示存在
		if !strings.Contains(g, "回退为自主模式") {
			t.Fatalf("fallback text missing: %s", g)
		}
	})
}

func TestEnabledStr(t *testing.T) {
	// given/when/then: 布尔转文本
	if enabledStr(true) != "enabled" || enabledStr(false) != "disabled" {
		t.Fatalf("enabledStr mismatch")
	}
}
