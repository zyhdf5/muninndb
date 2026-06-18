package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/codedb/engine/erf"
	"github.com/scrypster/muninndb/codedb/engine/keys"
	"github.com/vmihailenco/msgpack/v5"
)

func (ps *PebbleStore) GetEngram(ctx context.Context, wsPrefix [8]byte, id ULID) (*Engram, error) {
	if eng, found := ps.cache.Get(wsPrefix, id); found {
		return eng, nil
	}

	val, err := Get(ps.pebbleReader(ctx), keys.EngramKey(wsPrefix, [16]byte(id)))
	if err != nil {
		return nil, fmt.Errorf("读取 engram 失败: %w", err)
	}
	if val == nil {
		return nil, fmt.Errorf("engram 不存在")
	}

	erfEng, err := erf.Decode(val)
	if err != nil {
		return nil, fmt.Errorf("解码 engram 失败: %w", err)
	}

	eng := fromERFEngram(erfEng)
	ps.cache.Set(wsPrefix, id, eng)
	return eng, nil
}

func (ps *PebbleStore) EngramLastAccessNs(wsPrefix [8]byte, id ULID) int64 {
	return ps.cache.LastAccessNs(wsPrefix, id)
}

func (ps *PebbleStore) GetEngrams(ctx context.Context, wsPrefix [8]byte, ids []ULID) ([]*Engram, error) {
	result := make([]*Engram, len(ids))

	missIdx := make([]int, 0, len(ids))
	missIDs := make([]ULID, 0, len(ids))
	missKeys := make([][]byte, 0, len(ids))

	for i, id := range ids {
		if eng, found := ps.cache.Get(wsPrefix, id); found {
			result[i] = eng
			continue
		}
		missIdx = append(missIdx, i)
		missIDs = append(missIDs, id)
		missKeys = append(missKeys, keys.EngramKey(wsPrefix, [16]byte(id)))
	}

	if len(missKeys) == 0 {
		return result, nil
	}

	vals, err := MultiGet(ps.pebbleReader(ctx), missKeys)
	if err != nil {
		return nil, fmt.Errorf("批量读取 engram 失败: %w", err)
	}

	for i, raw := range vals {
		if raw == nil {
			continue
		}
		erfEng, decErr := erf.Decode(raw)
		if decErr != nil {
			continue
		}
		eng := fromERFEngram(erfEng)
		ps.cache.Set(wsPrefix, missIDs[i], eng)
		result[missIdx[i]] = eng
	}

	return result, nil
}

func (ps *PebbleStore) GetMetadata(ctx context.Context, wsPrefix [8]byte, ids []ULID) ([]*EngramMeta, error) {
	result := make([]*EngramMeta, len(ids))

	missIdx := make([]int, 0, len(ids))
	missIDs := make([]ULID, 0, len(ids))
	missKeys := make([][]byte, 0, len(ids))

	for i, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if meta, ok := ps.metaCache.Get([16]byte(id)); ok {
			result[i] = meta
			continue
		}

		if eng, found := ps.cache.Get(wsPrefix, id); found {
			meta := &EngramMeta{
				ID:          eng.ID,
				CreatedAt:   eng.CreatedAt,
				UpdatedAt:   eng.UpdatedAt,
				LastAccess:  eng.LastAccess,
				Confidence:  eng.Confidence,
				Relevance:   eng.Relevance,
				Stability:   eng.Stability,
				AccessCount: eng.AccessCount,
				State:       eng.State,
				AssocCount:  uint16(len(eng.Associations)),
				EmbedDim:    eng.EmbedDim,
				MemoryType:  eng.MemoryType,
			}
			ps.metaCache.Add([16]byte(id), meta)
			result[i] = meta
			continue
		}

		missIdx = append(missIdx, i)
		missIDs = append(missIDs, id)
		missKeys = append(missKeys, keys.MetaKey(wsPrefix, [16]byte(id)))
	}

	if len(missKeys) == 0 {
		return result, nil
	}

	vals, err := MultiGet(ps.pebbleReader(ctx), missKeys)
	if err != nil {
		return nil, fmt.Errorf("批量读取 metadata 失败: %w", err)
	}

	for i, raw := range vals {
		if raw == nil {
			continue
		}
		erfMeta, decErr := erf.DecodeMeta(raw)
		if decErr != nil {
			return nil, fmt.Errorf("解码 metadata 失败: %w", decErr)
		}
		meta := &EngramMeta{
			ID:          ULID(erfMeta.ID),
			CreatedAt:   erfMeta.CreatedAt,
			UpdatedAt:   erfMeta.UpdatedAt,
			LastAccess:  erfMeta.LastAccess,
			Confidence:  erfMeta.Confidence,
			Relevance:   erfMeta.Relevance,
			Stability:   erfMeta.Stability,
			AccessCount: erfMeta.AccessCount,
			State:       LifecycleState(erfMeta.State),
			AssocCount:  erfMeta.AssocCount,
			EmbedDim:    EmbedDimension(erfMeta.EmbedDim),
			MemoryType:  MemoryType(erfMeta.MemoryType),
		}
		ps.metaCache.Add([16]byte(missIDs[i]), meta)
		result[missIdx[i]] = meta
	}

	return result, nil
}

func (ps *PebbleStore) UpdateMetadata(ctx context.Context, wsPrefix [8]byte, id ULID, meta *EngramMeta) error {
	oldMetas, err := ps.GetMetadata(ctx, wsPrefix, []ULID{id})
	if err != nil {
		return err
	}
	if len(oldMetas) == 0 || oldMetas[0] == nil {
		return fmt.Errorf("engram 不存在")
	}

	oldState := oldMetas[0].State
	oldRelevance := oldMetas[0].Relevance
	var oldLastAccessMillis int64
	if !oldMetas[0].LastAccess.IsZero() {
		oldLastAccessMillis = oldMetas[0].LastAccess.UnixMilli()
	}

	engramKey := keys.EngramKey(wsPrefix, [16]byte(id))
	raw, err := Get(ps.pebbleReader(ctx), engramKey)
	if err != nil {
		return fmt.Errorf("读取 engram 原始数据失败: %w", err)
	}
	if raw == nil {
		return fmt.Errorf("engram 不存在")
	}

	if err := erf.PatchAllMeta(raw,
		meta.UpdatedAt, meta.LastAccess,
		meta.Confidence, meta.Relevance, meta.Stability,
		meta.AccessCount, uint8(meta.State),
	); err != nil {
		return fmt.Errorf("补丁 metadata 失败: %w", err)
	}

	batch := ps.db.NewBatch()
	defer batch.Close()

	if oldState != meta.State {
		BatchDelete(batch, keys.StateIndexKey(wsPrefix, uint8(oldState), [16]byte(id)))
		BatchSet(batch, keys.StateIndexKey(wsPrefix, uint8(meta.State), [16]byte(id)), []byte{})
	}

	if oldRelevance != meta.Relevance {
		BatchDelete(batch, keys.RelevanceBucketKey(wsPrefix, oldRelevance, [16]byte(id)))
		BatchSet(batch, keys.RelevanceBucketKey(wsPrefix, meta.Relevance, [16]byte(id)), []byte{})
	}

	newLastAccessMillis := int64(0)
	if !meta.LastAccess.IsZero() {
		newLastAccessMillis = meta.LastAccess.UnixMilli()
	}
	if oldLastAccessMillis != newLastAccessMillis {
		if oldLastAccessMillis != 0 {
			BatchDelete(batch, keys.LastAccessIndexKey(wsPrefix, oldLastAccessMillis, [16]byte(id)))
		}
		if newLastAccessMillis != 0 {
			BatchSet(batch, keys.LastAccessIndexKey(wsPrefix, newLastAccessMillis, [16]byte(id)), nil)
		}
	}

	BatchSet(batch, engramKey, raw)
	BatchSet(batch, keys.MetaKey(wsPrefix, [16]byte(id)), erf.MetaKeySlice(raw))

	ps.cache.Delete(wsPrefix, id)
	ps.metaCache.Remove([16]byte(id))

	if err := batch.Commit(pebble.NoSync); err != nil {
		return fmt.Errorf("提交 metadata 更新失败: %w", err)
	}

	return nil
}

func (ps *PebbleStore) UpdateRelevance(ctx context.Context, wsPrefix [8]byte, id ULID, relevance, stability float32) error {
	metas, err := ps.GetMetadata(ctx, wsPrefix, []ULID{id})
	if err != nil {
		return err
	}
	if len(metas) == 0 || metas[0] == nil {
		return fmt.Errorf("engram 不存在")
	}
	oldRelevance := metas[0].Relevance

	engramKey := keys.EngramKey(wsPrefix, [16]byte(id))
	raw, err := Get(ps.pebbleReader(ctx), engramKey)
	if err != nil {
		return fmt.Errorf("读取 engram 原始数据失败: %w", err)
	}
	if raw == nil {
		return fmt.Errorf("engram 不存在")
	}

	if err := erf.PatchRelevance(raw, time.Now(), relevance, stability); err != nil {
		return fmt.Errorf("补丁 relevance 失败: %w", err)
	}

	batch := ps.db.NewBatch()
	defer batch.Close()

	BatchDelete(batch, keys.RelevanceBucketKey(wsPrefix, oldRelevance, [16]byte(id)))
	BatchSet(batch, keys.RelevanceBucketKey(wsPrefix, relevance, [16]byte(id)), []byte{})
	BatchSet(batch, engramKey, raw)
	BatchSet(batch, keys.MetaKey(wsPrefix, [16]byte(id)), erf.MetaKeySlice(raw))

	ps.cache.Delete(wsPrefix, id)
	ps.metaCache.Remove([16]byte(id))

	if err := batch.Commit(pebble.NoSync); err != nil {
		return fmt.Errorf("提交 relevance 更新失败: %w", err)
	}

	return nil
}

func (ps *PebbleStore) WriteEngram(ctx context.Context, wsPrefix [8]byte, eng *Engram) (ULID, error) {
	if eng == nil {
		return ULID{}, fmt.Errorf("engram 不能为空")
	}

	normalizeEngramForWrite(eng)

	erfBytes, err := erf.EncodeV2(toERFEngram(eng))
	if err != nil {
		return ULID{}, fmt.Errorf("编码 engram 失败: %w", err)
	}

	batch := ps.db.NewBatch()
	defer batch.Close()

	if err := writeEngramToBatch(batch, wsPrefix, eng, erfBytes); err != nil {
		return ULID{}, err
	}

	if err := batch.Commit(pebble.NoSync); err != nil {
		return ULID{}, fmt.Errorf("提交 engram 写入失败: %w", err)
	}

	vc := ps.getOrInitCounter(ctx, wsPrefix)
	newCount := vc.count.Add(1)
	ps.persistVaultCount(wsPrefix, newCount)

	return eng.ID, nil
}

func (ps *PebbleStore) WriteEngramBatch(ctx context.Context, items []EngramBatchItem) ([]ULID, []error) {
	n := len(items)
	ids := make([]ULID, n)
	errs := make([]error, n)
	if n == 0 {
		return ids, errs
	}

	batch := ps.db.NewBatch()
	defer batch.Close()

	for i := range items {
		if items[i].Engram == nil {
			errs[i] = fmt.Errorf("item[%d] engram 不能为空", i)
			continue
		}

		eng := items[i].Engram
		normalizeEngramForWrite(eng)

		erfBytes, err := erf.EncodeV2(toERFEngram(eng))
		if err != nil {
			errs[i] = fmt.Errorf("编码 engram 失败: %w", err)
			continue
		}

		if err := writeEngramToBatch(batch, items[i].WSPrefix, eng, erfBytes); err != nil {
			errs[i] = err
			continue
		}

		ids[i] = eng.ID
	}

	if err := batch.Commit(pebble.NoSync); err != nil {
		for i := range errs {
			if errs[i] == nil {
				errs[i] = fmt.Errorf("提交批量写入失败: %w", err)
			}
		}
		return ids, errs
	}

	for i := range items {
		if errs[i] != nil {
			continue
		}
		vc := ps.getOrInitCounter(ctx, items[i].WSPrefix)
		newCount := vc.count.Add(1)
		ps.persistVaultCount(items[i].WSPrefix, newCount)
	}

	return ids, errs
}

func (ps *PebbleStore) DeleteEngram(ctx context.Context, wsPrefix [8]byte, id ULID) error {
	eng, err := ps.GetEngram(ctx, wsPrefix, id)
	if err != nil {
		batch := ps.db.NewBatch()
		defer batch.Close()
		BatchDelete(batch, keys.EngramKey(wsPrefix, [16]byte(id)))
		BatchDelete(batch, keys.MetaKey(wsPrefix, [16]byte(id)))
		BatchDelete(batch, keys.EmbeddingKey(wsPrefix, [16]byte(id)))
		ps.cache.Delete(wsPrefix, id)
		ps.metaCache.Remove([16]byte(id))
		if commitErr := batch.Commit(pebble.NoSync); commitErr != nil {
			return fmt.Errorf("删除 engram 失败: %w", commitErr)
		}
		return nil
	}

	batch := ps.db.NewBatch()
	defer batch.Close()

	id16 := [16]byte(id)
	BatchDelete(batch, keys.EngramKey(wsPrefix, id16))
	BatchDelete(batch, keys.MetaKey(wsPrefix, id16))
	BatchDelete(batch, keys.EmbeddingKey(wsPrefix, id16))

	BatchDelete(batch, keys.StateIndexKey(wsPrefix, uint8(eng.State), id16))
	BatchDelete(batch, keys.CreatorIndexKey(wsPrefix, keys.Hash(eng.CreatedBy), id16))
	BatchDelete(batch, keys.RelevanceBucketKey(wsPrefix, eng.Relevance, id16))
	for _, tag := range eng.Tags {
		BatchDelete(batch, keys.TagIndexKey(wsPrefix, keys.Hash(tag), id16))
	}
	if !eng.LastAccess.IsZero() {
		BatchDelete(batch, keys.LastAccessIndexKey(wsPrefix, eng.LastAccess.UnixMilli(), id16))
	}

	if err := deleteAssocKeysForEngram(ps.db, batch, wsPrefix, id16); err != nil {
		return err
	}
	if err := deleteOrdinalKeysForEngram(ps.db, batch, wsPrefix, id16); err != nil {
		return err
	}

	entityNames, err := deleteEntityLinksForEngram(ps.db, batch, wsPrefix, id16)
	if err != nil {
		slog.Warn("删除 engram 时清理实体链接失败", "engram", id.String(), "error", err)
	}

	if err := batch.Commit(pebble.NoSync); err != nil {
		return fmt.Errorf("删除 engram 提交失败: %w", err)
	}

	ps.cache.Delete(wsPrefix, id)
	ps.metaCache.Remove([16]byte(id))

	for _, name := range entityNames {
		if err := ps.DecrementEntityMentionCount(ctx, name); err != nil {
			slog.Warn("递减实体提及计数失败", "entity", name, "engram", id.String(), "error", err)
		}
	}

	const maxCoOccurrenceEntities = 50
	coNames := entityNames
	if len(coNames) > maxCoOccurrenceEntities {
		coNames = coNames[:maxCoOccurrenceEntities]
	}
	for i := 0; i < len(coNames); i++ {
		for j := i + 1; j < len(coNames); j++ {
			if err := ps.DecrementEntityCoOccurrence(ctx, wsPrefix, coNames[i], coNames[j]); err != nil {
				slog.Warn("递减实体共现计数失败", "a", coNames[i], "b", coNames[j], "engram", id.String(), "error", err)
			}
		}
	}

	vc := ps.getOrInitCounter(ctx, wsPrefix)
	newCount := vc.count.Add(-1)
	if newCount < 0 {
		vc.count.Store(0)
		newCount = 0
	}
	ps.persistVaultCount(wsPrefix, newCount)

	return nil
}

func (ps *PebbleStore) SoftDelete(ctx context.Context, wsPrefix [8]byte, id ULID) error {
	eng, err := ps.GetEngram(ctx, wsPrefix, id)
	if err != nil {
		return err
	}

	oldState := eng.State
	eng.State = StateSoftDeleted
	eng.UpdatedAt = time.Now()

	erfBytes, err := erf.Encode(toERFEngram(eng))
	if err != nil {
		return fmt.Errorf("编码 engram 失败: %w", err)
	}

	batch := ps.db.NewBatch()
	defer batch.Close()

	BatchDelete(batch, keys.StateIndexKey(wsPrefix, uint8(oldState), [16]byte(id)))
	BatchSet(batch, keys.StateIndexKey(wsPrefix, uint8(StateSoftDeleted), [16]byte(id)), []byte{})
	BatchSet(batch, keys.EngramKey(wsPrefix, [16]byte(id)), erfBytes)
	BatchSet(batch, keys.MetaKey(wsPrefix, [16]byte(id)), erf.MetaKeySlice(erfBytes))

	if err := batch.Commit(pebble.NoSync); err != nil {
		return fmt.Errorf("提交软删除失败: %w", err)
	}

	ps.cache.Set(wsPrefix, id, eng)
	ps.metaCache.Remove([16]byte(id))
	_ = ctx
	return nil
}

func (ps *PebbleStore) UpdateTags(ctx context.Context, wsPrefix [8]byte, id ULID, tags []string) error {
	eng, err := ps.GetEngram(ctx, wsPrefix, id)
	if err != nil {
		return err
	}

	eng.Tags = tags
	eng.UpdatedAt = time.Now()

	erfBytes, err := erf.Encode(toERFEngram(eng))
	if err != nil {
		return fmt.Errorf("编码 engram 失败: %w", err)
	}

	batch := ps.db.NewBatch()
	defer batch.Close()

	BatchSet(batch, keys.EngramKey(wsPrefix, [16]byte(id)), erfBytes)
	BatchSet(batch, keys.MetaKey(wsPrefix, [16]byte(id)), erf.MetaKeySlice(erfBytes))
	for _, tag := range tags {
		BatchSet(batch, keys.TagIndexKey(wsPrefix, keys.Hash(tag), [16]byte(id)), []byte{})
	}

	ps.cache.Delete(wsPrefix, id)
	ps.metaCache.Remove([16]byte(id))

	if err := batch.Commit(pebble.NoSync); err != nil {
		return fmt.Errorf("提交标签更新失败: %w", err)
	}

	_ = ctx
	return nil
}

func (ps *PebbleStore) GetEmbedding(ctx context.Context, wsPrefix [8]byte, id ULID) ([]float32, error) {
	val, err := Get(ps.pebbleReader(ctx), keys.EmbeddingKey(wsPrefix, [16]byte(id)))
	if err != nil {
		return nil, fmt.Errorf("读取 embedding 失败: %w", err)
	}
	if len(val) < 8 {
		return nil, nil
	}
	var paramBytes [8]byte
	copy(paramBytes[:], val[:8])
	params := erf.DecodeQuantizeParams(paramBytes)
	q := make([]int8, len(val)-8)
	for i := range q {
		q[i] = int8(val[8+i])
	}
	return erf.Dequantize(q, params), nil
}

func (ps *PebbleStore) GetConfidence(ctx context.Context, wsPrefix [8]byte, id ULID) (float32, error) {
	val, err := Get(ps.pebbleReader(ctx), keys.MetaKey(wsPrefix, [16]byte(id)))
	if err != nil {
		return 0, fmt.Errorf("读取 metadata 失败: %w", err)
	}
	if val == nil {
		return 0, fmt.Errorf("metadata 不存在")
	}
	erfMeta, err := erf.DecodeMeta(val)
	if err != nil {
		return 0, fmt.Errorf("解码 metadata 失败: %w", err)
	}
	return erfMeta.Confidence, nil
}

func (ps *PebbleStore) UpdateConfidence(ctx context.Context, wsPrefix [8]byte, id ULID, confidence float32) error {
	eng, err := ps.GetEngram(ctx, wsPrefix, id)
	if err != nil {
		return err
	}

	eng.Confidence = confidence
	eng.UpdatedAt = time.Now()

	erfBytes, err := erf.Encode(toERFEngram(eng))
	if err != nil {
		return fmt.Errorf("编码 engram 失败: %w", err)
	}

	batch := ps.db.NewBatch()
	defer batch.Close()
	BatchSet(batch, keys.EngramKey(wsPrefix, [16]byte(id)), erfBytes)
	BatchSet(batch, keys.MetaKey(wsPrefix, [16]byte(id)), erf.MetaKeySlice(erfBytes))

	if err := batch.Commit(pebble.NoSync); err != nil {
		return fmt.Errorf("提交置信度更新失败: %w", err)
	}

	ps.cache.Set(wsPrefix, id, eng)
	ps.metaCache.Remove([16]byte(id))
	_ = ctx
	return nil
}

func (ps *PebbleStore) ScanEngrams(ctx context.Context, wsPrefix [8]byte, fn func(*Engram) error) error {
	prefix := make([]byte, 1+8)
	prefix[0] = 0x01
	copy(prefix[1:], wsPrefix[:])

	iter, err := PrefixIterator(ps.pebbleReader(ctx), prefix)
	if err != nil {
		return fmt.Errorf("创建 engram 扫描迭代器失败: %w", err)
	}
	defer iter.Close()

	for ok := iter.First(); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}

		k := iter.Key()
		if len(k) < 25 {
			continue
		}

		raw := make([]byte, len(iter.Value()))
		copy(raw, iter.Value())

		erfEng, decErr := erf.Decode(raw)
		if decErr != nil {
			continue
		}
		eng := fromERFEngram(erfEng)

		var id ULID
		copy(id[:], k[9:25])
		emb, embErr := ps.GetEmbedding(ctx, wsPrefix, id)
		if embErr == nil && len(emb) > 0 {
			eng.Embedding = emb
		}

		if err := fn(eng); err != nil {
			return err
		}
	}

	if err := iter.Error(); err != nil {
		return fmt.Errorf("扫描 engram 失败: %w", err)
	}
	return nil
}

func fromERFEngram(e *erf.Engram) *Engram {
	assocs := make([]Association, len(e.Associations))
	for i, a := range e.Associations {
		assocs[i] = Association{
			TargetID:      ULID(a.TargetID),
			RelType:       RelType(a.RelType),
			Weight:        a.Weight,
			Confidence:    a.Confidence,
			CreatedAt:     a.CreatedAt,
			LastActivated: a.LastActivated,
		}
	}

	return &Engram{
		ID:             ULID(e.ID),
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
		LastAccess:     e.LastAccess,
		Confidence:     e.Confidence,
		Relevance:      e.Relevance,
		Stability:      e.Stability,
		AccessCount:    e.AccessCount,
		State:          LifecycleState(e.State),
		EmbedDim:       EmbedDimension(e.EmbedDim),
		Concept:        e.Concept,
		CreatedBy:      e.CreatedBy,
		Content:        e.Content,
		Tags:           e.Tags,
		Associations:   assocs,
		Embedding:      e.Embedding,
		Summary:        e.Summary,
		KeyPoints:      e.KeyPoints,
		MemoryType:     MemoryType(e.MemoryType),
		TypeLabel:      e.TypeLabel,
		Classification: e.Classification,
	}
}

func normalizeEngramForWrite(eng *Engram) {
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
}

func writeEngramToBatch(batch *pebble.Batch, wsPrefix [8]byte, eng *Engram, erfBytes []byte) error {
	id16 := [16]byte(eng.ID)

	BatchSet(batch, keys.EngramKey(wsPrefix, id16), erfBytes)
	BatchSet(batch, keys.MetaKey(wsPrefix, id16), erf.MetaKeySlice(erfBytes))

	if len(eng.Embedding) > 0 {
		params, quantized := erf.Quantize(eng.Embedding)
		paramsBuf := erf.EncodeQuantizeParams(params)
		embedBytes := make([]byte, 8+len(quantized))
		copy(embedBytes[:8], paramsBuf[:])
		for i, v := range quantized {
			embedBytes[8+i] = byte(v)
		}
		BatchSet(batch, keys.EmbeddingKey(wsPrefix, id16), embedBytes)
	}

	for _, assoc := range eng.Associations {
		peak := assoc.PeakWeight
		if peak == 0 {
			peak = assoc.Weight
		}
		av := encodeAssocValue(assoc.RelType, assoc.Confidence, assoc.CreatedAt, assoc.LastActivated, peak, assoc.CoActivationCount)
		BatchSet(batch, keys.AssocFwdKey(wsPrefix, id16, assoc.Weight, [16]byte(assoc.TargetID)), av[:])
		BatchSet(batch, keys.AssocRevKey(wsPrefix, [16]byte(assoc.TargetID), assoc.Weight, id16), av[:])
		var wi [4]byte
		binary.BigEndian.PutUint32(wi[:], math.Float32bits(assoc.Weight))
		BatchSet(batch, keys.AssocWeightIndexKey(wsPrefix, id16, [16]byte(assoc.TargetID)), wi[:])
	}

	BatchSet(batch, keys.StateIndexKey(wsPrefix, uint8(eng.State), id16), []byte{})
	for _, tag := range eng.Tags {
		BatchSet(batch, keys.TagIndexKey(wsPrefix, keys.Hash(tag), id16), []byte{})
	}
	BatchSet(batch, keys.CreatorIndexKey(wsPrefix, keys.Hash(eng.CreatedBy), id16), []byte{})
	BatchSet(batch, keys.RelevanceBucketKey(wsPrefix, eng.Relevance, id16), []byte{})
	BatchSet(batch, keys.LastAccessIndexKey(wsPrefix, eng.LastAccess.UnixMilli(), id16), nil)

	return nil
}

func deleteAssocKeysForEngram(reader pebble.Reader, batch *pebble.Batch, wsPrefix [8]byte, id [16]byte) error {
	fwdPrefix := keys.AssocFwdPrefixForID(wsPrefix, id)
	fwdIter, err := PrefixIterator(reader, fwdPrefix)
	if err != nil {
		return fmt.Errorf("创建前向关联迭代器失败: %w", err)
	}
	for ok := fwdIter.First(); ok; ok = fwdIter.Next() {
		k := append([]byte(nil), fwdIter.Key()...)
		if len(k) < 45 {
			continue
		}
		var wc [4]byte
		copy(wc[:], k[25:29])
		w := keys.WeightFromComplement(wc)
		var dst [16]byte
		copy(dst[:], k[29:45])
		BatchDelete(batch, k)
		BatchDelete(batch, keys.AssocRevKey(wsPrefix, dst, w, id))
		BatchDelete(batch, keys.AssocWeightIndexKey(wsPrefix, id, dst))
	}
	if err := fwdIter.Close(); err != nil {
		return fmt.Errorf("关闭前向关联迭代器失败: %w", err)
	}

	revPrefix := keys.AssocRevPrefixForID(wsPrefix, id)
	revIter, err := PrefixIterator(reader, revPrefix)
	if err != nil {
		return fmt.Errorf("创建反向关联迭代器失败: %w", err)
	}
	for ok := revIter.First(); ok; ok = revIter.Next() {
		k := append([]byte(nil), revIter.Key()...)
		if len(k) < 45 {
			continue
		}
		var wc [4]byte
		copy(wc[:], k[25:29])
		w := keys.WeightFromComplement(wc)
		var src [16]byte
		copy(src[:], k[29:45])
		BatchDelete(batch, k)
		BatchDelete(batch, keys.AssocFwdKey(wsPrefix, src, w, id))
		BatchDelete(batch, keys.AssocWeightIndexKey(wsPrefix, src, id))
	}
	if err := revIter.Close(); err != nil {
		return fmt.Errorf("关闭反向关联迭代器失败: %w", err)
	}

	return nil
}

func deleteOrdinalKeysForEngram(reader pebble.Reader, batch *pebble.Batch, wsPrefix [8]byte, id [16]byte) error {
	ordIter, err := PrefixIterator(reader, keys.OrdinalWorkspacePrefix(wsPrefix))
	if err != nil {
		return fmt.Errorf("创建序号全量迭代器失败: %w", err)
	}
	for ok := ordIter.First(); ok; ok = ordIter.Next() {
		k := ordIter.Key()
		if len(k) != 41 {
			continue
		}
		if bytes.Equal(k[25:41], id[:]) {
			BatchDelete(batch, append([]byte(nil), k...))
		}
	}
	if err := ordIter.Close(); err != nil {
		return fmt.Errorf("关闭序号全量迭代器失败: %w", err)
	}

	parentIter, err := PrefixIterator(reader, keys.OrdinalPrefixForParent(wsPrefix, id))
	if err != nil {
		return fmt.Errorf("创建父序号迭代器失败: %w", err)
	}
	for ok := parentIter.First(); ok; ok = parentIter.Next() {
		BatchDelete(batch, append([]byte(nil), parentIter.Key()...))
	}
	if err := parentIter.Close(); err != nil {
		return fmt.Errorf("关闭父序号迭代器失败: %w", err)
	}

	return nil
}

func deleteEntityLinksForEngram(reader pebble.Reader, batch *pebble.Batch, wsPrefix [8]byte, id [16]byte) ([]string, error) {
	names := make([]string, 0, 8)

	linkIter, err := PrefixIterator(reader, keys.EntityEngramLinkPrefix(wsPrefix, id))
	if err != nil {
		return nil, fmt.Errorf("创建实体链接迭代器失败: %w", err)
	}
	for ok := linkIter.First(); ok; ok = linkIter.Next() {
		k := append([]byte(nil), linkIter.Key()...)
		v := append([]byte(nil), linkIter.Value()...)
		if len(k) != 33 {
			continue
		}
		var nameHash [8]byte
		copy(nameHash[:], k[25:33])
		BatchDelete(batch, k)
		BatchDelete(batch, keys.EntityReverseIndexKey(nameHash, wsPrefix, id))
		if len(v) > 0 {
			names = append(names, string(v))
		}
	}
	if err := linkIter.Close(); err != nil {
		return nil, fmt.Errorf("关闭实体链接迭代器失败: %w", err)
	}

	relIter, err := PrefixIterator(reader, keys.RelationshipEngramPrefix(wsPrefix, id))
	if err != nil {
		return names, fmt.Errorf("创建关系迭代器失败: %w", err)
	}
	for ok := relIter.First(); ok; ok = relIter.Next() {
		k := append([]byte(nil), relIter.Key()...)
		BatchDelete(batch, k)

		var rec RelationshipRecord
		if err := msgpack.Unmarshal(relIter.Value(), &rec); err == nil {
			fromHash := keys.EntityNameHash(rec.FromEntity)
			toHash := keys.EntityNameHash(rec.ToEntity)
			BatchDelete(batch, keys.RelEntityIndexKey(wsPrefix, fromHash, id))
			BatchDelete(batch, keys.RelEntityIndexKey(wsPrefix, toHash, id))
		}
	}
	if err := relIter.Close(); err != nil {
		return names, fmt.Errorf("关闭关系迭代器失败: %w", err)
	}

	return names, nil
}
