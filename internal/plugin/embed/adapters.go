package embed

import (
	"context"
	"strings"

	"github.com/scrypster/muninndb/internal/engine/activation"
	"github.com/scrypster/muninndb/internal/plugin"
)

// embedServiceAdapter wraps plugin.EmbedPlugin to satisfy activation.Embedder.
type embedServiceAdapter struct {
	svc plugin.EmbedPlugin
}

func (a *embedServiceAdapter) Embed(ctx context.Context, texts []string) ([]float32, error) {
	return a.svc.Embed(ctx, texts)
}

func (a *embedServiceAdapter) Tokenize(text string) []string {
	return strings.Fields(text)
}

// NewEmbedServiceAdapter returns an activation.Embedder backed by the given EmbedPlugin.
func NewEmbedServiceAdapter(svc plugin.EmbedPlugin) activation.Embedder {
	return &embedServiceAdapter{svc: svc}
}
