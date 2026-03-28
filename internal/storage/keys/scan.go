package keys

// PrefixLowerBound returns the lower bound for a prefix scan (inclusive).
func PrefixLowerBound(prefix []byte) []byte {
	return prefix
}

// PrefixUpperBound returns the exclusive upper bound for a prefix scan.
// Increments the last byte and returns, or appends 0x00 if overflow.
func PrefixUpperBound(prefix []byte) []byte {
	if len(prefix) == 0 {
		return []byte{0x01}
	}

	// Try to increment the last byte
	bound := make([]byte, len(prefix))
	copy(bound, prefix)

	for i := len(bound) - 1; i >= 0; i-- {
		if bound[i] < 0xFF {
			bound[i]++
			return bound
		}
		// Overflow: keep iterating to next byte
	}

	// All bytes are 0xFF, append a new byte
	return append(prefix, 0x00)
}
