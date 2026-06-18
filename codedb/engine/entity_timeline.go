package engine

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// TimelineEntry 表示实体时间线中的单个条目。
type TimelineEntry struct {
	EngramID  string    `json:"engram_id"`  // engram 的字符串 ID
	Concept   string    `json:"concept"`    // engram 的概念标签
	CreatedAt time.Time `json:"created_at"` // engram 创建时间
	Summary   string    `json:"summary"`    // engram 摘要
}

// EntityTimeline 表示实体从首次出现到当前的完整时间线。
type EntityTimeline struct {
	Entity       string          `json:"entity"`        // 实体名称
	FirstSeen    time.Time       `json:"first_seen"`    // 首次出现时间
	MentionCount int             `json:"mention_count"` // 提及总次数
	Entries      []TimelineEntry `json:"timeline"`      // 时间线条目列表
	Count        int             `json:"count"`         // 返回的条目数
}

// GetEntityTimeline 返回实体的时间线视图：按创建时间升序排列的 engram 列表。
// 扫描 0x23 反向索引查找所有提及该实体的 engram，结果上限为 limit（默认 10，最大 50）。
func (e *Engine) GetEntityTimeline(ctx context.Context, vault string, entityName string, limit int) (*EntityTimeline, error) {
	if entityName == "" {
		return nil, fmt.Errorf("entity_name 不能为空")
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	// 获取实体记录，确认实体存在并读取 FirstSeen 和 MentionCount。
	entityRecord, err := e.store.GetEntityRecord(ctx, entityName)
	if err != nil {
		return nil, fmt.Errorf("获取实体记录: %w", err)
	}
	if entityRecord == nil {
		return nil, fmt.Errorf("实体注册表中未找到 entity_name")
	}

	ws := e.store.ResolveVaultPrefix(vault)

	// 扫描所有提及该实体的 engram。
	var entries []TimelineEntry
	err = e.store.ScanEntityEngrams(ctx, entityName, func(gotWS [8]byte, id ULID) error {
		if gotWS != ws {
			return nil
		}
		if len(entries) >= limit {
			return fmt.Errorf("limit reached")
		}

		eng, err := e.store.GetEngram(ctx, ws, id)
		if err != nil || eng == nil {
			return nil
		}
		if eng.State == StateSoftDeleted {
			return nil
		}

		entries = append(entries, TimelineEntry{
			EngramID:  id.String(),
			Concept:   eng.Concept,
			CreatedAt: eng.CreatedAt,
			Summary:   eng.Summary,
		})
		return nil
	})
	if err != nil && err.Error() != "limit reached" {
		return nil, fmt.Errorf("扫描实体 engram: %w", err)
	}

	// 按创建时间升序排列（最早的在前）。
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt.Before(entries[j].CreatedAt)
	})

	firstSeen := time.Unix(0, entityRecord.FirstSeen).UTC()

	return &EntityTimeline{
		Entity:       entityName,
		FirstSeen:    firstSeen,
		MentionCount: int(entityRecord.MentionCount),
		Entries:      entries,
		Count:        len(entries),
	}, nil
}
