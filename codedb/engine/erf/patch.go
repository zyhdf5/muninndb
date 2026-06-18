package erf

import (
	"encoding/binary"
	"errors"
	"math"
	"time"
)

func PatchEmbedDim(raw []byte, dim uint8) error {
	if len(raw) < VariableDataStart+TrailerSize {
		return errors.New("erf: record too short for PatchEmbedDim")
	}
	raw[OffsetEmbedDim] = dim
	binary.BigEndian.PutUint32(raw[len(raw)-TrailerSize:], ComputeCRC32(raw[:len(raw)-TrailerSize]))
	return nil
}

func PatchRelevance(raw []byte, updatedAt time.Time, relevance, stability float32) error {
	if len(raw) < VariableDataStart+TrailerSize {
		return errors.New("erf: record too short for PatchRelevance")
	}
	binary.BigEndian.PutUint64(raw[OffsetUpdatedAt:OffsetUpdatedAt+8], uint64(updatedAt.UnixNano()))
	binary.BigEndian.PutUint32(raw[OffsetRelevance:OffsetRelevance+4], math.Float32bits(relevance))
	binary.BigEndian.PutUint32(raw[OffsetStability:OffsetStability+4], math.Float32bits(stability))
	binary.BigEndian.PutUint32(raw[len(raw)-TrailerSize:], ComputeCRC32(raw[:len(raw)-TrailerSize]))
	return nil
}

func PatchAllMeta(raw []byte, updatedAt, lastAccess time.Time, confidence, relevance, stability float32, accessCount uint32, state uint8) error {
	if len(raw) < VariableDataStart+TrailerSize {
		return errors.New("erf: record too short for PatchAllMeta")
	}
	binary.BigEndian.PutUint64(raw[OffsetUpdatedAt:OffsetUpdatedAt+8], uint64(updatedAt.UnixNano()))
	binary.BigEndian.PutUint64(raw[OffsetLastAccess:OffsetLastAccess+8], uint64(lastAccess.UnixNano()))
	binary.BigEndian.PutUint32(raw[OffsetConfidence:OffsetConfidence+4], math.Float32bits(confidence))
	binary.BigEndian.PutUint32(raw[OffsetRelevance:OffsetRelevance+4], math.Float32bits(relevance))
	binary.BigEndian.PutUint32(raw[OffsetStability:OffsetStability+4], math.Float32bits(stability))
	binary.BigEndian.PutUint32(raw[OffsetAccessCount:OffsetAccessCount+4], accessCount)
	raw[OffsetState] = state
	binary.BigEndian.PutUint32(raw[len(raw)-TrailerSize:], ComputeCRC32(raw[:len(raw)-TrailerSize]))
	return nil
}
