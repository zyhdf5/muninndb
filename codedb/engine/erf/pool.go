package erf

import "sync"

type erfBuffer struct {
	buf []byte
}

func (b *erfBuffer) Reset() {
	b.buf = b.buf[:0]
}

func (b *erfBuffer) Bytes() []byte {
	return b.buf
}

var erfPool = &sync.Pool{
	New: func() any {
		return &erfBuffer{buf: make([]byte, 0, 8192)}
	},
}

func GetBuffer() *erfBuffer {
	return erfPool.Get().(*erfBuffer)
}

func PutBuffer(b *erfBuffer) {
	if cap(b.buf) > 32*1024 {
		return
	}
	b.Reset()
	erfPool.Put(b)
}
