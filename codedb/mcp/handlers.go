package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// IdempotencyLocker 定义可选的幂等互斥能力。
type IdempotencyLocker interface {
	Lock(opID string)
	Unlock(opID string)
}

func normalizeLimit(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func parseContextArg(args map[string]any) ([]string, error) {
	raw, ok := args["context"]
	if !ok {
		return nil, fmt.Errorf("invalid params: 'context' is required")
	}
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("invalid params: 'context' must not be empty")
		}
		return []string{v}, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, it := range v {
			s, ok := it.(string)
			if ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("invalid params: 'context' must contain at least one string")
		}
		return out, nil
	default:
		return nil, fmt.Errorf("invalid params: 'context' must be a string or array of strings")
	}
}

// HandleRemember 处理单条记忆写入。
func HandleRemember(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	content := parseStringArg(args, "content", "")
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("invalid params: 'content' is required")
	}
	req := &WriteRequest{Content: content, Concept: parseStringArg(args, "concept", ""), OpID: parseStringArg(args, "op_id", "")}
	req.Tags = parseStringSliceArg(args, "tags")
	if len(req.Tags) > 50 {
		req.Tags = req.Tags[:50]
	}
	req.Confidence = float32(parseFloatArg(args, "confidence", 0))
	if req.Confidence < 0 {
		req.Confidence = 0
	}
	if req.Confidence > 1 {
		req.Confidence = 1
	}
	if caStr := parseStringArg(args, "created_at", ""); caStr != "" {
		t, err := time.Parse(time.RFC3339, caStr)
		if err != nil {
			return nil, fmt.Errorf("invalid 'created_at': must be ISO 8601")
		}
		req.CreatedAt = &t
	}
	applyTypeArgs(req, args)
	malformed := applyEnrichmentArgs(req, args)
	if emb, err := parseEmbeddingArg(args); err != nil {
		return nil, err
	} else {
		req.Embedding = emb
	}
	resp, err := eng.Write(ctx, vault, req)
	if err != nil {
		return nil, err
	}
	if malformed > 0 {
		resp.Warnings = append(resp.Warnings, fmt.Sprintf("%d entity item(s) malformed and skipped", malformed))
	}
	if len(content) > 500 {
		resp.Hint = "记忆建议保持原子化，可使用 remember_batch 拆分写入"
	}
	return resp, nil
}

// HandleRememberBatch 处理批量记忆写入。
func HandleRememberBatch(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	memoriesAny, ok := args["memories"].([]any)
	if !ok || len(memoriesAny) == 0 {
		return nil, fmt.Errorf("invalid params: 'memories' is required and must be a non-empty array")
	}
	if len(memoriesAny) > 50 {
		return nil, fmt.Errorf("invalid params: 'memories' exceeds maximum of 50")
	}
	reqs := make([]*WriteRequest, 0, len(memoriesAny))
	malformed := make([]int, 0, len(memoriesAny))
	for i, mAny := range memoriesAny {
		m, ok := mAny.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid params: memories[%d] must be an object", i)
		}
		content := parseStringArg(m, "content", "")
		if strings.TrimSpace(content) == "" {
			return nil, fmt.Errorf("invalid params: memories[%d].content is required", i)
		}
		req := &WriteRequest{Content: content, Concept: parseStringArg(m, "concept", "")}
		req.Tags = parseStringSliceArg(m, "tags")
		if len(req.Tags) > 50 {
			req.Tags = req.Tags[:50]
		}
		req.Confidence = float32(parseFloatArg(m, "confidence", 0))
		if req.Confidence < 0 {
			req.Confidence = 0
		}
		if req.Confidence > 1 {
			req.Confidence = 1
		}
		if caStr := parseStringArg(m, "created_at", ""); caStr != "" {
			t, err := time.Parse(time.RFC3339, caStr)
			if err != nil {
				return nil, fmt.Errorf("invalid 'created_at' in memories[%d]: must be ISO 8601", i)
			}
			req.CreatedAt = &t
		}
		applyTypeArgs(req, m)
		mf := applyEnrichmentArgs(req, m)
		if emb, err := parseEmbeddingArg(m); err != nil {
			return nil, fmt.Errorf("invalid params: memories[%d].%s", i, strings.TrimPrefix(err.Error(), "invalid params: "))
		} else {
			req.Embedding = emb
		}
		reqs = append(reqs, req)
		malformed = append(malformed, mf)
	}
	resps, errs := eng.WriteBatch(ctx, vault, reqs)
	items := make([]map[string]any, len(reqs))
	for i := range reqs {
		items[i] = map[string]any{"index": i}
		if i < len(errs) && errs[i] != nil {
			items[i]["status"] = "error"
			items[i]["error"] = errs[i].Error()
		} else {
			items[i]["status"] = "ok"
			if i < len(resps) && resps[i] != nil {
				items[i]["id"] = resps[i].ID
				items[i]["concept"] = resps[i].Concept
			}
		}
		if malformed[i] > 0 {
			items[i]["hint"] = fmt.Sprintf("%d entity item(s) malformed and skipped", malformed[i])
		}
	}
	return map[string]any{"results": items, "total": len(items)}, nil
}

// HandleRecall 处理语义召回。
func HandleRecall(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	contexts, err := parseContextArg(args)
	if err != nil {
		return nil, err
	}
	threshold := float32(parseFloatArg(args, "threshold", 0.5))
	if threshold < 0 {
		threshold = 0
	}
	if threshold > 1 {
		threshold = 1
	}
	limit := normalizeLimit(parseIntArg(args, "limit", 10), 1, 100)
	req := &ActivateRequest{Context: contexts, Threshold: threshold, MaxResults: limit, Profile: parseStringArg(args, "profile", "")}
	if mode := parseStringArg(args, "mode", ""); mode != "" {
		preset, modeErr := LookupMode(mode)
		if modeErr != nil {
			return nil, modeErr
		}
		req.Mode = mode
		if _, ok := args["threshold"]; !ok && preset.Threshold > 0 {
			req.Threshold = preset.Threshold
		}
		if preset.MaxHops > 0 {
			req.MaxHops = preset.MaxHops
		}
		if preset.SemanticSimilarity > 0 || preset.FullTextRelevance > 0 || preset.Recency > 0 || preset.DisableACTR {
			req.Weights = &Weights{
				SemanticSimilarity: preset.SemanticSimilarity,
				FullTextRelevance:  preset.FullTextRelevance,
				Recency:            preset.Recency,
				DisableACTR:        preset.DisableACTR,
			}
		}
	}
	filters := make([]Filter, 0, 2)
	if sinceStr := parseStringArg(args, "since", ""); sinceStr != "" {
		t, parseErr := time.Parse(time.RFC3339, sinceStr)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid 'since': must be ISO 8601")
		}
		filters = append(filters, Filter{Field: "created_after", Op: ">=", Value: t})
	}
	if beforeStr := parseStringArg(args, "before", ""); beforeStr != "" {
		t, parseErr := time.Parse(time.RFC3339, beforeStr)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid 'before': must be ISO 8601")
		}
		filters = append(filters, Filter{Field: "created_before", Op: "<", Value: t})
	}
	req.Filters = filters
	if emb, embErr := parseEmbeddingArg(args); embErr != nil {
		return nil, embErr
	} else {
		req.Embedding = emb
	}
	resp, actErr := eng.Activate(ctx, vault, req)
	if actErr != nil {
		return nil, actErr
	}
	out := map[string]any{"memories": resp.Memories, "total": resp.TotalFound}
	if len(resp.Memories) == 0 {
		out["hint"] = "无匹配结果：可尝试 mode=recent 或 muninn_where_left_off"
	}
	return out, nil
}

func HandleRead(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	id := parseStringArg(args, "id", "")
	if id == "" {
		return nil, fmt.Errorf("invalid params: 'id' is required")
	}
	return eng.Read(ctx, vault, id)
}

func HandleForget(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	id := parseStringArg(args, "id", "")
	if id == "" {
		return nil, fmt.Errorf("invalid params: 'id' is required")
	}
	if err := eng.Forget(ctx, vault, id); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func HandleLink(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	sourceID := parseStringArg(args, "source_id", "")
	targetID := parseStringArg(args, "target_id", "")
	relation := parseStringArg(args, "relation", "")
	if sourceID == "" || targetID == "" || relation == "" {
		return nil, fmt.Errorf("invalid params: 'source_id', 'target_id', 'relation' are required")
	}
	relCode, ok := relTypeFromString(relation)
	if !ok {
		relCode = relTypeMap["relates_to"]
	}
	weight := float32(parseFloatArg(args, "weight", 0.8))
	if weight < 0 {
		weight = 0
	}
	if weight > 1 {
		weight = 1
	}
	if err := eng.Link(ctx, vault, sourceID, targetID, relCode, weight); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}

func HandleContradictions(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	pairs, err := eng.GetContradictions(ctx, vault)
	if err != nil {
		return nil, err
	}
	return map[string]any{"contradictions": pairs}, nil
}

func HandleStatus(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	return eng.GetStatus(ctx, vault)
}

func HandleEvolve(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	id := parseStringArg(args, "id", "")
	newContent := parseStringArg(args, "new_content", "")
	reason := parseStringArg(args, "reason", "")
	if id == "" || newContent == "" || reason == "" {
		return nil, fmt.Errorf("invalid params: 'id', 'new_content', 'reason' are required")
	}
	if err := eng.Evolve(ctx, vault, id, newContent, reason); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "updated": true}, nil
}

func HandleConsolidate(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	ids := parseStringSliceArg(args, "ids")
	if len(ids) < 2 {
		return nil, fmt.Errorf("invalid params: 'ids' must contain at least 2 valid engram IDs")
	}
	if len(ids) > 50 {
		return nil, fmt.Errorf("invalid params: 'ids' exceeds maximum of 50")
	}
	mergedContent := parseStringArg(args, "merged_content", "")
	if mergedContent == "" {
		return nil, fmt.Errorf("invalid params: 'merged_content' is required")
	}
	id, err := eng.Consolidate(ctx, vault, ids, mergedContent)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": id}, nil
}

func HandleSession(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	since := parseStringArg(args, "since", "")
	if since == "" {
		return nil, fmt.Errorf("invalid params: 'since' is required (ISO 8601)")
	}
	if _, err := time.Parse(time.RFC3339, since); err != nil {
		return nil, fmt.Errorf("invalid params: 'since' must be ISO 8601")
	}
	return eng.GetSession(ctx, vault, since)
}

func HandleDecide(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	decision := parseStringArg(args, "decision", "")
	rationale := parseStringArg(args, "rationale", "")
	if decision == "" || rationale == "" {
		return nil, fmt.Errorf("invalid params: 'decision' and 'rationale' are required")
	}
	alternatives := parseStringSliceArg(args, "alternatives")
	evidenceIDs := parseStringSliceArg(args, "evidence_ids")
	id, err := eng.Decide(ctx, vault, decision, rationale, alternatives, evidenceIDs)
	if err != nil {
		return nil, err
	}
	return DecideResult{ID: id}, nil
}

func HandleRestore(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	id := parseStringArg(args, "id", "")
	if id == "" {
		return nil, fmt.Errorf("invalid params: 'id' is required")
	}
	if err := eng.Restore(ctx, vault, id); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "restored": true}, nil
}

func HandleTraverse(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	startID := parseStringArg(args, "start_id", "")
	if startID == "" {
		return nil, fmt.Errorf("invalid params: 'start_id' is required")
	}
	maxHops := normalizeLimit(parseIntArg(args, "max_hops", 2), 0, 5)
	maxNodes := normalizeLimit(parseIntArg(args, "max_nodes", 20), 0, 100)
	relTypes := parseStringSliceArg(args, "rel_types")
	followEntities := parseBoolArg(args, "follow_entities", false)
	return eng.Traverse(ctx, vault, startID, maxHops, maxNodes, relTypes, followEntities)
}

func HandleExplain(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	engramID := parseStringArg(args, "engram_id", "")
	if engramID == "" {
		return nil, fmt.Errorf("invalid params: 'engram_id' is required")
	}
	query := parseStringSliceArg(args, "query")
	if len(query) == 0 {
		return nil, fmt.Errorf("invalid params: 'query' is required and must be a non-empty array of strings")
	}
	emb, err := parseEmbeddingArg(args)
	if err != nil {
		return nil, err
	}
	return eng.Explain(ctx, vault, engramID, query, emb)
}

func HandleState(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	id := parseStringArg(args, "id", "")
	state := parseStringArg(args, "state", "")
	if id == "" {
		return nil, fmt.Errorf("invalid params: 'id' is required")
	}
	if !validLifecycleStates[state] {
		return nil, fmt.Errorf("invalid params: 'state' must be one of: planning, active, paused, blocked, completed, cancelled, archived")
	}
	if err := eng.SetState(ctx, vault, id, state, parseStringArg(args, "reason", "")); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "state": state, "updated": true}, nil
}

func HandleListDeleted(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	limit := normalizeLimit(parseIntArg(args, "limit", 20), 0, 100)
	deleted, err := eng.ListDeleted(ctx, vault, limit)
	if err != nil {
		return nil, err
	}
	if deleted == nil {
		deleted = []DeletedEngram{}
	}
	return map[string]any{"deleted": deleted, "count": len(deleted)}, nil
}

func HandleRetryEnrich(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	id := parseStringArg(args, "id", "")
	if id == "" {
		return nil, fmt.Errorf("invalid params: 'id' is required")
	}
	if err := eng.RetryEnrich(ctx, vault, id); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "queued": true}, nil
}

func HandleGuide(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	guide, err := eng.GetGuide(ctx, vault)
	if err == nil && guide != "" {
		return textContent(guide), nil
	}
	pl, err1 := eng.ResolvePlasticity(ctx, vault)
	st, err2 := eng.GetStatus(ctx, vault)
	if err1 != nil {
		return nil, err1
	}
	if err2 != nil {
		return nil, err2
	}
	return textContent(GenerateGuide(vault, *pl, EngineStats{EngramCount: st.EngramCount, VaultCount: st.VaultCount})), nil
}

func HandleWhereLeftOff(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	limit := normalizeLimit(parseIntArg(args, "limit", 10), 1, 50)
	entries, err := eng.WhereLeftOff(ctx, vault, limit)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []WhereLeftOffEntry{}
	}
	return map[string]any{"memories": entries, "count": len(entries)}, nil
}

func HandleRememberTree(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	rootRaw, ok := args["root"]
	if !ok {
		return nil, fmt.Errorf("invalid params: 'root' is required")
	}
	b, err := json.Marshal(rootRaw)
	if err != nil {
		return nil, fmt.Errorf("invalid params: cannot marshal root")
	}
	var root TreeNodeInput
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("invalid params: root must match TreeNodeInput schema")
	}
	if strings.TrimSpace(root.Concept) == "" {
		return nil, fmt.Errorf("invalid params: root.concept is required")
	}
	return eng.RememberTree(ctx, vault, root)
}

func HandleRecallTree(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	rootID := parseStringArg(args, "root_id", "")
	if rootID == "" {
		return nil, fmt.Errorf("invalid params: 'root_id' is required")
	}
	maxDepth := parseIntArg(args, "max_depth", 10)
	if maxDepth < 0 {
		maxDepth = 0
	}
	if maxDepth > 50 {
		maxDepth = 50
	}
	limit := parseIntArg(args, "limit", 0)
	if limit > 1000 {
		limit = 1000
	}
	includeCompleted := parseBoolArg(args, "include_completed", true)
	return eng.RecallTree(ctx, vault, rootID, maxDepth, limit, includeCompleted)
}

func HandleAddChild(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	parentID := parseStringArg(args, "parent_id", "")
	concept := parseStringArg(args, "concept", "")
	content := parseStringArg(args, "content", "")
	if parentID == "" || strings.TrimSpace(concept) == "" || content == "" {
		return nil, fmt.Errorf("invalid params: 'parent_id', 'concept', 'content' are required")
	}
	typ := parseStringArg(args, "type", "")
	tags := parseStringSliceArg(args, "tags")
	var ordinal *int
	if _, ok := args["ordinal"]; ok {
		o := parseIntArg(args, "ordinal", 0)
		ordinal = &o
	}
	emb, err := parseEmbeddingArg(args)
	if err != nil {
		return nil, err
	}
	id, err := eng.AddChild(ctx, vault, parentID, concept, content, typ, tags, ordinal, emb)
	if err != nil {
		return nil, err
	}
	return map[string]any{"child_id": id}, nil
}

func HandleFindByEntity(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	entityName := parseStringArg(args, "entity_name", "")
	if entityName == "" {
		return nil, fmt.Errorf("invalid params: 'entity_name' is required")
	}
	limit := normalizeLimit(parseIntArg(args, "limit", 20), 1, 50)
	engrams, err := eng.FindByEntity(ctx, vault, entityName, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]map[string]any, 0, len(engrams))
	for _, e := range engrams {
		entries = append(entries, map[string]any{
			"id":      e.ID.String(),
			"concept": e.Concept,
			"summary": e.Summary,
			"state":   lifecycleStateLabel(e.State),
		})
	}
	return map[string]any{"entity": entityName, "engrams": entries, "count": len(entries)}, nil
}

func HandleEntityState(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	entityName := parseStringArg(args, "entity_name", "")
	state := parseStringArg(args, "state", "")
	if entityName == "" || !validEntityStates[state] {
		return nil, fmt.Errorf("invalid params: 'entity_name' and valid 'state' are required")
	}
	mergedInto := parseStringArg(args, "merged_into", "")
	if state == "merged" && mergedInto == "" {
		return nil, fmt.Errorf("invalid params: 'merged_into' is required when state=merged")
	}
	entityType := parseStringArg(args, "type", "")
	ops := []EntityStateOp{{EntityName: entityName, State: state, MergedInto: mergedInto, EntityType: entityType}}
	errList, err := eng.SetEntityState(ctx, vault, ops)
	if err != nil {
		return nil, err
	}
	if len(errList) > 0 && errList[0] != nil {
		return nil, errList[0]
	}
	resp := map[string]any{"entity": entityName, "state": state, "ok": true}
	if entityType != "" {
		resp["type"] = entityType
	}
	return resp, nil
}

func HandleEntityStateBatch(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	opsAny, ok := args["operations"].([]any)
	if !ok || len(opsAny) == 0 {
		return nil, fmt.Errorf("invalid params: 'operations' is required and must be a non-empty array")
	}
	if len(opsAny) > 50 {
		return nil, fmt.Errorf("invalid params: 'operations' exceeds maximum of 50")
	}
	ops := make([]EntityStateOp, 0, len(opsAny))
	for i, opAny := range opsAny {
		op, ok := opAny.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid params: operations[%d] must be an object", i)
		}
		entityName := parseStringArg(op, "entity_name", "")
		state := parseStringArg(op, "state", "")
		if entityName == "" || !validEntityStates[state] {
			return nil, fmt.Errorf("invalid params: operations[%d] requires entity_name and valid state", i)
		}
		mergedInto := parseStringArg(op, "merged_into", "")
		if state == "merged" && mergedInto == "" {
			return nil, fmt.Errorf("invalid params: operations[%d].merged_into is required when state=merged", i)
		}
		ops = append(ops, EntityStateOp{
			EntityName: entityName,
			State:      state,
			MergedInto: mergedInto,
			EntityType: parseStringArg(op, "type", ""),
		})
	}
	errList, err := eng.SetEntityState(ctx, vault, ops)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, len(ops))
	for i := range ops {
		results[i] = map[string]any{"index": i, "entity": ops[i].EntityName}
		if i < len(errList) && errList[i] != nil {
			results[i]["status"] = "error"
			results[i]["error"] = errList[i].Error()
		} else {
			results[i]["status"] = "ok"
			results[i]["state"] = ops[i].State
		}
	}
	return map[string]any{"results": results, "total": len(results)}, nil
}

func HandleEntityClusters(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	minCount := normalizeLimit(parseIntArg(args, "min_count", 2), 1, 1<<30)
	topN := normalizeLimit(parseIntArg(args, "top_n", 20), 1, 1<<30)
	return eng.GetEntityClusters(ctx, vault, minCount, topN)
}

func HandleExportGraph(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	format := parseStringArg(args, "format", "json-ld")
	if format != "json-ld" && format != "graphml" {
		return nil, fmt.Errorf("invalid params: 'format' must be 'json-ld' or 'graphml'")
	}
	includeEngrams := parseBoolArg(args, "include_engrams", false)
	data, err := eng.ExportGraph(ctx, vault, format, includeEngrams)
	if err != nil {
		return nil, err
	}
	return map[string]any{"format": format, "data": data}, nil
}

func HandleSimilarEntities(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	threshold := parseFloatArg(args, "threshold", 0.85)
	if threshold < 0 || threshold > 1 {
		return nil, fmt.Errorf("invalid params: 'threshold' must be between 0.0 and 1.0")
	}
	topN := normalizeLimit(parseIntArg(args, "top_n", 20), 1, 100)
	pairs, err := eng.FindSimilarEntities(ctx, vault, threshold, topN)
	if err != nil {
		return nil, err
	}
	return map[string]any{"similar": pairs, "count": len(pairs)}, nil
}

func HandleMergeEntity(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	entityA := parseStringArg(args, "entity_a", "")
	entityB := parseStringArg(args, "entity_b", "")
	if entityA == "" || entityB == "" {
		return nil, fmt.Errorf("invalid params: 'entity_a' and 'entity_b' are required")
	}
	dryRun := parseBoolArg(args, "dry_run", false)
	result, err := eng.MergeEntity(ctx, vault, entityA, entityB, dryRun)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"merged":           !dryRun,
		"entity_a":         result.EntityA,
		"entity_b":         result.EntityB,
		"engrams_relinked": result.EngramsRelinked,
		"dry_run":          result.DryRun,
	}, nil
}

func HandleEntityTimeline(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	entityName := parseStringArg(args, "entity_name", "")
	if entityName == "" {
		return nil, fmt.Errorf("invalid params: 'entity_name' is required")
	}
	limit := normalizeLimit(parseIntArg(args, "limit", 10), 1, 50)
	return eng.GetEntityTimeline(ctx, vault, entityName, limit)
}

func HandleReplayEnrichment(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	stages := parseStringSliceArg(args, "stages")
	limit := normalizeLimit(parseIntArg(args, "limit", 50), 1, 200)
	dryRun := parseBoolArg(args, "dry_run", false)
	return eng.ReplayEnrichment(ctx, vault, stages, limit, dryRun)
}

func HandleProvenance(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	id := parseStringArg(args, "id", "")
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	entries, err := eng.GetProvenance(ctx, vault, id)
	if err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []ProvenanceEntry{}
	}
	return &ProvenanceResult{ID: id, Entries: entries}, nil
}

func HandleFeedback(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	engramID := parseStringArg(args, "engram_id", "")
	if engramID == "" {
		return nil, fmt.Errorf("engram_id is required")
	}
	useful := parseBoolArg(args, "useful", false)
	if err := eng.RecordFeedback(ctx, vault, engramID, useful); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "engram_id": engramID, "useful": useful}, nil
}

func HandleEntity(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	name := parseStringArg(args, "name", "")
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	limit := normalizeLimit(parseIntArg(args, "limit", 20), 1, 1<<30)
	return eng.GetEntity(ctx, vault, name, limit)
}

func HandleEntities(ctx context.Context, eng EngineInterface, vault string, args map[string]any) (any, error) {
	limit := normalizeLimit(parseIntArg(args, "limit", 50), 1, 1<<30)
	state := parseStringArg(args, "state", "")
	entities, err := eng.ListEntities(ctx, vault, state, limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{"entities": entities, "count": len(entities)}, nil
}
