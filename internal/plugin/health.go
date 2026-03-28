package plugin

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// PluginHealthChecker periodically checks the health of registered plugins.
type PluginHealthChecker struct {
	registry *Registry
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewHealthChecker creates a new PluginHealthChecker.
func NewHealthChecker(registry *Registry) *PluginHealthChecker {
	return &PluginHealthChecker{
		registry: registry,
		stopCh:   make(chan struct{}),
	}
}

// Start launches the background health checking goroutine.
// It will:
// - Check embed plugins every 30 seconds
// - Check enrich plugins every 60 seconds
// - Mark plugins unhealthy after 3 consecutive failures
// - Mark plugins healthy again on recovery
func (h *PluginHealthChecker) Start(ctx context.Context) {
	h.wg.Add(1)
	go h.run(ctx)
}

// Stop gracefully shuts down the health checker.
func (h *PluginHealthChecker) Stop() {
	close(h.stopCh)
	h.wg.Wait()
}

func (h *PluginHealthChecker) run(ctx context.Context) {
	defer h.wg.Done()

	// Track consecutive failures per plugin
	failures := make(map[string]int)

	embedTicker := time.NewTicker(30 * time.Second)
	defer embedTicker.Stop()

	enrichTicker := time.NewTicker(60 * time.Second)
	defer enrichTicker.Stop()

	for {
		select {
		case <-h.stopCh:
			return
		case <-ctx.Done():
			return
		case <-embedTicker.C:
			// Check embed plugin
			embed := h.registry.GetEmbed()
			if embed != nil {
				name := embed.Name()
				err := h.probeEmbed(ctx, embed)
				if err != nil {
					failures[name]++
					if failures[name] >= 3 {
						h.registry.SetHealthy(name, false)
						slog.Warn("plugin marked unhealthy", "name", name, "error", err)
					}
				} else {
					// Success - reset failure count and mark healthy
					failures[name] = 0
					h.registry.SetHealthy(name, true)
				}
			}

		case <-enrichTicker.C:
			// Check enrich plugin
			enrich := h.registry.GetEnrich()
			if enrich != nil {
				name := enrich.Name()
				err := h.probeEnrich(ctx, enrich)
				if err != nil {
					failures[name]++
					if failures[name] >= 3 {
						h.registry.SetHealthy(name, false)
						slog.Warn("plugin marked unhealthy", "name", name, "error", err)
					}
				} else {
					// Success - reset failure count and mark healthy
					failures[name] = 0
					h.registry.SetHealthy(name, true)
				}
			}
		}
	}
}

// probeEmbed attempts to embed a health check text.
func (h *PluginHealthChecker) probeEmbed(ctx context.Context, embed EmbedPlugin) error {
	_, err := embed.Embed(ctx, []string{"health check"})
	return err
}

// probeEnrich attempts to enrich a dummy engram.
func (h *PluginHealthChecker) probeEnrich(ctx context.Context, enrich EnrichPlugin) error {
	dummy := &Engram{
		Concept: "health",
		Content: "check",
	}
	_, err := enrich.Enrich(ctx, dummy)
	return err
}
