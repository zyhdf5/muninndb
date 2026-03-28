package erf

import (
	"encoding/binary"
	"errors"
	"math"
	"time"
)

// Association - local copy to avoid circular imports
type Association struct {
	TargetID      [16]byte
	RelType       uint16
	Weight        float32
	Confidence    float32
	CreatedAt     time.Time
	LastActivated int32
}

// EncodeAssociation encodes an Association into exactly 40 bytes.
func EncodeAssociation(buf []byte, a *Association) error {
	if len(buf) != AssocRecordSize {
		return errors.New("association buffer must be exactly 40 bytes")
	}

	copy(buf[0:16], a.TargetID[:])
	binary.BigEndian.PutUint16(buf[16:18], a.RelType)
	binary.BigEndian.PutUint32(buf[18:22], math.Float32bits(a.Weight))
	binary.BigEndian.PutUint32(buf[22:26], math.Float32bits(a.Confidence))
	binary.BigEndian.PutUint64(buf[26:34], uint64(a.CreatedAt.UnixNano()))
	binary.BigEndian.PutUint32(buf[34:38], uint32(a.LastActivated))
	buf[38] = 0
	buf[39] = 0

	return nil
}

// DecodeAssociation decodes an Association from exactly 40 bytes.
func DecodeAssociation(buf []byte) (Association, error) {
	if len(buf) != AssocRecordSize {
		return Association{}, errors.New("association buffer must be exactly 40 bytes")
	}

	var assoc Association
	copy(assoc.TargetID[:], buf[0:16])
	assoc.RelType = binary.BigEndian.Uint16(buf[16:18])
	assoc.Weight = math.Float32frombits(binary.BigEndian.Uint32(buf[18:22]))
	assoc.Confidence = math.Float32frombits(binary.BigEndian.Uint32(buf[22:26]))
	nanos := int64(binary.BigEndian.Uint64(buf[26:34]))
	assoc.CreatedAt = time.Unix(0, nanos)
	assoc.LastActivated = int32(binary.BigEndian.Uint32(buf[34:38]))

	return assoc, nil
}
