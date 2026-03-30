# Query-Aware Activation Brief

## What it does

When you call `ACTIVATE`, MuninnDB returns the top engrams relevant to your query. With the activation brief, it also returns the **top 5 sentences across all result engrams** that are most relevant to your query — a cross-engram extractive summary, computed without any LLM.

## Why it matters

Activation returns up to 100 engrams. Reading all of them is impractical. An LLM could synthesize them, but that's expensive and requires configuration. The activation brief gives you the signal immediately, for free, using the same TF-IDF weights already in the FTS index.

## How it works

After the 6-phase activation pipeline runs and returns ranked engrams:

1. The query terms are tokenized and stop-word filtered into a query term set.
2. Each result engram's content is split into sentences (heuristic splitter: punctuation + capital letter, newline-separated paragraphs).
3. Each sentence is scored: `score = sum(term_frequency_in_sentence)` for each query term.
4. All `(sentence, score, engramID)` tuples across all engrams are sorted descending.
5. The top 5 are returned as `Brief` in the `ActivateResponse`.

## Example

Query: `["how does attention work in transformers"]`

Brief result:
```json
[
  {"engram_id": "01JM...", "text": "Transformer attention uses query, key, and value matrices.", "score": 3.0},
  {"engram_id": "01JM...", "text": "Self-attention allows each position to attend to all positions.", "score": 2.0},
  {"engram_id": "01JN...", "text": "Scaled dot-product attention divides by sqrt(d_k) for stability.", "score": 2.0}
]
```

## What is new / never done before (to our knowledge)

**Query-aware cross-engram extractive summarization built into the database read path.** Most search systems return ranked documents. Some return snippets from within a document. MuninnDB scores sentences **across multiple documents simultaneously** against the query, and returns the best sentences from the collective result set. This is a database-layer synthesis step, not an application-layer post-processing step.

The LLM path (when configured) uses the same API surface — callers don't change code when upgrading from extractive to LLM synthesis.

## API

Add `brief_mode` to your ACTIVATE request:

| Value | Behavior |
|-------|----------|
| `""` or `"auto"` | Extractive (default; falls back to extractive if LLM unavailable) |
| `"extractive"` | Always extractive, never calls LLM |
| `"llm"` | LLM synthesis (requires `MUNINN_ENRICH_URL`; empty brief if LLM unavailable) |

## Python SDK

```python
result = await client.activate(
    vault="default",
    context=["how does memory consolidation work"],
    brief_mode="extractive"
)
for sentence in result.brief:
    print(f"[{sentence.score:.1f}] {sentence.text}")
```
