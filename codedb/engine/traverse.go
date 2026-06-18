package engine

import (
	"context"
	"fmt"
)

// entityHopWeight 是通过共享实体链接（而非直接关联边）到达的 engram 所分配的边权重。
// 刻意低于典型的直接关联权重（0.3–1.0），使实体跳达的邻居浮出但排在结构邻居之下。
const entityHopWeight = 0.1

// TraversalNode 是图遍历中返回的单个 engram 节点。
type TraversalNode struct {
	ID      ULID   // engram ID
	Concept string // 概念标签
	HopDist int    // 距离起点的跳数
	Summary string // engram 摘要
}

// TraversalEdge 是图遍历中返回的关联边。
type TraversalEdge struct {
	From    ULID    // 源 engram ID
	To      ULID    // 目标 engram ID
	RelType RelType // 关系类型；合成的实体跳边为零值
	Weight  float32 // 边权重
}

// Traverse 从 startID 出发执行有界 BFS，沿关联边扩展。
// 当 followEntities 为 true 时，BFS 额外通过共享实体链接进行扩展：
// 对于深度 d 出队的每个 engram，查找其提及的所有实体，
// 再查找同 vault 中提及这些实体的其他 engram，以 entityHopWeight 权重在深度 d+1 入队。
func (e *Engine) Traverse(ctx context.Context, vault, startID string, maxHops, maxNodes int, followEntities bool) ([]TraversalNode, []TraversalEdge, error) {
	ws := e.store.ResolveVaultPrefix(vault)
	start, err := ParseULID(startID)
	if err != nil {
		return nil, nil, fmt.Errorf("解析起始 ID: %w", err)
	}

	visited := map[ULID]struct{}{start: {}}
	queue := []ULID{start}
	hopMap := map[ULID]int{start: 0}

	var nodes []TraversalNode
	var edges []TraversalEdge

	for hop := 0; hop <= maxHops && len(queue) > 0 && len(nodes) < maxNodes; hop++ {
		assocMap, err := e.store.GetAssociations(ctx, ws, queue, 20)
		if err != nil {
			return nil, nil, err
		}
		engrams, err := e.store.GetEngrams(ctx, ws, queue)
		if err != nil {
			return nil, nil, err
		}

		var next []ULID
		for i, src := range queue {
			if len(nodes) >= maxNodes {
				break
			}
			eng := engrams[i]
			if eng != nil && eng.State != StateSoftDeleted {
				nodes = append(nodes, TraversalNode{
					ID:      eng.ID,
					Concept: eng.Concept,
					HopDist: hopMap[src],
					Summary: eng.Summary,
				})
			}
			for _, assoc := range assocMap[src] {
				edges = append(edges, TraversalEdge{From: src, To: assoc.TargetID, RelType: assoc.RelType, Weight: assoc.Weight})
				if _, seen := visited[assoc.TargetID]; !seen {
					visited[assoc.TargetID] = struct{}{}
					hopMap[assoc.TargetID] = hop + 1
					next = append(next, assoc.TargetID)
				}
			}

			// 实体跳：通过共享实体名称发现可达邻居。
			if followEntities && hop < maxHops {
				_ = e.store.ScanEngramEntities(ctx, ws, src, func(entityName string) error {
					return e.store.ScanEntityEngrams(ctx, entityName, func(entityWS [8]byte, neighborID ULID) error {
						if entityWS != ws {
							return nil
						}
						if _, seen := visited[neighborID]; seen {
							return nil
						}
						visited[neighborID] = struct{}{}
						hopMap[neighborID] = hop + 1
						next = append(next, neighborID)
						edges = append(edges, TraversalEdge{From: src, To: neighborID, Weight: entityHopWeight})
						return nil
					})
				})
			}
		}
		queue = next
	}
	return nodes, edges, nil
}
