"""Full cognitive loop: write → activate → link associations.

This example demonstrates a complete cognitive workflow:
1. Write a cluster of related memories
2. Activate memory with semantic query
3. Create associations between memories
4. Verify results and inspect database state

Run with: python examples/cognitive_loop.py
"""

import asyncio

from muninn import MuninnClient


async def main():
    """Cognitive loop example."""
    async with MuninnClient("http://localhost:8476") as client:
        print("MuninnDB Cognitive Loop Demo\n")

        vault = "cognitive"

        # Write a cluster of related memories about transformers
        memories = [
            (
                "attention mechanism",
                "Transformer attention: query, key, value matrices. Scaled dot-product attention computes weights as softmax(Q·K^T/√d). Foundation of modern NLP.",
                ["transformer", "attention", "ml"],
            ),
            (
                "self-attention",
                "Self-attention allows each position in a sequence to attend to all other positions. Enables parallel processing and long-range dependencies.",
                ["transformer", "attention", "ml"],
            ),
            (
                "positional encoding",
                "Since transformers have no recurrence, positional encodings inject sequence order. Sinusoidal embeddings with alternating sin/cos frequencies.",
                ["transformer", "encoding", "ml"],
            ),
            (
                "feed-forward network",
                "MLP applied to each position separately and identically. Typically: Dense → ReLU → Dense. Provides non-linearity and feature transformation.",
                ["transformer", "architecture", "ml"],
            ),
            (
                "layer normalization",
                "Normalization applied to each attention and FFN sublayer. Stabilizes training and improves gradient flow. Applied before or after sublayer.",
                ["transformer", "training", "ml"],
            ),
        ]

        print(f"Writing {len(memories)} related engrams to vault '{vault}'...\n")
        ids = {}
        for concept, content, tags in memories:
            eid = await client.write(
                vault=vault,
                concept=concept,
                content=content,
                tags=tags,
                confidence=0.95,
                stability=0.8,
            )
            ids[concept] = eid
            print(f"  ✓ {concept}")
            print(f"    → {eid}")

        print(f"\nWrote {len(ids)} engrams\n")

        # Activate to find relevant memories
        print("Activating with query: 'how does attention work in transformers?'\n")
        result = await client.activate(
            vault=vault,
            context=["attention mechanism", "transformers", "sequence modeling"],
            max_results=10,
            threshold=0.0,
        )

        print(f"Found {result.total_found} relevant memories:")
        for i, item in enumerate(result.activations, 1):
            print(f"  {i}. [{item.score:.3f}] {item.concept}")

        # Create associations between related engrams
        print(f"\nCreating associations between engrams...")

        associations = [
            ("attention mechanism", "self-attention"),
            ("self-attention", "positional encoding"),
            ("attention mechanism", "layer normalization"),
            ("feed-forward network", "layer normalization"),
        ]

        for source_concept, target_concept in associations:
            source_id = ids[source_concept]
            target_id = ids[target_concept]
            await client.link(
                source_id=source_id,
                target_id=target_id,
                vault=vault,
                rel_type=5,
                weight=0.9,
            )
            print(f"  ✓ {source_concept} → {target_concept}")

        print("\n" + "=" * 50)

        # Get database stats including coherence
        stats = await client.stats()
        print(f"\nDatabase statistics:")
        print(f"  Total engrams: {stats.engram_count}")
        print(f"  Total vaults: {stats.vault_count}")
        print(f"  Storage: {stats.storage_bytes:,} bytes")

        if stats.coherence and vault in stats.coherence:
            coherence = stats.coherence[vault]
            print(f"\nVault '{vault}' coherence:")
            print(f"  Score: {coherence.score:.3f}")
            print(f"  Total engrams: {coherence.total_engrams}")
            print(f"  Orphan ratio: {coherence.orphan_ratio:.3f}")
            print(f"  Contradiction density: {coherence.contradiction_density:.3f}")
            print(f"  Duplication pressure: {coherence.duplication_pressure:.3f}")

        # Read one engram to show full details
        print(f"\n" + "=" * 50)
        attention_id = ids["attention mechanism"]
        print(f"\nReading engram: attention mechanism")
        engram = await client.read(attention_id, vault=vault)
        print(f"  ID: {engram.id}")
        print(f"  Concept: {engram.concept}")
        print(f"  Confidence: {engram.confidence:.2f}")
        print(f"  Stability: {engram.stability:.2f}")
        print(f"  Tags: {', '.join(engram.tags)}")
        print(f"  Access count: {engram.access_count}")
        print(f"  State: {engram.state}")


if __name__ == "__main__":
    asyncio.run(main())
