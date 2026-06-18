package engine

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ────────────────────────────────────────────────────────────────────
// mockStore — 内存实现的 Store 接口，用于单元测试
// ────────────────────────────────────────────────────────────────────

// mockStore 是一个纯内存的 Store 实现，所有数据保存在 map/slice 中。
// 线程安全（通过 sync.RWMutex 保护），满足并发测试需求。
type mockStore struct {
	mu sync.RWMutex

	// engrams 按 (vaultPrefix, ULID) 存储 engram。
	engrams map[[8]byte]map[ULID]*Engram

	// associations 按 (vaultPrefix, sourceULID) 存储关联列表。
	associations map[[8]byte]map[ULID][]Association

	// entityRecords 按规范名称存储全局实体记录。
	entityRecords map[string]*EntityRecord

	// entityEngrams 按实体名称存储 (ws, engramID) 反向索引。
	entityEngrams map[string][]entityEngramLink

	// engramEntities 按 (ws, engramID) 存储实体名称前向索引。
	engramEntities map[[8]byte]map[ULID][]string

	// vaultEntityNames 按 vault 存储去重的实体名称集合。
	vaultEntityNames map[[8]byte]map[string]struct{}

	// relationships 按 vault 存储关系记录列表。
	relationships map[[8]byte][]RelationshipRecord

	// entityRelationships 按 (vault, entityName) 存储关系记录。
	entityRelationships map[[8]byte]map[string][]RelationshipRecord

	// entityClusters 按 vault 存储共现记录。
	entityClusters map[[8]byte][]clusterEntry
}

// entityEngramLink 表示 (vault前缀, engramID) 对。
type entityEngramLink struct {
	ws [8]byte
	id ULID
}

// clusterEntry 表示共现记录。
type clusterEntry struct {
	nameA, nameB string
	count        int
}

// newMockStore 创建一个空的 mockStore。
func newMockStore() *mockStore {
	return &mockStore{
		engrams:             make(map[[8]byte]map[ULID]*Engram),
		associations:        make(map[[8]byte]map[ULID][]Association),
		entityRecords:       make(map[string]*EntityRecord),
		entityEngrams:       make(map[string][]entityEngramLink),
		engramEntities:      make(map[[8]byte]map[ULID][]string),
		vaultEntityNames:    make(map[[8]byte]map[string]struct{}),
		relationships:       make(map[[8]byte][]RelationshipRecord),
		entityRelationships: make(map[[8]byte]map[string][]RelationshipRecord),
		entityClusters:      make(map[[8]byte][]clusterEntry),
	}
}

// ── 辅助写入方法（测试用，非 Store 接口） ──

// addEngram 向指定 vault 添加一个 engram。
func (m *mockStore) addEngram(ws [8]byte, eng *Engram) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.engrams[ws] == nil {
		m.engrams[ws] = make(map[ULID]*Engram)
	}
	m.engrams[ws][eng.ID] = eng
}

// addAssociation 向指定 vault 的源 engram 添加关联。
func (m *mockStore) addAssociation(ws [8]byte, sourceID ULID, assoc Association) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.associations[ws] == nil {
		m.associations[ws] = make(map[ULID][]Association)
	}
	m.associations[ws][sourceID] = append(m.associations[ws][sourceID], assoc)
}

// addEntityRecord 注册一个全局实体记录。
func (m *mockStore) addEntityRecord(rec EntityRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := rec
	m.entityRecords[rec.Name] = &cp
}

// addEntityEngramLink 添加实体-engram 反向索引条目。
func (m *mockStore) addEntityEngramLink(entityName string, ws [8]byte, id ULID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entityEngrams[entityName] = append(m.entityEngrams[entityName], entityEngramLink{ws: ws, id: id})

	// 同时维护前向索引和 vault 实体名称集合。
	if m.engramEntities[ws] == nil {
		m.engramEntities[ws] = make(map[ULID][]string)
	}
	m.engramEntities[ws][id] = append(m.engramEntities[ws][id], entityName)

	if m.vaultEntityNames[ws] == nil {
		m.vaultEntityNames[ws] = make(map[string]struct{})
	}
	m.vaultEntityNames[ws][entityName] = struct{}{}
}

// addRelationship 向指定 vault 添加关系记录。
func (m *mockStore) addRelationship(ws [8]byte, rec RelationshipRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.relationships[ws] = append(m.relationships[ws], rec)

	// 同时维护 entityRelationships 索引。
	if m.entityRelationships[ws] == nil {
		m.entityRelationships[ws] = make(map[string][]RelationshipRecord)
	}
	m.entityRelationships[ws][rec.FromEntity] = append(m.entityRelationships[ws][rec.FromEntity], rec)
	if rec.FromEntity != rec.ToEntity {
		m.entityRelationships[ws][rec.ToEntity] = append(m.entityRelationships[ws][rec.ToEntity], rec)
	}
}

// addEntityCluster 向指定 vault 添加共现记录。
func (m *mockStore) addEntityCluster(ws [8]byte, nameA, nameB string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entityClusters[ws] = append(m.entityClusters[ws], clusterEntry{nameA: nameA, nameB: nameB, count: count})
}

// ── Store 接口实现 ──

// ResolveVaultPrefix 使用 SHA-256 前 8 字节作为确定性 vault 前缀。
func (m *mockStore) ResolveVaultPrefix(name string) [8]byte {
	h := sha256.Sum256([]byte(name))
	var ws [8]byte
	copy(ws[:], h[:8])
	return ws
}

// GetEngram 按 ID 读取 engram。
func (m *mockStore) GetEngram(_ context.Context, ws [8]byte, id ULID) (*Engram, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	vaultMap := m.engrams[ws]
	if vaultMap == nil {
		return nil, nil
	}
	eng, ok := vaultMap[id]
	if !ok {
		return nil, nil
	}
	return eng, nil
}

// GetEngrams 批量读取 engram，返回切片与 ids 一一对应。
func (m *mockStore) GetEngrams(_ context.Context, ws [8]byte, ids []ULID) ([]*Engram, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Engram, len(ids))
	vaultMap := m.engrams[ws]
	if vaultMap == nil {
		return result, nil
	}
	for i, id := range ids {
		result[i] = vaultMap[id]
	}
	return result, nil
}

// GetAssociations 返回源节点的前向关联。
func (m *mockStore) GetAssociations(_ context.Context, ws [8]byte, ids []ULID, maxPerNode int) (map[ULID][]Association, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[ULID][]Association, len(ids))
	vaultMap := m.associations[ws]
	if vaultMap == nil {
		return result, nil
	}
	for _, id := range ids {
		assocs := vaultMap[id]
		if maxPerNode > 0 && len(assocs) > maxPerNode {
			assocs = assocs[:maxPerNode]
		}
		result[id] = assocs
	}
	return result, nil
}

// GetEntityRecord 按名称读取全局实体记录。
func (m *mockStore) GetEntityRecord(_ context.Context, name string) (*EntityRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec, ok := m.entityRecords[name]
	if !ok {
		return nil, nil
	}
	cp := *rec
	return &cp, nil
}

// UpsertEntityRecord 插入或更新实体记录。
func (m *mockStore) UpsertEntityRecord(_ context.Context, record EntityRecord, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := record
	if cp.UpdatedAt == 0 {
		cp.UpdatedAt = time.Now().UnixNano()
	}
	m.entityRecords[record.Name] = &cp
	return nil
}

// ScanEntityEngrams 扫描实体名称的反向索引。
func (m *mockStore) ScanEntityEngrams(_ context.Context, entityName string, fn func(ws [8]byte, engramID ULID) error) error {
	m.mu.RLock()
	links := make([]entityEngramLink, len(m.entityEngrams[entityName]))
	copy(links, m.entityEngrams[entityName])
	m.mu.RUnlock()
	for _, link := range links {
		if err := fn(link.ws, link.id); err != nil {
			return err
		}
	}
	return nil
}

// ScanEngramEntities 扫描 engram 的前向实体索引。
func (m *mockStore) ScanEngramEntities(_ context.Context, ws [8]byte, engramID ULID, fn func(entityName string) error) error {
	m.mu.RLock()
	var names []string
	if vaultMap := m.engramEntities[ws]; vaultMap != nil {
		names = make([]string, len(vaultMap[engramID]))
		copy(names, vaultMap[engramID])
	}
	m.mu.RUnlock()
	for _, name := range names {
		if err := fn(name); err != nil {
			return err
		}
	}
	return nil
}

// ScanVaultEntityNames 扫描 vault 中所有唯一实体名称。
func (m *mockStore) ScanVaultEntityNames(_ context.Context, ws [8]byte, fn func(name string) error) error {
	m.mu.RLock()
	nameSet := m.vaultEntityNames[ws]
	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	m.mu.RUnlock()
	for _, name := range names {
		if err := fn(name); err != nil {
			return err
		}
	}
	return nil
}

// ScanRelationships 全量扫描 vault 中所有关系记录。
func (m *mockStore) ScanRelationships(_ context.Context, ws [8]byte, fn func(record RelationshipRecord) error) error {
	m.mu.RLock()
	rels := make([]RelationshipRecord, len(m.relationships[ws]))
	copy(rels, m.relationships[ws])
	m.mu.RUnlock()
	for _, rel := range rels {
		if err := fn(rel); err != nil {
			return err
		}
	}
	return nil
}

// ScanEntityRelationships 扫描指定实体参与的所有关系记录。
func (m *mockStore) ScanEntityRelationships(_ context.Context, ws [8]byte, entityName string, fn func(record RelationshipRecord) error) error {
	m.mu.RLock()
	var rels []RelationshipRecord
	if vaultMap := m.entityRelationships[ws]; vaultMap != nil {
		rels = make([]RelationshipRecord, len(vaultMap[entityName]))
		copy(rels, vaultMap[entityName])
	}
	m.mu.RUnlock()
	for _, rel := range rels {
		if err := fn(rel); err != nil {
			return err
		}
	}
	return nil
}

// ScanEntityClusters 扫描 vault 的共现索引。
func (m *mockStore) ScanEntityClusters(_ context.Context, ws [8]byte, minCount int, fn func(nameA, nameB string, count int) error) error {
	m.mu.RLock()
	entries := make([]clusterEntry, len(m.entityClusters[ws]))
	copy(entries, m.entityClusters[ws])
	m.mu.RUnlock()
	for _, entry := range entries {
		if entry.count >= minCount {
			if err := fn(entry.nameA, entry.nameB, entry.count); err != nil {
				return err
			}
		}
	}
	return nil
}

// ── 测试辅助函数 ──

// makeEngram 创建一个带有指定 ID 和概念标签的测试 engram。
func makeEngram(id ULID, concept string) *Engram {
	return &Engram{
		ID:        id,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		State:     StateActive,
		Concept:   concept,
		Content:   fmt.Sprintf("测试内容: %s", concept),
	}
}

// makeEngramWithState 创建一个带有指定状态的测试 engram。
func makeEngramWithState(id ULID, concept string, state LifecycleState) *Engram {
	eng := makeEngram(id, concept)
	eng.State = state
	return eng
}

// makeEngramWithTime 创建一个带有指定创建时间的测试 engram。
func makeEngramWithTime(id ULID, concept string, createdAt time.Time) *Engram {
	eng := makeEngram(id, concept)
	eng.CreatedAt = createdAt
	return eng
}

// makeEngramFull 创建一个完整字段的测试 engram。
func makeEngramFull(id ULID, concept, summary string, state LifecycleState, createdAt time.Time) *Engram {
	return &Engram{
		ID:        id,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
		State:     state,
		Concept:   concept,
		Content:   fmt.Sprintf("测试内容: %s", concept),
		Summary:   summary,
	}
}

// testVault 是测试中使用的默认 vault 名称。
const testVault = "test-vault"

// setupTestEngine 创建一个带有 mockStore 的测试引擎。
func setupTestEngine() (*Engine, *mockStore) {
	store := newMockStore()
	eng := New(store)
	return eng, store
}

// multiIDGen 为测试生成一组递增的 ULID（基于递增时间戳）。
func multiIDGen(count int) []ULID {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ids := make([]ULID, count)
	for i := range count {
		ids[i] = NewULIDWithTime(base.Add(time.Duration(i) * time.Minute))
	}
	return ids
}

// mustContain 断言字符串包含子串（用于简化 XML/JSON 内容验证）。
func mustContain(s, substr string) bool {
	return strings.Contains(s, substr)
}
