package erf

import (
	"sync"

	"github.com/klauspost/compress/zstd"
)

var zstdEncoderPool = &sync.Pool{
	New: func() any {
		enc, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest))
		return enc
	},
}

var zstdDecoderPool = &sync.Pool{
	New: func() any {
		dec, _ := zstd.NewReader(nil)
		return dec
	},
}

func Compress(content []byte) ([]byte, bool) {
	if len(content) <= ContentCompressThreshold {
		return content, false
	}

	enc := zstdEncoderPool.Get().(*zstd.Encoder)
	defer zstdEncoderPool.Put(enc)
	return enc.EncodeAll(content, nil), true
}

func Decompress(compressed []byte) ([]byte, error) {
	dec := zstdDecoderPool.Get().(*zstd.Decoder)
	defer zstdDecoderPool.Put(dec)
	return dec.DecodeAll(compressed, nil)
}
