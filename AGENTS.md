# AGENTS.md

Single-file Go tool (`main.go` only; `cmd/` is empty) that splits Markdown files into size-bounded chunks, with two runtime modes selected by the `-cli` flag:

- **CLI mode** (`-cli -file X.md -size 4000`): writes chunks + JSON index next to the source file.
- **MCP mode** (default, no flags): serves the `split_markdown` tool over stdio via `github.com/mark3labs/mcp-go`.

## Commands

```bash
go build -o mcp-md-splitter .
./mcp-md-splitter                                  # run MCP server on stdio
./mcp-md-splitter -cli -file test.md -size 4000    # CLI export
go test -v ./...                                    # unit tests (split_test.go)
go vet ./...

smoke test:
rm -rf chunks && go run . -cli -file test.md && ls chunks/   # expect test-part-01..03.md + index.json
```

`test.md` is a fixture; `chunks/` (and the root `mcp-md-splitter` binary) are generated artifacts and currently present, even though there is no `.gitignore`. Don't treat stale `chunks/` output as input.

## Architecture

Two-phase pipeline in `main.go`:

1. `extractBlocks(content) []string` — parses Markdown line-by-line into **atomic blocks**: code fences (open fence to closing fence, never split), table row groups (consecutive `|...|` lines), lists (item + indented continuation, terminated by blank line / heading / fence / table row), headings (own block, preferred split point), and prose paragraphs.
2. `packChunks(blocks, maxSize) []string` — greedy packing: append a block to the current chunk only if it still fits in `maxSize` (byte length incl. separator); otherwise start a new chunk. No block is ever split across chunks; oversized atomic blocks emit a `> 2x Ziel` warning to stderr.

`extractBlocks` and `packChunks` are pure string functions — all testable without file I/O; `getMarkdownChunks` is the only wrapper that reads files.
- CLI (`runCLIMode`) writes `<sourceDir>/chunks/<base>-part-NN.<ext>` (zero-padded 2-digit index) plus `chunks/index.json` (`TranslationIndex`, fields `source_file`, `total_parts`, `chunks`). MCP mode returns the full concatenated chunks as one text result — it does not write files.
- Chunk budget checks use byte length of the joined string, not rune/character count — German/emoji-heavy content will differ from CLI display output.

## Gotchas / project-specific conventions

- **Language**: all user-facing strings (CLI output, error messages) are **German**; code comments and type names mix German and English. Keep this split.
- **`goldmark` is in `go.mod` but unused** (`gopls go mod tidy` warns). Splitting is hand-rolled regex, not a Markdown library. Do not "fix" the warning by adding goldmark usage unless the user asks; run `go mod tidy` only when changing dependencies.
- **Regex quirks**: `listRegex` at `main.go:209` reads as `^\s*[-*+]\s+` *or* `\s*\d+\.\s+` anywhere — it will match stray `1.` inside prose lines; list/table blocks also terminate by lookahead of the next line, not trailing state.
- **Table/HTML block detection runs per line** and only checks single-line HTML tags; multi-line `<div>` bodies are held open only while tag stack is non-empty.
- **No tests exist.** Any change to the chunker should be verified by rerunning the CLI smoke test and diffing `chunks/*.md` against a saved copy (chunk boundaries are sensitive; off-by-one in the size accounting is easy to miss).
- gopls has open style hints in `main.go` (`slices.Contains`, `CutPrefix`) but the code does not follow modern-go style consistently — don't mass-refactor.

## Style

- German comments at function/section level, 4-space indentation (tabs per gofmt), no external structure beyond stdlib + mcp-go. Match existing tone; do not add English boilerplate comments.
