# AGENTS.md

Go tool that splits Markdown files into size-bounded chunks, with three runtime modes selected by the `-cli` / `-merge` flags (default: MCP). Standard `cmd`/`internal` layout:

- **CLI mode** (`bin/mcp-md-splitter -cli -file X.md -size 4000`): writes chunks + JSON index next to the source file.
- **Merge mode** (`bin/mcp-md-splitter -merge -dir chunks/ [-out X.md]`): Rückweg – reassembles chunks from the `index.json` manifest, preferring edited parts over originals; compares byte-exactly (`split.Canonical`) first and only then tolerantly (`split.Normalize`), printing ✅ byte-exact / ✅ whitespace-only / ⚠️ diverging.
- **MCP mode** (default, no flags): serves a five-tool job workflow over stdio via `github.com/mark3labs/mcp-go` — `split_markdown` (returns a manifest, never the content), `get_chunk`, `put_chunk`, `job_status`, `merge_chunks`.

## Commands

```bash
# Taskfile (https://taskfile.dev, `task` is on PATH)
task build     # -> bin/mcp-md-splitter (ldflags injects VERSION into main.version)
task test      # go test -v -count=1 ./...
task vet       # go vet ./...
task check     # vet + test + build
task smoke     # rm -rf chunks && run CLI on test.md && list output
task roundtrip # CLI-Split von test.md + Merge zurück (Roundtrip-Check in der Konsole)
task install   # go install -> $GOBIN (~/go/bin) – global `mcp-md-splitter` (Basis für .mcp.json)
task clean     # rm -rf bin chunks

# Plain Go
go build -o bin/mcp-md-splitter ./cmd/mcp-md-splitter
go test -v ./...
```

`test.md` is a fixture; `chunks/` and `bin/` are generated artifacts (currently present; no `.gitignore`). Don't treat stale `chunks/` output as input.

## Structure

```
cmd/mcp-md-splitter/main.go   # entrypoint: flags (cli / merge) + version var
cmd/mcp-md-splitter/cli.go    # runCLIMode, TranslationIndex (file export mode)
cmd/mcp-md-splitter/merge.go  # runMergeMode: Rückweg, liest index.json (Fallback: Glob *-part-*.md)
cmd/mcp-md-splitter/mcp.go    # runMCPMode (stdio MCP server: split_markdown, merge_chunks)
.mcp.json                     # client registration (md-splitter)
internal/split/html.go        # HTML tag maps + isHTMLElementLine / htmlLineDelta
internal/split/extract.go     # block regexes + ExtractBlocks (atomic-block parser)
internal/split/pack.go        # PackChunks, Split, SplitFile (chunk packing + API)
internal/split/join.go        # Normalize, Join, MergeFiles (Rückweg-Logik, kanonisch)
internal/split/block_test.go  # block-/pack-Tests + TestSplitFile_RoundTrip, TestFullSplit_testMd
internal/split/join_test.go   # Roundtrip- & Normalization-Tests (TestRoundtrip_*)
VERSION                       # 1.1.0 (injected via -ldflags "-X main.version=...")
```

Rückweg: CLI `-merge -dir chunks/ [-out X.md]` → `runMergeMode` → `split.MergeFiles(paths)` → `Join` → `Normalize`. Roundtrip-Garantie ist **kanonisch** (`Normalize(merged) == Normalize(orig)`), kein Byte-for-Byte-Vergleich.

## Architecture

Two-phase pipeline in `internal/split`:

1. `ExtractBlocks(content string) []string` — parses Markdown line-by-line into **atomic blocks**: code fences (open fence to closing fence, never split), table row groups (consecutive `|...|` lines), lists (item + indented continuation, terminated by blank line / heading / fence / table row), HTML blocks (multi-line `<div>`, `<table>` etc. tracked via a tag stack), headings (own block, preferred split point), and prose paragraphs.
2. `PackChunks(blocks []string, maxSize int) []string` — greedy packing: append a block to the current chunk only if it still fits in `maxSize` (byte length incl. separator); otherwise start a new chunk. No block is ever split across chunks; oversized atomic blocks emit a `> 2x Ziel` warning to stderr.

`SplitFile(path string, maxSize int) ([]string, error)` wraps `os.ReadFile` + `Split` — the only function with I/O.

- CLI (`runCLIMode`) writes `<sourceDir>/chunks/<base>-part-NN.<ext>` (zero-padded 2-digit index) plus `chunks/index.json` (`TranslationIndex`, fields `source_file`, `total_parts`, `chunks`). MCP mode: `split_markdown` returns the full concatenated chunks as one text result (no file writes); `merge_chunks` is the Rückweg – reassembles from a `chunks/` dir and reports the roundtrip check.
- Merge (`runMergeMode`) resolves chunk paths from `chunks/index.json` (relative to cwd OR to the parent of the chunks dir) with a `*-part-*.md` glob fallback; writes `<source>.merged` by default.
- Chunk budget checks use byte length of the joined string, not rune/character count — German/emoji-heavy content will differ from CLI display output.

## Gotchas / project-specific conventions

- **Language**: all user-facing strings (CLI output, error messages) are **German**; code comments and type names mix German and English. Keep this split.
- **`goldmark` is in `go.mod` but unused** (`gopls go mod tidy` warns). It is reserved for planned AST-based splitting. Do not "fix" the warning by adding goldmark usage, and do not run `go mod tidy` unless the user asks (it would drop the dep).
- **Regex quirks**: `listRx` reads as `^\s*[-*+]\s+` *or* `\s*\d+\.\s+` anywhere — it will match stray `1.` inside prose lines; list/table blocks also terminate by lookahead of the next line, not trailing state.
- **HTML block detection runs per line** with a tag stack; a multi-line `<div>` body stays open while the stack is non-empty.
- **Testing convention**: chunk boundaries are sensitive (off-by-one in size accounting is easy to miss). Any change must pass `task check` and, for chunker changes, `task smoke` — diff `chunks/*.md` against a saved copy.
- gopls has open style hints (`slices.Contains`, unused param `blocks` in `bufIsNotList`) — the code does not follow modern-go style consistently; don't mass-refactor.

## Style

- German comments at function/section level, gofmt (tabs), stdlib + mcp-gol. Match existing tone; do not add English boilerplate comments.
- Library code lives in `internal/split` and is exposed via exported functions (`ExtractBlocks`, `PackChunks`, `Split`, `SplitFile`) — keep internals unexported in that package.

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
