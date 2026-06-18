package engine

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/codedb/engine/keys"
	"github.com/vmihailenco/msgpack/v5"
	"golang.org/x/text/unicode/norm"
)

var validEntityStates = map[string]bool{
	"active": true, "deprecated": true, "merged": true, "resolved": true,
}

var relTypeBytes = map[string]uint8{
	"manages": 0x01, "uses": 0x02, "depends_on": 0x03,
	"implements": 0x04, "created_by": 0x05, "part_of": 0x06,
	"causes": 0x07, "contradicts": 0x08, "supports": 0x09,
	"co_occurs_with": 0x0A, "caches_with": 0x0B,
}

type coOccurrenceRecord struct {
	NameA string `msgpack:"a"`
	NameB string `msgpack:"b"`
	Count uint32 `msgpack:"n"`
}

func relTypeByteFromString(relType string) uint8 {
	if b, ok := relTypeBytes[relType]; ok {
		return b
	}
	return 0xFF
}

func canonicalEntityLockKey(name string) []byte {
	return []byte(strings.ToLower(strings.TrimSpace(norm.NFKC.String(name))))
}

func canonicalCoOccurrencePair(nameA, nameB string) (canonA, canonB string, hashA, hashB [8]byte) {
	hashA = keys.EntityNameHash(nameA)
	hashB = keys.EntityNameHash(nameB)
	canonA, canonB = nameA, nameB
	if bytes.Compare(hashA[:], hashB[:]) > 0 {
		hashA, hashB = hashB, hashA
		canonA, canonB = nameB, nameA
	}
	return canonA, canonB, hashA, hashB
}

func (ps *PebbleStore) UpsertEntityRecord(ctx context.Context, record EntityRecord, source string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	mu := ps.entityLocks.For(canonicalEntityLockKey(record.Name))
	mu.Lock()
	defer mu.Unlock()

	existing, err := ps.GetEntityRecord(ctx, record.Name)
	if err != nil {
		return fmt.Errorf("读取实体旧记录失败: %w", err)
	}

	now := time.Now().UnixNano()
	if existing != nil {
		if existing.FirstSeen != 0 {
			record.FirstSeen = existing.FirstSeen
		}
		record.MentionCount = existing.MentionCount + 1
		if record.State == "" {
			record.State = existing.State
		}
		if record.MergedInto == "" && record.State == "merged" {
			record.MergedInto = existing.MergedInto
		}
		if existing.Confidence > record.Confidence {
			record.Confidence = existing.Confidence
		}
	} else {
		record.FirstSeen = now
		record.MentionCount = 1
		if record.State == "" {
			record.State = "active"
		}
	}

	if record.State == "" {
		record.State = "active"
	}
	if !validEntityStates[record.State] {
		return fmt.Errorf("非法实体状态 %q", record.State)
	}
	if record.MergedInto != "" && record.State != "merged" {
		return fmt.Errorf("state=%q 时不能设置 merged_into", record.State)
	}

	record.Source = source
	record.UpdatedAt = now

	val, err := msgpack.Marshal(record)
	if err != nil {
		return fmt.Errorf("编码实体记录失败: %w", err)
	}
	return ps.db.Set(keys.EntityKey(keys.EntityNameHash(record.Name)), val, pebble.NoSync)
}

func (ps *PebbleStore) GetEntityRecord(ctx context.Context, name string) (*EntityRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	val, err := Get(ps.db, keys.EntityKey(keys.EntityNameHash(name)))
	if err != nil {
		return nil, fmt.Errorf("读取实体记录失败: %w", err)
	}
	if val == nil {
		return nil, nil
	}

	var rec EntityRecord
	if err := msgpack.Unmarshal(val, &rec); err != nil {
		return nil, fmt.Errorf("解码实体记录失败: %w", err)
	}
	return &rec, nil
}

func (ps *PebbleStore) WriteEntityEngramLink(ctx context.Context, ws [8]byte, engramID ULID, entityName string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	nameHash := keys.EntityNameHash(entityName)
	fwdKey := keys.EntityEngramLinkKey(ws, [16]byte(engramID), nameHash)
	revKey := keys.EntityReverseIndexKey(nameHash, ws, [16]byte(engramID))

	batch := ps.db.NewBatch()
	defer batch.Close()
	if err := batch.Set(fwdKey, []byte(entityName), nil); err != nil {
		return fmt.Errorf("写入实体前向索引失败: %w", err)
	}
	if err := batch.Set(revKey, nil, nil); err != nil {
		return fmt.Errorf("写入实体反向索引失败: %w", err)
	}
	return batch.Commit(pebble.NoSync)
}

func (ps *PebbleStore) RelinkEntityEngramLink(ctx context.Context, ws [8]byte, engramID ULID, fromEntity, toEntity string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	id := [16]byte(engramID)
	fromHash := keys.EntityNameHash(fromEntity)
	toHash := keys.EntityNameHash(toEntity)

	batch := ps.db.NewBatch()
	defer batch.Close()

	if err := batch.Set(keys.EntityEngramLinkKey(ws, id, toHash), []byte(toEntity), nil); err != nil {
		return fmt.Errorf("重链实体前向索引失败: %w", err)
	}
	if err := batch.Set(keys.EntityReverseIndexKey(toHash, ws, id), nil, nil); err != nil {
		return fmt.Errorf("重链实体反向索引失败: %w", err)
	}
	if err := batch.Delete(keys.EntityEngramLinkKey(ws, id, fromHash), nil); err != nil {
		return fmt.Errorf("删除旧实体前向索引失败: %w", err)
	}
	if err := batch.Delete(keys.EntityReverseIndexKey(fromHash, ws, id), nil); err != nil {
		return fmt.Errorf("删除旧实体反向索引失败: %w", err)
	}
	return batch.Commit(pebble.NoSync)
}

func (ps *PebbleStore) ScanEntityEngrams(ctx context.Context, entityName string, fn func(ws [8]byte, engramID ULID) error) error {
	iter, err := PrefixIterator(ps.db, keys.EntityReverseIndexPrefix(keys.EntityNameHash(entityName)))
	if err != nil {
		return fmt.Errorf("创建实体反向索引迭代器失败: %w", err)
	}
	defer iter.Close()

	for ok := iter.First(); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}

		k := iter.Key()
		if len(k) != 33 {
			continue
		}

		var ws [8]byte
		copy(ws[:], k[9:17])
		var id [16]byte
		copy(id[:], k[17:33])

		if err := fn(ws, ULID(id)); err != nil {
			return err
		}
	}
	if err := iter.Error(); err != nil {
		return fmt.Errorf("扫描实体反向索引失败: %w", err)
	}
	return nil
}

func (ps *PebbleStore) ScanEngramEntities(ctx context.Context, ws [8]byte, engramID ULID, fn func(entityName string) error) error {
	iter, err := PrefixIterator(ps.db, keys.EntityEngramLinkPrefix(ws, [16]byte(engramID)))
	if err != nil {
		return fmt.Errorf("创建 engram 实体索引迭代器失败: %w", err)
	}
	defer iter.Close()

	for ok := iter.First(); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}

		name := string(iter.Value())
		if name == "" {
			continue
		}
		if err := fn(name); err != nil {
			return err
		}
	}
	if err := iter.Error(); err != nil {
		return fmt.Errorf("扫描 engram 实体索引失败: %w", err)
	}
	return nil
}

func (ps *PebbleStore) ScanVaultEntityNames(ctx context.Context, ws [8]byte, fn func(name string) error) error {
	prefix := make([]byte, 1+8)
	prefix[0] = 0x20
	copy(prefix[1:], ws[:])

	iter, err := PrefixIterator(ps.db, prefix)
	if err != nil {
		return fmt.Errorf("创建 vault 实体名称迭代器失败: %w", err)
	}
	defer iter.Close()

	seen := make(map[string]struct{})
	for ok := iter.First(); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}

		name := string(iter.Value())
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		if err := fn(name); err != nil {
			return err
		}
	}
	if err := iter.Error(); err != nil {
		return fmt.Errorf("扫描 vault 实体名称失败: %w", err)
	}
	return nil
}

func (ps *PebbleStore) UpsertRelationshipRecord(ctx context.Context, ws [8]byte, engramID ULID, record RelationshipRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	record.UpdatedAt = time.Now().UnixNano()
	val, err := msgpack.Marshal(record)
	if err != nil {
		return fmt.Errorf("编码关系记录失败: %w", err)
	}

	id := [16]byte(engramID)
	fromHash := keys.EntityNameHash(record.FromEntity)
	toHash := keys.EntityNameHash(record.ToEntity)
	relKey := keys.RelationshipKey(ws, id, fromHash, relTypeByteFromString(record.RelType), toHash)
	idxFromKey := keys.RelEntityIndexKey(ws, fromHash, id)
	idxToKey := keys.RelEntityIndexKey(ws, toHash, id)

	batch := ps.db.NewBatch()
	defer batch.Close()
	if err := batch.Set(relKey, val, nil); err != nil {
		return fmt.Errorf("写入关系主记录失败: %w", err)
	}
	if err := batch.Set(idxFromKey, nil, nil); err != nil {
		return fmt.Errorf("写入关系 from 索引失败: %w", err)
	}
	if err := batch.Set(idxToKey, nil, nil); err != nil {
		return fmt.Errorf("写入关系 to 索引失败: %w", err)
	}
	return batch.Commit(pebble.NoSync)
}

func (ps *PebbleStore) ScanEngramRelationships(ctx context.Context, ws [8]byte, engramID ULID, fn func(record RelationshipRecord) error) error {
	iter, err := PrefixIterator(ps.db, keys.RelationshipEngramPrefix(ws, [16]byte(engramID)))
	if err != nil {
		return fmt.Errorf("创建 engram 关系迭代器失败: %w", err)
	}
	defer iter.Close()

	for ok := iter.First(); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}

		var rec RelationshipRecord
		if err := msgpack.Unmarshal(iter.Value(), &rec); err != nil {
			continue
		}
		if err := fn(rec); err != nil {
			return err
		}
	}
	if err := iter.Error(); err != nil {
		return fmt.Errorf("扫描 engram 关系失败: %w", err)
	}
	return nil
}

func (ps *PebbleStore) ScanRelationships(ctx context.Context, ws [8]byte, fn func(record RelationshipRecord) error) error {
	iter, err := PrefixIterator(ps.db, keys.RelationshipPrefix(ws))
	if err != nil {
		return fmt.Errorf("创建关系全量迭代器失败: %w", err)
	}
	defer iter.Close()

	for ok := iter.First(); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}

		var rec RelationshipRecord
		if err := msgpack.Unmarshal(iter.Value(), &rec); err != nil {
			continue
		}
		if err := fn(rec); err != nil {
			return err
		}
	}
	if err := iter.Error(); err != nil {
		return fmt.Errorf("扫描关系全量失败: %w", err)
	}
	return nil
}

func (ps *PebbleStore) ScanEntityRelationships(ctx context.Context, ws [8]byte, entityName string, fn func(record RelationshipRecord) error) error {
	entityHash := keys.EntityNameHash(entityName)
	idxIter, err := PrefixIterator(ps.db, keys.RelEntityIndexPrefix(ws, entityHash))
	if err != nil {
		return fmt.Errorf("创建关系实体索引迭代器失败: %w", err)
	}

	seen := make(map[[16]byte]struct{})
	var engramIDs [][16]byte
	for ok := idxIter.First(); ok; ok = idxIter.Next() {
		if err := ctx.Err(); err != nil {
			_ = idxIter.Close()
			return err
		}

		k := idxIter.Key()
		if len(k) != 33 {
			continue
		}
		var id [16]byte
		copy(id[:], k[17:33])
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		engramIDs = append(engramIDs, id)
	}
	if err := idxIter.Close(); err != nil {
		return fmt.Errorf("关闭关系实体索引迭代器失败: %w", err)
	}

	for _, id := range engramIDs {
		relIter, err := PrefixIterator(ps.db, keys.RelationshipEngramPrefix(ws, id))
		if err != nil {
			return fmt.Errorf("创建 engram 关系迭代器失败: %w", err)
		}
		for ok := relIter.First(); ok; ok = relIter.Next() {
			if err := ctx.Err(); err != nil {
				_ = relIter.Close()
				return err
			}

			var rec RelationshipRecord
			if err := msgpack.Unmarshal(relIter.Value(), &rec); err != nil {
				continue
			}
			if rec.FromEntity != entityName && rec.ToEntity != entityName {
				continue
			}
			if err := fn(rec); err != nil {
				_ = relIter.Close()
				return err
			}
		}
		if err := relIter.Close(); err != nil {
			return fmt.Errorf("关闭 engram 关系迭代器失败: %w", err)
		}
	}

	return nil
}

func (ps *PebbleStore) DeleteEntityEngramLink(ctx context.Context, ws [8]byte, engramID ULID, entityName string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	nameHash := keys.EntityNameHash(entityName)
	fwdKey := keys.EntityEngramLinkKey(ws, [16]byte(engramID), nameHash)
	revKey := keys.EntityReverseIndexKey(nameHash, ws, [16]byte(engramID))

	batch := ps.db.NewBatch()
	defer batch.Close()
	if err := batch.Delete(fwdKey, nil); err != nil {
		return fmt.Errorf("删除实体前向索引失败: %w", err)
	}
	if err := batch.Delete(revKey, nil); err != nil {
		return fmt.Errorf("删除实体反向索引失败: %w", err)
	}
	return batch.Commit(pebble.NoSync)
}

func (ps *PebbleStore) RelinkRelationshipEntity(ctx context.Context, ws [8]byte, oldName, newName string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	oldHash := keys.EntityNameHash(oldName)
	newHash := keys.EntityNameHash(newName)
	if oldHash == newHash {
		return nil
	}

	idxIter, err := PrefixIterator(ps.db, keys.RelEntityIndexPrefix(ws, oldHash))
	if err != nil {
		return fmt.Errorf("创建旧实体关系索引迭代器失败: %w", err)
	}

	seen := make(map[[16]byte]struct{})
	var engramIDs [][16]byte
	for ok := idxIter.First(); ok; ok = idxIter.Next() {
		k := idxIter.Key()
		if len(k) != 33 {
			continue
		}
		var id [16]byte
		copy(id[:], k[17:33])
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		engramIDs = append(engramIDs, id)
	}
	if err := idxIter.Close(); err != nil {
		return fmt.Errorf("关闭旧实体关系索引迭代器失败: %w", err)
	}

	const (
		relKeyLen        = 42
		relFromHashStart = 25
		relToHashStart   = 34
		relTypeBytePos   = 33
	)

	type relUpdate struct {
		oldKey []byte
		newKey []byte
		newVal []byte
	}

	for _, id := range engramIDs {
		if err := ctx.Err(); err != nil {
			return err
		}

		relIter, err := PrefixIterator(ps.db, keys.RelationshipEngramPrefix(ws, id))
		if err != nil {
			return fmt.Errorf("创建关系记录迭代器失败: %w", err)
		}

		var updates []relUpdate
		for ok := relIter.First(); ok; ok = relIter.Next() {
			k := relIter.Key()
			if len(k) != relKeyLen {
				continue
			}

			var rec RelationshipRecord
			if err := msgpack.Unmarshal(relIter.Value(), &rec); err != nil {
				continue
			}

			fromMatch := rec.FromEntity == oldName
			toMatch := rec.ToEntity == oldName
			if !fromMatch && !toMatch {
				continue
			}

			if fromMatch {
				rec.FromEntity = newName
			}
			if toMatch {
				rec.ToEntity = newName
			}
			rec.UpdatedAt = time.Now().UnixNano()

			newVal, err := msgpack.Marshal(rec)
			if err != nil {
				_ = relIter.Close()
				return fmt.Errorf("编码替换后的关系记录失败: %w", err)
			}

			var fromHash, toHash [8]byte
			copy(fromHash[:], k[relFromHashStart:relFromHashStart+8])
			copy(toHash[:], k[relToHashStart:relToHashStart+8])
			if fromMatch {
				fromHash = newHash
			}
			if toMatch {
				toHash = newHash
			}

			oldKey := make([]byte, len(k))
			copy(oldKey, k)
			newKey := keys.RelationshipKey(ws, id, fromHash, k[relTypeBytePos], toHash)
			updates = append(updates, relUpdate{oldKey: oldKey, newKey: newKey, newVal: newVal})
		}
		if err := relIter.Close(); err != nil {
			return fmt.Errorf("关闭关系记录迭代器失败: %w", err)
		}

		if len(updates) == 0 {
			continue
		}

		batch := ps.db.NewBatch()
		for _, u := range updates {
			if err := batch.Delete(u.oldKey, nil); err != nil {
				batch.Close()
				return fmt.Errorf("删除旧关系键失败: %w", err)
			}
			if err := batch.Set(u.newKey, u.newVal, nil); err != nil {
				batch.Close()
				return fmt.Errorf("写入新关系键失败: %w", err)
			}
		}
		if err := batch.Delete(keys.RelEntityIndexKey(ws, oldHash, id), nil); err != nil {
			batch.Close()
			return fmt.Errorf("删除旧关系实体索引失败: %w", err)
		}
		if err := batch.Set(keys.RelEntityIndexKey(ws, newHash, id), nil, nil); err != nil {
			batch.Close()
			return fmt.Errorf("写入新关系实体索引失败: %w", err)
		}
		if err := batch.Commit(pebble.NoSync); err != nil {
			batch.Close()
			return fmt.Errorf("提交关系重链批次失败: %w", err)
		}
		batch.Close()
	}

	return nil
}

func (ps *PebbleStore) IncrementEntityCoOccurrence(ctx context.Context, ws [8]byte, nameA, nameB string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	canonA, canonB, hashA, hashB := canonicalCoOccurrencePair(nameA, nameB)
	lockKey := make([]byte, 16)
	copy(lockKey[:8], hashA[:])
	copy(lockKey[8:], hashB[:])

	mu := ps.coOccurrenceLocks.For(lockKey)
	mu.Lock()
	defer mu.Unlock()

	key := keys.CoOccurrenceKey(ws, hashA, hashB)
	existing, err := Get(ps.db, key)
	if err != nil {
		return fmt.Errorf("读取共现记录失败: %w", err)
	}

	var rec coOccurrenceRecord
	if existing != nil {
		if err := msgpack.Unmarshal(existing, &rec); err != nil {
			return fmt.Errorf("解码共现记录失败: %w", err)
		}
	} else {
		rec.NameA = canonA
		rec.NameB = canonB
	}
	rec.Count++

	val, err := msgpack.Marshal(rec)
	if err != nil {
		return fmt.Errorf("编码共现记录失败: %w", err)
	}
	return ps.db.Set(key, val, pebble.NoSync)
}

func (ps *PebbleStore) ScanEntityClusters(ctx context.Context, ws [8]byte, minCount int, fn func(nameA, nameB string, count int) error) error {
	iter, err := PrefixIterator(ps.db, keys.CoOccurrencePrefix(ws))
	if err != nil {
		return fmt.Errorf("创建共现索引迭代器失败: %w", err)
	}
	defer iter.Close()

	for ok := iter.First(); ok; ok = iter.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}

		var rec coOccurrenceRecord
		if err := msgpack.Unmarshal(iter.Value(), &rec); err != nil {
			continue
		}
		if int(rec.Count) < minCount {
			continue
		}
		if err := fn(rec.NameA, rec.NameB, int(rec.Count)); err != nil {
			return err
		}
	}
	if err := iter.Error(); err != nil {
		return fmt.Errorf("扫描共现索引失败: %w", err)
	}
	return nil
}

func (ps *PebbleStore) DecrementEntityMentionCount(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	mu := ps.entityLocks.For(canonicalEntityLockKey(name))
	mu.Lock()
	defer mu.Unlock()

	rec, err := ps.GetEntityRecord(ctx, name)
	if err != nil {
		return fmt.Errorf("读取实体记录失败: %w", err)
	}
	if rec == nil {
		return nil
	}

	rec.MentionCount--
	if rec.MentionCount < 0 {
		rec.MentionCount = 0
	}

	nameHash := keys.EntityNameHash(name)
	entityKey := keys.EntityKey(nameHash)
	if rec.MentionCount == 0 {
		iter, err := PrefixIterator(ps.db, keys.EntityReverseIndexPrefix(nameHash))
		if err == nil {
			hasRefs := iter.First()
			_ = iter.Close()
			if !hasRefs {
				return ps.db.Delete(entityKey, pebble.NoSync)
			}
		}
	}

	rec.UpdatedAt = time.Now().UnixNano()
	val, err := msgpack.Marshal(rec)
	if err != nil {
		return fmt.Errorf("编码实体记录失败: %w", err)
	}
	return ps.db.Set(entityKey, val, pebble.NoSync)
}

func (ps *PebbleStore) DecrementEntityCoOccurrence(ctx context.Context, ws [8]byte, nameA, nameB string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	_, _, hashA, hashB := canonicalCoOccurrencePair(nameA, nameB)
	lockKey := make([]byte, 16)
	copy(lockKey[:8], hashA[:])
	copy(lockKey[8:], hashB[:])

	mu := ps.coOccurrenceLocks.For(lockKey)
	mu.Lock()
	defer mu.Unlock()

	key := keys.CoOccurrenceKey(ws, hashA, hashB)
	val, err := Get(ps.db, key)
	if err != nil {
		return fmt.Errorf("读取共现记录失败: %w", err)
	}
	if val == nil {
		return nil
	}

	var rec coOccurrenceRecord
	if err := msgpack.Unmarshal(val, &rec); err != nil {
		return fmt.Errorf("解码共现记录失败: %w", err)
	}

	if rec.Count <= 1 {
		return ps.db.Delete(key, pebble.NoSync)
	}
	rec.Count--

	newVal, err := msgpack.Marshal(rec)
	if err != nil {
		return fmt.Errorf("编码共现记录失败: %w", err)
	}
	return ps.db.Set(key, newVal, pebble.NoSync)
}
