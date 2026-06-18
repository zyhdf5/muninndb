package engine

import (
	"context"
	"errors"
	"sort"
)

// Engine 是图数据库的核心结构，封装存储接口和并发合并守卫。
type Engine struct {
	store Store      // 底层存储接口
	guard mergeGuard // 实体合并操作的并发串行化守卫
}

// New 创建一个新的 Engine 实例。
// store 参数提供底层存储操作，不能为 nil。
func New(store Store) *Engine {
	return &Engine{
		store: store,
	}
}

// ExportGraph 构建 vault 的实体→关系图。
// 节点来自关系记录中出现的唯一实体名称。
// 当 includeEngrams 为 true 时，从实体注册表中丰富节点的类型信息。
// 边按 (From, To, RelType) 去重：每个三元组仅保留最高权重的记录。
func (e *Engine) ExportGraph(ctx context.Context, vault string, includeEngrams bool) (*ExportGraph, error) {
	ws := e.store.ResolveVaultPrefix(vault)

	// 按 (From, To, RelType) 去重边：保留每个三元组中权重最高的记录。
	type edgeKey struct{ From, To, RelType string }
	edgeBest := make(map[edgeKey]GraphEdge)
	nodeSet := make(map[string]struct{})

	err := e.store.ScanRelationships(ctx, ws, func(rec RelationshipRecord) error {
		k := edgeKey{From: rec.FromEntity, To: rec.ToEntity, RelType: rec.RelType}
		existing, seen := edgeBest[k]
		if !seen || rec.Weight > existing.Weight {
			edgeBest[k] = GraphEdge{
				From:    rec.FromEntity,
				To:      rec.ToEntity,
				RelType: rec.RelType,
				Weight:  rec.Weight,
			}
		}
		nodeSet[rec.FromEntity] = struct{}{}
		nodeSet[rec.ToEntity] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, err
	}

	edges := make([]GraphEdge, 0, len(edgeBest))
	for _, edge := range edgeBest {
		edges = append(edges, edge)
	}

	nodes := make([]GraphNode, 0, len(nodeSet))
	for name := range nodeSet {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		node := GraphNode{ID: name}
		if includeEngrams {
			rec, recErr := e.store.GetEntityRecord(ctx, name)
			if recErr != nil {
				if errors.Is(recErr, context.Canceled) || errors.Is(recErr, context.DeadlineExceeded) {
					return nil, recErr
				}
			} else if rec != nil {
				node.Type = rec.Type
			}
		}
		nodes = append(nodes, node)
	}

	// 确定性排序：边按 From、To、RelType 排列。
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		return edges[i].RelType < edges[j].RelType
	})

	// 确定性排序：节点按 ID 排列。
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})

	return &ExportGraph{
		Nodes: nodes,
		Edges: edges,
	}, nil
}
