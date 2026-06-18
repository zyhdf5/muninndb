package engine

import (
	"context"
	"log/slog"
	"math"
)

// ────────────────────────────────────────────────────────────────────
// BFS 遍历常量
// ────────────────────────────────────────────────────────────────────

const (
	// bfsHopPenalty 是每一跳的衰减因子。
	// 距离种子越远，传播的分数越低。
	bfsHopPenalty = 0.7

	// bfsMinHopScore 是继续传播的最小分数阈值。
	// 当传播分数低于此值时停止展开该分支。
	bfsMinHopScore = 0.05

	// bfsMaxNodes 是 BFS 展开的最大节点数，防止图过大时失控。
	bfsMaxNodes = 500

	// bfsMaxEdgesPerNode 是每个节点返回的最大前向关联数。
	bfsMaxEdgesPerNode = 20
)

// ────────────────────────────────────────────────────────────────────
// 关联条目 — 从邻接存储中解析出的单条关联
// ────────────────────────────────────────────────────────────────────

// AssocEntry 是从图中解析出的单条关联记录。
type AssocEntry struct {
	TargetID   ULID    // 目标 engram 的 ID
	Weight     float32 // 关联权重
	Confidence float32 // 关联置信度
	RelType    uint16  // 关系类型编码
}

// ────────────────────────────────────────────────────────────────────
// 遍历结果 — BFS 发现的节点
// ────────────────────────────────────────────────────────────────────

// TraversalResult 表示 BFS 遍历中发现的一个节点。
type TraversalResult struct {
	ID       ULID    // 节点 engram 的 ID
	Score    float64 // 传播到该节点的分数
	HopPath  []ULID  // 从种子到该节点的完整路径
	RelType  uint16  // 到达该节点的关联类型
	HopDepth int     // 距离种子的跳数
}

// ────────────────────────────────────────────────────────────────────
// BFS 遍历 — 基于 Store 接口的广度优先搜索
// ────────────────────────────────────────────────────────────────────

// BFSTraverse 从种子节点出发执行广度优先搜索（BFS），沿关联边传播分数。
//
// 算法要点：
//   - 每一跳分数乘以 bfsHopPenalty 进行衰减
//   - 传播分数低于 bfsMinHopScore 时停止展开（利用权重降序排列提前终止）
//   - 最多展开 bfsMaxNodes 个节点以防止在大图上失控
//   - 种子节点本身不出现在结果中（已被标记为已访问）
//
// 参数：
//   - store: 存储接口，提供 GetAssociations 方法
//   - ws: vault 的 8 字节工作空间前缀
//   - seeds: 起始种子节点 ID 列表
//   - threshold: 最小分数阈值（低于此分数的结果被丢弃）
//   - maxDepth: 最大 BFS 深度
func BFSTraverse(
	ctx context.Context,
	store Store,
	ws [8]byte,
	seeds []ULID,
	threshold float64,
	maxDepth int,
) ([]TraversalResult, error) {

	// queueItem 是 BFS 队列中的工作项。
	type queueItem struct {
		id        ULID
		baseScore float64
		depth     int
		hopPath   []ULID
	}

	// 初始化已访问集合，种子节点标记为已访问。
	seen := make(map[ULID]bool, len(seeds)+bfsMaxNodes)
	for _, s := range seeds {
		seen[s] = true
	}

	// 初始化 BFS 队列，将所有种子以 baseScore=1.0 入队。
	queue := make([]queueItem, 0, len(seeds)*4)
	for _, s := range seeds {
		queue = append(queue, queueItem{
			id:        s,
			baseScore: 1.0,
			depth:     0,
			hopPath:   []ULID{s},
		})
	}

	var results []TraversalResult
	expanded := 0

	for len(queue) > 0 && expanded < bfsMaxNodes {
		curr := queue[0]
		queue = queue[1:]

		if curr.depth >= maxDepth {
			continue
		}

		// 获取当前节点的前向关联。
		assocMap, err := store.GetAssociations(ctx, ws, []ULID{curr.id}, bfsMaxEdgesPerNode)
		if err != nil {
			slog.Warn("BFS 遍历：读取边失败", "node", curr.id, "err", err)
			continue
		}
		assocs := assocMap[curr.id]

		for _, assoc := range assocs {
			if seen[assoc.TargetID] {
				continue
			}

			// 计算传播分数：基础分数 × 关联权重 × 跳数衰减。
			propagated := curr.baseScore * float64(assoc.Weight) * math.Pow(bfsHopPenalty, float64(curr.depth+1))
			if propagated < bfsMinHopScore {
				break // 关联按权重降序排列，后续只会更小，提前终止
			}

			seen[assoc.TargetID] = true
			expanded++

			// 构建到达该节点的完整路径。
			newPath := make([]ULID, len(curr.hopPath)+1)
			copy(newPath, curr.hopPath)
			newPath[len(curr.hopPath)] = assoc.TargetID

			results = append(results, TraversalResult{
				ID:       assoc.TargetID,
				Score:    propagated,
				HopPath:  newPath,
				RelType:  uint16(assoc.RelType),
				HopDepth: curr.depth + 1,
			})

			// 如果还未到达最大深度，将该节点加入队列继续展开。
			if curr.depth+1 < maxDepth {
				queue = append(queue, queueItem{
					id:        assoc.TargetID,
					baseScore: propagated,
					depth:     curr.depth + 1,
					hopPath:   newPath,
				})
			}
		}
	}

	return results, nil
}
