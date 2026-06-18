package replication

import "errors"

var ErrStaleFencingToken = errors.New("replication: stale fencing token — request from old primary rejected")

func ValidateFencingToken(current, provided uint64) error {
	if provided != current {
		return ErrStaleFencingToken
	}
	return nil
}
