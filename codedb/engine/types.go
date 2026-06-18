// Package engine 实现了 MuninnDB 认知数据库的图数据库层。
// 本包提供实体图导出、图遍历、实体状态管理、实体时间线、
// 共现聚类、实体增强等功能，是从 internal/engine 中
// 抽离出来的独立图数据库实现。
package engine

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
)

// ────────────────────────────────────────────────────────────────────
// ULID — 全局唯一、可字典序排序的标识符
// ────────────────────────────────────────────────────────────────────

// ULID 是 16 字节的全局唯一标识符（Universally Unique Lexicographically Sortable Identifier）。
// 内部以原始字节存储，对外通过 26 字符的 Crockford base32 编码表示。
type ULID [16]byte

// NewULID 使用当前时间戳和 crypto/rand 熵源生成新的 ULID。
func NewULID() ULID {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id := ulid.MustNew(ulid.Timestamp(time.Now()), entropy)
	var u ULID
	copy(u[:], id[:])
	return u
}

// NewULIDWithTime 使用指定时间戳生成 ULID。
func NewULIDWithTime(t time.Time) ULID {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id := ulid.MustNew(ulid.Timestamp(t), entropy)
	var u ULID
	copy(u[:], id[:])
	return u
}

// String 返回 26 字符的 Crockford base32 字符串表示。
func (u ULID) String() string {
	var id ulid.ULID
	copy(id[:], u[:])
	return id.String()
}

// ParseULID 将 26 字符的字符串解析为 ULID。
func ParseULID(s string) (ULID, error) {
	id, err := ulid.Parse(s)
	if err != nil {
		return ULID{}, fmt.Errorf("解析 ULID: %w", err)
	}
	var u ULID
	copy(u[:], id[:])
	return u, nil
}

// CompareULIDs 按字典序比较两个 ULID，返回 -1、0 或 1。
func CompareULIDs(a, b ULID) int {
	return bytes.Compare(a[:], b[:])
}

// ────────────────────────────────────────────────────────────────────
// 生命周期状态 — Engram 的状态机
// ────────────────────────────────────────────────────────────────────

// LifecycleState 表示 engram 的生命周期状态（磁盘上为 uint8）。
type LifecycleState uint8

const (
	StatePlanning    LifecycleState = 0x00 // 规划中
	StateActive      LifecycleState = 0x01 // 活跃（写入时的默认状态）
	StatePaused      LifecycleState = 0x02 // 暂停
	StateBlocked     LifecycleState = 0x03 // 阻塞
	StateCompleted   LifecycleState = 0x04 // 已完成
	StateCancelled   LifecycleState = 0x05 // 已取消
	StateArchived    LifecycleState = 0x06 // 已归档
	StateSoftDeleted LifecycleState = 0x7F // 软删除
)

// String 返回生命周期状态的可读名称。
func (s LifecycleState) String() string {
	switch s {
	case StatePlanning:
		return "planning"
	case StateActive:
		return "active"
	case StatePaused:
		return "paused"
	case StateBlocked:
		return "blocked"
	case StateCompleted:
		return "completed"
	case StateCancelled:
		return "cancelled"
	case StateArchived:
		return "archived"
	case StateSoftDeleted:
		return "soft_deleted"
	default:
		return fmt.Sprintf("unknown(%d)", uint8(s))
	}
}

// ParseLifecycleState 将字符串解析为生命周期状态。
func ParseLifecycleState(s string) (LifecycleState, error) {
	states := map[string]LifecycleState{
		"planning":  StatePlanning,
		"active":    StateActive,
		"paused":    StatePaused,
		"blocked":   StateBlocked,
		"completed": StateCompleted,
		"cancelled": StateCancelled,
		"archived":  StateArchived,
	}
	if st, ok := states[s]; ok {
		return st, nil
	}
	return 0, fmt.Errorf("未知的生命周期状态 %q", s)
}

// ────────────────────────────────────────────────────────────────────
// 关联类型 — 两个 engram 之间的关系类型
// ────────────────────────────────────────────────────────────────────

// RelType 表示关联的关系类型（磁盘上为 uint16）。
type RelType uint16

const (
	RelSupports         RelType = 0x0001 // 支持
	RelContradicts      RelType = 0x0002 // 矛盾
	RelDependsOn        RelType = 0x0003 // 依赖
	RelSupersedes       RelType = 0x0004 // 取代
	RelRelatesTo        RelType = 0x0005 // 关联
	RelIsPartOf         RelType = 0x0006 // 从属（子→父）
	RelCauses           RelType = 0x0007 // 因果
	RelPrecededBy       RelType = 0x0008 // 时序前驱
	RelFollowedBy       RelType = 0x0009 // 时序后继
	RelCreatedByPerson  RelType = 0x000A // 由人创建
	RelBelongsToProject RelType = 0x000B // 归属项目
	RelReferences       RelType = 0x000C // 引用
	RelImplements       RelType = 0x000D // 实现
	RelBlocks           RelType = 0x000E // 阻塞
	RelResolves         RelType = 0x000F // 解决
	RelRefines          RelType = 0x0010 // 精炼（近似重复的修订）
	RelUserDefined      RelType = 0x8000 // 用户自定义
)

// ────────────────────────────────────────────────────────────────────
// 嵌入维度 — 向量嵌入的维度编码
// ────────────────────────────────────────────────────────────────────

// EmbedDimension 编码嵌入向量的维度（磁盘上为 uint8）。
type EmbedDimension uint8

const (
	EmbedNone  EmbedDimension = 0    // 无嵌入
	Embed384   EmbedDimension = 1    // 384 维
	Embed768   EmbedDimension = 2    // 768 维
	Embed1536  EmbedDimension = 3    // 1536 维
	Embed3072  EmbedDimension = 4    // 3072 维
	EmbedOther EmbedDimension = 0xFF // 其他未知维度
)

// ────────────────────────────────────────────────────────────────────
// 记忆类型 — 基于规则的分类
// ────────────────────────────────────────────────────────────────────

// MemoryType 表示基于规则的记忆分类。
type MemoryType uint8

const (
	TypeFact        MemoryType = 0  // 事实信息
	TypeDecision    MemoryType = 1  // 附带理由的决策
	TypeObservation MemoryType = 2  // 观察、洞察
	TypePreference  MemoryType = 3  // 观点、个人偏好
	TypeIssue       MemoryType = 4  // 缺陷、问题
	TypeTask        MemoryType = 5  // 待办事项
	TypeProcedure   MemoryType = 6  // 操作流程
	TypeEvent       MemoryType = 7  // 事件（时间性）
	TypeGoal        MemoryType = 8  // 目标、意图
	TypeConstraint  MemoryType = 9  // 约束、限制
	TypeIdentity    MemoryType = 10 // 关于人、角色、实体
	TypeReference   MemoryType = 11 // 文档、规范
)

// TypeBugfix 是 TypeIssue 的向后兼容别名。
const TypeBugfix = TypeIssue

// String 返回 MemoryType 的规范字符串名称。
func (m MemoryType) String() string {
	switch m {
	case TypeFact:
		return "fact"
	case TypeDecision:
		return "decision"
	case TypeObservation:
		return "observation"
	case TypePreference:
		return "preference"
	case TypeIssue:
		return "issue"
	case TypeTask:
		return "task"
	case TypeProcedure:
		return "procedure"
	case TypeEvent:
		return "event"
	case TypeGoal:
		return "goal"
	case TypeConstraint:
		return "constraint"
	case TypeIdentity:
		return "identity"
	case TypeReference:
		return "reference"
	default:
		return "fact"
	}
}

// ParseMemoryType 将字符串解析为 MemoryType。
// 如果字符串不是已知的类型名称，返回 TypeFact 和 false。
func ParseMemoryType(s string) (MemoryType, bool) {
	switch s {
	case "fact":
		return TypeFact, true
	case "decision":
		return TypeDecision, true
	case "observation":
		return TypeObservation, true
	case "preference":
		return TypePreference, true
	case "issue":
		return TypeIssue, true
	case "bugfix", "bug_report":
		return TypeIssue, true
	case "task":
		return TypeTask, true
	case "procedure":
		return TypeProcedure, true
	case "event", "experience":
		return TypeEvent, true
	case "goal":
		return TypeGoal, true
	case "constraint":
		return TypeConstraint, true
	case "identity":
		return TypeIdentity, true
	case "reference":
		return TypeReference, true
	default:
		return TypeFact, false
	}
}

// ────────────────────────────────────────────────────────────────────
// ERF 标志位 — 记录格式中的标志字节
// ────────────────────────────────────────────────────────────────────

const (
	FlagHasEmbedding      uint8 = 1 << 0 // 包含嵌入向量
	FlagContentCompressed uint8 = 1 << 1 // 内容已压缩
	FlagEmbedQuantized    uint8 = 1 << 2 // 嵌入已量化
	FlagDormant           uint8 = 1 << 3 // 休眠状态
	FlagSoftDeleted       uint8 = 1 << 4 // 软删除
	FlagDirty             uint8 = 1 << 5 // 脏标记
)

// ────────────────────────────────────────────────────────────────────
// Engram — 存储记忆的完整内存表示
// ────────────────────────────────────────────────────────────────────

// Engram 是存储记忆的完整内存表示。
type Engram struct {
	ID             ULID           // 全局唯一标识符
	CreatedAt      time.Time      // 创建时间
	UpdatedAt      time.Time      // 更新时间
	LastAccess     time.Time      // 最后访问时间
	Confidence     float32        // 置信度 0.0-1.0
	Relevance      float32        // 当前的艾宾浩斯分数（ACTIVATE 读取时计算）
	Stability      float32        // 衰减抗性（天）
	AccessCount    uint32         // 访问计数
	State          LifecycleState // 生命周期状态
	EmbedDim       EmbedDimension // 嵌入维度
	Concept        string         // 概念标签（必填，最大 512 字节）
	CreatedBy      string         // 创建者（最大 64 字节）
	Content        string         // 内容（必填，最大 16KB）
	Tags           []string       // 标签列表
	Associations   []Association  // 关联列表
	Embedding      []float32      // 嵌入向量（无嵌入时为 nil）
	Summary        string         // 提取式摘要（前 2 句）
	KeyPoints      []string       // 按 IDF 稀有度排名的前 5 句
	MemoryType     MemoryType     // 记忆类型
	TypeLabel      string         // 自由格式标签，如 "architectural_decision"
	Classification uint16         // 概念聚类 ID
}

// EngramMeta 是 100 字节的固定元数据部分。
// 用于衰减工作器、激活评分以及不需要完整内容/嵌入的路径。
type EngramMeta struct {
	ID          ULID           // 全局唯一标识符
	CreatedAt   time.Time      // 创建时间
	UpdatedAt   time.Time      // 更新时间
	LastAccess  time.Time      // 最后访问时间
	Confidence  float32        // 置信度
	Relevance   float32        // 相关度
	Stability   float32        // 稳定性
	AccessCount uint32         // 访问计数
	State       LifecycleState // 生命周期状态
	AssocCount  uint16         // 关联数量
	EmbedDim    EmbedDimension // 嵌入维度
	MemoryType  MemoryType     // 记忆类型
}

// ────────────────────────────────────────────────────────────────────
// Association — engram 之间的有向加权关联
// ────────────────────────────────────────────────────────────────────

// Association 表示两个 engram 之间的有向加权链接。
// 磁盘上固定大小约 40 字节。
type Association struct {
	TargetID          ULID      // 目标 engram 的 ID
	RelType           RelType   // 关系类型
	Weight            float32   // 权重 0.0-1.0，可通过 Hebbian 学习调整
	Confidence        float32   // 置信度 0.0-1.0
	CreatedAt         time.Time // 创建时间
	LastActivated     int32     // 最后激活时间（Unix 秒）
	PeakWeight        float32   // 历史最大权重；0 表示未跟踪（升级前的旧数据）
	CoActivationCount uint32    // Hebbian 共激活累计次数；0 表示特性前/未知
	RestoredAt        int32     // 恢复时间（Unix 秒）；0 表示从未恢复
}

// ────────────────────────────────────────────────────────────────────
// EntityRecord — 全局命名实体记录
// ────────────────────────────────────────────────────────────────────

// EntityRecord 是存储在全局注册表中的命名实体记录。
// 记录不区分 vault，实体-engram 链接在 vault 级别隔离。
type EntityRecord struct {
	Name         string  `msgpack:"name"`          // 实体名称
	Type         string  `msgpack:"type"`          // 实体类型
	Confidence   float32 `msgpack:"confidence"`    // 置信度
	Source       string  `msgpack:"source"`        // 来源，如 "inline"、"plugin:enrich"
	UpdatedAt    int64   `msgpack:"updated_at"`    // 更新时间（Unix 纳秒）
	FirstSeen    int64   `msgpack:"first_seen"`    // 首次出现时间（Unix 纳秒）
	MentionCount int32   `msgpack:"mention_count"` // 提及次数
	State        string  `msgpack:"state"`         // 状态："active"、"deprecated"、"merged"、"resolved"
	MergedInto   string  `msgpack:"merged_into"`   // 合并目标（仅当 State == "merged" 时设置）
}

// ────────────────────────────────────────────────────────────────────
// RelationshipRecord — vault 级别的实体间关系记录
// ────────────────────────────────────────────────────────────────────

// RelationshipRecord 表示从特定 engram 中提取的实体间类型化关系。
// 存储在 vault 级别。
type RelationshipRecord struct {
	FromEntity string  `msgpack:"from_entity"` // 源实体名称
	ToEntity   string  `msgpack:"to_entity"`   // 目标实体名称
	RelType    string  `msgpack:"rel_type"`    // 关系类型字符串
	Weight     float32 `msgpack:"weight"`      // 关系权重
	Source     string  `msgpack:"source"`      // 来源标识
	UpdatedAt  int64   `msgpack:"updated_at"`  // 更新时间（Unix 纳秒）
}

// ────────────────────────────────────────────────────────────────────
// ScoredEngram — 带评分的 engram（用于实体增强）
// ────────────────────────────────────────────────────────────────────

// ScoredEngram 是带有相关性评分的 engram 引用。
// 用于实体增强（entity boost）的传播激活过程。
type ScoredEngram struct {
	Engram *Engram // 指向 engram 的指针
	Score  float64 // 相关性分数
}
