package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mcp-md-splitter/internal/job"
	"mcp-md-splitter/internal/llm"
	"mcp-md-splitter/internal/split"
	"mcp-md-splitter/internal/translate"

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
			return nil, fmt.Errorf("resolve directory: %w", err)
		}
		return job.Load(abs)
	}
	return nil, fmt.Errorf("pass either jobId (from split_markdown) or chunksDir")
}

// llmCfg holds the endpoint configuration for the whole server process. It is
// set once at startup and never taken from a tool argument - see package llm
// for why.
var llmCfg llm.Config

// runMCPMode startet den stdio-MCP-Server.
func runMCPMode(cfg llm.Config) {
	llmCfg = cfg
	s := server.NewMCPServer("Markdown-Splitter", version,
		server.WithToolCapabilities(true),
		server.WithInputSchemaValidation(),
	)

	s.AddTool(splitTool(), splitHandler)
	s.AddTool(getChunkTool(), getChunkHandler)
	s.AddTool(putChunkTool(), putChunkHandler)
	s.AddTool(jobStatusTool(), jobStatusHandler)
	s.AddTool(mergeTool(), mergeHandler)
	if llmCfg.Ready() {
		s.AddTool(translateChunkTool(), translateChunkHandler)
	}

	fmt.Fprintln(os.Stderr, "markdown-splitter MCP server ready ("+version+")")
	if llmCfg.Ready() {
		fmt.Fprintln(os.Stderr, "translate_chunk enabled: "+llmCfg.Describe())
	} else {
		fmt.Fprintln(os.Stderr, "translate_chunk disabled: set MDSPLIT_LLM_MODEL (and MDSPLIT_LLM_URL) to enable it")
	}
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

// --- split_markdown -----------------------------------------------------

func splitTool() mcp.Tool {
	return mcp.NewTool("split_markdown",
		mcp.WithDescription(
			"Split a Markdown file into section-aligned chunks and write them to disk. "+
				"Returns ONLY the manifest (jobId, plus each part's heading and length), never the "+
				"document text, so the context cost stays flat however large the file is. "+
				"Then work part by part: get_chunk -> translate or edit -> put_chunk, and "+
				"merge_chunks at the end. Code fences, tables, lists and HTML blocks are never "+
				"split, and chunks start at a heading wherever possible."),
		mcp.WithString("filePath", mcp.Required(),
			mcp.Description("path to the Markdown file")),
		mcp.WithNumber("size",
			mcp.Description("soft character budget per chunk (default: 8000); indivisible blocks may exceed it")),
		mcp.WithString("target",
			mcp.Description("suffix for the edited version, e.g. \"de\"; names the files put_chunk writes (default: \"out\")")),
		mcp.WithString("outDir",
			mcp.Description("directory for the chunks (default: chunks/ next to the source file)")),
	)
}

func splitHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := argString(req, "filePath")
	if path == "" {
		return mcp.NewToolResultError("filePath is required"), nil
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return mcp.NewToolResultError(fmt.Sprintf("file not found: %s", path)), nil
	}
	size := argInt(req, "size", 8000)
	if size < 200 {
		return mcp.NewToolResultError("size must be at least 200"), nil
	}

	doc, err := split.SplitFileDoc(path, size)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("split failed: %v", err)), nil
	}
	if len(doc.Chunks) == 0 {
		return mcp.NewToolResultError("no chunks produced - is the file empty?"), nil
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
		return mcp.NewToolResultError(fmt.Sprintf("cannot write chunks: %v", err)), nil
	}

	b := &strings.Builder{}
	fmt.Fprintf(b, "Split %s into %d parts (budget %d chars)\n", filepath.Base(path), m.TotalParts, size)
	fmt.Fprintf(b, "jobId: %s\ndirectory: %s\n\n", m.ID, outDir)
	for _, p := range m.Parts {
		title := p.Heading
		if title == "" {
			title = "(no heading)"
		}
		fmt.Fprintf(b, "  %2d/%d  %6d chars  %s\n", p.Part, m.TotalParts, p.Chars, title)
	}
	fmt.Fprintf(b, "\nNext: get_chunk(jobId=%q, part=1). Content arrives one part at a time, never all at once.\n", m.ID)
	return mcp.NewToolResultText(b.String()), nil
}

// --- get_chunk ----------------------------------------------------------

func getChunkTool() mcp.Tool {
	return mcp.NewTool("get_chunk",
		mcp.WithDescription("Return the text of exactly one part of a split. "+
			"If that part was already written back with put_chunk, the edited version is returned."),
		mcp.WithString("jobId", mcp.Required(),
			mcp.Description("job ID returned by split_markdown")),
		mcp.WithNumber("part", mcp.Required(),
			mcp.Description("part number, 1-based")),
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
	state := "original"
	if edited {
		state = "already edited"
	}
	return mcp.NewToolResultText(fmt.Sprintf("--- part %d/%d (%s, %d chars) ---\n%s",
		n, m.TotalParts, state, len(text), text)), nil
}

// --- put_chunk ----------------------------------------------------------

func putChunkTool() mcp.Tool {
	return mcp.NewTool("put_chunk",
		mcp.WithDescription("Write back the edited (e.g. translated) version of one part. "+
			"The original chunk is left untouched, so a run can be resumed or redone part by "+
			"part; merge_chunks assembles everything at the end."),
		mcp.WithString("jobId", mcp.Required(),
			mcp.Description("job ID returned by split_markdown")),
		mcp.WithNumber("part", mcp.Required(),
			mcp.Description("part number, 1-based")),
		mcp.WithString("text", mcp.Required(),
			mcp.Description("the edited Markdown text for this part")),
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
		return mcp.NewToolResultError("text is required"), nil
	}
	if strings.TrimSpace(text) == "" {
		return mcp.NewToolResultError("text is empty - that would discard the part"), nil
	}
	if err := m.WriteChunk(n, text); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	done, missing := m.Progress()
	b := &strings.Builder{}
	fmt.Fprintf(b, "Saved part %d (%d chars). Progress: %d/%d\n", n, len(text), done, m.TotalParts)
	if len(missing) > 0 {
		fmt.Fprintf(b, "Remaining: %s\n", joinInts(missing))
		fmt.Fprintf(b, "Next: get_chunk(jobId=%q, part=%d)\n", m.ID, missing[0])
	} else {
		fmt.Fprintf(b, "All parts are in - call merge_chunks(jobId=%q).\n", m.ID)
	}
	return mcp.NewToolResultText(b.String()), nil
}

// --- job_status ---------------------------------------------------------

func jobStatusTool() mcp.Tool {
	return mcp.NewTool("job_status",
		mcp.WithDescription("Show progress and the part list of a split without loading any content."),
		mcp.WithString("jobId",
			mcp.Description("job ID returned by split_markdown")),
		mcp.WithString("chunksDir",
			mcp.Description("alternative to jobId: the chunk directory containing index.json")),
	)
}

func jobStatusHandler(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m, err := resolveJob(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	done, missing := m.Progress()
	b := &strings.Builder{}
	fmt.Fprintf(b, "Job %s - %s\ndirectory: %s\nprogress: %d/%d edited\n\n",
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
			title = "(no heading)"
		}
		fmt.Fprintf(b, "  %s %2d  %6d chars  %s\n", mark, p.Part, p.Chars, title)
	}
	if len(missing) > 0 {
		fmt.Fprintf(b, "\nRemaining: %s\n", joinInts(missing))
	}
	return mcp.NewToolResultText(b.String()), nil
}

// --- merge_chunks -------------------------------------------------------

func mergeTool() mcp.Tool {
	return mcp.NewTool("merge_chunks",
		mcp.WithDescription("Reassemble the parts of a split into one document. Each part uses its "+
			"edited version if one exists, otherwise the original. When nothing was edited, the "+
			"round-trip is additionally verified byte for byte against the source."),
		mcp.WithString("jobId",
			mcp.Description("job ID returned by split_markdown")),
		mcp.WithString("chunksDir",
			mcp.Description("alternative to jobId: the chunk directory containing index.json")),
		mcp.WithString("out",
			mcp.Description("output path (default: <source>.<target>.md, or <source>.merged)")),
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
		return mcp.NewToolResultError(fmt.Sprintf("merge failed: %v", err)), nil
	}

	out := argString(req, "out")
	if out == "" {
		out = defaultMergeTarget(m)
	}
	if err := os.WriteFile(out, []byte(merged), 0o644); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cannot write %s: %v", out, err)), nil
	}

	b := &strings.Builder{}
	fmt.Fprintf(b, "Merged %d parts into %s (%d chars)\n", len(paths), out, len(merged))
	if translated > 0 {
		fmt.Fprintf(b, "Edited parts used: %d/%d", translated, m.TotalParts)
		if translated < m.TotalParts {
			b.WriteString("; the rest came unchanged from the original")
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

// --- translate_chunk -----------------------------------------------------

func translateChunkTool() mcp.Tool {
	return mcp.NewTool("translate_chunk",
		mcp.WithDescription(
			"Translate exactly one part of a split using the server's configured LLM endpoint, "+
				"and store the result. The chunk text and the translation never pass through this "+
				"conversation - only a one-line status comes back - so translating a 500-part "+
				"document costs the same context as translating a 5-part one. "+
				"The reply is checked against the source structure before it is stored: same blocks "+
				"in the same order, code fences reproduced verbatim. A drifted reply is rejected and "+
				"the part stays open. Call this once per part, then merge_chunks."),
		mcp.WithString("jobId", mcp.Required(),
			mcp.Description("job ID returned by split_markdown")),
		mcp.WithNumber("part", mcp.Required(),
			mcp.Description("part number, 1-based")),
		mcp.WithString("language",
			mcp.Description("target language, e.g. \"de\" or \"German\"; defaults to the language recorded at split time")),
		mcp.WithString("instruction",
			mcp.Description("replaces the default translation task, e.g. \"Rewrite the Markdown below in plain English.\"")),
		mcp.WithString("mode",
			mcp.Description("\"block\" (default) sends only prose fragments and reproduces code, tables and markers mechanically - a model cannot damage what it never receives. \"chunk\" sends the whole part at once: more context per request, but it needs a model that follows instructions.")),
	)
}

func translateChunkHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m, err := resolveJob(req)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	n := argInt(req, "part", 0)

	lang := argString(req, "language")
	if lang == "" {
		lang = m.Language
	}
	if lang == "" {
		lang = m.Target
	}
	instruction := argString(req, "instruction")
	if lang == "" && instruction == "" {
		return mcp.NewToolResultError("no target language - pass language, or set it when splitting"), nil
	}

	mode := translate.Mode(argString(req, "mode"))
	if mode == "" {
		mode = translate.ModeBlock
	}
	if mode != translate.ModeBlock && mode != translate.ModeChunk {
		return mcp.NewToolResultError(fmt.Sprintf("unknown mode %q - use \"block\" or \"chunk\"", mode)), nil
	}

	res, err := translate.Part(ctx, llm.New(llmCfg), m, n, translate.Options{
		Language:    lang,
		Instruction: instruction,
		Glossary:    m.Glossary,
		Mode:        mode,
	})
	if err != nil {
		var se *split.StructureError
		if errors.As(err, &se) {
			b := &strings.Builder{}
			fmt.Fprintf(b, "Part %d was NOT stored - the reply does not match the source structure:\n", n)
			for _, r := range se.Reasons {
				fmt.Fprintf(b, "  - %s\n", r)
			}
			b.WriteString("The part is still open; call translate_chunk again to retry it.")
			return mcp.NewToolResultError(b.String()), nil
		}
		return mcp.NewToolResultError(err.Error()), nil
	}

	done, missing := m.Progress()
	b := &strings.Builder{}
	fmt.Fprintf(b, "Translated part %d/%d in %s mode (%d -> %d chars", n, m.TotalParts, res.Mode, res.InChars, res.OutChars)
	if res.Requests > 0 && res.Mode == translate.ModeBlock {
		fmt.Fprintf(b, ", %d fragments sent", res.Requests)
	}
	if res.Kept > 0 {
		fmt.Fprintf(b, ", %d kept in the source language", res.Kept)
	}
	if res.Glossary > 0 {
		fmt.Fprintf(b, ", %d glossary terms applied", res.Glossary)
	}
	fmt.Fprintf(b, "). Structure verified. Progress: %d/%d\n", done, m.TotalParts)
	if len(missing) > 0 {
		fmt.Fprintf(b, "Next: translate_chunk(jobId=%q, part=%d)\n", m.ID, missing[0])
	} else {
		fmt.Fprintf(b, "All parts done - call merge_chunks(jobId=%q).\n", m.ID)
	}
	return mcp.NewToolResultText(b.String()), nil
}
