package mcp

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTypes_JSONRoundTrip(t *testing.T) {
	// given: 含时间与嵌套字段的 Memory
	in := Memory{
		ID:        "m1",
		Concept:   "概念",
		Content:   "内容",
		Summary:   "摘要",
		Tags:      []string{"a", "b"},
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		Entities:  []ReadEntity{{Name: "postgres", Type: "database"}},
	}

	// when: 执行 JSON 序列化与反序列化
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var out Memory
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// then: 关键字段保持一致
	if out.ID != in.ID || out.Concept != in.Concept || out.Summary != in.Summary {
		t.Fatalf("roundtrip mismatch: in=%+v out=%+v", in, out)
	}
	if len(out.Entities) != 1 || out.Entities[0].Name != "postgres" {
		t.Fatalf("entities mismatch: %+v", out.Entities)
	}
}

func TestTypes_TreeNodeInput_JSONRoundTrip(t *testing.T) {
	// given: 树形输入
	in := TreeNodeInput{
		Concept: "root",
		Content: "root content",
		Type:    "task",
		Tags:    []string{"x"},
		Children: []TreeNodeInput{{
			Concept: "child",
			Content: "child content",
		}},
	}

	// when: JSON 往返
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var out TreeNodeInput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// then: 子节点应完整保留
	if out.Concept != "root" || len(out.Children) != 1 || out.Children[0].Concept != "child" {
		t.Fatalf("tree roundtrip mismatch: %+v", out)
	}
}
