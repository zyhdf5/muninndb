package engine

import (
	"context"
	"time"
)

// ────────────────────────────────────────────────────────────────────
// 辅助类型
// ────────────────────────────────────────────────────────────────────

// AssocWeightUpdate 表示单条关联权重批量更新请求。
type AssocWeightUpdate struct {
	WS         [8]byte // vault 工作空间前缀
	Src        ULID    // 源 engram ID
	Dst        ULID    // 目标 engram ID
	Weight     float32 // 新权重值
	CountDelta uint32  // Hebbian 共激活增量（累加到 CoActivationCount）
}

// OrdinalEntry 是 ListChildOrdinals 返回的 (子 ID, 序号) 对。
type OrdinalEntry struct {
	ChildID ULID  // 子 engram ID
	Ordinal int32 // 在父节点下的位置
}

// IdempotencyReceipt 是幂等性收据，映射 opID → engramID。
type IdempotencyReceipt struct {
	OpID      string // 操作 ID
	EngramID  string // 关联的 engram ID
	CreatedAt int64  // 创建时间（Unix 纳秒）
}

// EngramBatchItem 将 vault 工作空间前缀与要写入的 engram 配对。
type EngramBatchItem struct {
	WSPrefix [8]byte // vault 工作空间前缀
	Engram   *Engram // 待写入的 engram
}

// ────────────────────────────────────────────────────────────────────
// StoreBatch — 原子多写操作句柄
// ────────────────────────────────────────────────────────────────────

// StoreBatch 是只写的原子多写操作句柄。
// 调用者必须且仅能调用 Commit 或 Discard 一次。
type StoreBatch interface {
	// WriteEngram 将 engram 写入排入批次。
	WriteEngram(ctx context.Context, wsPrefix [8]byte, eng *Engram) error

	// WriteAssociation 将前向 (0x03) 和反向 (0x04) 关联键排入批次。
	WriteAssociation(ctx context.Context, wsPrefix [8]byte, src, dst ULID, assoc *Association) error

	// WriteOrdinal 将 (parentID, childID) 的序号键排入批次。
	WriteOrdinal(ctx context.Context, wsPrefix [8]byte, parentID, childID ULID, ordinal int32) error

	// UpdateEngramState 将 engram 状态更新排入批次。
	// 从底层存储读取当前 engram，设置新状态后排入 0x01 和 0x02 键的更新。
	UpdateEngramState(ctx context.Context, ws [8]byte, id ULID, newState LifecycleState) error

	// Commit 原子提交所有排队的写操作。
	Commit() error

	// Discard 释放批次而不写入任何内容。
	// 在 Commit 之后调用也安全（幂等）。
	Discard()
}

// ────────────────────────────────────────────────────────────────────
// Store — 图数据库层所需的存储接口（最小方法集）
// ────────────────────────────────────────────────────────────────────

// Store 是图数据库层所需存储操作的抽象接口。
// 生产环境中由 PebbleStore 实现，测试中可使用内存 mock 替代。
// 该接口仅包含图操作所需的最小方法集，与完整的 EngineStore 解耦。
type Store interface {
	// ResolveVaultPrefix 将 vault 名称解析为 8 字节工作空间前缀。
	// 该前缀用于在底层存储中隔离不同 vault 的数据。
	ResolveVaultPrefix(name string) [8]byte

	// GetEngram 按 ID 从指定 vault 读取完整的 engram 记录。
	// 如果 engram 不存在，返回 (nil, nil)。
	GetEngram(ctx context.Context, ws [8]byte, id ULID) (*Engram, error)

	// GetEngrams 批量从指定 vault 读取 engram 记录。
	// 返回的切片与 ids 一一对应，不存在的记录对应位置为 nil。
	GetEngrams(ctx context.Context, ws [8]byte, ids []ULID) ([]*Engram, error)

	// GetAssociations 返回一批源节点的前向关联，按权重降序排列。
	// maxPerNode 限制每个源节点返回的最大关联数。
	GetAssociations(ctx context.Context, ws [8]byte, ids []ULID, maxPerNode int) (map[ULID][]Association, error)

	// GetEntityRecord 按规范名称从全局注册表读取实体记录。
	// 如果实体不存在，返回 (nil, nil)。
	GetEntityRecord(ctx context.Context, name string) (*EntityRecord, error)

	// UpsertEntityRecord 存储或更新全局实体记录。
	// source 标识写入来源（如 "mcp:entity_state"）。
	// 内部实现保证置信度保持合并：如果已有记录的置信度更高，则保留旧值。
	UpsertEntityRecord(ctx context.Context, record EntityRecord, source string) error

	// ScanEntityEngrams 扫描给定实体名称的 0x23 反向索引，
	// 对每个 (ws, engramID) 对调用 fn 回调。
	// fn 返回非 nil 错误时扫描停止。
	ScanEntityEngrams(ctx context.Context, entityName string, fn func(ws [8]byte, engramID ULID) error) error

	// ScanEngramEntities 扫描给定 engram 在指定 vault 中提到的所有实体名称。
	// 使用 0x20 前向索引。fn 返回非 nil 错误时扫描停止。
	ScanEngramEntities(ctx context.Context, ws [8]byte, engramID ULID, fn func(entityName string) error) error

	// ScanVaultEntityNames 扫描 vault 中所有不同实体名称。
	// 同一实体名称可能出现多次（每个 engram 链接一次），fn 对每个唯一名称仅调用一次。
	ScanVaultEntityNames(ctx context.Context, ws [8]byte, fn func(name string) error) error

	// ScanRelationships 全量扫描 vault 中所有实体关系记录（0x21 前缀）。
	// fn 返回非 nil 错误时扫描停止。
	ScanRelationships(ctx context.Context, ws [8]byte, fn func(record RelationshipRecord) error) error

	// ScanEntityRelationships 使用 0x26 关系实体索引，扫描指定实体参与的所有关系记录。
	// 比 ScanRelationships 更高效：O(该实体的 engram 数) 而非 O(vault 所有关系)。
	ScanEntityRelationships(ctx context.Context, ws [8]byte, entityName string, fn func(record RelationshipRecord) error) error

	// ScanEntityClusters 扫描 vault 的 0x24 共现索引，
	// 对每个 count >= minCount 的实体对调用 fn。
	// 结果未排序；调用者应自行排序。
	ScanEntityClusters(ctx context.Context, ws [8]byte, minCount int, fn func(nameA, nameB string, count int) error) error
}

// ────────────────────────────────────────────────────────────────────
// EngineStore — 完整的 Pebble 后端存储接口
// ────────────────────────────────────────────────────────────────────

// EngineStore 是 MuninnDB 引擎的完整存储接口。
// 由 PebbleStore 实现。所有操作通过键构造中的 vault 前缀实现数据隔离。
type EngineStore interface {
	Store // 嵌入图层最小接口

	// ── 批量写入 ──────────────────────────────────────────────────

	// NewBatch 返回原子多写操作的 StoreBatch 句柄。
	// 调用者必须且仅能调用 Commit 或 Discard 一次。
	NewBatch() StoreBatch

	// ── Engram CRUD ──────────────────────────────────────────────

	// WriteEngram 原子写入完整 engram 记录 (0x01) 和纯元数据副本 (0x02)。
	// 同时写入关联前向/反向键 (0x03/0x04)、二级索引 (0x0B/0x0C/0x0D)
	// 和相关性桶 (0x10)，全部在一个 Pebble 批次中完成。
	// 返回分配的 ULID。
	WriteEngram(ctx context.Context, wsPrefix [8]byte, eng *Engram) (ULID, error)

	// WriteEngramBatch 在单次 Pebble 批次提交中原子写入多个 engram。
	// 摊销 fsync 成本；每个 engram 获得独立的 ULID、默认值、ERF 编码和索引键。
	WriteEngramBatch(ctx context.Context, items []EngramBatchItem) ([]ULID, []error)

	// GetMetadata 从 0x02 前缀读取 100 字节固定元数据。
	GetMetadata(ctx context.Context, wsPrefix [8]byte, ids []ULID) ([]*EngramMeta, error)

	// UpdateMetadata 仅更新变更的元数据字段（状态、置信度、相关性桶、访问次数、时间戳）。
	// 同时更新 0x01 和 0x02 键。
	UpdateMetadata(ctx context.Context, wsPrefix [8]byte, id ULID, meta *EngramMeta) error

	// UpdateRelevance 更新 engram 的相关性和稳定性。
	// 移动 0x10 相关性桶键并更新 0x01/0x02 元数据。
	UpdateRelevance(ctx context.Context, wsPrefix [8]byte, id ULID, relevance, stability float32) error

	// DeleteEngram 硬删除：移除 0x01、0x02 及所有关联键和二级索引。
	DeleteEngram(ctx context.Context, wsPrefix [8]byte, id ULID) error

	// SoftDelete 将状态设为 StateSoftDeleted 并在记录中设置 FlagSoftDeleted。
	SoftDelete(ctx context.Context, wsPrefix [8]byte, id ULID) error

	// GetEmbedding 读取独立存储的嵌入向量（0x18 键），反量化后返回。
	GetEmbedding(ctx context.Context, wsPrefix [8]byte, id ULID) ([]float32, error)

	// GetConfidence 从 0x02 元数据读取 engram 的置信度值。
	GetConfidence(ctx context.Context, wsPrefix [8]byte, id ULID) (float32, error)

	// UpdateConfidence 更新 0x02 元数据和 0x01 完整记录中的置信度。
	UpdateConfidence(ctx context.Context, wsPrefix [8]byte, id ULID, confidence float32) error

	// ScanEngrams 扫描 vault 中所有 engram（0x01 前缀），
	// 可选联合 0x18 嵌入键实现双迭代器扫描。
	ScanEngrams(ctx context.Context, wsPrefix [8]byte, fn func(eng *Engram) error) error

	// ── 关联 ──────────────────────────────────────────────────────

	// WriteAssociation 写入前向 (0x03) 和反向 (0x04) 关联键。
	WriteAssociation(ctx context.Context, wsPrefix [8]byte, src, dst ULID, assoc *Association) error

	// GetAssocWeight 读取 (a, b) 前向关联的权重（0x14 索引实现 O(1) 查询）。
	// 不存在关联时返回 0.0。
	GetAssocWeight(ctx context.Context, wsPrefix [8]byte, a, b ULID) (float32, error)

	// UpdateAssocWeight 更新 (a, b) 的 0x03/0x04 关联键。
	// countDelta 累加到 CoActivationCount（饱和到 MaxUint32）。
	UpdateAssocWeight(ctx context.Context, wsPrefix [8]byte, a, b ULID, weight float32, countDelta uint32) error

	// UpdateAssocWeightBatch 在单次批次中原子更新多条关联权重。
	UpdateAssocWeightBatch(ctx context.Context, updates []AssocWeightUpdate) error

	// DecayAssocWeights 将 vault 中所有关联权重乘以衰减因子，
	// 删除低于最小权重的条目。archiveThreshold > 0 时将强底线边移入 0x25 归档。
	// 返回删除计数。
	DecayAssocWeights(ctx context.Context, wsPrefix [8]byte, decayFactor float64, minWeight float32, archiveThreshold float64) (int, error)

	// GetConceptAssociations 返回扩散激活所用的最多 maxN 个邻居 ID。
	GetConceptAssociations(ctx context.Context, wsPrefix [8]byte, id ULID, maxN int) ([]ULID, error)

	// GetChildrenByParent 返回所有通过 is_part_of 关联指向 parentID 的子 engram ID。
	// 扫描 0x04 反向索引。
	GetChildrenByParent(ctx context.Context, wsPrefix [8]byte, parentID ULID) ([]ULID, error)

	// FlagContradiction 为 (a, b) 对写入 0x0A 矛盾标记键。
	FlagContradiction(ctx context.Context, wsPrefix [8]byte, a, b ULID) error

	// GetContradictions 扫描 0x0A 前缀返回 vault 中所有矛盾对。
	GetContradictions(ctx context.Context, wsPrefix [8]byte) ([][2]ULID, error)

	// ResolveContradiction 删除 (a, b) 对的矛盾标记（双向移除）。
	ResolveContradiction(ctx context.Context, wsPrefix [8]byte, a, b ULID) error

	// ── 查询 ──────────────────────────────────────────────────────

	// RecentActive 返回 vault 中相关性最高的 topK 个 engram ID。
	// 使用 0x10 相关性桶索引实现 O(k) 扫描。
	RecentActive(ctx context.Context, wsPrefix [8]byte, topK int) ([]ULID, error)

	// ListByState 使用 0x0B 状态二级索引，返回最多 limit 个匹配状态的 engram ID。
	ListByState(ctx context.Context, wsPrefix [8]byte, state LifecycleState, limit int) ([]ULID, error)

	// EngramsByCreatedSince 返回 since 之后创建的 engram，按创建时间升序排列，
	// 支持 offset/limit 分页。
	EngramsByCreatedSince(ctx context.Context, wsPrefix [8]byte, since time.Time, offset, limit int) ([]*Engram, error)

	// CountEngramsByDay 返回 since 到 until（包含）之间每天的 engram 计数。
	// 返回的 map 键为 UTC "YYYY-MM-DD" 格式。
	CountEngramsByDay(ctx context.Context, wsPrefix [8]byte, since, until time.Time) (map[string]int64, error)

	// EngramIDsByCreatedRange 返回指定时间范围内的 engram ID 列表。
	EngramIDsByCreatedRange(ctx context.Context, wsPrefix [8]byte, since, until time.Time) ([]ULID, error)

	// LowestRelevanceIDs 返回 vault 中相关性最低的 n 个 engram ID。
	// 使用 0x10 桶索引反向扫描。
	LowestRelevanceIDs(ctx context.Context, wsPrefix [8]byte, n int) ([]ULID, error)

	// ── Vault 管理 ────────────────────────────────────────────────

	// VaultPrefix 计算 vault 名称的 8 字节 SipHash 前缀。
	VaultPrefix(vault string) [8]byte

	// WriteVaultName 持久化人类可读的 vault 名称。
	// 安全地在每次写入时调用（幂等且低成本）。
	WriteVaultName(wsPrefix [8]byte, name string) error

	// ListVaultNames 返回所有已持久化的 vault 名称。
	ListVaultNames() ([]string, error)

	// GetVaultCount 返回 vault 当前的 engram 计数。
	GetVaultCount(ctx context.Context, wsPrefix [8]byte) int64

	// ── 实体额外操作 ────────────────────────────────────────────────

	// WriteEntityEngramLink 写入 vault 域 engram→实体链接 (0x20/0x23)。
	WriteEntityEngramLink(ctx context.Context, ws [8]byte, engramID ULID, entityName string) error

	// RelinkEntityEngramLink 原子移动 vault 域 engram 链接（从 fromEntity 到 toEntity）。
	RelinkEntityEngramLink(ctx context.Context, ws [8]byte, engramID ULID, fromEntity, toEntity string) error

	// DeleteEntityEngramLink 原子删除 (engram, entity) 对的 0x20/0x23 键。
	DeleteEntityEngramLink(ctx context.Context, ws [8]byte, engramID ULID, entityName string) error

	// UpsertRelationshipRecord 写入 vault 域关系记录 (0x21 + 0x26 索引)。
	UpsertRelationshipRecord(ctx context.Context, ws [8]byte, engramID ULID, record RelationshipRecord) error

	// ScanEngramRelationships 扫描指定 engram 的所有关系记录 (0x21 前缀)。
	ScanEngramRelationships(ctx context.Context, ws [8]byte, engramID ULID, fn func(record RelationshipRecord) error) error

	// RelinkRelationshipEntity 更新所有包含 oldName 的关系记录，替换为 newName。
	RelinkRelationshipEntity(ctx context.Context, ws [8]byte, oldName, newName string) error

	// IncrementEntityCoOccurrence 递增 vault 中两个实体的共现计数 (0x24 索引)。
	IncrementEntityCoOccurrence(ctx context.Context, ws [8]byte, nameA, nameB string) error

	// DecrementEntityMentionCount 递减实体提及计数，计数归零时删除实体记录。
	DecrementEntityMentionCount(ctx context.Context, name string) error

	// DecrementEntityCoOccurrence 递减 vault 中两个实体的共现计数。
	DecrementEntityCoOccurrence(ctx context.Context, ws [8]byte, nameA, nameB string) error

	// ── 序号 ──────────────────────────────────────────────────────

	// WriteOrdinal 原子写入 childID 在 parentID 下的序号。
	WriteOrdinal(ctx context.Context, wsPrefix [8]byte, parentID, childID ULID, ordinal int32) error

	// ReadOrdinal 读取 (parentID, childID) 的序号。
	// found=false 表示键不存在。
	ReadOrdinal(ctx context.Context, wsPrefix [8]byte, parentID, childID ULID) (ordinal int32, found bool, err error)

	// DeleteOrdinal 删除 (parentID, childID) 的序号键。不存在时无操作。
	DeleteOrdinal(ctx context.Context, wsPrefix [8]byte, parentID, childID ULID) error

	// DeleteEngramOrdinal 删除 (parentID, childID) 的序号键。
	// 删除 engram 时用于清理树成员关系。不存在时无操作。
	DeleteEngramOrdinal(ctx context.Context, wsPrefix [8]byte, parentID, childID ULID) error

	// ListChildOrdinals 返回 parentID 下所有 (childID, ordinal) 对，
	// 按序号升序排列。
	ListChildOrdinals(ctx context.Context, wsPrefix [8]byte, parentID ULID) ([]OrdinalEntry, error)

	// ── 最后访问索引 ────────────────────────────────────────────────

	// WriteLastAccessEntry 写入/更新 0x22 最后访问索引条目。
	// prevMillis 是旧的 LastAccess 毫秒值（首次写入为 0）。
	WriteLastAccessEntry(ctx context.Context, ws [8]byte, id ULID, prevMillis, newMillis int64) error

	// ScanLastAccessDesc 按 LastAccess 降序扫描 0x22 索引。
	ScanLastAccessDesc(ctx context.Context, ws [8]byte, fn func(id ULID, lastAccessMillis int64) error) error

	// DeleteLastAccessEntry 删除已删除 engram 的 0x22 索引条目。
	DeleteLastAccessEntry(ctx context.Context, ws [8]byte, id ULID, lastAccessMillis int64) error

	// ── 幂等性 ──────────────────────────────────────────────────────

	// CheckIdempotency 查找 opID 收据。未找到时返回 (nil, nil)。
	CheckIdempotency(ctx context.Context, opID string) (*IdempotencyReceipt, error)

	// WriteIdempotency 存储幂等性收据 (opID → engramID)。
	WriteIdempotency(ctx context.Context, opID, engramID string) error

	// ── 磁盘信息 ──────────────────────────────────────────────────

	// DiskSize 返回所有数据库文件的磁盘总大小（字节）。
	DiskSize() int64

	// ── 生命周期 ──────────────────────────────────────────────────

	// Close 刷新所有挂起写入并关闭 Pebble 数据库。
	Close() error
}
