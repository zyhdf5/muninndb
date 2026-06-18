package engine

// DimFromLen 将向量长度（float32 元素数或量化后字节数）映射为 EmbedDimension 枚举值。
// 对于非零的未知长度返回 EmbedOther。
func DimFromLen(n int) EmbedDimension {
	switch n {
	case 0:
		return EmbedNone
	case 384:
		return Embed384
	case 768:
		return Embed768
	case 1536:
		return Embed1536
	case 3072:
		return Embed3072
	default:
		if n > 0 {
			return EmbedOther
		}
		return EmbedNone
	}
}
