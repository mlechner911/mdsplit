package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mcp-md-splitter/internal/job"
	"mcp-md-splitter/internal/split"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// args holt die Argumente einer Anfrage als Map.
func args(req mcp.CallToolRequest) map[string]any {
	m, _ := req.Params.Arguments.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

func argString(req mcp.CallToolRequest, key string) string {
	s, _ := args(req)[key].(string)
	return strings.TrimSpace(s)
}

func argInt(req mcp.CallToolRequest, key string, def int) int {
	if n, ok := args(req)[key].(float64); ok {
		return int(n)
	}
	return def
}

// resolveJob findet den Auftrag über jobId oder - ersatzweise - über den
// Chunk-Ordner.
func resolveJob(req mcp.CallToolRequest) (*job.Manifest, error) {
	if id := argString(req, "jobId"); id != "" {
		return job.LoadByID(id)
	}
	if dir := argString(req, "chunksDir"); dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("ordner auflösen: %w", err)
		}
		return job.Load(abs)
	}
	return nil, fmt.Errorf("bitte jobId (aus split_markdown) oder chunksDir angeben")
}

// runMCPMode startet den stdio-MCP-Server.
func runMCPMode() {
	s := server.NewMCPServer("Markdown-Splitter", version,
		server.WithToolCapabilities(true),
		server.WithInputSchemaValidation(),
	)

	s.AddTool(splitTool(), splitHandler)
	s.AddTool(getChunkTool(), getChunkHandler)
	s.AddTool(putChunkTool(), putChunkHandler)
	s.AddTool(jobStatusTool(), jobStatusHandler)
	s.AddTool(mergeTool(), mergeHandler)

	fmt.Fprintln(os.Stderr, "🚀 MCP-Server gestartet (Markdown-Splitter "+version+")")
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Server-Fehler: %v\n", err)
		os.Exit(1)
	}
}

// --- split_markdown -----------------------------------------------------

func splitTool() mcp.Tool {
	return mcp.NewTool("split_markdown",
		mcp.WithDescription(
			"Teilt eine Markdown-Datei in abschnittsweise Chunks und legt sie als Dateien ab. "+
				"Gibt NUR das Manifest zurück (jobId, Teile mit Überschrift und Länge), nicht den Inhalt - "+
				"so bleibt der Kontextbedarf konstant, egal wie groß die Datei ist. "+
				"Danach je Teil get_chunk → übersetzen/bearbeiten → put_chunk, am Ende merge_chunks. "+
				"Code-Zäune, Tabellen, Listen und HTML-Blöcke bleiben immer vollständig; "+
				"Chunks beginnen bevorzugt an einer Überschrift."),
		mcp.WithString("filePath", mcp.Required(),
			mcp.Description("Pfad zur Markdown-Datei")),
		mcp.WithNumber("size",
			mcp.Description("Weiches Zeichenbudget pro Chunk (Standard: 8000). Unteilbare Blöcke dürfen es überschreiten.")),
		mcp.WithString("target",
			mcp.Description("Kürzel für die bearbeitete Fassung, z. B. \"de\". Bestimmt den Dateinamen der Rückschreibung (Standard: \"out\").")),
		mcp.WithString("outDir",
			mcp.Description("Zielordner für die Chunks (Standard: chunks/ neben der Quelldatei)")),
	)
}

func splitHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := argString(req, "filePath")
	if path == "" {
		return mcp.NewToolResultError("filePath ist erforderlich"), nil
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return mcp.NewToolResultError(fmt.Sprintf("Datei nicht gefunden: %s", path)), nil
	}
	size := argInt(req, "size", 8000)
	if size < 200 {
		return mcp.NewToolResultError("size muss mindestens 200 sein"), nil
	}

	doc, err := split.SplitFileDoc(path, size)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Fehler beim Verarbeiten: %v", err)), nil
	}
	if len(doc.Chunks) == 0 {
		return mcp.NewToolResultError("Keine Chunks erstellt - Datei ist leer?"), nil
	}

	outDir := argString(req, "outDir")
	if outDir == "" {
		outDir = filepath.Join(filepath.Dir(path), "chunks")
	}
	if abs, err := filepath.Abs(outDir); err == nil {
		outDir = abs
	}

	m := job.New(path, size, argString(req, "target"), doc)
	if err := m.Write(outDir, doc.Chunks); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Fehler beim Schreiben: %v", err)), nil
	}

	b := &strings.Builder{}
	fmt.Fprintf(b, "✅ %s in %d Teile gesplittet (Budget %d Zeichen)\n", filepath.Base(path), m.TotalParts, size)
	fmt.Fprintf(b, "jobId: %s\nOrdner: %s\n\n", m.ID, outDir)
	for _, p := range m.Parts {
		title := p.Heading
		if title == "" {
			title = "(ohne Überschrift)"
		}
		fmt.Fprintf(b, "  %2d/%d  %6d Zeichen  %s\n", p.Part, m.TotalParts, p.Chars, title)
	}
	fmt.Fprintf(b, "\nNächster Schritt: get_chunk(jobId=%q, part=1) - der Inhalt kommt teilweise, nie am Stück.\n", m.ID)
	return mcp.NewToolResultText(b.String()), nil
}

// --- get_chunk ----------------------------------------------------------

func getChunkTool() mcp.Tool {
	return mcp.NewTool("get_chunk",
		mcp.WithDescription("Liefert den Text genau eines Teils aus einem Split. "+
			"Wurde der Teil schon per put_chunk zurückgeschrieben, kommt die bearbeitete Fassung."),
		mcp.WithString("jobId", mcp.Required(),
			mcp.Description("Auftrags-ID aus split_markdown")),
		mcp.WithNumber("part", mcp.Required(),
			mcp.Description("Teilnummer, 1-basiert")),
	)
}

func getChunkHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m, err := resolveJob(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	n := argInt(req, "part", 0)
	text, edited, err := m.ReadChunk(n)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	state := "Original"
	if edited {
		state = "bereits bearbeitet"
	}
	return mcp.NewToolResultText(fmt.Sprintf("--- Teil %d/%d (%s, %d Zeichen) ---\n%s",
		n, m.TotalParts, state, len(text), text)), nil
}

// --- put_chunk ----------------------------------------------------------

func putChunkTool() mcp.Tool {
	return mcp.NewTool("put_chunk",
		mcp.WithDescription("Schreibt die bearbeitete (z. B. übersetzte) Fassung eines Teils zurück. "+
			"Das Original bleibt unangetastet; merge_chunks setzt später alles zusammen."),
		mcp.WithString("jobId", mcp.Required(),
			mcp.Description("Auftrags-ID aus split_markdown")),
		mcp.WithNumber("part", mcp.Required(),
			mcp.Description("Teilnummer, 1-basiert")),
		mcp.WithString("text", mcp.Required(),
			mcp.Description("Der bearbeitete Markdown-Text dieses Teils")),
	)
}

func putChunkHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m, err := resolveJob(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	n := argInt(req, "part", 0)
	text, ok := args(req)["text"].(string)
	if !ok {
		return mcp.NewToolResultError("text ist erforderlich"), nil
	}
	if strings.TrimSpace(text) == "" {
		return mcp.NewToolResultError("text ist leer - das würde den Teil verlieren"), nil
	}
	if err := m.WriteChunk(n, text); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	done, missing := m.Progress()
	b := &strings.Builder{}
	fmt.Fprintf(b, "✅ Teil %d gespeichert (%d Zeichen). Fortschritt: %d/%d\n", n, len(text), done, m.TotalParts)
	if len(missing) > 0 {
		fmt.Fprintf(b, "Offen: %s\n", joinInts(missing))
		fmt.Fprintf(b, "Nächster Schritt: get_chunk(jobId=%q, part=%d)\n", m.ID, missing[0])
	} else {
		fmt.Fprintf(b, "Alle Teile da - jetzt merge_chunks(jobId=%q).\n", m.ID)
	}
	return mcp.NewToolResultText(b.String()), nil
}

// --- job_status ---------------------------------------------------------

func jobStatusTool() mcp.Tool {
	return mcp.NewTool("job_status",
		mcp.WithDescription("Zeigt Fortschritt und Teileliste eines Splits, ohne Inhalt zu laden."),
		mcp.WithString("jobId",
			mcp.Description("Auftrags-ID aus split_markdown")),
		mcp.WithString("chunksDir",
			mcp.Description("Alternativ: Chunk-Ordner mit index.json")),
	)
}

func jobStatusHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m, err := resolveJob(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	done, missing := m.Progress()
	b := &strings.Builder{}
	fmt.Fprintf(b, "Job %s - %s\nOrdner: %s\nFortschritt: %d/%d bearbeitet\n\n",
		m.ID, filepath.Base(m.SourceFile), m.Dir(), done, m.TotalParts)
	for _, p := range m.Parts {
		mark := "·"
		if tp, err := m.TargetPath(p.Part); err == nil {
			if _, err := os.Stat(tp); err == nil {
				mark = "✓"
			}
		}
		title := p.Heading
		if title == "" {
			title = "(ohne Überschrift)"
		}
		fmt.Fprintf(b, "  %s %2d  %6d Zeichen  %s\n", mark, p.Part, p.Chars, title)
	}
	if len(missing) > 0 {
		fmt.Fprintf(b, "\nOffen: %s\n", joinInts(missing))
	}
	return mcp.NewToolResultText(b.String()), nil
}

// --- merge_chunks -------------------------------------------------------

func mergeTool() mcp.Tool {
	return mcp.NewTool("merge_chunks",
		mcp.WithDescription("Setzt die Teile eines Splits wieder zu einem Dokument zusammen. "+
			"Je Teil wird die bearbeitete Fassung genommen, falls vorhanden, sonst das Original. "+
			"Wurde nichts bearbeitet, läuft zusätzlich der Roundtrip-Check gegen die Quelle."),
		mcp.WithString("jobId",
			mcp.Description("Auftrags-ID aus split_markdown")),
		mcp.WithString("chunksDir",
			mcp.Description("Alternativ: Chunk-Ordner mit index.json")),
		mcp.WithString("out",
			mcp.Description("Zielpfad (Standard: <Quelle>.<target>.md bzw. <Quelle>.merged)")),
	)
}

func mergeHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m, err := resolveJob(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	paths, translated, err := m.MergePaths()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	merged, err := split.MergeFilesGaps(paths, m.Gaps)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Fehler beim Zusammenfügen: %v", err)), nil
	}

	out := argString(req, "out")
	if out == "" {
		out = defaultMergeTarget(m)
	}
	if err := os.WriteFile(out, []byte(merged), 0o644); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Fehler beim Schreiben von %s: %v", out, err)), nil
	}

	b := &strings.Builder{}
	fmt.Fprintf(b, "✅ %d Teile zusammengefügt → %s (%d Zeichen)\n", len(paths), out, len(merged))
	if translated > 0 {
		fmt.Fprintf(b, "Davon bearbeitet: %d/%d", translated, m.TotalParts)
		if translated < m.TotalParts {
			b.WriteString(" - der Rest stammt unverändert aus dem Original")
		}
		b.WriteString("\n")
	} else if m.SourceFile != "" {
		if data, err := os.ReadFile(m.SourceFile); err == nil {
			b.WriteString(roundtripVerdict(merged, string(data)) + "\n")
		}
	}
	return mcp.NewToolResultText(b.String()), nil
}

// defaultMergeTarget legt den Zielpfad neben die Quelle.
func defaultMergeTarget(m *job.Manifest) string {
	if m.SourceFile == "" {
		return filepath.Join(m.Dir(), "merged.md")
	}
	ext := filepath.Ext(m.SourceFile)
	base := strings.TrimSuffix(m.SourceFile, ext)
	if m.Target != "" {
		return base + "." + m.Target + ext
	}
	return m.SourceFile + ".merged"
}

// joinInts formatiert eine Teilliste kompakt.
func joinInts(ns []int) string {
	parts := make([]string, len(ns))
	for i, n := range ns {
		parts[i] = fmt.Sprint(n)
	}
	return strings.Join(parts, ", ")
}
