package engine

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── ULID 测试 ──

func TestNewULID_Unique(t *testing.T) {
	a := NewULID()
	b := NewULID()
	assert.NotEqual(t, a, b, "两个连续生成的 ULID 应该不同")
}

func TestNewULID_NonZero(t *testing.T) {
	id := NewULID()
	assert.NotEqual(t, ULID{}, id, "新生成的 ULID 不应为零值")
}

func TestNewULIDWithTime_EmbeddsTimestamp(t *testing.T) {
	ts := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	id := NewULIDWithTime(ts)
	assert.NotEqual(t, ULID{}, id)
}

func TestULID_StringRoundtrip(t *testing.T) {
	original := NewULID()
	s := original.String()
	assert.Len(t, s, 26, "ULID 字符串应为 26 字符")

	parsed, err := ParseULID(s)
	require.NoError(t, err)
	assert.Equal(t, original, parsed, "解析后应与原始值相等")
}

func TestParseULID_InvalidString(t *testing.T) {
	_, err := ParseULID("not-a-valid-ulid")
	assert.Error(t, err, "无效字符串应返回错误")
}

func TestParseULID_EmptyString(t *testing.T) {
	_, err := ParseULID("")
	assert.Error(t, err, "空字符串应返回错误")
}

func TestCompareULIDs_Equal(t *testing.T) {
	id := NewULID()
	assert.Equal(t, 0, CompareULIDs(id, id))
}

func TestCompareULIDs_Ordering(t *testing.T) {
	// 时间戳更早的 ULID 在字典序上更小。
	earlier := NewULIDWithTime(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	later := NewULIDWithTime(time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, -1, CompareULIDs(earlier, later))
	assert.Equal(t, 1, CompareULIDs(later, earlier))
}

// ── LifecycleState 测试 ──

func TestLifecycleState_String(t *testing.T) {
	tests := []struct {
		state LifecycleState
		want  string
	}{
		{StatePlanning, "planning"},
		{StateActive, "active"},
		{StatePaused, "paused"},
		{StateBlocked, "blocked"},
		{StateCompleted, "completed"},
		{StateCancelled, "cancelled"},
		{StateArchived, "archived"},
		{StateSoftDeleted, "soft_deleted"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, tc.state.String())
	}
}

func TestLifecycleState_StringUnknown(t *testing.T) {
	s := LifecycleState(0xFF).String()
	assert.Contains(t, s, "unknown")
}

func TestParseLifecycleState_AllValid(t *testing.T) {
	validStates := map[string]LifecycleState{
		"planning":  StatePlanning,
		"active":    StateActive,
		"paused":    StatePaused,
		"blocked":   StateBlocked,
		"completed": StateCompleted,
		"cancelled": StateCancelled,
		"archived":  StateArchived,
	}
	for name, want := range validStates {
		got, err := ParseLifecycleState(name)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
}

func TestParseLifecycleState_Invalid(t *testing.T) {
	_, err := ParseLifecycleState("nonexistent")
	assert.Error(t, err)
}

func TestParseLifecycleState_SoftDeletedNotParseable(t *testing.T) {
	// soft_deleted 不在可解析列表中（设计上只通过内部设置）。
	_, err := ParseLifecycleState("soft_deleted")
	assert.Error(t, err)
}

// ── MemoryType 测试 ──

func TestMemoryType_String(t *testing.T) {
	tests := []struct {
		mt   MemoryType
		want string
	}{
		{TypeFact, "fact"},
		{TypeDecision, "decision"},
		{TypeObservation, "observation"},
		{TypePreference, "preference"},
		{TypeIssue, "issue"},
		{TypeTask, "task"},
		{TypeProcedure, "procedure"},
		{TypeEvent, "event"},
		{TypeGoal, "goal"},
		{TypeConstraint, "constraint"},
		{TypeIdentity, "identity"},
		{TypeReference, "reference"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, tc.mt.String())
	}
}

func TestMemoryType_StringUnknownDefaultsFact(t *testing.T) {
	assert.Equal(t, "fact", MemoryType(0xFF).String())
}

func TestParseMemoryType_AllValid(t *testing.T) {
	valid := map[string]MemoryType{
		"fact":        TypeFact,
		"decision":    TypeDecision,
		"observation": TypeObservation,
		"preference":  TypePreference,
		"issue":       TypeIssue,
		"bugfix":      TypeIssue,
		"bug_report":  TypeIssue,
		"task":        TypeTask,
		"procedure":   TypeProcedure,
		"event":       TypeEvent,
		"experience":  TypeEvent,
		"goal":        TypeGoal,
		"constraint":  TypeConstraint,
		"identity":    TypeIdentity,
		"reference":   TypeReference,
	}
	for name, want := range valid {
		got, ok := ParseMemoryType(name)
		assert.True(t, ok, "ParseMemoryType(%q) should succeed", name)
		assert.Equal(t, want, got, "ParseMemoryType(%q)", name)
	}
}

func TestParseMemoryType_UnknownReturnsFact(t *testing.T) {
	got, ok := ParseMemoryType("unknown_type")
	assert.False(t, ok)
	assert.Equal(t, TypeFact, got)
}

func TestTypeBugfix_IsTypeIssue(t *testing.T) {
	assert.Equal(t, TypeIssue, TypeBugfix, "TypeBugfix 应是 TypeIssue 的别名")
}

// ── ERF 标志位测试 ──

func TestERFFlags_NoDuplicateBits(t *testing.T) {
	flags := []uint8{
		FlagHasEmbedding,
		FlagContentCompressed,
		FlagEmbedQuantized,
		FlagDormant,
		FlagSoftDeleted,
		FlagDirty,
	}
	combined := uint8(0)
	for _, f := range flags {
		assert.Equal(t, uint8(0), combined&f, "标志位 0x%02X 与先前标志位重叠", f)
		combined |= f
	}
}

// ── EmbedDimension 常量测试 ──

func TestEmbedDimension_Values(t *testing.T) {
	assert.Equal(t, EmbedDimension(0), EmbedNone)
	assert.Equal(t, EmbedDimension(1), Embed384)
	assert.Equal(t, EmbedDimension(2), Embed768)
	assert.Equal(t, EmbedDimension(3), Embed1536)
	assert.Equal(t, EmbedDimension(4), Embed3072)
}
