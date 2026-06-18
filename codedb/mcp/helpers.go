package mcp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/scrypster/muninndb/codedb/engine"
)

var relTypeMap = map[string]uint16{
	"supports":           0x0001,
	"contradicts":        0x0002,
	"depends_on":         0x0003,
	"supersedes":         0x0004,
	"relates_to":         0x0005,
	"is_part_of":         0x0006,
	"causes":             0x0007,
	"preceded_by":        0x0008,
	"followed_by":        0x0009,
	"created_by_person":  0x000A,
	"belongs_to_project": 0x000B,
	"references":         0x000C,
	"implements":         0x000D,
	"blocks":             0x000E,
	"resolves":           0x000F,
	"refines":            0x0010,
}

var relTypeReverseMap = func() map[uint16]string {
	m := make(map[uint16]string, len(relTypeMap))
	for k, v := range relTypeMap {
		m[v] = k
	}
	return m
}()

var validLifecycleStates = map[string]bool{
	"planning":  true,
	"active":    true,
	"paused":    true,
	"blocked":   true,
	"completed": true,
	"cancelled": true,
	"archived":  true,
}

var validEntityStates = map[string]bool{
	"active":     true,
	"deprecated": true,
	"merged":     true,
	"resolved":   true,
}

var validEntityTypes = map[string]bool{
	"person": true, "organization": true, "location": true, "concept": true,
	"technology": true, "project": true, "tool": true, "database": true,
	"service": true, "framework": true, "language": true, "product": true,
	"event": true, "other": true,
}

// RelTypeFromString 将关系名转换为关系编码。
func RelTypeFromString(s string) (uint16, bool) {
	v, ok := relTypeMap[s]
	return v, ok
}

// RelTypeToString 将关系编码转换为关系名。
func RelTypeToString(code uint16) string {
	if s, ok := relTypeReverseMap[code]; ok {
		return s
	}
	return ""
}

// relTypeFromString 将关系名转换为关系编码并返回是否命中。
func relTypeFromString(s string) (uint16, bool) {
	return RelTypeFromString(s)
}

// relTypeToString 将关系编码转换为关系名。
func relTypeToString(code uint16) string {
	return RelTypeToString(code)
}

// lifecycleStateLabel 将引擎状态转为文本标签。
func lifecycleStateLabel(state engine.LifecycleState) string {
	switch state {
	case engine.StateActive:
		return "active"
	case engine.StatePaused:
		return "paused"
	case engine.StateArchived:
		return "archived"
	case engine.StateBlocked:
		return "blocked"
	case engine.StatePlanning:
		return "planning"
	case engine.StateCompleted:
		return "completed"
	case engine.StateCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// parseStringArg 读取字符串参数。
func parseStringArg(args map[string]any, key, defaultVal string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	s, ok := v.(string)
	if !ok {
		return defaultVal
	}
	return s
}

// parseIntArg 读取整型参数。
func parseIntArg(args map[string]any, key string, defaultVal int) int {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		iv, err := strconv.Atoi(n)
		if err == nil {
			return iv
		}
	}
	return defaultVal
}

// parseFloatArg 读取浮点参数。
func parseFloatArg(args map[string]any, key string, defaultVal float64) float64 {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case string:
		fv, err := strconv.ParseFloat(n, 64)
		if err == nil {
			return fv
		}
	}
	return defaultVal
}

// parseBoolArg 读取布尔参数。
func parseBoolArg(args map[string]any, key string, defaultVal bool) bool {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	b, ok := v.(bool)
	if !ok {
		return defaultVal
	}
	return b
}

// parseStringSliceArg 读取字符串数组参数。
func parseStringSliceArg(args map[string]any, key string) []string {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}
	if arr, ok := v.([]string); ok {
		out := make([]string, 0, len(arr))
		for _, s := range arr {
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	if arr, ok := v.([]any); ok {
		out := make([]string, 0, len(arr))
		for _, it := range arr {
			s, ok := it.(string)
			if ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	if s, ok := v.(string); ok && s != "" {
		return []string{s}
	}
	return nil
}

// parseEmbeddingArg 解析 embedding 参数。
func parseEmbeddingArg(args map[string]any) ([]float32, error) {
	v, ok := args["embedding"]
	if !ok || v == nil {
		return nil, nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid params: 'embedding' must be an array")
	}
	if len(arr) == 0 {
		return nil, nil
	}
	if len(arr) > 4096 {
		return nil, fmt.Errorf("invalid params: 'embedding' exceeds maximum length of 4096")
	}
	out := make([]float32, len(arr))
	for i, it := range arr {
		f, ok := it.(float64)
		if !ok {
			return nil, fmt.Errorf("invalid params: embedding[%d] must be a number", i)
		}
		out[i] = float32(f)
	}
	return out, nil
}

// applyTypeArgs 将 type 与 type_label 写入请求。
func applyTypeArgs(req *WriteRequest, args map[string]any) {
	typeStr := parseStringArg(args, "type", "")
	explicitLabel := parseStringArg(args, "type_label", "")
	if typeStr != "" {
		if mt, ok := engine.ParseMemoryType(typeStr); ok {
			req.MemoryType = uint8(mt)
			if explicitLabel == "" {
				req.TypeLabel = typeStr
			}
		} else {
			req.MemoryType = uint8(engine.TypeFact)
			if explicitLabel == "" {
				req.TypeLabel = typeStr
			}
		}
	}
	if explicitLabel != "" {
		req.TypeLabel = explicitLabel
	}
}

// applyEnrichmentArgs 将 summary/entities/relationships 写入请求。
func applyEnrichmentArgs(req *WriteRequest, args map[string]any) int {
	malformed := 0
	if summary := parseStringArg(args, "summary", ""); summary != "" {
		req.Summary = summary
	}

	if entitiesAny, ok := args["entities"].([]any); ok {
		for i, eAny := range entitiesAny {
			if i >= 20 {
				break
			}
			eMap, ok := eAny.(map[string]any)
			if !ok {
				malformed++
				continue
			}
			name := strings.ToLower(strings.TrimSpace(parseStringArg(eMap, "name", "")))
			typ := strings.ToLower(strings.TrimSpace(parseStringArg(eMap, "type", "")))
			if name == "" || typ == "" {
				continue
			}
			if !validEntityTypes[typ] {
				typ = "other"
			}
			req.Entities = append(req.Entities, InlineEntity{Name: name, Type: typ})
		}
	}

	if relsAny, ok := args["relationships"].([]any); ok {
		for i, rAny := range relsAny {
			if i >= 30 {
				break
			}
			rMap, ok := rAny.(map[string]any)
			if !ok {
				continue
			}
			targetID := parseStringArg(rMap, "target_id", "")
			relation := parseStringArg(rMap, "relation", "")
			if targetID == "" || relation == "" {
				continue
			}
			weight := float32(parseFloatArg(rMap, "weight", 0.9))
			if weight < 0 {
				weight = 0
			}
			if weight > 1 {
				weight = 1
			}
			req.Relationships = append(req.Relationships, InlineRelationship{TargetID: targetID, Relation: relation, Weight: weight})
		}
	}

	if erAny, ok := args["entity_relationships"].([]any); ok {
		for i, eAny := range erAny {
			if i >= 30 {
				break
			}
			eMap, ok := eAny.(map[string]any)
			if !ok {
				continue
			}
			fromEntity := parseStringArg(eMap, "from_entity", "")
			toEntity := parseStringArg(eMap, "to_entity", "")
			relType := parseStringArg(eMap, "rel_type", "")
			if fromEntity == "" || toEntity == "" || relType == "" {
				continue
			}
			weight := float32(parseFloatArg(eMap, "weight", 0.9))
			if weight < 0 {
				weight = 0
			}
			if weight > 1 {
				weight = 1
			}
			req.EntityRelationships = append(req.EntityRelationships, InlineEntityRelationship{
				FromEntity: fromEntity,
				ToEntity:   toEntity,
				RelType:    relType,
				Weight:     weight,
			})
		}
	}

	return malformed
}
