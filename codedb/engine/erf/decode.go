package erf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

func Decode(data []byte) (*Engram, error) {
	if len(data) < FixedOverhead {
		return nil, errors.New("data too short for ERF record")
	}
	if !VerifyCRC16(data[:8]) {
		return nil, errors.New("erf crc16 check failed")
	}

	ver := data[4]
	if ver != Version && ver != Version2 {
		return nil, fmt.Errorf("unsupported erf version 0x%02x", ver)
	}

	flags := data[5]

	conceptOff := binary.BigEndian.Uint32(data[OffsetConceptOff : OffsetConceptOff+4])
	conceptLen := binary.BigEndian.Uint16(data[OffsetConceptLen : OffsetConceptLen+2])
	createdByOff := binary.BigEndian.Uint32(data[OffsetCreatedByOff : OffsetCreatedByOff+4])
	createdByLen := binary.BigEndian.Uint16(data[OffsetCreatedByLen : OffsetCreatedByLen+2])
	contentOff := binary.BigEndian.Uint32(data[OffsetContentOff : OffsetContentOff+4])
	contentLen := binary.BigEndian.Uint32(data[OffsetContentLen : OffsetContentLen+4])
	tagsOff := binary.BigEndian.Uint32(data[OffsetTagsOff : OffsetTagsOff+4])
	tagsLen := binary.BigEndian.Uint32(data[OffsetTagsLen : OffsetTagsLen+4])
	assocOff := binary.BigEndian.Uint32(data[OffsetAssocOff : OffsetAssocOff+4])
	assocLen := binary.BigEndian.Uint32(data[OffsetAssocLen : OffsetAssocLen+4])
	embedOff := binary.BigEndian.Uint32(data[OffsetEmbedOff : OffsetEmbedOff+4])
	embedLen := binary.BigEndian.Uint32(data[OffsetEmbedLen : OffsetEmbedLen+4])

	dataLen := uint64(len(data))
	if uint64(conceptOff)+uint64(conceptLen) > dataLen {
		return nil, errors.New("erf: concept field out of bounds")
	}
	if uint64(createdByOff)+uint64(createdByLen) > dataLen {
		return nil, errors.New("erf: createdBy field out of bounds")
	}
	if uint64(contentOff)+uint64(contentLen) > dataLen {
		return nil, errors.New("erf: content field out of bounds")
	}
	if uint64(tagsOff)+uint64(tagsLen) > dataLen {
		return nil, errors.New("erf: tags field out of bounds")
	}
	if uint64(assocOff)+uint64(assocLen) > dataLen {
		return nil, errors.New("erf: associations field out of bounds")
	}
	if uint64(embedOff)+uint64(embedLen) > dataLen {
		return nil, errors.New("erf: embedding field out of bounds")
	}

	concept := string(data[conceptOff : conceptOff+uint32(conceptLen)])
	createdBy := string(data[createdByOff : createdByOff+uint32(createdByLen)])

	content := data[contentOff : contentOff+contentLen]
	if flags&FlagContentCompressed != 0 {
		dec, err := Decompress(content)
		if err != nil {
			return nil, err
		}
		content = dec
	}

	var tags []string
	if tagsLen > 0 {
		if err := msgpack.Unmarshal(data[tagsOff:tagsOff+tagsLen], &tags); err != nil {
			return nil, err
		}
	}

	var associations []Association
	if assocLen > 0 {
		if assocLen%uint32(AssocRecordSize) != 0 {
			return nil, errors.New("erf: association data length not multiple of record size")
		}
		n := assocLen / uint32(AssocRecordSize)
		associations = make([]Association, n)
		for i := range int(n) {
			off := assocOff + uint32(i)*uint32(AssocRecordSize)
			a, err := DecodeAssociation(data[off : off+uint32(AssocRecordSize)])
			if err != nil {
				return nil, err
			}
			associations[i] = a
		}
	}

	var embedding []float32
	if flags&FlagHasEmbedding != 0 && embedLen > 0 {
		if flags&FlagEmbedQuantized != 0 {
			if embedLen < 8 {
				return nil, errors.New("erf: quantized embedding too short")
			}
			params := DecodeQuantizeParams([8]byte(data[embedOff : embedOff+8]))
			q := make([]int8, embedLen-8)
			for i := range len(q) {
				q[i] = int8(data[embedOff+8+uint32(i)])
			}
			embedding = Dequantize(q, params)
		} else {
			if embedLen%4 != 0 {
				return nil, errors.New("erf: embedding length not multiple of 4")
			}
			n := embedLen / 4
			embedding = make([]float32, n)
			for i := range int(n) {
				off := embedOff + uint32(i)*4
				embedding[i] = math.Float32frombits(binary.BigEndian.Uint32(data[off : off+4]))
			}
		}
	}

	eng := &Engram{
		Concept:      concept,
		CreatedBy:    createdBy,
		Content:      string(content),
		Tags:         tags,
		Associations: associations,
		Embedding:    embedding,
	}

	copy(eng.ID[:], data[OffsetID:OffsetID+16])
	eng.CreatedAt = time.Unix(0, int64(binary.BigEndian.Uint64(data[OffsetCreatedAt:OffsetCreatedAt+8])))
	eng.UpdatedAt = time.Unix(0, int64(binary.BigEndian.Uint64(data[OffsetUpdatedAt:OffsetUpdatedAt+8])))
	eng.LastAccess = time.Unix(0, int64(binary.BigEndian.Uint64(data[OffsetLastAccess:OffsetLastAccess+8])))
	eng.Confidence = math.Float32frombits(binary.BigEndian.Uint32(data[OffsetConfidence : OffsetConfidence+4]))
	eng.Relevance = math.Float32frombits(binary.BigEndian.Uint32(data[OffsetRelevance : OffsetRelevance+4]))
	eng.Stability = math.Float32frombits(binary.BigEndian.Uint32(data[OffsetStability : OffsetStability+4]))
	eng.AccessCount = binary.BigEndian.Uint32(data[OffsetAccessCount : OffsetAccessCount+4])
	eng.State = data[OffsetState]
	eng.EmbedDim = data[OffsetEmbedDim]
	eng.MemoryType = data[OffsetMemoryType]
	eng.Classification = binary.BigEndian.Uint16(data[OffsetClassification : OffsetClassification+2])

	varEnd := maxVarEnd(conceptOff, uint32(conceptLen), createdByOff, uint32(createdByLen), contentOff, contentLen, tagsOff, tagsLen, assocOff, assocLen, embedOff, embedLen)
	if trailer := uint32(len(data)) - 4; varEnd < trailer {
		parseTaggedFields(data[varEnd:trailer], eng)
	}

	if !VerifyCRC32(data) {
		return nil, errors.New("erf crc32 check failed")
	}

	return eng, nil
}

func maxVarEnd(pairs ...uint32) uint32 {
	var m uint32
	for i := 0; i+1 < len(pairs); i += 2 {
		if end := pairs[i] + pairs[i+1]; end > m {
			m = end
		}
	}
	return m
}

func parseTaggedFields(buf []byte, eng *Engram) {
	for len(buf) >= 3 {
		tag := buf[0]
		length := int(buf[1])<<8 | int(buf[2])
		buf = buf[3:]
		if len(buf) < length {
			return
		}

		switch tag {
		case TagTypeLabel:
			eng.TypeLabel = string(buf[:length])
		case TagSummary:
			eng.Summary = string(buf[:length])
		case TagKeyPoints:
			var kp []string
			if err := msgpack.Unmarshal(buf[:length], &kp); err == nil {
				eng.KeyPoints = kp
			}
		}
		buf = buf[length:]
	}
}
