package erf

import (
	"encoding/binary"
	"hash/crc32"
)

var crc32Table = crc32.MakeTable(crc32.Castagnoli)

func ComputeCRC16(data []byte) uint16 {
	crc := uint32(0xFFFF)
	for _, b := range data {
		crc ^= uint32(b) << 8
		for range 8 {
			crc <<= 1
			if crc&0x10000 != 0 {
				crc ^= CRC16Polynomial
			}
		}
	}
	return uint16(crc ^ 0xFFFF)
}

func VerifyCRC16(data []byte) bool {
	if len(data) < 8 {
		return false
	}
	return binary.BigEndian.Uint16(data[6:8]) == ComputeCRC16(data[:6])
}

func ComputeCRC32(data []byte) uint32 {
	return crc32.Checksum(data, crc32Table)
}

func VerifyCRC32(data []byte) bool {
	if len(data) < 5 {
		return false
	}
	trailerPos := len(data) - 4
	return binary.BigEndian.Uint32(data[trailerPos:]) == ComputeCRC32(data[:trailerPos])
}
