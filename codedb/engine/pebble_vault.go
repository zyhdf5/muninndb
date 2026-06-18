package engine

import (
	"fmt"

	"github.com/cockroachdb/pebble"
	"github.com/scrypster/muninndb/codedb/engine/keys"
)

// WriteVaultName 持久化 vault 名称并维护名称索引。
func (ps *PebbleStore) WriteVaultName(wsPrefix [8]byte, name string) error {
	if _, ok := ps.vaultNameWritten.Load(wsPrefix); ok {
		return nil
	}

	metaKey := keys.VaultMetaKey(wsPrefix)
	if _, closer, err := ps.db.Get(metaKey); err == nil {
		closer.Close()
		ps.vaultNameWritten.Store(wsPrefix, struct{}{})
		ps.vaultPrefixCache.Add(name, wsPrefix)
		return nil
	}

	batch := ps.db.NewBatch()
	defer batch.Close()
	if err := batch.Set(metaKey, []byte(name), nil); err != nil {
		return fmt.Errorf("写入 vault 元数据失败: %w", err)
	}
	if err := batch.Set(keys.VaultNameIndexKey(name), wsPrefix[:], nil); err != nil {
		return fmt.Errorf("写入 vault 名称索引失败: %w", err)
	}
	if err := batch.Commit(pebble.NoSync); err != nil {
		return fmt.Errorf("提交 vault 名称写入失败: %w", err)
	}

	ps.vaultNameWritten.Store(wsPrefix, struct{}{})
	ps.vaultPrefixCache.Add(name, wsPrefix)
	return nil
}

// ResolveVaultPrefix 通过缓存与索引将名称解析为 8 字节前缀。
func (ps *PebbleStore) ResolveVaultPrefix(name string) [8]byte {
	if ws, ok := ps.vaultPrefixCache.Get(name); ok {
		return ws
	}

	idxKey := keys.VaultNameIndexKey(name)
	if val, closer, err := ps.db.Get(idxKey); err == nil {
		defer closer.Close()
		if len(val) == 8 {
			var ws [8]byte
			copy(ws[:], val)
			ps.vaultPrefixCache.Add(name, ws)
			return ws
		}
	}

	ws := keys.VaultPrefix(name)
	ps.vaultPrefixCache.Add(name, ws)
	return ws
}

// ListVaultNames 扫描 0x0F 名称索引并返回所有名称。
func (ps *PebbleStore) ListVaultNames() ([]string, error) {
	iter, err := ps.db.NewIter(&pebble.IterOptions{LowerBound: []byte{0x0F}, UpperBound: []byte{0x10}})
	if err != nil {
		return nil, fmt.Errorf("创建 vault 名称扫描迭代器失败: %w", err)
	}
	defer iter.Close()

	names := make([]string, 0, 16)
	for ok := iter.First(); ok; ok = iter.Next() {
		k := iter.Key()
		if len(k) != 9 || k[0] != 0x0F {
			continue
		}
		v := iter.Value()
		if len(v) != 8 {
			continue
		}
		var ws [8]byte
		copy(ws[:], v)
		nameBytes, err := Get(ps.db, keys.VaultMetaKey(ws))
		if err != nil || len(nameBytes) == 0 {
			continue
		}
		names = append(names, string(nameBytes))
	}
	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("扫描 vault 名称失败: %w", err)
	}
	return names, nil
}
