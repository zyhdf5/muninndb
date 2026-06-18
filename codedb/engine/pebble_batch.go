package engine

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/codedb/engine/erf"
	"github.com/scrypster/muninndb/codedb/engine/keys"
)

// pebbleStoreBatch 用单个 Pebble 批次实现 StoreBatch。
type pebbleStoreBatch struct {
	ps        *PebbleStore
	batch     *pebble.Batch
	committed bool
	closed    bool
}

// NewBatch 创建新的原子写批次。
func (ps *PebbleStore) NewBatch() StoreBatch {
	return &pebbleStoreBatch{ps: ps, batch: ps.db.NewBatch()}
}

// WriteEngram 将 engram 相关键写入批次（不立即提交）。
func (b *pebbleStoreBatch) WriteEngram(ctx context.Context, wsPrefix [8]byte, eng *Engram) error {
	_ = ctx
	if err := b.ensureOpen(); err != nil {
		return err
	}
	applyEngramDefaults(eng)

	erfBytes, err := erf.EncodeV2(toERFEngram(eng))
	if err != nil {
		return fmt.Errorf("批次编码 engram 失败: %w", err)
	}
	id16 := [16]byte(eng.ID)

	if err := b.batch.Set(keys.EngramKey(wsPrefix, id16), erfBytes, nil); err != nil {
		return fmt.Errorf("批次写入 0x01 失败: %w", err)
	}
	if err := b.batch.Set(keys.MetaKey(wsPrefix, id16), erf.MetaKeySlice(erfBytes), nil); err != nil {
		return fmt.Errorf("批次写入 0x02 失败: %w", err)
	}
	if len(eng.Embedding) > 0 {
		params, q := erf.Quantize(eng.Embedding)
		pb := erf.EncodeQuantizeParams(params)
		embed := make([]byte, 8+len(q))
		copy(embed[:8], pb[:])
		for i, v := range q {
			embed[8+i] = byte(v)
		}
		if err := b.batch.Set(keys.EmbeddingKey(wsPrefix, id16), embed, nil); err != nil {
			return fmt.Errorf("批次写入 0x18 失败: %w", err)
		}
	}

	if err := b.batch.Set(keys.StateIndexKey(wsPrefix, uint8(eng.State), id16), []byte{}, nil); err != nil {
		return fmt.Errorf("批次写入状态索引失败: %w", err)
	}
	for _, tag := range eng.Tags {
		if err := b.batch.Set(keys.TagIndexKey(wsPrefix, keys.Hash(tag), id16), []byte{}, nil); err != nil {
			return fmt.Errorf("批次写入标签索引失败: %w", err)
		}
	}
	if err := b.batch.Set(keys.CreatorIndexKey(wsPrefix, keys.Hash(eng.CreatedBy), id16), []byte{}, nil); err != nil {
		return fmt.Errorf("批次写入创建者索引失败: %w", err)
	}
	if err := b.batch.Set(keys.RelevanceBucketKey(wsPrefix, eng.Relevance, id16), []byte{}, nil); err != nil {
		return fmt.Errorf("批次写入相关性桶失败: %w", err)
	}
	if err := b.batch.Set(keys.LastAccessIndexKey(wsPrefix, eng.LastAccess.UnixMilli(), id16), nil, nil); err != nil {
		return fmt.Errorf("批次写入最后访问索引失败: %w", err)
	}

	for _, assoc := range eng.Associations {
		peak := assoc.PeakWeight
		if peak == 0 {
			peak = assoc.Weight
		}
		av := encodeAssocValue(assoc.RelType, assoc.Confidence, assoc.CreatedAt, assoc.LastActivated, peak, assoc.CoActivationCount)
		if err := b.batch.Set(keys.AssocFwdKey(wsPrefix, id16, assoc.Weight, [16]byte(assoc.TargetID)), av[:], nil); err != nil {
			return fmt.Errorf("批次写入前向关联失败: %w", err)
		}
		if err := b.batch.Set(keys.AssocRevKey(wsPrefix, [16]byte(assoc.TargetID), assoc.Weight, id16), av[:], nil); err != nil {
			return fmt.Errorf("批次写入反向关联失败: %w", err)
		}
		var wb [4]byte
		binary.BigEndian.PutUint32(wb[:], math.Float32bits(assoc.Weight))
		if err := b.batch.Set(keys.AssocWeightIndexKey(wsPrefix, id16, [16]byte(assoc.TargetID)), wb[:], nil); err != nil {
			return fmt.Errorf("批次写入关联权重索引失败: %w", err)
		}
	}

	return nil
}

// WriteAssociation 将关联键写入批次。
func (b *pebbleStoreBatch) WriteAssociation(ctx context.Context, wsPrefix [8]byte, src, dst ULID, assoc *Association) error {
	_ = ctx
	if err := b.ensureOpen(); err != nil {
		return err
	}
	peak := assoc.PeakWeight
	if peak == 0 {
		peak = assoc.Weight
	}
	av := encodeAssocValue(assoc.RelType, assoc.Confidence, assoc.CreatedAt, assoc.LastActivated, peak, assoc.CoActivationCount)
	if err := b.batch.Set(keys.AssocFwdKey(wsPrefix, [16]byte(src), assoc.Weight, [16]byte(dst)), av[:], nil); err != nil {
		return err
	}
	if err := b.batch.Set(keys.AssocRevKey(wsPrefix, [16]byte(dst), assoc.Weight, [16]byte(src)), av[:], nil); err != nil {
		return err
	}
	var wb [4]byte
	binary.BigEndian.PutUint32(wb[:], math.Float32bits(assoc.Weight))
	return b.batch.Set(keys.AssocWeightIndexKey(wsPrefix, [16]byte(src), [16]byte(dst)), wb[:], nil)
}

// WriteOrdinal 将序号键写入批次。
func (b *pebbleStoreBatch) WriteOrdinal(ctx context.Context, wsPrefix [8]byte, parentID, childID ULID, ordinal int32) error {
	_ = ctx
	if err := b.ensureOpen(); err != nil {
		return err
	}
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], uint32(ordinal))
	return b.batch.Set(keys.OrdinalKey(wsPrefix, [16]byte(parentID), [16]byte(childID)), buf[:], nil)
}

// UpdateEngramState 在批次内更新 engram 状态与状态索引。
func (b *pebbleStoreBatch) UpdateEngramState(ctx context.Context, ws [8]byte, id ULID, newState LifecycleState) error {
	if err := b.ensureOpen(); err != nil {
		return err
	}
	raw, err := Get(b.ps.pebbleReader(ctx), keys.EngramKey(ws, [16]byte(id)))
	if err != nil {
		return fmt.Errorf("读取 engram 失败: %w", err)
	}
	if raw == nil {
		return fmt.Errorf("engram 不存在: %s", id.String())
	}

	decoded, err := erf.Decode(raw)
	if err != nil {
		return fmt.Errorf("解码 engram 失败: %w", err)
	}
	oldState := LifecycleState(decoded.State)
	decoded.State = uint8(newState)
	decoded.UpdatedAt = time.Now()

	updatedRaw, err := erf.EncodeV2(decoded)
	if err != nil {
		return fmt.Errorf("重编码 engram 失败: %w", err)
	}

	id16 := [16]byte(id)
	if err := b.batch.Delete(keys.StateIndexKey(ws, uint8(oldState), id16), nil); err != nil {
		return err
	}
	if err := b.batch.Set(keys.StateIndexKey(ws, uint8(newState), id16), []byte{}, nil); err != nil {
		return err
	}
	if err := b.batch.Set(keys.EngramKey(ws, id16), updatedRaw, nil); err != nil {
		return err
	}
	return b.batch.Set(keys.MetaKey(ws, id16), erf.MetaKeySlice(updatedRaw), nil)
}

// Commit 提交批次。
func (b *pebbleStoreBatch) Commit() error {
	if err := b.ensureOpen(); err != nil {
		return err
	}
	b.committed = true
	if err := b.batch.Commit(pebble.NoSync); err != nil {
		return err
	}
	b.closed = true
	return b.batch.Close()
}

// Discard 释放批次资源。
func (b *pebbleStoreBatch) Discard() {
	if b.closed {
		return
	}
	b.closed = true
	_ = b.batch.Close()
}

// applyEngramDefaults 填充批量写入需要的默认值。
func applyEngramDefaults(eng *Engram) {
	if eng.ID == (ULID{}) {
		if !eng.CreatedAt.IsZero() {
			eng.ID = NewULIDWithTime(eng.CreatedAt)
		} else {
			eng.ID = NewULID()
		}
	}
	if eng.State == 0 {
		eng.State = StateActive
	}
	if eng.Confidence == 0 {
		eng.Confidence = 1.0
	}
	if eng.Stability == 0 {
		eng.Stability = 30.0
	}
	if eng.CreatedAt.IsZero() {
		eng.CreatedAt = time.Now()
	}
	if eng.UpdatedAt.IsZero() {
		eng.UpdatedAt = eng.CreatedAt
	}
	if eng.LastAccess.IsZero() {
		eng.LastAccess = eng.CreatedAt
	}
	if eng.EmbedDim == 0 {
		eng.EmbedDim = DimFromLen(len(eng.Embedding))
	}
}

// toERFEngram 将 engine.Engram 转换为 ERF 编码结构。
func toERFEngram(eng *Engram) *erf.Engram {
	assocs := make([]erf.Association, 0, len(eng.Associations))
	for _, a := range eng.Associations {
		assocs = append(assocs, erf.Association{
			TargetID:      [16]byte(a.TargetID),
			RelType:       uint16(a.RelType),
			Weight:        a.Weight,
			Confidence:    a.Confidence,
			CreatedAt:     a.CreatedAt,
			LastActivated: a.LastActivated,
		})
	}
	return &erf.Engram{
		ID:             [16]byte(eng.ID),
		CreatedAt:      eng.CreatedAt,
		UpdatedAt:      eng.UpdatedAt,
		LastAccess:     eng.LastAccess,
		Confidence:     eng.Confidence,
		Relevance:      eng.Relevance,
		Stability:      eng.Stability,
		AccessCount:    eng.AccessCount,
		State:          uint8(eng.State),
		EmbedDim:       uint8(eng.EmbedDim),
		Concept:        eng.Concept,
		CreatedBy:      eng.CreatedBy,
		Content:        eng.Content,
		Tags:           eng.Tags,
		Associations:   assocs,
		Embedding:      eng.Embedding,
		Summary:        eng.Summary,
		KeyPoints:      eng.KeyPoints,
		MemoryType:     uint8(eng.MemoryType),
		TypeLabel:      eng.TypeLabel,
		Classification: eng.Classification,
	}
}

// ensureOpen 保证批次仍可写。
func (b *pebbleStoreBatch) ensureOpen() error {
	if b.closed {
		return fmt.Errorf("批次已释放")
	}
	if b.committed {
		return fmt.Errorf("批次已提交")
	}
	return nil
}
