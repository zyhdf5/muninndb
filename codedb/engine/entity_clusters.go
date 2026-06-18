package engine

import (
	"context"
	"sort"
)

// EntityCluster 表示在同一 vault 的 engram 中频繁共现的实体对。
type EntityCluster struct {
	EntityA string // 实体 A 名称
	EntityB string // 实体 B 名称
	Count   int    // 共现次数
}

// GetEntityClusters 返回 vault 中共现的实体对，按共现次数降序排列。
// 仅包含 count >= minCount 的实体对，结果上限为 topN。
func (e *Engine) GetEntityClusters(ctx context.Context, vault string, minCount, topN int) ([]EntityCluster, error) {
	ws := e.store.ResolveVaultPrefix(vault)

	var clusters []EntityCluster
	err := e.store.ScanEntityClusters(ctx, ws, minCount, func(nameA, nameB string, count int) error {
		clusters = append(clusters, EntityCluster{
			EntityA: nameA,
			EntityB: nameB,
			Count:   count,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].Count > clusters[j].Count
	})

	if topN > 0 && len(clusters) > topN {
		clusters = clusters[:topN]
	}

	return clusters, nil
}
