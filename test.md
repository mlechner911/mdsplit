# go-time-graph — Server-Side SVG Time-Series Chart Library

[![Version](https://img.shields.io/badge/version-1.1.1-blue)](#versioning)
[![Go](https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![License](https://img.shields.io/badge/license-MIT%20with%20Attribution-green)](../LICENSE)

**A Go (Golang) port of [ml-time-graph](../packages/ml-time-graph)** — pure server-side rendering of
time-series charts to SVG, with zero DOM dependencies.

> Fully typed Go structs · fluent Builder API · HTTP API + Swagger · CLI tool · baseline test suite

---

## Overview

go-time-graph is a 1:1 port of the TypeScript `ml-time-graph` library to Go. It accepts time-series
configuration as JSON (or via the fluent Builder API) and renders complete SVG charts server-side.

Use cases: backend report generation, PDF embedding (via headless browsers), email attachments,
CLI-based benchmarking, and any environment where a browser is unavailable.

There are three ways to use it — pick one:

1. **As a Go library** — import the packages and build charts in-process (`go get`, see below).
2. **As an HTTP service** — run `graph-server` and POST JSON, get SVG back.
3. **As a CLI** — `graph-cli -in config.json -out chart.svg`.

## Gallery

All rendered server-side by go-time-graph from the JSON configs in [test_data/](test_data/) — click
any image for the full SVG.

<p align="center">
  <a href="https://gitlab.com/mlc0911/mlctimegraph/-/blob/main/go-time-graph/test_data/output_go_color_thresholds_demo.svg"><img src="https://gitlab.com/mlc0911/mlctimegraph/-/raw/main/go-time-graph/test_data/output_go_color_thresholds_demo.svg" alt="Color-by-threshold zoned line colouring" width="300"></a>
  <a href="https://gitlab.com/mlc0911/mlctimegraph/-/blob/main/go-time-graph/test_data/output_go_thresholds_fill_demo.svg"><img src="https://gitlab.com/mlc0911/mlctimegraph/-/raw/main/go-time-graph/test_data/output_go_thresholds_fill_demo.svg" alt="Threshold lines with above/below fills" width="300"></a>
  <a href="https://gitlab.com/mlc0911/mlctimegraph/-/blob/main/go-time-graph/test_data/output_go_temp_highlights.svg"><img src="https://gitlab.com/mlc0911/mlctimegraph/-/raw/main/go-time-graph/test_data/output_go_temp_highlights.svg" alt="Highlighted time ranges / annotation bands" width="300"></a>
</p>

Zoned colour-by-threshold (left), threshold lines with above/below fills (center), and highlighted
time ranges (right). More examples in the [test_data catalog](test_data/index.md).

## Install

### As a library (`go get`)

```bash
go get gitlab.com/mlc0911/mlctimegraph/go-time-graph@latest
```

> **Note on the module path & tags:** this library lives in the `go-time-graph/` subdirectory of the
> `mlctimegraph` monorepo, so its release tags are **prefixed with the subpath** — e.g.
> `go-time-graph/v1.0.6`. To pin an exact release use the bare semver the Go tool expects:
> `go get gitlab.com/mlc0911/mlctimegraph/go-time-graph@v1.0.6`. See [Versioning](#versioning).

### As binaries (from source)

```bash
cd go-time-graph
task build        # → ./graph-server and ./graph-cli   (or: go build ./cmd/...)
```

Or install them onto your `$PATH`:

```bash
task install      # go install ./cmd/server && go install ./cmd/cli
```

## Library usage

A minimal program that builds a chart with the fluent Builder API and renders it to an SVG file:

```go
package main

import (
	"log"
	"os"

	"gitlab.com/mlc0911/mlctimegraph/go-time-graph/pkg/chart"
	"gitlab.com/mlc0911/mlctimegraph/go-time-graph/pkg/models"
	"gitlab.com/mlc0911/mlctimegraph/go-time-graph/pkg/renderer"
)

func main() {
	// 1. Build a time series (millisecond epoch timestamps + values).
	temp := chart.NewTimeSeries("Room Temperature").
		AddPoint(1738368000000, 18.5).
		AddPoint(1738371600000, 22.0).
		AddNullPoint(1738375200000). // sensor gap
		AddPoint(1738378800000, 24.5).
		SetLineStyle("#e53935", 3.0, models.LineStyleSolid).
		SetSmoothing(true).
		Build()

	// 2. Add a threshold line with a shaded "above" fill.
	warn := chart.NewThreshold("Warning High", 23.0).
		SetColor("#fb8c00").
		SetLine(models.LineStyleDashed).
		SetFill(models.ThresholdFillAbove, 0.12).
		Build()

	// 3. Assemble the chart options.
	opts := chart.NewBuilder().
		SetSize(800, 400).
		SetLocale("de-DE").
		SetMargin(20, 20, 40, 60).
		SetLegend(true, models.LegendPositionOutsideRight, models.LegendOrientationVertical).
		AddTimeSeries(temp).
		AddThreshold(warn).
		Build()

	// 4. Render to draw-commands, then to an SVG string.
	commands := chart.NewMLTimeGraph(opts).RenderCommands()
	svg := renderer.NewSVGRenderer("800", "400").Render(commands)

	// 5. Write it out.
	if err := os.WriteFile("chart.svg", []byte(svg), 0o644); err != nil {
		log.Fatal(err)
	}
}
```

Prefer JSON over the Builder? Unmarshal straight into `models.MLTimeGraphOptions` and skip step 3 —
see [docs/JSON_CONFIG.md](docs/JSON_CONFIG.md). The current version is available at runtime via
`version.Version` (package `pkg/version`).

## HTTP server

```bash
# Start with defaults (port 8080, prefix /api/v1)
task run:server          # or: ./graph-server

# Custom port and prefix
PORT=9090 API_PREFIX=/v1 ./graph-server
```

Render a chart:

```bash
curl -X POST http://localhost:8080/api/v1/chart/render \
  -H "Content-Type: application/json" \
  -d @test_data/temp24h.json
```

Endpoints:

| Method | Path | Purpose |
| :--- | :--- | :--- |
| `POST` | `/api/v1/chart/render` | Accepts `MLTimeGraphOptions` JSON, returns an SVG (`text/xml`) |
| `GET`  | `/health` | Liveness probe — returns `{"status":"UP","version":"…"}` |
| `GET`  | `/swagger/index.html` | Interactive Swagger UI |

### Swagger / OpenAPI

The REST API is documented with [swaggo](https://github.com/swaggo/swag). Browse it live at
`http://localhost:8080/swagger/index.html` while the server runs, or read the committed specs in
[docs/swagger.yaml](docs/swagger.yaml) / [docs/swagger.json](docs/swagger.json). Regenerate after
changing the API annotations with `task swagger`.

## CLI tool

```bash
# Render from JSON to SVG
./graph-cli -in test_data/temp24h.json -out chart.svg

# With a white background
./graph-cli -in test_data/temp24h.json -out chart.svg -bg white

# Print the version
./graph-cli -version
```

## Package structure

| Package | Purpose | Docs |
| :--- | :--- | :--- |
| [`pkg/models`](pkg/models/) | Data types, config structs, constants | [API Reference](docs/API_REFERENCE.md#models) |
| [`pkg/chart`](pkg/chart/) | MLTimeGraph engine + Builder API | [API Reference](docs/API_REFERENCE.md#chart) |
| [`pkg/renderer`](pkg/renderer/) | SVG renderer, series/threshold/highlight rendering | [API Reference](docs/API_REFERENCE.md#renderer) |
| [`pkg/core`](pkg/core/) | Layout bounds, LinearScale, TimeScale | [API Reference](docs/API_REFERENCE.md#core) |
| [`pkg/axis`](pkg/axis/) | Time and value axis generators | [API Reference](docs/API_REFERENCE.md#axis) |
| [`pkg/series`](pkg/series/) | Data processing: runs, interpolation, zone-splitting | [API Reference](docs/API_REFERENCE.md#series) |
| [`pkg/api`](pkg/api/) | Gin HTTP handler | [API Reference](docs/API_REFERENCE.md#api) |
| [`pkg/config`](pkg/config/) | Server configuration (env vars) | [API Reference](docs/API_REFERENCE.md#config) |
| [`pkg/version`](pkg/version/) | Single-source library version | — |

## Documentation

- **[Usage Guide](docs/USAGE.md)** — Series, thresholds, legends, axes, gaps, annotations
- **[Architecture](docs/ARCHITECTURE.md)** — Rendering pipeline and DrawCommand IR
- **[Builder API](docs/BUILDER.md)** — Programmatic chart construction with examples
- **[JSON Config](docs/JSON_CONFIG.md)** — Reference for the JSON API schema
- **[API Reference](docs/API_REFERENCE.md)** — Complete Go package documentation
- **Swagger UI** — `/swagger/index.html` (live) · [swagger.yaml](docs/swagger.yaml) (static)

## Development

This subdirectory ships its own [Taskfile.yml](Taskfile.yml) ([go-task](https://taskfile.dev)). Run
`task --list` for everything; the common targets:

| Task | Action |
| :--- | :--- |
| `task build` | Build `graph-server` + `graph-cli` (version stamped via `-ldflags`) |
| `task test` | Run the full test suite |
| `task test:cover` | Tests with a coverage profile + function summary |
| `task lint` | `golangci-lint run ./...` |
| `task swagger` | Regenerate the Swagger docs |
| `task check` | Pre-commit gate: fmt → vet → lint → test |
| `task render IN=… OUT=…` | Render a JSON config to SVG via the CLI |

> A repo-wide `Makefile` at the monorepo root also wraps these (`make build-go`, `make test-go`,
> `make swagger`) alongside the TypeScript build.

## Testing

```bash
task test          # or: go test ./...
```

The baseline suite (`pkg/chart/baseline_test.go`) renders every example from `test_data/` and writes
baseline SVG files (`output_go_*.svg`). See [test_data/index.md](test_data/index.md) for the full catalog.

## Versioning

go-time-graph follows [Semantic Versioning](https://semver.org) and is **released in lockstep with the
TypeScript [`ml-time-graph`](../packages/ml-time-graph) library** — both always carry the same version.
The Go version is defined once in [`pkg/version/version.go`](pkg/version/version.go) and surfaced
through the CLI (`-version`), the server startup log, the `/health` endpoint and the Swagger
`info.version`.

Use the repo-root release command to bump both libraries and write every version slot at once (run
from the monorepo root):

```bash
make release V=1.0.7          # explicit version  (or V=patch|minor|major)
make release V=1.0.7 TAG=1    # also create the two git tags
# equivalently: pnpm release 1.0.7 --tag
```

Because the module lives in a monorepo subdirectory, **two tags are created per release** — the npm
marker plus the subpath-prefixed Go module tag that the Go tooling requires:

```bash
v1.0.7                 # TypeScript / npm
go-time-graph/v1.0.7   # Go module (prefix is mandatory)
```

Consumers reference the full module path with a plain semver selector:

```bash
go get gitlab.com/mlc0911/mlctimegraph/go-time-graph@v1.0.7
```

## License

© 2026 Michael Lechner — MIT with Attribution (see [LICENSE](../LICENSE)).
