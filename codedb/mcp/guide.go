package mcp

import (
	"fmt"
	"strings"
)

// EngineStats 表示指南生成所需的统计信息。
type EngineStats struct {
	EngramCount int64
	VaultCount  int
}

// GenerateGuide 生成面向 MCP 使用者的指南文本。
func GenerateGuide(vaultName string, resolved ResolvedPlasticity, stats EngineStats) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# MuninnDB 记忆使用指南（vault: %s）\n\n", vaultName)
	b.WriteString("## 记忆策略\n\n")
	switch resolved.BehaviorMode {
	case "prompted":
		b.WriteString("仅在用户明确要求时写入记忆；当用户要求检索时再召回。\n")
	case "selective":
		b.WriteString("自动记录决策、错误与修复；其他信息按需记录。开始相关任务前先召回。\n")
	case "custom":
		if resolved.BehaviorInstructions != "" {
			b.WriteString(resolved.BehaviorInstructions)
			b.WriteString("\n")
		} else {
			b.WriteString("已配置 custom 但未提供说明，回退为自主模式。\n")
		}
	default:
		b.WriteString("主动记录关键信息：决策与理由、偏好、错误与修复、项目上下文。任务开始前召回，完成后沉淀结果。\n\n")
		b.WriteString("建议会话启动流程：\n")
		b.WriteString("1. `muninn_recall(context=[\"session start\"], mode=\"recent\")`\n")
		b.WriteString("2. 拿到用户主题后再做一次语义召回。\n")
		b.WriteString("也可直接使用 `muninn_where_left_off`。\n")
	}

	if resolved.InlineEnrichment != "background_only" && resolved.InlineEnrichment != "disabled" {
		b.WriteString("\n## 富化\n\n")
		switch resolved.BehaviorMode {
		case "autonomous":
			b.WriteString("写入时尽量携带 type、summary、entities，可减少后台补全成本并提升召回质量。\n")
		case "selective":
			b.WriteString("对决策与问题类记忆，优先补充 type 与 summary。\n")
		}
	}

	b.WriteString("\n## 常用工具\n\n")
	b.WriteString("- muninn_remember / muninn_remember_batch\n")
	b.WriteString("- muninn_recall / muninn_where_left_off / muninn_read\n")
	b.WriteString("- muninn_link / muninn_traverse / muninn_explain\n")
	b.WriteString("- muninn_state / muninn_decide / muninn_evolve\n")
	b.WriteString("- muninn_remember_tree / muninn_recall_tree / muninn_add_child\n")

	b.WriteString("\n## Vault 配置\n\n")
	fmt.Fprintf(&b, "- 记忆数量: %d\n", stats.EngramCount)
	fmt.Fprintf(&b, "- Vault 数量: %d\n", stats.VaultCount)
	fmt.Fprintf(&b, "- 行为模式: %s\n", resolved.BehaviorMode)
	fmt.Fprintf(&b, "- Hebbian: %s\n", enabledStr(resolved.HebbianEnabled))
	fmt.Fprintf(&b, "- 预测激活: %s\n", enabledStr(resolved.PredictiveActivation))
	fmt.Fprintf(&b, "- 时间衰减: %s\n", enabledStr(resolved.TemporalEnabled))
	fmt.Fprintf(&b, "- Hop 深度: %d\n", resolved.HopDepth)
	fmt.Fprintf(&b, "- Inline 富化: %s\n", resolved.InlineEnrichment)
	if resolved.MaxEngrams > 0 {
		fmt.Fprintf(&b, "- 最大 engram: %d\n", resolved.MaxEngrams)
	}
	if resolved.RetentionDays > 0 {
		fmt.Fprintf(&b, "- 保留天数: %.0f\n", resolved.RetentionDays)
	}

	b.WriteString("\n## 写入建议\n\n")
	b.WriteString("保持原子性：一条记忆只表达一个事实、一个决策或一个结论。多主题请批量拆分写入。\n")

	return b.String()
}

// enabledStr 将布尔值转为启用状态文本。
func enabledStr(v bool) string {
	if v {
		return "enabled"
	}
	return "disabled"
}
