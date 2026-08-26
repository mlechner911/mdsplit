package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TranslationIndex für den CLI-Export
type TranslationIndex struct {
	SourceFile string   `json:"source_file"`
	TotalParts int      `json:"total_parts"`
	Chunks     []string `json:"chunks"`
}

var (
	fence = "```"
	codeStartRegex = regexp.MustCompile("^[ \t]*" + "`" + "`" + "`[a-zA-Z0-9_-]*[ \t]*$")
	codeEndRegex = regexp.MustCompile("^[ \t]*" + "`" + "`" + "`[ \t]*$")
	tableRowRx     = regexp.MustCompile(`^\s*\|.*\|\s*$`)
	headingRx      = regexp.MustCompile(`^#{1,6}\s+\S`)
	listRx         = regexp.MustCompile(`^\s*(?:[-*+]\s+|\d+[.)]\s+)`)
	listSepRx      = regexp.MustCompile(`^\s+\S`)
	blankRx        = regexp.MustCompile(`^\s*$`)
	tagRx          = regexp.MustCompile(`<\/?([a-zA-Z][a-zA-Z0-9]*)[^>]*>`)
)

// htmlBlocks sind tags, die einen Block öffnen/abschließen (Stack-Parser).
var htmlBlocks = map[string]bool{
	"div": true, "p": true, "table": true, "section": true, "article": true,
	"main": true, "aside": true, "header": true, "footer": true, "nav": true,
	"details": true, "figure": true, "form": true, "ul": true, "ol": true,
	"blockquote": true, "pre": true, "h1": true, "h2": true, "h3": true,
	"h4": true, "h5": true, "h6": true,
}

// voidTags sind selbsterfüllend (einzeilige HTML-Blöcke).
var voidTags = map[string]bool{
	"hr": true, "br": true, "img": true, "iframe": true, "meta": true,
	"link": true, "input": true, "source": true, "audio": true, "video": true,
}

// isHTMLElementLine erkennt einzeilige HTML-Blöcke (Void-Tags oder
// geöffneter + geschlossener Block-Tag auf einer Zeile).
func isHTMLElementLine(line string) bool {
	if !strings.Contains(line, "<") || !strings.Contains(line, ">") {
		return false
	}
	for _, m := range tagRx.FindAllStringSubmatchIndex(line, -1) {
		tagName := strings.ToLower(line[m[2]:m[3]])
		full := line[m[0]:m[1]]
		if strings.HasPrefix(full, `<!--`) || strings.HasSuffix(full, "-->") {
			continue
		}
		if voidTags[tagName] && !strings.Contains(tagName, "/") {
			return true
		}
		if htmlBlocks[tagName] && !strings.HasPrefix(full, "</") && strings.HasSuffix(full, "/>") {
			return true
		}
	}
	return false
}
// isListItem prüft, ob eine Zeile ein Listen-Element ist.
func isListItem(line string) bool { return listRx.MatchString(line) }

// extractBlocks zerlegt Markdown in atomare Blöcke:
// Code-Zaun, Tabelle und Liste sind unteilbar (bleiben in einem Chunk),
// Prosa-Absätze sind teilbar und werden an Überschriften getrennt.
func extractBlocks(content string) []string {
	lines := strings.Split(content, "\n")
	var blocks []string

	inCode := false
	var buf []string

	flushBuf := func() {
		text := strings.TrimSpace(strings.Join(buf, "\n"))
		if text != "" {
			blocks = append(blocks, text)
		}
		buf = nil
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Code-Blöcke: Zaun bis schließender Zaun (inklusive), nie teilen
		if codeStartRegex.MatchString(trimmed) && !inCode {
			flushBuf()
			inCode = true
			buf = []string{line}
			continue
		}
		if inCode {
			buf = append(buf, line)
			if codeEndRegex.MatchString(trimmed) {
				flushBuf()
				inCode = false
			}
			continue
		}

		switch {
		case blankRx.MatchString(trimmed):
			flushBuf()

		case tableRowRx.MatchString(trimmed):
			// Tabelle: solange aufeinanderfolgende Zeile eine Zeile ist, zusammenfassen
			if listRx.MatchString(trimmed) || headingRx.MatchString(trimmed) {
				flushBuf()
			}
			buf = append(buf, line)
			for i+1 < len(lines) && tableRowRx.MatchString(strings.TrimSpace(lines[i+1])) {
				i++
				buf = append(buf, lines[i])
			}
			flushBuf()

		case headingRx.MatchString(trimmed):
			flushBuf()
			blocks = append(blocks, trimmed)

		case listRx.MatchString(trimmed) || isListItem(line):
			// Liste: Zeile und ihre Einrückungs-Continuationen sammeln,
			// abbrechen bei leerer Zeile oder nicht-eingerückter Nicht-Listenzeile
			if len(buf) > 0 && bufIsNotList(blocks, buf) {
				flushBuf()
			}
			buf = append(buf, line)
			for i+1 < len(lines) {
				next := lines[i+1]
				ntrim := strings.TrimSpace(next)
				if blankRx.MatchString(ntrim) || headingRx.MatchString(ntrim) ||
					codeStartRegex.MatchString(ntrim) || tableRowRx.MatchString(ntrim) {
					break
				}
				if listRx.MatchString(next) || (len(next) > 0 && (next[0] == ' ' || next[0] == '\t') && !isListItem(next)) {
					i++
					buf = append(buf, lines[i])
					continue
				}
				break
			}
			flushBuf()

		case listSepRx.MatchString(trimmed) && len(buf) > 0:
			// indizierte Fortsetzung der aktiven (Liste/Prosa-)Zeilen
			buf = append(buf, line)

		default:
			if len(buf) > 0 && headingRx.MatchString(trimmed) {
				flushBuf()
			}
			buf = append(buf, line)
		}
	}
	flushBuf()
	return blocks
}

// bufIsNotList liefert true, wenn der aktuelle Puffer keine Liste ist (zuvor Prosa).
func bufIsNotList(blocks []string, buf []string) bool {
	for _, l := range buf {
		if isListItem(l) {
			return false
		}
	}
	return true
}

// packChunks gruppiert Blöcke so, dass ≤ maxSize Zeichen pro Chunk bleiben.
// Regel: ein Block wandert nie in zwei Chunks.
// Bevorzugte Trennpunkte sind Überschriften (Trennung wird vor die Überschrift gesetzt),
// sonst leere Zeilen; Notfall-Trennung nur bei > 2x Zielgröße.
func packChunks(blocks []string, maxSize int) []string {
	var chunks []string
	var cur []string
	curLen := 0

	addLine := func(l string, sep bool) {
		if sep && len(cur) > 0 {
			cur = append(cur, "")
			curLen++
		}
		cur = append(cur, l)
		curLen += len(l) + 1
	}

	save := func() {
		if len(cur) > 0 {
			for len(cur) > 0 && cur[len(cur)-1] == "" {
				cur = cur[:len(cur)-1]
			}
			chunks = append(chunks, strings.TrimSuffix(strings.Join(cur, "\n"), "\n"))
			cur = nil
			curLen = 0
		}
	}

	for _, blk := range blocks {
		sepCost := 2 // Trennleerezeile + Newline
		if len(cur) == 0 {
			sepCost = 1
		}
		sz := len(blk) + sepCost

		if curLen+sz > maxSize && len(cur) > 0 {
			save()
		}
		addLine(blk, true)
	}
	save()

	if len(chunks) == 0 && len(blocks) > 0 {
		chunks = []string{strings.Join(blocks, "\n\n")}
	}

	// Warnen, wenn ein Chunk > 2x Zielgröße (unvermeidbar durch unteilbaren Block)
	for i, c := range chunks {
		if len(c) > maxSize*2 {
			fmt.Fprintf(os.Stderr, "⚠️  Chunk %d: %d Zeichen (> 2x Ziel)\n", i+1, len(c))
		}
	}

	return chunks
}

// getMarkdownChunks liest eine Datei und liefert die Chunks.
func getMarkdownChunks(path string, maxSize int) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("datei lesen: %w", err)
	}
	content := string(data)
	blocks := extractBlocks(content)
	chunks := packChunks(blocks, maxSize)
	if len(chunks) == 0 && strings.TrimSpace(content) != "" {
		chunks = []string{content}
	}
	return chunks, nil
}

// MODUS 1: eigenständiges CLI-Tool
func runCLIMode(path string, size int) {
	if path == "" {
		fmt.Println("❌ Fehler: Bitte einen Dateipfad via -file angeben.")
		os.Exit(1)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Printf("❌ Datei nicht gefunden: %s\n", path)
		os.Exit(1)
	}

	fmt.Printf("📖 Verarbeite: %s (max. %d Zeichen pro Chunk)\n", path, size)

	chunks, err := getMarkdownChunks(path, size)
	if err != nil {
		fmt.Printf("❌ Fehler beim Verarbeiten: %v\n", err)
		os.Exit(1)
	}
	if len(chunks) == 0 {
		fmt.Println("❌ Keine Chunks erstellt - Datei ist leer?")
		os.Exit(1)
	}

	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	index := TranslationIndex{SourceFile: path, TotalParts: len(chunks)}

	fmt.Printf("📦 Splitte in %d Teile...\n", len(chunks))
	outputDir := filepath.Join(filepath.Dir(path), "chunks")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("❌ Fehler beim Erstellen des Output-Ordners: %v\n", err)
		os.Exit(1)
	}

	total, mx, mn := 0, 0, int(^uint(0)>>1)
	for i, chunk := range chunks {
		fn := filepath.Join(outputDir, fmt.Sprintf("%s-part-%02d%s", filepath.Base(base), i+1, ext))
		if err := os.WriteFile(fn, []byte(chunk), 0644); err != nil {
			fmt.Printf("❌ Fehler beim Schreiben von %s: %v\n", fn, err)
			os.Exit(1)
		}
		fmt.Printf("  ✅ %s (%d Zeichen)\n", fn, len(chunk))
		index.Chunks = append(index.Chunks, fn)
		total += len(chunk)
		if len(chunk) > mx {
			mx = len(chunk)
		}
		if len(chunk) < mn {
			mn = len(chunk)
		}
	}

	idx := filepath.Join(outputDir, "index.json")
	data, _ := json.MarshalIndent(index, "", "  ")
	if err := os.WriteFile(idx, data, 0644); err != nil {
		fmt.Printf("❌ Fehler beim Schreiben des Index: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n📝 Index: %s\n", idx)

	fmt.Printf("\n📊 Statistik:\n  Total: %d Zeichen\n  Durchschnitt: %d Zeichen\n  Min: %d Zeichen\n  Max: %d Zeichen\n",
		total, total/len(chunks), mn, mx)
}

// MODUS 2: MCP-Server für Crush/Claude
func runMCPMode() {
	s := server.NewMCPServer("Markdown-Splitter", "1.0.0")

	tool := mcp.NewTool("split_markdown",
		mcp.WithString("filePath", mcp.Required(),
			mcp.Description("Pfad zur Markdown-Datei")),
		mcp.WithNumber("size", mcp.Description("Maximale Zeichenanzahl pro Chunk (Standard: 8000)")),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]interface{})
		if args == nil {
			return mcp.NewToolResultError("Ungültige Argumente"), nil
		}
		filePath, _ := args["filePath"].(string)
		if filePath == "" {
			return mcp.NewToolResultError("filePath ist erforderlich"), nil
		}
		size := 8000
		if n, ok := args["size"].(float64); ok && n > 0 {
			size = int(n)
		}
		chunks, err := getMarkdownChunks(filePath, size)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Fehler beim Verarbeiten: %v", err)), nil
		}
		if len(chunks) == 0 {
			return mcp.NewToolResultError("Keine Chunks erstellt - Datei ist leer?"), nil
		}

		b := &strings.Builder{}
		fmt.Fprintf(b, "✅ Datei in %d Chunks aufgeteilt:\n\n", len(chunks))
		for i, c := range chunks {
			fmt.Fprintf(b, "--- CHUNK %d/%d (%d Zeichen) ---\n%s\n\n", i+1, len(chunks), len(c), c)
		}
		return mcp.NewToolResultText(b.String()), nil
	})

	fmt.Fprintln(os.Stderr, "🚀 MCP-Server gestartet (Markdown-Splitter)")
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Server-Fehler: %v\n", err)
		os.Exit(1)
	}
}

func main() {
	cliMode := flag.Bool("cli", false, "Aktiviert den eigenständigen CLI-Export")
	filePath := flag.String("file", "", "Pfad zur Markdown-Datei (CLI)")
	chunkSize := flag.Int("size", 8000, "Maximale Zeichenanzahl pro Chunk")
	flag.Parse()

	if *cliMode {
		runCLIMode(*filePath, *chunkSize)
	} else {
		runMCPMode()
	}
}
