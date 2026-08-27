package split

import (
	"fmt"
	"os"
	"strings"
)

// Canonical ist die Form, die Split + JoinGaps byte-genau reproduzieren:
// Leerzeilen am Dokumentanfang und -ende fallen weg, das Dokument endet mit
// genau einem "\n", und reine Whitespace-Zeilen werden zur leeren Zeile.
// Alles andere - Einrückung, harte Zeilenumbrüche (zwei Endleerzeichen),
// Leerzeilenläufe im Text - bleibt unangetastet.
func Canonical(s string) string {
	lines := blankOut(strings.Split(s, "\n"))
	a, b := 0, len(lines)
	for a < b && lines[a] == "" {
		a++
	}
	for b > a && lines[b-1] == "" {
		b--
	}
	if a >= b {
		return ""
	}
	return strings.Join(lines[a:b], "\n") + "\n"
}

// Normalize ist der tolerante Vergleich: zusätzlich werden Leerzeilenläufe auf
// eine Leerzeile eingedampft und hängende Endleerzeichen entfernt. Nur für die
// Diagnose gedacht - für den Vertrag gilt Canonical.
func Normalize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var out []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimRight(l, " \t\r")
		if len(out) > 0 && out[len(out)-1] == "" && l == "" {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n") + "\n"
}

// JoinGaps setzt Chunks in gegebener Reihenfolge zusammen. gaps[i] ist die
// Zahl der Leerzeilen zwischen Chunk i und i+1; fehlende Einträge gelten als
// eine Leerzeile.
func JoinGaps(chunks []string, gaps []int) string {
	var parts []string
	var sep []int
	for i, c := range chunks {
		c = strings.Trim(c, "\n")
		if strings.TrimSpace(c) == "" {
			continue
		}
		if len(parts) > 0 {
			g := 1
			if i-1 >= 0 && i-1 < len(gaps) && gaps[i-1] >= 1 {
				g = gaps[i-1]
			}
			sep = append(sep, g)
		}
		parts = append(parts, c)
	}
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteString(strings.Repeat("\n", sep[i-1]+1))
		}
		b.WriteString(p)
	}
	return b.String() + "\n"
}

// Join setzt Chunks mit je einer Leerzeile zusammen.
func Join(chunks []string) string { return JoinGaps(chunks, nil) }

// MergeFiles liest die Dateien in der angegebenen Ordnung und fügt sie mit je
// einer Leerzeile zusammen.
func MergeFiles(paths []string) (string, error) { return MergeFilesGaps(paths, nil) }

// MergeFilesGaps ist der exakte Rückweg: mit den Abständen aus dem Index
// entsteht wieder Canonical(Original).
func MergeFilesGaps(paths []string, gaps []int) (string, error) {
	chunks := make([]string, 0, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("chunk lesen %s: %w", p, err)
		}
		chunks = append(chunks, string(data))
	}
	return JoinGaps(chunks, gaps), nil
}
