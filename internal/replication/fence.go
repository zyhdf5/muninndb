package replication

import "errors"

var (
	// ErrStaleFencingToken is returned when a write request contains a fencing token
	// that is less than the current token. This indicates the request came from a
	// demoted primary and should be rejected to prevent split-brain writes.
	ErrStaleFencingToken = errors.New("replication: stale fencing token — request from old primary rejected")
)

// ValidateFencingToken checks if the provided token is valid against the current token.
// Returns ErrStaleFencingToken if the provided token does not exactly equal the
// current token. This prevents writes from both demoted primaries (token < current)
// and any node operating with a stale or future token (token > current).
//
// Strict equality is safe because the fencing token IS the epoch: a node's
// current epoch should always match the cluster's current epoch. A token greater
// than the current epoch indicates a logic error (e.g., a node that somehow
// advanced its epoch without cluster consensus) and must also be rejected.
// The HandleHandoff path increments the epoch on the target before it can accept
// writes, so legitimate writes always carry the exact current epoch.
func ValidateFencingToken(current, provided uint64) error {
	if provided != current {
		return ErrStaleFencingToken
	}
	return nil
}
