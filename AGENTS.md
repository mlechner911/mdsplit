# AGENTS.md

Go tool that cuts Markdown into chunks a language model can process one at a
time, and puts them back together byte for byte. Standard `cmd`/`internal`
layout. Six runtime modes, selected by flags; MCP is the default.

| Mode | Flag | What it does |
|---|---|---|
| MCP server | *(none)* | seven tools over stdio via `github.com/mark3labs/mcp-go` |
| Split | `-cli -file X.md -size N [-out-dir D]` | writes chunk files plus `index.json` |
| Merge | `-merge -dir D [-out F] [-stamp]` | reassembles; edited parts win over originals |
| Translate | `-translate -dir D -lang de [-mode block\|chunk]` | one isolated request per piece |
| Glossary | `-glossary -dir D -lang de` | proposes terminology, writes `glossary.json` |
| Check | `-check -dir D` | progress, and whether the source changed since the split |
| Outline | `-outline -file X.md [-section "Usage > CLI"]` | headings with section sizes, or one section verbatim |

MCP tools: `split_markdown` (returns a manifest, **never** the content),
`get_chunk`, `put_chunk`, `job_status`, `merge_chunks`, `outline` and
`read_section` (these last two need no job at all), plus — only when an
endpoint is configured — `translate_chunk` and `build_glossary`.

## Commands

```bash
task check      # vet + test + build. Run this before claiming anything works.
task build      # -> bin/mcp-md-splitter (ldflags injects VERSION into main.version)
task test       # go test -v -count=1 ./...  — hermetic: no network, no containers
task roundtrip  # split testdata/sample.md, merge it back, fail unless byte-identical
task smoke      # CLI smoke test on the testdata/sample.md fixture
task install    # go install -> $GOBIN, so `mcp-md-splitter` is on PATH for MCP clients

task i18n           # translate README.md into every language in LANGS
task i18n:glossary  # build glossary.json for languages that have none
task i18n:check     # per language: is the translation still current?
```

The `i18n` tasks need `MDSPLIT_LLM_URL` and `MDSPLIT_LLM_MODEL`. `testdata/sample.md` is a fixture. `bin/`, `chunks/`, `i18n/*/*-part-*.md` and merge output are generated
and gitignored; `i18n/*/glossary.json` is **not**, because it holds reviewed
decisions.

## Structure

```
cmd/mcp-md-splitter/
  main.go        flags for all six modes; sets meta.Version
  cli.go         runCLIMode - split and write
  merge.go       runMergeMode, runCheckMode, roundtripVerdict
  translate.go   runTranslateMode - the CLI translation loop
  glossary.go    runGlossaryMode
  mcp.go         the stdio server and its seven tool handlers
internal/split/
  block.go       Block{Text,Gap,Kind,Level} + render/rangeSize
  extract.go     ExtractBlocks - the atomic-block parser
  pack.go        groupBlocks, packRanges, SplitDoc, SplitFile
  join.go        Canonical, Normalize, JoinGaps, MergeFilesGaps
  html.go        tag maps, htmlLineDelta, htmlOpensBlock, stripInline
  verify.go      VerifyStructure - is this a translation or damage?
  outline.go     Outline, Section - the reading side
internal/job/
  job.go         Manifest, Provenance, chunk paths, jobId registry
  stamp.go       YAML front-matter stamping (merges, never prepends)
internal/llm/    OpenAI-compatible client; chat and completions transports
internal/translate/
  translate.go   Options, Part - dispatches on mode
  blocks.go      the block-mode planner, placeholder protect/restore
  chunkmask.go   chunk-mode masking of code and protected spans
internal/glossary/
  extract.go     Candidates - terminology found without a model
  build.go       one request, tolerant JSON parse, File load/save
internal/meta/   tool name, URL and version, in one place
```

## Architecture

**Splitting is two phases.** `ExtractBlocks` parses Markdown line by line into
atomic blocks, each carrying its exact text, the blank-line `Gap` that followed
it, and its `Kind`. `groupBlocks` then bonds what must not be separated — blocks
with no blank line between them, and a heading with its section. `packRanges`
fills chunks from those groups, preferring a cut before a heading once the chunk
has substance, so the size is a **soft** budget. `SplitDoc` returns
`Doc{Chunks, Gaps}`; `JoinGaps` is its exact inverse.

**Translating is one isolated request per piece.** No history, ever — that is
what keeps the context flat. Block mode sends only prose fragments and
reproduces code, HTML, bullets, pipes and indentation mechanically. Chunk mode
sends a whole part with code and protected spans replaced by `⟦n⟧` sentinels.
Before anything is stored: every sentinel must return exactly once,
`finish_reason` must be `stop`, and `VerifyStructure` must find the source's
shape. A part that fails is not stored and stays open.

**Reading is jobless.** `Outline` and `Section` write nothing and need no
manifest. A section is not a chunk: chunks follow the byte budget, sections
follow the outline, so `Section` cuts from a heading to the next one of the
same or a shallower level.

**The glossary is built before translating and then frozen.** Candidates are
found without a model; the list is translated in a single request into a
`glossary.json` a person edits.

## Conventions

- **Everything a user or a model reads is English**: CLI output, error
  messages, flag help, MCP tool descriptions. Code comments and test failure
  messages are **German**. That split is deliberate — keep it.
- Error strings follow Go convention: lowercase, no trailing punctuation,
  wrapped verbs (`fmt.Errorf("read manifest: %w", err)`).
- No Markdown AST library, on purpose. An AST would have to be lowered back to
  source to cut on, and preserving the source bytes exactly is what the
  round-trip contract rests on. `go mod tidy` is expected to stay clean; CI
  checks it.
- Comments explain **why**, not what. The reasons in this file and in the
  commit log are the record of what went wrong before; do not paraphrase them
  away.
- Chunk budgets count bytes, not runes. German and CJK content will differ from
  a character count.

## Testing

- `task test` is hermetic. The network path is covered against an `httptest`
  server in `internal/llm`; nothing reaches out.
- `TestRoundtrip_ProjectDocs` runs the splitter over every `*.md` in the repo
  root at three budgets and asserts byte-exactness — the cheapest real corpus
  available without leaving the repo.
- Chunk boundaries are sensitive; an off-by-one in size accounting is easy to
  miss. Any chunker change must pass `task check` and `task roundtrip`.

## Invariants worth not breaking

These are load-bearing; the tests encode them.

- **`ExtractBlocks` never trims a block's lines.** Indentation is meaning in
  Markdown (a fence inside a list item, a continuation paragraph, an indented
  blockquote). A block's `Gap` field carries the blank lines that followed it —
  that pair is what makes the round-trip byte-exact.
- **Round-trip is asserted against `split.Canonical(source)`, not against
  `Normalize(x) == Normalize(y)`.** The old test compared the same
  normalization on both sides and therefore could not fail on the class of loss
  it was meant to catch.
- **A fence opener is `^[ \t]*(`{3,}|~{3,})` with any info string.** Requiring a
  bare language word silently turned ` ```js title="a.js" ` into prose and let
  the *closing* fence open a block — the parser then ran inverted.
- **HTML blocks start at the beginning of a line and ignore inline code.** A
  sentence mentioning `` `<pre>` `` is prose. Without both rules one such
  sentence swallowed 2600 lines of CHANGELOG.md into a single block.
- **Indivisible blocks may exceed the size budget.** Code and HTML are never
  split; the budget is soft by design and oversized chunks report on stderr
  rather than failing.
- **`split_markdown` must not return document content.** That is the entire
  point of the job workflow; `TestSplitReturnsNoContent` guards it.
- **`put_chunk` writes beside the original, never over it.** Runs stay
  resumable and re-doable per part.
- **Glossary candidate extraction is English-shaped and says so.** The stopword
  list is English and words are found by splitting on spaces. `glossary.Support`
  classifies a source language so the caller warns (other space-separated
  languages) or refuses (Chinese, Japanese, Korean, Thai) instead of returning
  a list of function words. Translation has no such limitation - only the
  terminology extraction does.
