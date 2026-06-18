package engine

import (
	"context"
	"sort"
)

const (
	// entityBoostFactor 是与 BFS 前 N 名结果共享命名实体的 engram 所获得的分数增量。
	// 刻意低于典型 BFS 关联权重（0.3–0.9），使增强内容浮出但不会喧宾夺主。
	entityBoostFactor = float64(0.15)

	// entityBoostTopN 是用作传播激活种子的 BFS 前 N 名结果数量。
	entityBoostTopN = 5
)

// ApplyEntityBoost 在 BFS 之后执行基于命名实体的传播激活。
//
// 算法流程：
//  1. 取 results 中前 entityBoostTopN 个结果作为种子
//  2. 对每个种子，通过 0x20 前向索引查找其关联的所有实体名称
//  3. 对每个实体名称，通过 0x23 反向索引查找同 vault 中所有提及该实体的 engram
//  4. 已存在于 results 中的 engram 加 entityBoostFactor 分数；新发现的 engram 以 entityBoostFactor 为初始分数加入
//  5. 重新按分数降序排列
func (e *Engine) ApplyEntityBoost(ctx context.Context, ws [8]byte, results []ScoredEngram) []ScoredEngram {
	if len(results) == 0 {
		return results
	}

	seedCount := len(results)
	if seedCount > entityBoostTopN {
		seedCount = entityBoostTopN
	}
	seeds := results[:seedCount]

	// 构建反向索引：ULID → results 切片中的位置。
	seenInResults := make(map[ULID]int, len(results))
	for i, r := range results {
		seenInResults[r.Engram.ID] = i
	}

	for _, topEng := range seeds {
		_ = e.store.ScanEngramEntities(ctx, ws, topEng.Engram.ID, func(entityName string) error {
			return e.store.ScanEntityEngrams(ctx, entityName, func(entityWS [8]byte, engramID ULID) error {
				if entityWS != ws {
					return nil
				}
				if engramID == topEng.Engram.ID {
					return nil
				}

				if idx, found := seenInResults[engramID]; found {
					results[idx].Score += entityBoostFactor
				} else {
					eng, err := e.store.GetEngram(ctx, ws, engramID)
					if err != nil || eng == nil {
						return nil
					}
					if eng.State == StateSoftDeleted || eng.State == StateArchived {
						return nil
					}
					results = append(results, ScoredEngram{
						Engram: eng,
						Score:  entityBoostFactor,
					})
					seenInResults[engramID] = len(results) - 1
				}
				return nil
			})
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}
