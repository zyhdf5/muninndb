package plugin

import (
	"context"
	"testing"
)

func TestPluginInterface(t *testing.T) {
	// Test that mockPlugin implements Plugin
	var _ Plugin = (*mockPlugin)(nil)

	// Test that mockEmbedPlugin implements EmbedPlugin
	var _ EmbedPlugin = (*mockEmbedPlugin)(nil)

	// Test that mockEnrichPlugin implements EnrichPlugin
	var _ EnrichPlugin = (*mockEnrichPlugin)(nil)
}

func TestMockEmbedPluginEmbed(t *testing.T) {
	embed := &mockEmbedPlugin{
		mockPlugin: mockPlugin{name: "test-embed", tier: TierEmbed},
	}

	ctx := context.Background()
	result, err := embed.Embed(ctx, []string{"hello", "world"})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	expectedLen := 2 * 384 // 2 texts * 384 dimension
	if len(result) != expectedLen {
		t.Errorf("expected embedding length %d, got %d", expectedLen, len(result))
	}

	if embed.Dimension() != 384 {
		t.Errorf("expected dimension 384, got %d", embed.Dimension())
	}
}

func TestMockEnrichPluginEnrich(t *testing.T) {
	enrich := &mockEnrichPlugin{
		mockPlugin: mockPlugin{name: "test-enrich", tier: TierEnrich},
	}

	ctx := context.Background()
	eng := &Engram{
		Concept: "test",
		Content: "content",
	}

	result, err := enrich.Enrich(ctx, eng)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	if result == nil {
		t.Error("Enrich should return a non-nil EnrichmentResult")
	}
}

func TestPluginName(t *testing.T) {
	p := &mockPlugin{name: "my-plugin", tier: TierEmbed}
	if p.Name() != "my-plugin" {
		t.Errorf("expected name 'my-plugin', got %q", p.Name())
	}
}

func TestPluginTier(t *testing.T) {
	embed := &mockPlugin{name: "embed", tier: TierEmbed}
	if embed.Tier() != TierEmbed {
		t.Errorf("expected tier TierEmbed, got %v", embed.Tier())
	}

	enrich := &mockPlugin{name: "enrich", tier: TierEnrich}
	if enrich.Tier() != TierEnrich {
		t.Errorf("expected tier TierEnrich, got %v", enrich.Tier())
	}
}

func TestPluginClose(t *testing.T) {
	p := &mockPlugin{name: "test", tier: TierEmbed, closed: false}

	if err := p.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if !p.closed {
		t.Error("plugin should be marked as closed")
	}
}
