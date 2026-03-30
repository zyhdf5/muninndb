package mbp

import (
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

// EncodeMsgpack encodes a value to msgpack bytes.
func EncodeMsgpack(v interface{}) ([]byte, error) {
	b, err := msgpack.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("msgpack encode: %w", err)
	}
	return b, nil
}

// DecodeMsgpack decodes msgpack bytes into a value.
func DecodeMsgpack(data []byte, v interface{}) error {
	err := msgpack.Unmarshal(data, v)
	if err != nil {
		return fmt.Errorf("msgpack decode: %w", err)
	}
	return nil
}
