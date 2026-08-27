// Package split zerlegt Markdown in Chunks, die sich einzeln übersetzen oder
// verarbeiten lassen, und wieder verlustfrei zusammensetzen.
//
// Zwei Regeln tragen das Ganze:
//
// Unteilbare Blöcke bleiben unteilbar. Code-Zäune, Tabellen, Listenpunkte samt
// Fortsetzungen und HTML-Elemente wandern immer vollständig in genau einen
// Chunk - auch dann, wenn sie das Zeichenbudget überschreiten. Die Zielgröße
// ist ein weiches Budget, kein Gesetz: geschnitten wird bevorzugt vor einer
// Überschrift, damit ein Chunk an einem Abschnitt beginnt und ihn ganz enthält.
//
// Der Rückweg ist byte-genau. Jeder Block merkt sich in Gap die Leerzeilen, die
// im Original hinter ihm standen; Doc.Gaps hält dieselbe Information für die
// Chunk-Grenzen. JoinGaps ist damit die exakte Umkehrung von SplitDoc:
//
//	doc := split.SplitDoc(src, 2000)
//	split.JoinGaps(doc.Chunks, doc.Gaps) == split.Canonical(src)
//
// Canonical beschreibt die einzige Normalisierung, die stattfindet: führende
// und abschließende Leerzeilen fallen weg, reine Whitespace-Zeilen werden zur
// leeren Zeile. Einrückung und harte Zeilenumbrüche bleiben erhalten.
package split
