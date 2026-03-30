package erf

import "sync"

// erfBuffer is a reusable byte buffer for ERF encoding.
type erfBuffer struct {
	buf []byte
}

// Reset clears the buffer for reuse.
func (b *erfBuffer) Reset() {
	b.buf = b.buf[:0]
}

// Bytes returns the current buffer contents.
func (b *erfBuffer) Bytes() []byte {
	return b.buf
}

// erfPool is a sync.Pool for zero-allocation write paths.
var erfPool = &sync.Pool{
	New: func() any {
		return &erfBuffer{buf: make([]byte, 0, 8192)}
	},
}

// GetBuffer borrows a buffer from the pool.
func GetBuffer() *erfBuffer {
	return erfPool.Get().(*erfBuffer)
}

// PutBuffer returns a buffer to the pool.
func PutBuffer(b *erfBuffer) {
	if cap(b.buf) > 32*1024 { // don't pool buffers that grew too large
		return
	}
	b.Reset()
	erfPool.Put(b)
}
