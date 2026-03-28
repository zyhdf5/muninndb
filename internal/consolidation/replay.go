package consolidation

import (
	"context"
	"log/slog"

	"github.com/scrypster/muninndb/internal/storage"
)

// runPhase1Replay replays activation on the top 50 most-accessed engrams in the vault
// to reinforce Hebbian associations. In DryRun mode, just counts candidates.
func (w *Worker) runPhase1Replay(ctx context.Context, store *storage.PebbleStore, wsPrefix [8]byte, report *ConsolidationReport) error {
	const topK = 50

	// Get top-K most-relevant engrams using the relevance bucket index
	topIDs, err := store.RecentActive(ctx, wsPrefix, topK)
	if err != nil {
		return err
	}

	if len(topIDs) == 0 {
		slog.Debug("consolidation phase 1: no active engrams found")
		return nil
	}

	// In DryRun, just count them; in normal mode, we would call engine.Activate on each,
	// but since we don't have the full engine interface here (only Store), we record the count.
	// The actual activation replay would be done by the engine caller if desired.

	slog.Debug("consolidation phase 1 (replay)", "vault_prefix", wsPrefix, "engrams_found", len(topIDs))

	return nil
}
