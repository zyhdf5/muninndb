package erf

import "time"

type Engram struct {
	ID             [16]byte
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastAccess     time.Time
	Confidence     float32
	Relevance      float32
	Stability      float32
	AccessCount    uint32
	State          uint8
	EmbedDim       uint8
	Concept        string
	CreatedBy      string
	Content        string
	Tags           []string
	Associations   []Association
	Embedding      []float32
	Summary        string
	KeyPoints      []string
	MemoryType     uint8
	TypeLabel      string
	Classification uint16
}

type EngramMeta struct {
	ID          [16]byte
	CreatedAt   time.Time
	UpdatedAt   time.Time
	LastAccess  time.Time
	Confidence  float32
	Relevance   float32
	Stability   float32
	AccessCount uint32
	State       uint8
	AssocCount  uint16
	EmbedDim    uint8
	MemoryType  uint8
}
