package replication

import (
	"errors"
	"fmt"
	"strings"
)

type ClusterConfig struct {
	Enabled                       bool      `json:"enabled"`
	NodeID                        string    `json:"node_id"`
	BindAddr                      string    `json:"bind_addr"`
	Seeds                         []string  `json:"seeds"`
	ClusterSecret                 string    `json:"cluster_secret"`
	Role                          string    `json:"role"`
	LeaseTTL                      int       `json:"lease_ttl"`
	HeartbeatMS                   int       `json:"heartbeat_ms"`
	SDOWNBeats                    int       `json:"sdown_beats"`
	CCSIntervalS                  int       `json:"ccs_interval_seconds"`
	ReconcileHeal                 bool      `json:"reconcile_on_heal"`
	TLS                           TLSConfig `json:"tls"`
	QuorumLossTimeoutSec          int       `json:"quorum_loss_timeout_sec"`
	JoinTokenTTLMin               int       `json:"join_token_ttl_min"`
	FailoverConvergenceTimeoutSec int       `json:"failover_convergence_timeout_sec"`
	HandoffAckTimeoutSec          int       `json:"handoff_ack_timeout_sec"`
	PruneIntervalSec              int       `json:"prune_interval_sec"`
	ReconDelayMs                  int       `json:"recon_delay_ms"`
}

type TLSConfig struct {
	Enabled    bool   `json:"enabled"`
	CAFile     string `json:"ca_file"`
	CertFile   string `json:"cert_file"`
	KeyFile    string `json:"key_file"`
	AutoGenDir string `json:"auto_gen_dir"`
}

func DefaultClusterConfig() ClusterConfig {
	return ClusterConfig{
		Enabled:                       false,
		Role:                          "auto",
		LeaseTTL:                      10,
		HeartbeatMS:                   1000,
		SDOWNBeats:                    3,
		CCSIntervalS:                  30,
		ReconcileHeal:                 true,
		QuorumLossTimeoutSec:          5,
		JoinTokenTTLMin:               15,
		FailoverConvergenceTimeoutSec: 30,
		HandoffAckTimeoutSec:          5,
		PruneIntervalSec:              60,
		ReconDelayMs:                  2000,
	}
}

var validRoles = map[string]bool{
	"auto":     true,
	"primary":  true,
	"replica":  true,
	"sentinel": true,
	"observer": true,
}

func (c ClusterConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if strings.TrimSpace(c.NodeID) == "" {
		return errors.New("cluster: node_id must not be empty when cluster is enabled")
	}
	if len(c.Seeds) == 0 && c.Role != "primary" {
		return errors.New("cluster: seeds must have at least one entry when cluster is enabled and role is not primary")
	}
	if !validRoles[c.Role] {
		return fmt.Errorf("cluster: invalid role %q — must be one of: auto, primary, replica, sentinel, observer", c.Role)
	}
	if c.LeaseTTL <= 0 {
		return errors.New("cluster: lease_ttl must be > 0")
	}
	if c.HeartbeatMS <= 0 {
		return errors.New("cluster: heartbeat_ms must be > 0")
	}
	if c.QuorumLossTimeoutSec < 0 {
		return errors.New("cluster: quorum_loss_timeout_sec must not be negative")
	}
	if c.JoinTokenTTLMin < 0 {
		return errors.New("cluster: join_token_ttl_min must not be negative")
	}
	if c.FailoverConvergenceTimeoutSec < 0 {
		return errors.New("cluster: failover_convergence_timeout_sec must not be negative")
	}
	if c.HandoffAckTimeoutSec < 0 {
		return errors.New("cluster: handoff_ack_timeout_sec must not be negative")
	}
	if c.PruneIntervalSec < 0 {
		return errors.New("cluster: prune_interval_sec must not be negative")
	}
	if c.ReconDelayMs < 0 {
		return errors.New("cluster: recon_delay_ms must not be negative")
	}
	return nil
}
