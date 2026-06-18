package engine

import (
	"context"
	"fmt"
)

// FindByEntity 返回 vault 中所有提及指定实体的 engram，
// 使用 0x23 反向索引实现 O(matches) 查找。
// 结果上限为 limit（默认 20，最大 50）。
func (e *Engine) FindByEntity(ctx context.Context, vault, entityName string, limit int) ([]*Engram, error) {
	if entityName == "" {
		return nil, fmt.Errorf("find_by_entity: entity_name 不能为空")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	ws := e.store.ResolveVaultPrefix(vault)

	var results []*Engram
	err := e.store.ScanEntityEngrams(ctx, entityName, func(gotWS [8]byte, id ULID) error {
		if gotWS != ws {
			return nil
		}
		if len(results) >= limit {
			return fmt.Errorf("limit reached")
		}
		eng, err := e.store.GetEngram(ctx, ws, id)
		if err != nil || eng == nil {
			return nil
		}
		if eng.State == StateSoftDeleted {
			return nil
		}
		results = append(results, eng)
		return nil
	})
	if err != nil && err.Error() != "limit reached" {
		return nil, fmt.Errorf("find_by_entity: 扫描: %w", err)
	}
	return results, nil
}
