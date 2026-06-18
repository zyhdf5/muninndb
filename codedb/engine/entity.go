package engine

import (
	"context"
	"fmt"
	"sort"
)

// ────────────────────────────────────────────────────────────────────
// 默认常量
// ────────────────────────────────────────────────────────────────────

const (
	defaultEntityEngramLimit   = 20 // GetEntityAggregate 默认 engram 数量上限
	defaultListEntitiesLimit   = 50 // ListEntities 默认返回数量上限
	entityCoOccurrenceMinCount = 2  // 共现最小计数阈值
	entityCoOccurrenceTopN     = 20 // 共现实体对最大返回数
)

// ────────────────────────────────────────────────────────────────────
// EntityCoOccEntry — 共现实体条目
// ────────────────────────────────────────────────────────────────────

// EntityCoOccEntry 表示与目标实体共现的另一个实体及其共现次数。
type EntityCoOccEntry struct {
	Name  string // 共现实体名称
	Count int    // 共现次数
}

// ────────────────────────────────────────────────────────────────────
// EntityAggregateData — 命名实体的完整聚合视图
// ────────────────────────────────────────────────────────────────────

// EntityAggregateData 保存命名实体的完整聚合信息：
// 实体元数据、引用该实体的 engram 列表、关系列表和共现实体列表。
type EntityAggregateData struct {
	Record      *EntityRecord        // 实体元数据记录
	Engrams     []*Engram            // 引用该实体的 engram 列表
	Relations   []RelationshipRecord // 涉及该实体的关系列表
	CoOccurring []EntityCoOccEntry   // 共现实体列表（按次数降序）
}

// ────────────────────────────────────────────────────────────────────
// GetEntityAggregate — 获取实体的完整聚合视图
// ────────────────────────────────────────────────────────────────────

// GetEntityAggregate 返回命名实体的完整聚合视图，包括：
//  1. 实体元数据记录（全局，不区分 vault）
//  2. 引用该实体的 engram 列表（vault 级别）
//  3. 涉及该实体的关系列表（vault 级别）
//  4. 共现实体列表（vault 级别，按次数降序，最多 entityCoOccurrenceTopN 条）
//
// 如果实体不存在，返回 (nil, nil)。
func (e *Engine) GetEntityAggregate(ctx context.Context, vault, entityName string, limit int) (*EntityAggregateData, error) {
	if limit <= 0 {
		limit = defaultEntityEngramLimit
	}

	// 1. 获取实体元数据记录（全局注册表）。
	rec, err := e.store.GetEntityRecord(ctx, entityName)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}

	ws := e.store.ResolveVaultPrefix(vault)

	// 2. 扫描引用该实体的 engram（通过 0x23 反向索引）。
	var engrams []*Engram
	scanErr := e.store.ScanEntityEngrams(ctx, entityName, func(gotWS [8]byte, id ULID) error {
		if gotWS != ws {
			return nil // 不同 vault，跳过
		}
		if len(engrams) >= limit {
			return fmt.Errorf("limit reached")
		}
		eng, err := e.store.GetEngram(ctx, ws, id)
		if err != nil || eng == nil {
			return nil // 跳过缺失/已删除的记录
		}
		if eng.State == StateSoftDeleted {
			return nil
		}
		engrams = append(engrams, eng)
		return nil
	})
	if scanErr != nil && scanErr.Error() != "limit reached" {
		return nil, scanErr
	}

	// 3. 扫描涉及该实体的关系（通过 0x26 关系实体索引）。
	var rels []RelationshipRecord
	err = e.store.ScanEntityRelationships(ctx, ws, entityName, func(r RelationshipRecord) error {
		rels = append(rels, r)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 4. 获取共现实体（通过 0x24 共现索引），按次数降序排列。
	var coEntries []EntityCoOccEntry
	err = e.store.ScanEntityClusters(ctx, ws, entityCoOccurrenceMinCount, func(nameA, nameB string, count int) error {
		if nameA == entityName {
			coEntries = append(coEntries, EntityCoOccEntry{Name: nameB, Count: count})
		} else if nameB == entityName {
			coEntries = append(coEntries, EntityCoOccEntry{Name: nameA, Count: count})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(coEntries, func(i, j int) bool { return coEntries[i].Count > coEntries[j].Count })
	if len(coEntries) > entityCoOccurrenceTopN {
		coEntries = coEntries[:entityCoOccurrenceTopN]
	}

	return &EntityAggregateData{
		Record:      rec,
		Engrams:     engrams,
		Relations:   rels,
		CoOccurring: coEntries,
	}, nil
}

// ────────────────────────────────────────────────────────────────────
// ListEntities — 列出 vault 中的实体
// ────────────────────────────────────────────────────────────────────

// ListEntities 返回 vault 中的实体记录列表，按提及次数降序排列。
// 可选按状态过滤（state 为空时不过滤）。
func (e *Engine) ListEntities(ctx context.Context, vault string, limit int, state string) ([]EntityRecord, error) {
	if limit <= 0 {
		limit = defaultListEntitiesLimit
	}

	ws := e.store.ResolveVaultPrefix(vault)

	var records []EntityRecord
	err := e.store.ScanVaultEntityNames(ctx, ws, func(name string) error {
		rec, err := e.store.GetEntityRecord(ctx, name)
		if err != nil || rec == nil {
			return nil
		}
		if state != "" && rec.State != state {
			return nil
		}
		records = append(records, *rec)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].MentionCount > records[j].MentionCount
	})
	if len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}
