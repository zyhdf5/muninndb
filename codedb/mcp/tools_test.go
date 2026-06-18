package mcp

import "testing"

func TestAllToolDefinitions(t *testing.T) {
	// given: 获取全部工具定义
	defs := AllToolDefinitions()

	// when/then: 数量应为 36，且每个工具必须有名称和描述
	if len(defs) != 36 {
		t.Fatalf("unexpected tool count: %d", len(defs))
	}
	for i, d := range defs {
		if d.Name == "" || d.Description == "" {
			t.Fatalf("tool[%d] invalid: %+v", i, d)
		}
	}
}
