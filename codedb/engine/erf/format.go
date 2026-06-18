package erf

const (
	Magic    uint32 = 0x4D554E4E
	Version  uint8  = 0x01
	Version2 uint8  = 0x02

	HeaderOffset    = 0
	HeaderSize      = 8
	MetadataOffset  = 8
	MetadataSize    = 100
	OffsetTablePos  = 108
	OffsetTableSize = 40
	FixedOverhead   = 152
	TrailerSize     = 4

	OffsetID             = 8
	OffsetCreatedAt      = 24
	OffsetUpdatedAt      = 32
	OffsetLastAccess     = 40
	OffsetConfidence     = 48
	OffsetRelevance      = 52
	OffsetStability      = 56
	OffsetAccessCount    = 60
	OffsetState          = 64
	OffsetAssocCount     = 65
	OffsetEmbedDim       = 67
	OffsetMemoryType     = 68
	OffsetClassification = 69
	OffsetReserved       = 71

	OffsetConceptOff   = 108
	OffsetConceptLen   = 112
	OffsetCreatedByOff = 114
	OffsetCreatedByLen = 118
	OffsetContentOff   = 120
	OffsetContentLen   = 124
	OffsetTagsOff      = 128
	OffsetTagsLen      = 132
	OffsetAssocOff     = 136
	OffsetAssocLen     = 140
	OffsetEmbedOff     = 144
	OffsetEmbedLen     = 148

	VariableDataStart = 152

	AssocRecordSize = 40

	MaxConceptBytes          = 512
	MaxCreatedByBytes        = 64
	MaxContentBytes          = 16 * 1024
	ContentCompressThreshold = 512

	CRC16Polynomial = 0x1021

	MetaKeySize = VariableDataStart + MaxConceptBytes
)

func MetaKeySlice(data []byte) []byte {
	n := min(MetaKeySize, len(data))
	return data[:n]
}

const (
	FlagHasEmbedding      uint8 = 1 << 0
	FlagContentCompressed uint8 = 1 << 1
	FlagEmbedQuantized    uint8 = 1 << 2
	FlagDormant           uint8 = 1 << 3
	FlagSoftDeleted       uint8 = 1 << 4
	FlagDirty             uint8 = 1 << 5
)

const (
	TagTypeLabel uint8 = 0x19
	TagSummary   uint8 = 0x1A
	TagKeyPoints uint8 = 0x1B
)

func appendTaggedString(buf []byte, tag uint8, s string) []byte {
	data := []byte(s)
	if len(data) > 0xFFFF {
		data = data[:0xFFFF]
	}
	buf = append(buf, tag)
	buf = append(buf, byte(len(data)>>8), byte(len(data)))
	buf = append(buf, data...)
	return buf
}

func appendTaggedBytes(buf []byte, tag uint8, data []byte) []byte {
	if len(data) > 0xFFFF {
		data = data[:0xFFFF]
	}
	buf = append(buf, tag)
	buf = append(buf, byte(len(data)>>8), byte(len(data)))
	buf = append(buf, data...)
	return buf
}
