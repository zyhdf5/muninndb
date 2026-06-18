package engine

import (
	"context"
	"fmt"
)

// SetEntityState 设置命名实体的生命周期状态，可选更正其类型。
// 当 state="merged" 时，mergedInto 必须为规范实体名称。
// entityType 为空时保留现有类型。
func (e *Engine) SetEntityState(ctx context.Context, entityName, state, mergedInto, entityType string) error {
	if entityName == "" {
		return fmt.Errorf("set_entity_state: entity_name 不能为空")
	}

	// 读取现有记录以保留其他字段。
	existing, err := e.store.GetEntityRecord(ctx, entityName)
	if err != nil {
		return fmt.Errorf("set_entity_state: 读取实体: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("set_entity_state: 实体 %q 不存在", entityName)
	}

	// 使用调用者提供的类型；未提供时沿用现有类型。
	resolvedType := existing.Type
	if entityType != "" {
		resolvedType = entityType
	}

	// 构建更新后的记录。UpsertEntityRecord 内部会校验 state 和 MergedInto 的一致性。
	record := EntityRecord{
		Name:       entityName,
		State:      state,
		MergedInto: mergedInto,
		Type:       resolvedType,
		Confidence: existing.Confidence,
	}

	return e.store.UpsertEntityRecord(ctx, record, "mcp:entity_state")
}

// EntityStateOp 是 SetEntityStateBatch 中的单个操作。
type EntityStateOp struct {
	EntityName string // 实体名称
	State      string // 目标状态
	MergedInto string // 合并目标（仅 state="merged" 时必填）
	EntityType string // 更正类型（可选，为空则保留现有类型）
}

// SetEntityStateBatch 批量执行实体状态更新。
// 返回值为每个操作对应的 error（nil 表示成功）。
// 不返回顶层错误——支持部分成功。在每个操作之间检查 context 取消。
func (e *Engine) SetEntityStateBatch(ctx context.Context, ops []EntityStateOp) []error {
	errs := make([]error, len(ops))
	for i, op := range ops {
		if ctx.Err() != nil {
			errs[i] = ctx.Err()
			continue
		}
		errs[i] = e.SetEntityState(ctx, op.EntityName, op.State, op.MergedInto, op.EntityType)
	}
	return errs
}
