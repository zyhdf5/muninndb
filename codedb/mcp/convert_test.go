package mcp

import (
	"strings"
	"testing"
)

func TestActivationItemToMemory_WithSummary(t *testing.T) {
	// given: 有 summary 的激活项
	item := activationItem{ID: "1", Concept: "c", Content: "content", Summary: "summary", Score: 0.9}

	// when: 转换为 Memory
	m := activationItemToMemory(item)

	// then: Content 使用 summary 展示
	if m.Content != "summary" || m.Summary != "summary" {
		t.Fatalf("unexpected memory: %+v", m)
	}
}

func TestActivationItemToMemory_WithoutSummaryAndTruncate(t *testing.T) {
	// given: 无 summary 且内容超长
	long := strings.Repeat("x", contentPreviewLen+20)
	item := activationItem{ID: "1", Concept: "c", Content: long}

	// when: 转换
	m := activationItemToMemory(item)

	// then: 应截断并追加省略号
	if !strings.HasSuffix(m.Content, "...") || len(m.Content) != contentPreviewLen+3 {
		t.Fatalf("unexpected truncated content len=%d content=%q", len(m.Content), m.Content)
	}
}

func TestTextContent_Format(t *testing.T) {
	// given: 普通文本
	out := textContent("hello")

	// when: 提取 MCP content 包装
	content, ok := out["content"].([]map[string]any)
	if !ok {
		t.Fatalf("content type mismatch: %#v", out["content"])
	}

	// then: 包装结构符合 MCP text envelope
	if len(content) != 1 || content[0]["type"] != "text" || content[0]["text"] != "hello" {
		t.Fatalf("unexpected envelope: %#v", content)
	}
}

func TestMustJSON(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// given: 可序列化对象
		s := mustJSON(map[string]any{"a": 1})
		// when/then: JSON 字符串不应为空对象
		if s == "{}" {
			t.Fatalf("unexpected empty json: %s", s)
		}
	})

	t.Run("unmarshalable returns empty object", func(t *testing.T) {
		// given: 不可序列化的 channel
		s := mustJSON(make(chan int))
		// when/then: 失败回退为 {}
		if s != "{}" {
			t.Fatalf("expected {}, got %s", s)
		}
	})
}
