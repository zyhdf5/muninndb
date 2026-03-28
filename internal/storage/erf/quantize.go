package erf

import (
	"encoding/binary"
	"math"
)

// QuantizeParams holds the scale and zero point for quantization.
type QuantizeParams struct {
	Scale     float32
	ZeroPoint float32
}

// Quantize converts float32 embeddings to int8 with per-vector scale and zero_point.
func Quantize(vec []float32) (QuantizeParams, []int8) {
	if len(vec) == 0 {
		return QuantizeParams{}, nil
	}

	minVal, maxVal := vec[0], vec[0]
	for _, v := range vec[1:] {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	scale := (maxVal - minVal) / 255.0
	if scale == 0 {
		scale = 1.0 // prevent division by zero
	}
	zeroPoint := minVal

	quantized := make([]int8, len(vec))
	for i, v := range vec {
		normalized := (v - zeroPoint) / scale
		quantized[i] = int8(math.Round(float64(normalized)) - 128)
	}

	return QuantizeParams{Scale: scale, ZeroPoint: zeroPoint}, quantized
}

// Dequantize converts quantized int8 embeddings back to float32.
func Dequantize(quantized []int8, params QuantizeParams) []float32 {
	out := make([]float32, len(quantized))
	for i, v := range quantized {
		out[i] = (float32(v)+128)*params.Scale + params.ZeroPoint
	}
	return out
}

// EncodeQuantizeParams encodes scale and zero_point to 8 bytes.
func EncodeQuantizeParams(params QuantizeParams) [8]byte {
	var buf [8]byte
	binary.BigEndian.PutUint32(buf[0:4], math.Float32bits(params.Scale))
	binary.BigEndian.PutUint32(buf[4:8], math.Float32bits(params.ZeroPoint))
	return buf
}

// DecodeQuantizeParams decodes scale and zero_point from 8 bytes.
func DecodeQuantizeParams(buf [8]byte) QuantizeParams {
	scale := math.Float32frombits(binary.BigEndian.Uint32(buf[0:4]))
	zeroPoint := math.Float32frombits(binary.BigEndian.Uint32(buf[4:8]))
	return QuantizeParams{Scale: scale, ZeroPoint: zeroPoint}
}
