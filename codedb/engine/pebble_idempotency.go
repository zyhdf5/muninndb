package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/codedb/engine/keys"
)

// CheckIdempotency 查询 opID 对应的收据。
func (ps *PebbleStore) CheckIdempotency(ctx context.Context, opID string) (*IdempotencyReceipt, error) {
	val, err := Get(ps.pebbleReader(ctx), keys.IdempotencyKey(opID))
	if err != nil {
		return nil, fmt.Errorf("查询幂等收据失败: %w", err)
	}
	if val == nil {
		return nil, nil
	}
	var receipt IdempotencyReceipt
	if err := json.Unmarshal(val, &receipt); err != nil {
		return nil, fmt.Errorf("解析幂等收据失败: %w", err)
	}
	return &receipt, nil
}

// WriteIdempotency 写入 opID 到 engramID 的收据。
func (ps *PebbleStore) WriteIdempotency(ctx context.Context, opID, engramID string) error {
	_ = ctx
	receipt := IdempotencyReceipt{
		OpID:      opID,
		EngramID:  engramID,
		CreatedAt: time.Now().UnixNano(),
	}
	val, err := json.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("序列化幂等收据失败: %w", err)
	}
	return ps.db.Set(keys.IdempotencyKey(opID), val, pebble.NoSync)
}
