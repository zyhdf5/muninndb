package erf

import (
	"encoding/binary"
	"math"
)

type QuantizeParams struct {
	Scale     float32
	ZeroPoint float32
}

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
		scale = 1
	}

	quantized := make([]int8, len(vec))
	for i, v := range vec {
		quantized[i] = int8(math.Round(float64((v-minVal)/scale)) - 128)
	}

	return QuantizeParams{Scale: scale, ZeroPoint: minVal}, quantized
}

func Dequantize(quantized []int8, params QuantizeParams) []float32 {
	out := make([]float32, len(quantized))
	for i, v := range quantized {
		out[i] = (float32(v)+128)*params.Scale + params.ZeroPoint
	}
	return out
}

func EncodeQuantizeParams(params QuantizeParams) [8]byte {
	var buf [8]byte
	binary.BigEndian.PutUint32(buf[0:4], math.Float32bits(params.Scale))
	binary.BigEndian.PutUint32(buf[4:8], math.Float32bits(params.ZeroPoint))
	return buf
}

func DecodeQuantizeParams(buf [8]byte) QuantizeParams {
	return QuantizeParams{
		Scale:     math.Float32frombits(binary.BigEndian.Uint32(buf[0:4])),
		ZeroPoint: math.Float32frombits(binary.BigEndian.Uint32(buf[4:8])),
	}
}
