package mcp

import "encoding/json"

const contentPreviewLen = 500

// activationItem 是激活结果的最小投影结构。
type activationItem struct {
	ID          string
	Concept     string
	Content     string
	Summary     string
	Score       float64
	VectorScore float64
	Confidence  float32
	Why         string
	Tags        []string
	State       string
	SourceType  string
}

// activationItemToMemory 将激活结果映射为统一内存结构。
func activationItemToMemory(item activationItem) Memory {
	display := item.Summary
	if display == "" {
		display = item.Content
		if len(display) > contentPreviewLen {
			display = display[:contentPreviewLen] + "..."
		}
	}
	return Memory{
		ID:          item.ID,
		Concept:     item.Concept,
		Content:     display,
		Summary:     item.Summary,
		Score:       item.Score,
		VectorScore: item.VectorScore,
		Confidence:  item.Confidence,
		Why:         item.Why,
		Tags:        item.Tags,
		State:       item.State,
		SourceType:  item.SourceType,
	}
}

// textContent 将文本包装为 MCP 内容数组。
func textContent(s string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": s}},
	}
}

// mustJSON 将任意对象序列化为 JSON 字符串。
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
