package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/codedb/engine/keys"
)

func encodeAssocValue(relType RelType, confidence float32, createdAt time.Time, lastActivated int32, peakWeight float32, coActivationCount uint32) [26]byte {
	var val [26]byte
	binary.BigEndian.PutUint16(val[0:2], uint16(relType))
	binary.BigEndian.PutUint32(val[2:6], math.Float32bits(confidence))
	var nanos int64
	if !createdAt.IsZero() {
		nanos = createdAt.UnixNano()
	}
	binary.BigEndian.PutUint64(val[6:14], uint64(nanos))
	binary.BigEndian.PutUint32(val[14:18], uint32(lastActivated))
	binary.BigEndian.PutUint32(val[18:22], math.Float32bits(peakWeight))
	binary.BigEndian.PutUint32(val[22:26], coActivationCount)
	return val
}

func decodeAssocValue(val []byte) (relType RelType, confidence float32, createdAt time.Time, lastActivated int32, peakWeight float32, coActivationCount uint32, restoredAt int32) {
	if len(val) < 18 {
		return 0, 1.0, time.Time{}, 0, 0, 0, 0
	}
	if len(val) == 18 {
		allZero := true
		for _, b := range val {
			if b != 0 {
				allZero = false
				break
			}
		}
		if allZero {
			return 0, 1.0, time.Time{}, 0, 0, 0, 0
		}
	}
	relType = RelType(binary.BigEndian.Uint16(val[0:2]))
	confidence = math.Float32frombits(binary.BigEndian.Uint32(val[2:6]))
	nanos := int64(binary.BigEndian.Uint64(val[6:14]))
	if nanos != 0 {
		createdAt = time.Unix(0, nanos)
	}
	lastActivated = int32(binary.BigEndian.Uint32(val[14:18]))
	if len(val) >= 22 {
		peakWeight = math.Float32frombits(binary.BigEndian.Uint32(val[18:22]))
	}
	if len(val) >= 26 {
		coActivationCount = binary.BigEndian.Uint32(val[22:26])
	}
	if len(val) >= 30 {
		restoredAt = int32(binary.BigEndian.Uint32(val[26:30]))
	}
	return
}

func parseAssocFwdKey(k []byte) (src ULID, dst ULID, weight float32, ok bool) {
	if len(k) < 45 || k[0] != 0x03 {
		return ULID{}, ULID{}, 0, false
	}
	copy(src[:], k[9:25])
	copy(dst[:], k[29:45])
	var wc [4]byte
	copy(wc[:], k[25:29])
	weight = keys.WeightFromComplement(wc)
	return src, dst, weight, true
}

func assocCacheKey(wsPrefix [8]byte, id ULID) [24]byte {
	var k [24]byte
	copy(k[:8], wsPrefix[:])
	copy(k[8:], id[:])
	return k
}

func (ps *PebbleStore) WriteAssociation(ctx context.Context, wsPrefix [8]byte, src, dst ULID, assoc *Association) error {
	batch := ps.db.NewBatch()
	defer batch.Close()

	const seedCount uint32 = 1
	v := encodeAssocValue(assoc.RelType, assoc.Confidence, assoc.CreatedAt, assoc.LastActivated, assoc.Weight, seedCount)
	BatchSet(batch, keys.AssocFwdKey(wsPrefix, [16]byte(src), assoc.Weight, [16]byte(dst)), v[:])
	BatchSet(batch, keys.AssocRevKey(wsPrefix, [16]byte(dst), assoc.Weight, [16]byte(src)), v[:])

	var weightBuf [4]byte
	binary.BigEndian.PutUint32(weightBuf[:], math.Float32bits(assoc.Weight))
	BatchSet(batch, keys.AssocWeightIndexKey(wsPrefix, [16]byte(src), [16]byte(dst)), weightBuf[:])

	if err := batch.Commit(pebble.NoSync); err != nil {
		return fmt.Errorf("提交关联写入批次失败: %w", err)
	}

	ps.assocCache.Remove(assocCacheKey(wsPrefix, src))
	_ = ctx
	return nil
}

func (ps *PebbleStore) GetAssociations(ctx context.Context, wsPrefix [8]byte, ids []ULID, maxPerNode int) (map[ULID][]Association, error) {
	result := make(map[ULID][]Association, len(ids))

	var uncached []ULID
	for _, id := range ids {
		ck := assocCacheKey(wsPrefix, id)
		if entry, ok := ps.assocCache.Get(ck); ok {
			n := len(entry.assocs)
			if maxPerNode > 0 && n > maxPerNode {
				n = maxPerNode
			}
			result[id] = append([]Association(nil), entry.assocs[:n]...)
			continue
		}
		uncached = append(uncached, id)
	}
	if len(uncached) == 0 {
		return result, nil
	}

	sort.Slice(uncached, func(i, j int) bool {
		return bytes.Compare(uncached[i][:], uncached[j][:]) < 0
	})

	reader := ps.pebbleReader(ctx)
	lower := keys.AssocFwdRangeStart(wsPrefix)
	upper := keys.AssocFwdRangeEnd(wsPrefix)
	iter, err := reader.NewIter(&pebble.IterOptions{LowerBound: lower, UpperBound: upper})
	if err != nil {
		return nil, fmt.Errorf("创建关联迭代器失败: %w", err)
	}
	defer iter.Close()

	for _, id := range uncached {
		prefix := keys.AssocFwdPrefixForID(wsPrefix, [16]byte(id))
		assocs := make([]Association, 0, 8)

		for ok := iter.SeekGE(prefix); ok; ok = iter.Next() {
			k := iter.Key()
			if len(k) < 25 || !bytes.Equal(k[:25], prefix) {
				break
			}
			if maxPerNode > 0 && len(assocs) >= maxPerNode {
				break
			}
			_, dst, w, parsed := parseAssocFwdKey(k)
			if !parsed {
				continue
			}
			relType, conf, createdAt, lastAct, peak, coAct, restoredAt := decodeAssocValue(iter.Value())
			assocs = append(assocs, Association{
				TargetID:          dst,
				RelType:           relType,
				Weight:            w,
				Confidence:        conf,
				CreatedAt:         createdAt,
				LastActivated:     lastAct,
				PeakWeight:        peak,
				CoActivationCount: coAct,
				RestoredAt:        restoredAt,
			})
		}

		result[id] = assocs
		ps.assocCache.Add(assocCacheKey(wsPrefix, id), &assocCacheEntry{assocs: assocs})
	}

	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("扫描关联失败: %w", err)
	}
	return result, nil
}

func (ps *PebbleStore) GetAssocWeight(ctx context.Context, wsPrefix [8]byte, a, b ULID) (float32, error) {
	reader := ps.pebbleReader(ctx)
	val, err := Get(reader, keys.AssocWeightIndexKey(wsPrefix, [16]byte(a), [16]byte(b)))
	if err != nil || len(val) < 4 {
		return 0, nil
	}
	return math.Float32frombits(binary.BigEndian.Uint32(val[:4])), nil
}

func (ps *PebbleStore) getAssocValue(ctx context.Context, wsPrefix [8]byte, a, b ULID, knownWeight float32) (RelType, float32, time.Time, int32, float32, uint32, int32) {
	if knownWeight <= 0 {
		return 0, 1.0, time.Time{}, 0, 0, 0, 0
	}
	reader := ps.pebbleReader(ctx)
	val, err := Get(reader, keys.AssocFwdKey(wsPrefix, [16]byte(a), knownWeight, [16]byte(b)))
	if err != nil || val == nil {
		return 0, 1.0, time.Time{}, 0, 0, 0, 0
	}
	return decodeAssocValue(val)
}

func (ps *PebbleStore) UpdateAssocWeight(ctx context.Context, wsPrefix [8]byte, a, b ULID, weight float32, countDelta uint32) error {
	batch := ps.db.NewBatch()
	defer batch.Close()

	oldWeight, _ := ps.GetAssocWeight(ctx, wsPrefix, a, b)
	relType, conf, createdAt, _, peak, coAct, _ := ps.getAssocValue(ctx, wsPrefix, a, b, oldWeight)

	if oldWeight > 0 {
		BatchDelete(batch, keys.AssocFwdKey(wsPrefix, [16]byte(a), oldWeight, [16]byte(b)))
		BatchDelete(batch, keys.AssocRevKey(wsPrefix, [16]byte(b), oldWeight, [16]byte(a)))
	}

	if peak == 0 || weight > peak {
		peak = weight
	}
	if countDelta > 0 {
		if coAct+countDelta < coAct {
			coAct = ^uint32(0)
		} else {
			coAct += countDelta
		}
	}

	lastActivated := int32(time.Now().Unix())
	v := encodeAssocValue(relType, conf, createdAt, lastActivated, peak, coAct)
	BatchSet(batch, keys.AssocFwdKey(wsPrefix, [16]byte(a), weight, [16]byte(b)), v[:])
	BatchSet(batch, keys.AssocRevKey(wsPrefix, [16]byte(b), weight, [16]byte(a)), v[:])

	var wi [4]byte
	binary.BigEndian.PutUint32(wi[:], math.Float32bits(weight))
	BatchSet(batch, keys.AssocWeightIndexKey(wsPrefix, [16]byte(a), [16]byte(b)), wi[:])

	if err := batch.Commit(pebble.NoSync); err != nil {
		return fmt.Errorf("提交关联权重更新失败: %w", err)
	}
	ps.assocCache.Remove(assocCacheKey(wsPrefix, a))
	return nil
}

func (ps *PebbleStore) UpdateAssocWeightBatch(ctx context.Context, updates []AssocWeightUpdate) error {
	batch := ps.db.NewBatch()
	defer batch.Close()

	now := int32(time.Now().Unix())
	for _, u := range updates {
		oldWeight, _ := ps.GetAssocWeight(ctx, u.WS, u.Src, u.Dst)
		relType, conf, createdAt, _, peak, coAct, _ := ps.getAssocValue(ctx, u.WS, u.Src, u.Dst, oldWeight)

		if oldWeight > 0 {
			BatchDelete(batch, keys.AssocFwdKey(u.WS, [16]byte(u.Src), oldWeight, [16]byte(u.Dst)))
			BatchDelete(batch, keys.AssocRevKey(u.WS, [16]byte(u.Dst), oldWeight, [16]byte(u.Src)))
		}

		if peak == 0 || u.Weight > peak {
			peak = u.Weight
		}
		if u.CountDelta > 0 {
			if coAct+u.CountDelta < coAct {
				coAct = ^uint32(0)
			} else {
				coAct += u.CountDelta
			}
		}

		v := encodeAssocValue(relType, conf, createdAt, now, peak, coAct)
		BatchSet(batch, keys.AssocFwdKey(u.WS, [16]byte(u.Src), u.Weight, [16]byte(u.Dst)), v[:])
		BatchSet(batch, keys.AssocRevKey(u.WS, [16]byte(u.Dst), u.Weight, [16]byte(u.Src)), v[:])

		var wi [4]byte
		binary.BigEndian.PutUint32(wi[:], math.Float32bits(u.Weight))
		BatchSet(batch, keys.AssocWeightIndexKey(u.WS, [16]byte(u.Src), [16]byte(u.Dst)), wi[:])
	}

	if err := batch.Commit(pebble.NoSync); err != nil {
		return fmt.Errorf("提交批量关联权重更新失败: %w", err)
	}

	seen := make(map[[24]byte]struct{}, len(updates))
	for _, u := range updates {
		ck := assocCacheKey(u.WS, u.Src)
		if _, ok := seen[ck]; ok {
			continue
		}
		seen[ck] = struct{}{}
		ps.assocCache.Remove(ck)
	}
	return nil
}

const assocDecayChunkSize = 10_000

func (ps *PebbleStore) DecayAssocWeights(ctx context.Context, wsPrefix [8]byte, decayFactor float64, minWeight float32, archiveThreshold float64) (int, error) {
	_ = archiveThreshold

	type op struct {
		src, dst          ULID
		oldWeight         float32
		newWeight         float32
		remove            bool
		relType           RelType
		confidence        float32
		createdAt         time.Time
		lastActivated     int32
		peakWeight        float32
		coActivationCount uint32
	}

	reader := ps.pebbleReader(ctx)
	iter, err := PrefixIterator(reader, keys.AssocFwdRangeStart(wsPrefix))
	if err != nil {
		return 0, fmt.Errorf("创建衰减扫描迭代器失败: %w", err)
	}
	defer iter.Close()

	removed := 0
	chunk := make([]op, 0, assocDecayChunkSize)

	flush := func() error {
		if len(chunk) == 0 {
			return nil
		}
		batch := ps.db.NewBatch()
		defer batch.Close()

		for _, e := range chunk {
			BatchDelete(batch, keys.AssocFwdKey(wsPrefix, [16]byte(e.src), e.oldWeight, [16]byte(e.dst)))
			BatchDelete(batch, keys.AssocRevKey(wsPrefix, [16]byte(e.dst), e.oldWeight, [16]byte(e.src)))

			if e.remove {
				BatchDelete(batch, keys.AssocWeightIndexKey(wsPrefix, [16]byte(e.src), [16]byte(e.dst)))
				continue
			}

			peak := e.peakWeight
			if peak == 0 {
				peak = e.oldWeight
			}
			v := encodeAssocValue(e.relType, e.confidence, e.createdAt, e.lastActivated, peak, e.coActivationCount)
			BatchSet(batch, keys.AssocFwdKey(wsPrefix, [16]byte(e.src), e.newWeight, [16]byte(e.dst)), v[:])
			BatchSet(batch, keys.AssocRevKey(wsPrefix, [16]byte(e.dst), e.newWeight, [16]byte(e.src)), v[:])

			var wi [4]byte
			binary.BigEndian.PutUint32(wi[:], math.Float32bits(e.newWeight))
			BatchSet(batch, keys.AssocWeightIndexKey(wsPrefix, [16]byte(e.src), [16]byte(e.dst)), wi[:])
		}

		if err := batch.Commit(pebble.NoSync); err != nil {
			return fmt.Errorf("提交关联衰减批次失败: %w", err)
		}

		for _, e := range chunk {
			ps.assocCache.Remove(assocCacheKey(wsPrefix, e.src))
		}
		chunk = chunk[:0]
		return nil
	}

	for ok := iter.First(); ok; ok = iter.Next() {
		k := iter.Key()
		src, dst, oldWeight, parsed := parseAssocFwdKey(k)
		if !parsed {
			continue
		}

		relType, conf, createdAt, lastAct, peak, coAct, _ := decodeAssocValue(iter.Value())
		newWeight := float32(float64(oldWeight) * decayFactor)
		entry := op{
			src: src, dst: dst,
			oldWeight: oldWeight, newWeight: newWeight,
			relType: relType, confidence: conf, createdAt: createdAt,
			lastActivated: lastAct, peakWeight: peak, coActivationCount: coAct,
		}
		if newWeight < minWeight {
			entry.remove = true
			removed++
		}
		chunk = append(chunk, entry)
		if len(chunk) >= assocDecayChunkSize {
			if err := flush(); err != nil {
				return removed, err
			}
		}
	}

	if err := iter.Error(); err != nil {
		return removed, fmt.Errorf("扫描关联衰减数据失败: %w", err)
	}
	if err := flush(); err != nil {
		return removed, err
	}
	return removed, nil
}

func (ps *PebbleStore) GetConceptAssociations(ctx context.Context, wsPrefix [8]byte, id ULID, maxN int) ([]ULID, error) {
	prefix := keys.AssocFwdPrefixForID(wsPrefix, [16]byte(id))
	iter, err := PrefixIterator(ps.pebbleReader(ctx), prefix)
	if err != nil {
		return nil, fmt.Errorf("创建概念关联迭代器失败: %w", err)
	}
	defer iter.Close()

	neighbors := make([]ULID, 0, maxN)
	for ok := iter.First(); ok; ok = iter.Next() {
		if maxN > 0 && len(neighbors) >= maxN {
			break
		}
		_, dst, _, parsed := parseAssocFwdKey(iter.Key())
		if !parsed {
			continue
		}
		neighbors = append(neighbors, dst)
	}
	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("扫描概念关联失败: %w", err)
	}
	return neighbors, nil
}

func (ps *PebbleStore) GetChildrenByParent(ctx context.Context, wsPrefix [8]byte, parentID ULID) ([]ULID, error) {
	prefix := keys.AssocRevPrefixForID(wsPrefix, [16]byte(parentID))
	iter, err := PrefixIterator(ps.pebbleReader(ctx), prefix)
	if err != nil {
		return nil, fmt.Errorf("创建父子反向迭代器失败: %w", err)
	}
	defer iter.Close()

	children := make([]ULID, 0, 8)
	for ok := iter.First(); ok; ok = iter.Next() {
		k := iter.Key()
		if len(k) < 45 {
			continue
		}
		relType, _, _, _, _, _, _ := decodeAssocValue(iter.Value())
		if relType != RelIsPartOf {
			continue
		}
		var child ULID
		copy(child[:], k[29:45])
		children = append(children, child)
	}
	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("扫描父子关联失败: %w", err)
	}
	return children, nil
}

func (ps *PebbleStore) FlagContradiction(ctx context.Context, wsPrefix [8]byte, a, b ULID) error {
	batch := ps.db.NewBatch()
	defer batch.Close()

	aBytes := [16]byte(a)
	bBytes := [16]byte(b)
	if CompareULIDs(a, b) > 0 {
		aBytes, bBytes = bBytes, aBytes
	}

	BatchSet(batch, keys.ContradictionKey(wsPrefix, 0, 0, aBytes), bBytes[:])
	BatchSet(batch, keys.ContradictionKey(wsPrefix, 0, 0, bBytes), aBytes[:])

	if err := batch.Commit(pebble.NoSync); err != nil {
		return fmt.Errorf("提交矛盾标记失败: %w", err)
	}
	_ = ctx
	return nil
}

func (ps *PebbleStore) GetContradictions(ctx context.Context, wsPrefix [8]byte) ([][2]ULID, error) {
	iter, err := PrefixIterator(ps.pebbleReader(ctx), keys.ContradictionKeyPrefix(wsPrefix))
	if err != nil {
		return nil, fmt.Errorf("创建矛盾扫描迭代器失败: %w", err)
	}
	defer iter.Close()

	const keyLen = 1 + 8 + 4 + 2 + 16
	const idOffset = 1 + 8 + 4 + 2

	seen := make(map[[32]byte]struct{})
	pairs := make([][2]ULID, 0, 16)

	for ok := iter.First(); ok; ok = iter.Next() {
		k := iter.Key()
		v := iter.Value()
		if len(k) < keyLen || len(v) < 16 {
			continue
		}

		var a ULID
		var b ULID
		copy(a[:], k[idOffset:idOffset+16])
		copy(b[:], v[:16])

		if CompareULIDs(a, b) > 0 {
			a, b = b, a
		}
		var dk [32]byte
		copy(dk[:16], a[:])
		copy(dk[16:], b[:])
		if _, ok := seen[dk]; ok {
			continue
		}
		seen[dk] = struct{}{}
		pairs = append(pairs, [2]ULID{a, b})
	}

	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("扫描矛盾数据失败: %w", err)
	}
	return pairs, nil
}

func (ps *PebbleStore) ResolveContradiction(ctx context.Context, wsPrefix [8]byte, a, b ULID) error {
	batch := ps.db.NewBatch()
	defer batch.Close()

	BatchDelete(batch, keys.ContradictionKey(wsPrefix, 0, 0, [16]byte(a)))
	BatchDelete(batch, keys.ContradictionKey(wsPrefix, 0, 0, [16]byte(b)))

	if err := batch.Commit(pebble.NoSync); err != nil {
		return fmt.Errorf("提交矛盾解除失败: %w", err)
	}
	_ = ctx
	return nil
}
