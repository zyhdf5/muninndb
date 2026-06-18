// Package keys 提供 MuninnDB 存储层的所有 Pebble 键构造函数。
//
// 键空间使用单字节前缀区分不同数据类型，后跟 8 字节 vault 工作空间前缀
// （由 vault 名称的 SipHash 计算得出），再跟实际标识符。
//
// 前缀分配:
//
//	0x01  Engram 完整记录
//	0x02  Engram 元数据（100 字节定长）
//	0x03  关联前向索引（按权重降序）
//	0x04  关联反向索引（按权重降序）
//	0x05  FTS 倒排索引
//	0x06  Trigram 索引
//	0x07  HNSW 节点邻居
//	0x08  FTS 全局统计
//	0x09  FTS 词项统计
//	0x0A  矛盾标记
//	0x0B  状态二级索引
//	0x0C  标签二级索引
//	0x0D  创建者二级索引
//	0x0E  Vault 元数据（wsPrefix → 名称）
//	0x0F  Vault 名称索引（名称哈希 → wsPrefix）
//	0x10  相关性桶索引
//	0x11  摘要标志（DigestFlags）
//	0x12  一致性计数器
//	0x13  评分权重
//	0x14  关联权重索引（O(1) 查询）
//	0x15  Vault 计数器
//	0x16  溯源记录
//	0x17  桶迁移标记
//	0x18  独立嵌入向量（ERF v2）
//	0x19  幂等性收据
//	0x1A  Episode 记录/帧
//	0x1B  FTS 版本标记
//	0x1C  PAS 转换表
//	0x1D  嵌入模型标记
//	0x1E  序号索引
//	0x1F  全局实体记录
//	0x20  Engram→实体前向链接
//	0x21  实体关系记录
//	0x22  最后访问索引（倒序）
//	0x23  实体→Engram 反向索引
//	0x24  实体共现索引
//	0x25  归档关联
//	0x26  关系实体索引
//	0x27  Dream 状态
package keys

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/dchest/siphash"
	"golang.org/x/text/unicode/norm"
)

// SipHash 密钥，用于 vault 前缀计算
var (
	sipKey0 uint64 = 0x736f6d6570736575 // "somepseu"
	sipKey1 uint64 = 0x646f72616e646f6d // "dorandum"
)

// VaultPrefix 计算 vault 名称的 8 字节 SipHash 前缀。
func VaultPrefix(vault string) [8]byte {
	hashVal := siphash.Hash(sipKey0, sipKey1, []byte(vault))
	var prefix [8]byte
	binary.BigEndian.PutUint64(prefix[:], hashVal)
	return prefix
}

// ────────────────────────────────────────────────────────────────────
// Engram 键 (0x01, 0x02)
// ────────────────────────────────────────────────────────────────────

// EngramKey 构造完整 engram 记录键（0x01 前缀）。
// 键: 0x01 | wsPrefix(8) | ulid(16) = 25 字节
func EngramKey(ws [8]byte, id [16]byte) []byte {
	key := make([]byte, 1+8+16)
	key[0] = 0x01
	copy(key[1:9], ws[:])
	copy(key[9:25], id[:])
	return key
}

// MetaKey 构造元数据记录键（0x02 前缀）。
func MetaKey(ws [8]byte, id [16]byte) []byte {
	key := make([]byte, 1+8+16)
	key[0] = 0x02
	copy(key[1:9], ws[:])
	copy(key[9:25], id[:])
	return key
}

// ────────────────────────────────────────────────────────────────────
// 关联键 (0x03, 0x04, 0x14, 0x25)
// ────────────────────────────────────────────────────────────────────

// AssocFwdKey 构造前向关联键（0x03 前缀）。
// 使用权重补数实现按权重降序排列。
func AssocFwdKey(ws [8]byte, src [16]byte, weight float32, dst [16]byte) []byte {
	key := make([]byte, 1+8+16+4+16)
	key[0] = 0x03
	copy(key[1:9], ws[:])
	copy(key[9:25], src[:])
	wc := WeightComplement(weight)
	copy(key[25:29], wc[:])
	copy(key[29:45], dst[:])
	return key
}

// AssocRevKey 构造反向关联键（0x04 前缀）。
func AssocRevKey(ws [8]byte, dst [16]byte, weight float32, src [16]byte) []byte {
	key := make([]byte, 1+8+16+4+16)
	key[0] = 0x04
	copy(key[1:9], ws[:])
	copy(key[9:25], dst[:])
	wc := WeightComplement(weight)
	copy(key[25:29], wc[:])
	copy(key[29:45], src[:])
	return key
}

// AssocWeightIndexKey 构造关联权重索引键（0x14 前缀）。
// 用于 O(1) 权重查询。
// 键: 0x14 | wsPrefix(8) | src(16) | dst(16) = 41 字节
func AssocWeightIndexKey(ws [8]byte, src [16]byte, dst [16]byte) []byte {
	key := make([]byte, 1+8+16+16)
	key[0] = 0x14
	copy(key[1:9], ws[:])
	copy(key[9:25], src[:])
	copy(key[25:41], dst[:])
	return key
}

// AssocFwdRangeStart 返回扫描 vault 中所有前向关联的起始下界。
func AssocFwdRangeStart(ws [8]byte) []byte {
	key := make([]byte, 1+8)
	key[0] = 0x03
	copy(key[1:9], ws[:])
	return key
}

// AssocFwdRangeEnd 返回扫描 vault 中所有前向关联的排他上界。
func AssocFwdRangeEnd(ws [8]byte) []byte {
	end := make([]byte, 1+8)
	end[0] = 0x03
	copy(end[1:9], ws[:])
	for i := len(end) - 1; i >= 1; i-- {
		end[i]++
		if end[i] != 0 {
			break
		}
	}
	return end
}

// AssocFwdPrefixForID 返回指定源 engram 的所有前向关联扫描前缀。
// 键: 0x03 | ws(8) | src(16) = 25 字节
func AssocFwdPrefixForID(ws [8]byte, id [16]byte) []byte {
	key := make([]byte, 1+8+16)
	key[0] = 0x03
	copy(key[1:9], ws[:])
	copy(key[9:25], id[:])
	return key
}

// AssocRevPrefixForID 返回指定目标 engram 的所有反向关联扫描前缀。
// 键: 0x04 | ws(8) | dstID(16) = 25 字节
func AssocRevPrefixForID(ws [8]byte, id [16]byte) []byte {
	key := make([]byte, 1+8+16)
	key[0] = 0x04
	copy(key[1:9], ws[:])
	copy(key[9:25], id[:])
	return key
}

// ArchiveAssocKey 构造归档关联键（0x25 前缀）。
// 无权重补数，不按权重排序。
// 键: 0x25 | wsPrefix(8) | src(16) | dst(16) = 41 字节
func ArchiveAssocKey(ws [8]byte, src [16]byte, dst [16]byte) []byte {
	key := make([]byte, 1+8+16+16)
	key[0] = 0x25
	copy(key[1:9], ws[:])
	copy(key[9:25], src[:])
	copy(key[25:41], dst[:])
	return key
}

// ArchiveAssocPrefixForID 返回指定源 engram 的归档关联扫描前缀。
func ArchiveAssocPrefixForID(ws [8]byte, src [16]byte) []byte {
	key := make([]byte, 1+8+16)
	key[0] = 0x25
	copy(key[1:9], ws[:])
	copy(key[9:25], src[:])
	return key
}

// ArchiveAssocRangeStart 返回 vault 内所有归档关联的扫描起始下界。
func ArchiveAssocRangeStart(ws [8]byte) []byte {
	key := make([]byte, 1+8)
	key[0] = 0x25
	copy(key[1:9], ws[:])
	return key
}

// ArchiveAssocRangeEnd 返回 vault 内所有归档关联的扫描排他上界。
func ArchiveAssocRangeEnd(ws [8]byte) []byte {
	end := make([]byte, 1+8)
	end[0] = 0x25
	copy(end[1:9], ws[:])
	for i := len(end) - 1; i >= 1; i-- {
		end[i]++
		if end[i] != 0 {
			break
		}
	}
	return end
}

// ────────────────────────────────────────────────────────────────────
// FTS / Trigram / HNSW 键 (0x05, 0x06, 0x07, 0x08, 0x09)
// ────────────────────────────────────────────────────────────────────

// FTSPostingKey 构造全文搜索倒排索引键（0x05 前缀）。
func FTSPostingKey(ws [8]byte, term string, id [16]byte) []byte {
	termBytes := []byte(term)
	key := make([]byte, 1+8+len(termBytes)+1+16)
	key[0] = 0x05
	copy(key[1:9], ws[:])
	copy(key[9:9+len(termBytes)], termBytes)
	key[9+len(termBytes)] = 0x00
	copy(key[10+len(termBytes):], id[:])
	return key
}

// TrigramKey 构造 trigram 索引键（0x06 前缀）。
func TrigramKey(ws [8]byte, trigram [3]byte, id [16]byte) []byte {
	key := make([]byte, 1+8+3+16)
	key[0] = 0x06
	copy(key[1:9], ws[:])
	copy(key[9:12], trigram[:])
	copy(key[12:28], id[:])
	return key
}

// HNSWNodeKey 构造 HNSW 节点邻居列表键（0x07 前缀）。
func HNSWNodeKey(ws [8]byte, id [16]byte, layer uint8) []byte {
	key := make([]byte, 1+8+16+1)
	key[0] = 0x07
	copy(key[1:9], ws[:])
	copy(key[9:25], id[:])
	key[25] = layer
	return key
}

// FTSStatsKey 构造 FTS 全局统计键（0x08 前缀）。
func FTSStatsKey(ws [8]byte) []byte {
	key := make([]byte, 1+8+5)
	key[0] = 0x08
	copy(key[1:9], ws[:])
	copy(key[9:14], []byte("stats"))
	return key
}

// TermStatsKey 构造单词统计键（0x09 前缀）。
func TermStatsKey(ws [8]byte, term string) []byte {
	termBytes := []byte(term)
	key := make([]byte, 1+8+len(termBytes))
	key[0] = 0x09
	copy(key[1:9], ws[:])
	copy(key[9:], termBytes)
	return key
}

// ────────────────────────────────────────────────────────────────────
// 二级索引键 (0x0A, 0x0B, 0x0C, 0x0D)
// ────────────────────────────────────────────────────────────────────

// ContradictionKeyPrefix 返回 vault 中所有矛盾标记的扫描前缀。
func ContradictionKeyPrefix(ws [8]byte) []byte {
	key := make([]byte, 9)
	key[0] = 0x0A
	copy(key[1:9], ws[:])
	return key
}

// ContradictionKey 构造矛盾索引键（0x0A 前缀）。
func ContradictionKey(ws [8]byte, conceptHash uint32, relType uint16, id [16]byte) []byte {
	key := make([]byte, 1+8+4+2+16)
	key[0] = 0x0A
	copy(key[1:9], ws[:])
	binary.BigEndian.PutUint32(key[9:13], conceptHash)
	binary.BigEndian.PutUint16(key[13:15], relType)
	copy(key[15:31], id[:])
	return key
}

// StateIndexKey 构造状态二级索引键（0x0B 前缀）。
func StateIndexKey(ws [8]byte, state uint8, id [16]byte) []byte {
	key := make([]byte, 1+8+1+16)
	key[0] = 0x0B
	copy(key[1:9], ws[:])
	key[9] = state
	copy(key[10:26], id[:])
	return key
}

// TagIndexKey 构造标签二级索引键（0x0C 前缀）。
func TagIndexKey(ws [8]byte, tagHash uint32, id [16]byte) []byte {
	key := make([]byte, 1+8+4+16)
	key[0] = 0x0C
	copy(key[1:9], ws[:])
	binary.BigEndian.PutUint32(key[9:13], tagHash)
	copy(key[13:29], id[:])
	return key
}

// CreatorIndexKey 构造创建者二级索引键（0x0D 前缀）。
func CreatorIndexKey(ws [8]byte, creatorHash uint32, id [16]byte) []byte {
	key := make([]byte, 1+8+4+16)
	key[0] = 0x0D
	copy(key[1:9], ws[:])
	binary.BigEndian.PutUint32(key[9:13], creatorHash)
	copy(key[13:29], id[:])
	return key
}

// ────────────────────────────────────────────────────────────────────
// Vault 管理键 (0x0E, 0x0F, 0x15)
// ────────────────────────────────────────────────────────────────────

// VaultMetaKey 构造 vault 元数据键（0x0E 前缀）。
// 值: 人类可读的 vault 名称字符串。
// 键: 0x0E | wsPrefix(8) = 9 字节
func VaultMetaKey(ws [8]byte) []byte {
	key := make([]byte, 1+8)
	key[0] = 0x0E
	copy(key[1:9], ws[:])
	return key
}

// VaultNameIndexKey 构造 vault 名称正向索引键（0x0F 前缀）。
// 通过名称的 SipHash 定位实际的 wsPrefix。
// 键: 0x0F | siphash(name)[8] = 9 字节
func VaultNameIndexKey(name string) []byte {
	nameHash := siphash.Hash(sipKey0, sipKey1, []byte(name))
	key := make([]byte, 1+8)
	key[0] = 0x0F
	binary.BigEndian.PutUint64(key[1:], nameHash)
	return key
}

// VaultCountKey 构造 vault 计数器键（0x15 前缀）。
// 键: 0x15 | wsPrefix(8) = 9 字节
// 值: BigEndian int64 — vault 中的 engram 总数。
func VaultCountKey(ws [8]byte) []byte {
	key := make([]byte, 1+8)
	key[0] = 0x15
	copy(key[1:9], ws[:])
	return key
}

// ────────────────────────────────────────────────────────────────────
// 相关性桶索引 (0x10)
// ────────────────────────────────────────────────────────────────────

// RelevanceBucketKey 构造相关性桶索引键（0x10 前缀）。
// storedBucket = 9 - clamp(floor(relevance*10), 0, 9)
// 高相关性产生低桶号，升序扫描时优先返回。
// 键: 0x10 | wsPrefix(8) | storedBucket(1) | id(16) = 26 字节
func RelevanceBucketKey(ws [8]byte, relevance float32, id [16]byte) []byte {
	key := make([]byte, 1+8+1+16)
	key[0] = 0x10
	copy(key[1:9], ws[:])

	floored := int(math.Floor(float64(relevance) * 10))
	clamped := floored
	if clamped < 0 {
		clamped = 0
	}
	if clamped > 9 {
		clamped = 9
	}
	key[9] = uint8(9 - clamped)
	copy(key[10:26], id[:])
	return key
}

// ────────────────────────────────────────────────────────────────────
// 嵌入向量键 (0x18)
// ────────────────────────────────────────────────────────────────────

// EmbeddingKey 构造独立嵌入向量键（0x18 前缀，ERF v2）。
// 值: 8 字节量化参数 + N×int8 量化字节。
// 键: 0x18 | wsPrefix(8) | ulid(16) = 25 字节
func EmbeddingKey(ws [8]byte, id [16]byte) []byte {
	key := make([]byte, 1+8+16)
	key[0] = 0x18
	copy(key[1:9], ws[:])
	copy(key[9:25], id[:])
	return key
}

// ────────────────────────────────────────────────────────────────────
// 序号索引 (0x1E)
// ────────────────────────────────────────────────────────────────────

// OrdinalKey 构造序号索引键（0x1E 前缀）。
// 存储 childID 在 parentID 下的排列位置。
// 键: 0x1E | wsPrefix(8) | parentID(16) | childID(16) = 41 字节
func OrdinalKey(ws [8]byte, parentID [16]byte, childID [16]byte) []byte {
	key := make([]byte, 1+8+16+16)
	key[0] = 0x1E
	copy(key[1:9], ws[:])
	copy(key[9:25], parentID[:])
	copy(key[25:41], childID[:])
	return key
}

// OrdinalPrefixForParent 返回指定父节点下所有子节点序号的扫描前缀。
// 键: 0x1E | ws(8) | parentID(16) = 25 字节
func OrdinalPrefixForParent(ws [8]byte, parentID [16]byte) []byte {
	key := make([]byte, 1+8+16)
	key[0] = 0x1E
	copy(key[1:9], ws[:])
	copy(key[9:25], parentID[:])
	return key
}

// OrdinalWorkspacePrefix 返回 vault 中所有序号键的扫描前缀。
// 删除 engram 时用于查找作为子节点的所有序号条目。
func OrdinalWorkspacePrefix(ws [8]byte) []byte {
	key := make([]byte, 1+8)
	key[0] = 0x1E
	copy(key[1:9], ws[:])
	return key
}

// ────────────────────────────────────────────────────────────────────
// 实体相关键 (0x1F, 0x20, 0x21, 0x23, 0x24, 0x26)
// ────────────────────────────────────────────────────────────────────

// EntityNameHash 计算实体名称的 8 字节 SipHash。
// 名称先经过 NFKC 规范化、小写转换和空白修剪。
func EntityNameHash(name string) [8]byte {
	normalized := strings.ToLower(strings.TrimSpace(norm.NFKC.String(name)))
	hashVal := siphash.Hash(sipKey0, sipKey1, []byte(normalized))
	var h [8]byte
	binary.BigEndian.PutUint64(h[:], hashVal)
	return h
}

// EntityKey 构造全局实体记录键（0x1F 前缀）。
// 键: 0x1F | nameHash(8) = 9 字节
func EntityKey(nameHash [8]byte) []byte {
	key := make([]byte, 1+8)
	key[0] = 0x1F
	copy(key[1:9], nameHash[:])
	return key
}

// EntityEngramLinkKey 构造 engram→实体前向链接键（0x20 前缀）。
// 键: 0x20 | wsPrefix(8) | engramID(16) | nameHash(8) = 33 字节
func EntityEngramLinkKey(ws [8]byte, engramID [16]byte, nameHash [8]byte) []byte {
	key := make([]byte, 1+8+16+8)
	key[0] = 0x20
	copy(key[1:9], ws[:])
	copy(key[9:25], engramID[:])
	copy(key[25:33], nameHash[:])
	return key
}

// EntityEngramLinkPrefix 返回指定 engram 的所有实体链接扫描前缀。
// 键: 0x20 | ws(8) | engramID(16) = 25 字节
func EntityEngramLinkPrefix(ws [8]byte, engramID [16]byte) []byte {
	key := make([]byte, 1+8+16)
	key[0] = 0x20
	copy(key[1:9], ws[:])
	copy(key[9:25], engramID[:])
	return key
}

// EntityReverseIndexKey 构造实体→engram 反向索引键（0x23 前缀）。
// 支持"哪些 engram 提到了实体 X？"查询。
// 键: 0x23 | nameHash(8) | wsPrefix(8) | engramID(16) = 33 字节
func EntityReverseIndexKey(nameHash [8]byte, ws [8]byte, engramID [16]byte) []byte {
	key := make([]byte, 1+8+8+16)
	key[0] = 0x23
	copy(key[1:9], nameHash[:])
	copy(key[9:17], ws[:])
	copy(key[17:33], engramID[:])
	return key
}

// EntityReverseIndexPrefix 返回指定实体的反向索引扫描前缀。
// 键: 0x23 | nameHash(8) = 9 字节
func EntityReverseIndexPrefix(nameHash [8]byte) []byte {
	key := make([]byte, 1+8)
	key[0] = 0x23
	copy(key[1:9], nameHash[:])
	return key
}

// RelationshipKey 构造 vault 级关系记录键（0x21 前缀）。
// 键: 0x21 | ws(8) | engramID(16) | fromNameHash(8) | relTypeByte(1) | toNameHash(8) = 42 字节
func RelationshipKey(ws [8]byte, engramID [16]byte, fromHash [8]byte, relTypeByte uint8, toHash [8]byte) []byte {
	key := make([]byte, 1+8+16+8+1+8)
	key[0] = 0x21
	copy(key[1:9], ws[:])
	copy(key[9:25], engramID[:])
	copy(key[25:33], fromHash[:])
	key[33] = relTypeByte
	copy(key[34:42], toHash[:])
	return key
}

// RelationshipPrefix 返回 vault 中所有关系记录的扫描前缀。
// 键: 0x21 | wsPrefix(8) = 9 字节
func RelationshipPrefix(ws [8]byte) []byte {
	key := make([]byte, 1+8)
	key[0] = 0x21
	copy(key[1:9], ws[:])
	return key
}

// RelationshipEngramPrefix 返回指定 engram 的所有关系记录扫描前缀。
// 键: 0x21 | ws(8) | engramID(16) = 25 字节
func RelationshipEngramPrefix(ws [8]byte, engramID [16]byte) []byte {
	key := make([]byte, 1+8+16)
	key[0] = 0x21
	copy(key[1:9], ws[:])
	copy(key[9:25], engramID[:])
	return key
}

// CoOccurrenceKey 构造实体共现索引键（0x24 前缀）。
// 始终以 hashA <= hashB 的规范顺序存储。
// 键: 0x24 | wsPrefix(8) | nameHashA(8) | nameHashB(8) = 25 字节
func CoOccurrenceKey(ws [8]byte, hashA, hashB [8]byte) []byte {
	key := make([]byte, 1+8+8+8)
	key[0] = 0x24
	copy(key[1:9], ws[:])
	copy(key[9:17], hashA[:])
	copy(key[17:25], hashB[:])
	return key
}

// CoOccurrencePrefix 返回 vault 中所有共现条目的扫描前缀。
func CoOccurrencePrefix(ws [8]byte) []byte {
	key := make([]byte, 1+8)
	key[0] = 0x24
	copy(key[1:9], ws[:])
	return key
}

// RelEntityIndexKey 构造关系实体索引键（0x26 前缀）。
// 每次 UpsertRelationshipRecord 时为 fromEntity 和 toEntity 各写入一条。
// 键: 0x26 | ws(8) | entityHash(8) | engramID(16) = 33 字节
func RelEntityIndexKey(ws [8]byte, entityHash [8]byte, engramID [16]byte) []byte {
	key := make([]byte, 1+8+8+16)
	key[0] = 0x26
	copy(key[1:9], ws[:])
	copy(key[9:17], entityHash[:])
	copy(key[17:33], engramID[:])
	return key
}

// RelEntityIndexPrefix 返回指定实体在 vault 中的关系 engram 扫描前缀。
// 键: 0x26 | ws(8) | entityHash(8) = 17 字节
func RelEntityIndexPrefix(ws [8]byte, entityHash [8]byte) []byte {
	key := make([]byte, 1+8+8)
	key[0] = 0x26
	copy(key[1:9], ws[:])
	copy(key[9:17], entityHash[:])
	return key
}

// ────────────────────────────────────────────────────────────────────
// 最后访问索引 (0x22)
// ────────────────────────────────────────────────────────────────────

// LastAccessIndexKey 构造最后访问索引键（0x22 前缀）。
// 使用倒转毫秒（^uint64(unixMillis)），升序扫描时最近访问的排在前面。
// 键: 0x22 | wsPrefix(8) | invertedMillis(8) | engramID(16) = 33 字节
func LastAccessIndexKey(ws [8]byte, lastAccessMillis int64, engramID [16]byte) []byte {
	key := make([]byte, 1+8+8+16)
	key[0] = 0x22
	copy(key[1:9], ws[:])
	inverted := ^uint64(lastAccessMillis)
	binary.BigEndian.PutUint64(key[9:17], inverted)
	copy(key[17:33], engramID[:])
	return key
}

// LastAccessIndexPrefix 返回 vault 中最后访问索引的扫描前缀。
func LastAccessIndexPrefix(ws [8]byte) []byte {
	key := make([]byte, 1+8)
	key[0] = 0x22
	copy(key[1:9], ws[:])
	return key
}

// ────────────────────────────────────────────────────────────────────
// 幂等性收据 (0x19)
// ────────────────────────────────────────────────────────────────────

// IdempotencyKey 构造全局幂等性收据键（0x19 前缀）。
// 键: 0x19 | siphash(op_id)(8) = 9 字节
func IdempotencyKey(opID string) []byte {
	hashVal := siphash.Hash(sipKey0, sipKey1, []byte(opID))
	key := make([]byte, 1+8)
	key[0] = 0x19
	binary.BigEndian.PutUint64(key[1:], hashVal)
	return key
}

// ────────────────────────────────────────────────────────────────────
// 其他杂项键
// ────────────────────────────────────────────────────────────────────

// DigestFlagsKey 构造摘要标志键（0x11 前缀）。
// 键: 0x11 | id(16) = 17 字节（全局键，无 vault 前缀）
func DigestFlagsKey(id [16]byte) []byte {
	key := make([]byte, 1+16)
	key[0] = 0x11
	copy(key[1:17], id[:])
	return key
}

// CoherenceKey 构造 vault 一致性计数器键（0x12 前缀）。
func CoherenceKey(vaultPrefix [8]byte) []byte {
	key := make([]byte, 9)
	key[0] = 0x12
	copy(key[1:], vaultPrefix[:])
	return key
}

// VaultWeightsKey 构造 vault 评分权重键（0x13 前缀）。
func VaultWeightsKey(ws [8]byte) []byte {
	key := make([]byte, 1+8)
	key[0] = 0x13
	copy(key[1:9], ws[:])
	return key
}

// BucketMigrationKey 构造桶迁移标记键（0x17 前缀）。
func BucketMigrationKey(ws [8]byte) []byte {
	key := make([]byte, 1+8)
	key[0] = 0x17
	copy(key[1:9], ws[:])
	return key
}

// FTSVersionKey 构造 FTS 版本标记键（0x1B 前缀）。
func FTSVersionKey(ws [8]byte) []byte {
	key := make([]byte, 1+8)
	key[0] = 0x1B
	copy(key[1:9], ws[:])
	return key
}

// EmbedModelKey 构造嵌入模型标记键（0x1D 前缀）。
func EmbedModelKey(ws [8]byte) []byte {
	key := make([]byte, 1+8)
	key[0] = 0x1D
	copy(key[1:9], ws[:])
	return key
}

// DreamStateKey 构造 vault dream 状态键（0x27 前缀）。
func DreamStateKey(vaultPrefix [8]byte) []byte {
	key := make([]byte, 9)
	key[0] = 0x27
	copy(key[1:], vaultPrefix[:])
	return key
}

// ────────────────────────────────────────────────────────────────────
// 辅助函数
// ────────────────────────────────────────────────────────────────────

// WeightComplement 计算权重补数，用于降序排列。
func WeightComplement(weight float32) [4]byte {
	w := uint32(weight * float32(math.MaxUint32))
	c := uint32(math.MaxUint32) - w
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], c)
	return buf
}

// WeightFromComplement 从补数还原权重值。
func WeightFromComplement(wc [4]byte) float32 {
	c := binary.BigEndian.Uint32(wc[:])
	w := uint32(math.MaxUint32) - c
	return float32(w) / float32(math.MaxUint32)
}

// IncrementWSPrefix 返回下一个 vault 前缀，用作 Pebble 范围操作的排他上界。
func IncrementWSPrefix(ws [8]byte) ([8]byte, error) {
	result := ws
	for i := 7; i >= 0; i-- {
		result[i]++
		if result[i] != 0 {
			return result, nil
		}
	}
	return [8]byte{}, fmt.Errorf("工作空间前缀溢出")
}

// Hash 计算字符串的 32 位 FNV-1a 哈希值，用于标签和创建者索引。
func Hash(s string) uint32 {
	h := uint32(2166136261)
	for _, c := range []byte(s) {
		h ^= uint32(c)
		h *= 16777619
	}
	return h
}
