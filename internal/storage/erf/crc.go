package erf

import (
	"encoding/binary"
	"hash/crc32"
)

var crc32Table = crc32.MakeTable(crc32.Castagnoli)

// ComputeCRC16 computes CRC-16/CCITT-FALSE for header bytes (6 bytes: 0-5).
// The polynomial is 0x1021.
func ComputeCRC16(data []byte) uint16 {
	crc := uint32(0xFFFF)
	for _, b := range data {
		crc ^= uint32(b) << 8
		for i := 0; i < 8; i++ {
			crc <<= 1
			if crc&0x10000 != 0 {
				crc ^= 0x1021
			}
		}
	}
	return uint16(crc ^ 0xFFFF)
}

// VerifyCRC16 checks the CRC16 at bytes 6-7 against bytes 0-5.
func VerifyCRC16(data []byte) bool {
	if len(data) < 8 {
		return false
	}
	expected := binary.BigEndian.Uint16(data[6:8])
	computed := ComputeCRC16(data[0:6])
	return expected == computed
}

// ComputeCRC32 computes CRC32 Castagnoli for the entire record (including header+metadata+offset table+variable data).
func ComputeCRC32(data []byte) uint32 {
	return crc32.Checksum(data, crc32Table)
}

// VerifyCRC32 checks the CRC32 trailer (last 4 bytes) against the rest of the record.
func VerifyCRC32(data []byte) bool {
	if len(data) < 5 { // at least 1 byte of payload + 4 byte trailer
		return false
	}
	trailerPos := len(data) - 4
	expected := binary.BigEndian.Uint32(data[trailerPos : trailerPos+4])
	computed := ComputeCRC32(data[:trailerPos])
	return expected == computed
}
