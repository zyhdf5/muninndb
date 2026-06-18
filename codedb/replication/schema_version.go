package replication

import (
	"encoding/binary"
	"fmt"
)

const CurrentSchemaVersion uint64 = 1

func schemaVersionKey() []byte {
	return []byte{0x19, 0x03, 's', 'c', 'h', 'e', 'm', 'a', '_', 'v'}
}

func CheckAndSetSchemaVersion(db KVStore) error {
	stored, err := readSchemaVersion(db)
	if err != nil {
		return fmt.Errorf("schema version: read: %w", err)
	}
	if stored == 0 {
		return writeSchemaVersion(db, CurrentSchemaVersion)
	}
	if stored > CurrentSchemaVersion {
		return fmt.Errorf("schema version: database was written by a newer binary (stored=%d, current=%d)", stored, CurrentSchemaVersion)
	}
	if stored < CurrentSchemaVersion {
		return writeSchemaVersion(db, CurrentSchemaVersion)
	}
	return nil
}

func readSchemaVersion(db KVStore) (uint64, error) {
	val, closer, err := db.Get(schemaVersionKey())
	if err == ErrKeyNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer closer.Close()
	if len(val) < 8 {
		return 0, fmt.Errorf("schema version: corrupt value (len=%d)", len(val))
	}
	return binary.BigEndian.Uint64(val), nil
}

func writeSchemaVersion(db KVStore, v uint64) error {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, v)
	return db.Set(schemaVersionKey(), buf)
}
