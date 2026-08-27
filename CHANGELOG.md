# Changelog

All notable changes to this project are documented here. Versions follow
[semantic versioning](https://semver.org/).

## [1.4.0] - 2026-08-27

The splitter learned to run the translation itself, one isolated request per
piece, and to say where its output came from.

### Added

- **Translation.** `-translate` on the CLI and `translate_chunk` over MCP send
  one part at a time to any OpenAI-compatible endpoint — a local Ollama or LM
  Studio server, or the OpenAI API. Nothing accumulates between requests, so a
  10 MB document costs the same context per step as a 10 KB one. Over MCP only
  a status line comes back; the text never passes through the conversation.
- **Two translation modes.** `-mode block` (default) sends only prose
  fragments and reproduces code, HTML, bullets, pipes and indentation
  mechanically — a model cannot damage what it never receives, which also makes
  a model with no instruction channel usable. `-mode chunk` sends a whole part
  with code and protected spans replaced by sentinels, keeping the prose
  connected at the cost of more fragile edges.
- **Structure verification.** A reply is checked against its source before it
  is stored: same block kinds in the same order, code fences byte for byte,
  same heading levels and table row counts. A part that fails is not stored and
  stays open, so rerunning retries exactly those.
- **Glossary.** `-glossary` proposes a document's terminology — found without a
  model, from words recurring across chunks that also appear inside code — and
  translates the list in a single request into a reviewable `glossary.json`.
  Only entries whose term occurs in a chunk are sent with it.
- **Provenance and staleness.** Every split records tool, version, project URL
  and a hash of the source. `-check` reports whether the source changed since
  the split, exiting non-zero if it did. `-merge -stamp` writes the record into
  the document's YAML front matter, merging into an existing block rather than
  prepending a second one.
- **`completions` transport** for models whose chat template an
  OpenAI-compatible layer cannot satisfy, with the prompt rendered here from a
  Go template. `-llm-template translategemma` is a shorthand for the Gemma
  family.
- **`-out-dir`**, so one document can be split for several languages at once,
  and `task i18n` / `i18n:glossary` / `i18n:check` to drive a whole set.

### Security

- Endpoint settings are process configuration, never tool arguments. A token
  passed through a tool call would land in the client's conversation
  transcript, and a caller-chosen URL would turn a tool that reads local files
  into an exfiltration channel the moment a translated document carried an
  injected instruction. `MDSPLIT_LLM_TOKEN` has no flag, so it stays out of the
  process list too, and the endpoint is never written into provenance.

### Fixed

- A translated fragment could smuggle structure back in: a newline or a blank
  line re-parses into a different block and shifts every block after it, and a
  reply starting with `- ` or `## ` turns a paragraph into a list.
- Identifiers no longer reach the glossary. `put chunk` comes from `put_chunk`,
  and a 7B rendered it "Chunk hinzufügen" — an entry that would have corrupted
  every call in the translated document.
- Terminology candidates no longer pair words across punctuation ("code
  fences, tables" produced the phantom term "fences tables"), no longer split
  hyphen variants of one term into two entries, and no longer treat a bare flag
  or path as prose.
- Glossary extraction no longer assumes an English source silently: the
  language pair reaches the prompt and is recorded, and a source it cannot
  handle warns or is refused rather than returning a list of function words.

## [1.3.0]

- **MCP job workflow.** `split_markdown` writes the chunks and returns only a
  manifest — 686 characters for a 10 KB file — instead of handing the whole
  document back as one text result. Content moves one part at a time through
  `get_chunk` / `put_chunk`, with `job_status` and `merge_chunks`. `put_chunk`
  writes beside the original rather than over it, so a run is resumable,
  redoable per part and mergeable half-finished.

## [1.2.0]

- **Byte-exact round-trip.** Blocks carry their exact text and the blank-line
  gap that followed them, so merging reproduces the source byte for byte.
  Previously 167 of 287 real documents lost content, and the round-trip test
  compared the same normalization on both sides and so could not fail on it.
- **Section-aligned packing.** The size is a soft budget: cuts land before a
  heading, and a heading never ends a chunk. Code fences and HTML are never
  split whatever the budget says.
- Fence openers accept any info string. Requiring a bare language word meant
  ```` ```js title="a.js" ```` was parsed as prose, the split ran through the
  middle of the code, and the closing fence opened a block — from there the
  parser ran inverted.
