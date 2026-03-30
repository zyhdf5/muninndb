package erf

import (
	"sync"

	"github.com/klauspost/compress/zstd"
)

// zstdEncoderPool is a sync.Pool for zstd encoders.
var zstdEncoderPool = &sync.Pool{
	New: func() any {
		enc, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest))
		return enc
	},
}

// zstdDecoderPool is a sync.Pool for zstd decoders.
var zstdDecoderPool = &sync.Pool{
	New: func() any {
		dec, _ := zstd.NewReader(nil)
		return dec
	},
}

// Compress compresses content using zstd level 1 if size > ContentCompressThreshold.
// Returns (compressed data, wasCompressed).
func Compress(content []byte) ([]byte, bool) {
	if len(content) <= ContentCompressThreshold {
		return content, false
	}

	enc := zstdEncoderPool.Get().(*zstd.Encoder)
	defer zstdEncoderPool.Put(enc)

	compressed := enc.EncodeAll(content, nil)
	return compressed, true
}

// Decompress decompresses zstd-compressed content.
func Decompress(compressed []byte) ([]byte, error) {
	dec := zstdDecoderPool.Get().(*zstd.Decoder)
	defer zstdDecoderPool.Put(dec)

	decompressed, err := dec.DecodeAll(compressed, nil)
	if err != nil {
		return nil, err
	}
	return decompressed, nil
}
