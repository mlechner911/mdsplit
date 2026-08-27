# Markdown Splitter

Splits Markdown documents into size-bounded chunks that stay safe for LLM translation or processing. Atomic blocks (code fences, tables, lists, multi-line HTML) are never split across chunk boundaries — only whole blocks move between chunks.

Implemented in Go with three runtime modes: split CLI, merge CLI (Round-trip), and an MCP (Model Context Protocol) server exposing a chunk-at-a-time job workflow.

## Motivation

Translating a long Markdown document with a local model is a context problem
before it is a language problem. A 60 KB handbook does not fit into an 8k
window, so it has to be cut — and the naive cuts are exactly the damaging ones.
Split every N characters and a code fence ends up half in one piece and half in
the next; the model dutifully "translates" the orphaned half, renames the
identifiers, and the block never closes again. Split at blank lines and a table
loses its header row. Split at headings only and one section is 200 bytes while
the next is 40 KB.

Handing the whole file to the model and asking it to chunk itself does not help
either: the file is already in the context by then, which was the thing to
avoid.

So this tool does the cutting outside the model, on two rules:

1. **Some blocks are indivisible.** Code fences, tables, list items with their
   continuations, HTML elements. They stay whole even when that means blowing
   through the size budget — a chunk that is twice as large is an inconvenience,
   a chunk that ends mid-fence is corruption.
2. **The size is a target, not a law.** Cuts land before headings, so a chunk
   starts at a section and carries it whole. A slightly small chunk that is
   self-contained translates better than a full one that starts mid-sentence.

The MCP side follows from the same concern. `split_markdown` returns a manifest
— part count, sizes, headings — and *not* the text. Content is pulled one part
at a time with `get_chunk` and written back with `put_chunk`, so the context
stays flat whether the source is 10 KB or 10 MB.

The last piece is the way back. Chunking is only safe if it is reversible, so
the manifest records the blank-line gap at every boundary and the merge is
verified byte for byte against the source. If the round-trip is exact for the
untranslated document, the pipeline did not silently eat anything — and any
difference after translation is the model's doing, not the splitter's.

Built for [Crush](https://github.com/charmbracelet/crush) driving a local Ollama
model, but nothing in it is specific to either.

## Features

- **Section-aligned chunks**: the size is a *soft* budget. The splitter prefers
  to cut before a heading rather than fill a chunk to the brim, so a chunk
  starts at a section and holds it whole. A heading never ends a chunk.
- **Atomic blocks are never split**, whatever the budget says: code fences
  (any info string — ` ```js title="a.js" `, ` ``` go `, `~~~`, `````), GFM
  table row groups, list items with their continuations, and multi-line HTML
  elements. An indivisible block that exceeds the budget gets its own chunk
  and a note on stderr.
- **Byte-exact round-trip**: `index.json` records the blank-line gap at every
  chunk boundary, so merging reproduces the source byte for byte. The only
  normalization is documented in `Canonical()`: leading/trailing blank lines
  are trimmed and whitespace-only lines become empty. Indentation and hard
  line breaks (two trailing spaces) survive.
- **CLI mode**: writes `<name>-part-NN.md` files plus an `index.json` manifest (source file, part count, ordered chunk list) next to the source file
- **Merge mode** ("Rückweg"): `-merge -dir chunks/` reassembles the chunks via the manifest and reports whether the result is byte-identical, whitespace-identical, or diverging
- **MCP job workflow**: `split_markdown` returns a manifest, not the document.
  Content moves one part at a time via `get_chunk` / `put_chunk`, so context
  stays constant no matter how large the source is — which is the whole point
  of running this against a small local model.

## Requirements

- Go 1.25+
- Optional: [taskfile](https://taskfile.dev/) (`go install github.com/go-task/task/v3/cmd/task@latest`)

## Build & Test

```bash
# with Taskfile
task build     # -> bin/mcp-md-splitter
task test      # go test -v ./...
task vet
task check     # vet + test + build (CI-style)
task roundtrip # Split von test.md + Merge zurück + Roundtrip-Check
task install   # go install -> ~/go/bin (global `mcp-md-splitter`)
task clean     # removes bin/ and chunks/

# or plain Go
go build -ldflags "-X main.version=$(cat VERSION)" -o bin/mcp-md-splitter ./cmd/mcp-md-splitter
go test ./...
```

## Usage

### CLI mode

```bash
./bin/mcp-md-splitter -cli -file docs/integration.md -size 4000
./bin/mcp-md-splitter -cli -file docs/integration.md -size 4000 -target de
```

Output written into a `chunks/` directory next to the source file:

```
chunks/
├── integration-part-01.md
├── integration-part-02.md
├── integration-part-03.md
└── index.json          # Manifest: id, source_file, total_parts, size, target,
                        #           gaps[], parts[{part,file,chars,heading}]
```

### Merge mode (Rückweg)

```bash
# Chunks wieder zusammensetzen (liest chunks/index.json, Ordner via -dir angibbar)
./bin/mcp-md-splitter -merge -dir chunks/
./bin/mcp-md-splitter -merge -dir chunks/ -out zusammen.md   # anderer Zielpfad
```

Die Ausgabe steht standardmäßig als `<quelle>.merged` neben der Originaldatei. Der
Roundtrip-Check vergleicht zuerst byte-genau (`Canonical`) und erst danach tolerant
(`Normalize`) — so kann die Normalisierung keine echte Abweichung mehr verdecken.
Gemeldet wird ✅ byte-genau / ✅ nur Whitespace / ⚠️ abweichend.
Ohne `index.json` werden alle `*-part-NN.md` lexikografisch sortiert zusammengesetzt;
ohne die `gaps` aus dem Index wird an jeder Grenze eine Leerzeile angenommen.

### MCP mode (default)

Run without flags to serve the stdio MCP server. The tools form a **job
workflow** so a small local model never has to hold the whole document:
`split_markdown` writes the chunks to disk and returns only a manifest, then
content moves one part at a time.

| Tool | Arguments | Returns |
|---|---|---|
| `split_markdown` | `filePath`, `size` (8000), `target`, `outDir` | manifest only — `jobId`, per-part size and heading. **Not** the content. |
| `get_chunk` | `jobId`, `part` | the text of that one part (the edited version if it exists) |
| `put_chunk` | `jobId`, `part`, `text` | stores the translated part, reports progress and the next open part |
| `job_status` | `jobId` \| `chunksDir` | progress and part list, no content |
| `merge_chunks` | `jobId` \| `chunksDir`, `out` | reassembles; edited parts win, untouched parts fall back to the original |

A translation run looks like this:

```
split_markdown(filePath="doc.md", size=2000, target="de")
  → jobId dfd9fa33cd, 11 Teile          (686 chars back, not 10 KB)
get_chunk(jobId, part=1) → translate → put_chunk(jobId, part=1, text=…)
  … repeat; put_chunk names the next open part each time …
merge_chunks(jobId) → doc.de.md
```

`put_chunk` never touches the original chunk, so a run can be resumed, redone
part by part, or merged half-finished. With nothing edited, `merge_chunks`
additionally verifies the byte-exact round-trip against the source.

Chunks land in `chunks/` next to the source (override with `outDir`); the
`jobId` is a stable hash of source path plus budget, so re-splitting the same
file with the same budget reuses the same job.

Install once globally (`go install` lands in `~/go/bin`, already on PATH), then register the command with any client — no path needed:

```bash
task install        # or: go install ./cmd/mcp-md-splitter
```

The repo ships a project-local `.mcp.json` that does exactly this for you, so a client started from this directory (Crush, Claude Desktop, OpenCode, …) picks the server up automatically:

```json
{
  "mcpServers": {
    "md-splitter": {
      "command": "mcp-md-splitter"
    }
  }
}
```

A global `crushrc` entry works the same way: `mcp add md-splitter --command mcp-md-splitter`.

## Project Structure

Standard Go `cmd`/`internal` layout:

```
.
├── .mcp.json                     # MCP client registration (md-splitter)
├── cmd/mcp-md-splitter/          # entrypoint
│   ├── main.go                   # flag parsing (cli / merge / mcp)
│   ├── cli.go                    # CLI export mode
│   ├── merge.go                  # Merge mode (Rückweg, -merge -dir …)
│   └── mcp.go                    # MCP stdio server (5 tools)
├── internal/job/                 # Manifest, chunk files, jobId registry
├── internal/split/               # splitter library (pure string functions)
│   ├── block.go                  # Block{Text,Gap,Kind,Level} + render helpers
│   ├── html.go                   # HTML block detection helpers
│   ├── extract.go                # ExtractBlocks (atomic-block parser)
│   ├── pack.go                   # groupBlocks, packRanges, SplitDoc, SplitFile
│   ├── join.go                   # Canonical, Normalize, JoinGaps, MergeFilesGaps
│   └── *_test.go                 # unit tests + byte-exact round-trip corpus
├── Taskfile.yaml                 # task build/test/vet/check/roundtrip/install/clean
├── VERSION                       # 1.3.0
└── test.md                       # fixture for the full-split test
```

Pipeline: `ExtractBlocks(content) []Block` parses Markdown into atomic blocks —
each carrying its exact text, the blank-line `Gap` that followed it, and its
`Kind`. `groupBlocks` then bonds what must not be separated (blocks with no
blank line between them; a heading and its section). `packRanges` fills chunks
from those groups, preferring a cut before a heading. `SplitDoc(content, max)`
returns `Doc{Chunks, Gaps}`; `JoinGaps(chunks, gaps)` is the exact inverse.

## Development Notes

- **Everything a user or a model reads is English**: CLI output, error
  messages, flag help and the MCP tool descriptions. Code comments and test
  failure messages are German — that split is deliberate, so keep it.
- No Markdown AST library. The parser is line-based on purpose: an AST would
  have to be lowered back to source to cut on, and preserving the source bytes
  exactly is the one thing the round-trip contract depends on.
- `TestRoundtrip_ProjectDocs` runs the splitter over every `*.md` in the repo
  root at three budgets and asserts byte-exactness — the cheapest real corpus
  available without leaving the repo.

## Continuous integration

[`.github/workflows/ci.yml`](.github/workflows/ci.yml) runs on every push:
`gofmt`/`go vet`/`go mod tidy` once, the test suite on Linux, macOS and Windows
(the splitter is mostly path handling, and the job registry lives in
`os.UserCacheDir()`), and a round-trip job that splits and merges every
Markdown file in the repo at three budgets, failing unless each one comes back
byte-identical.

## License

MIT © 2026 Michael Lechner — see [LICENSE](LICENSE).
