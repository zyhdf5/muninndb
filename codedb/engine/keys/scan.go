package keys

// PrefixLowerBound 返回前缀扫描的起始下界（包含）。
func PrefixLowerBound(prefix []byte) []byte {
	return prefix
}

// PrefixUpperBound 返回前缀扫描的排他上界。
// 递增最后一个字节；全部溢出时追加 0x00。
func PrefixUpperBound(prefix []byte) []byte {
	if len(prefix) == 0 {
		return []byte{0x01}
	}

	bound := make([]byte, len(prefix))
	copy(bound, prefix)

	for i := len(bound) - 1; i >= 0; i-- {
		if bound[i] < 0xFF {
			bound[i]++
			return bound
		}
	}

	return append(prefix, 0x00)
}
