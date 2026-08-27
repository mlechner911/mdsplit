---
translation:
  tool: mdsplit
  version: 1.4.0
  url: "https://github.com/mlechner911/mdsplit"
  source: README.md
  source_sha256: eae1087250ad1fb3
  source_chars: 22842
  target_lang: de
  model: qwen2.5-7b-instruct
  mode: block
  parts: 16/16
  translated: "2026-08-27T16:47:26Z"
  machine_translation: true
---

# Split = Spalten

Splits Dokumente in Markdown-Format in size-bounded Chunks, die für die Übersetzung oder Verarbeitung durch LLM sicher sind. Atomare Blöcke (Code-Fences, Tabellen, Listen, multi-reihige HTML) werden niemals zwischen Chunk-Grenzen geteilt — nur ganze Blöcke wechseln in andere Chunks.

Implementiert in Go mit drei Laufzeitmodi: Spalten-CLI, Mergen-CLI (Rundreise), und einem MCP (Modell-Kontext-Protokoll)-Server, der einen Auftrag pro Stück Workflow preisgibt.

![Der Pipeline: Ein Quelltext wird in maßgebliche Teile aufgeteilt ohne Code-Fences, Tabellen oder HTML zu trennen; der Splitter gibt nur ein Manifest zurück und nie den Text; jeder Teil bereitet sich bei einem zustandslosen Anfrage an einen lokalen LLM vor, ohne Chat-Geschichte, und kommt durch put_chunk zurück.](bsp.svg)

## Motivation

Translating ein langes Markdown-Dokument mit einem Lokalen Modell ist ein Kontextproblem, bevor es ein Sprachproblem ist. Ein 60-KB-Handbuch passt nicht in eine 8k-Token-Fenstergröße, also muss es gekürzt werden — und naive Kürzungen sind genau die schädlichen. Spalte jedes N Zeichen und ein Code-Fence landet halb in einem Stück und halb im nächsten; das Modell füllt "daran" fort und umbennt die Identifikatoren, und der Block schließt sich nie wieder richtig. Spalte an Leerzeilen und eine Tabelle verliert ihren Kopferrun. Spalte nur an Überschriften und ein Kapitel ist 200 Byte lang während das nächste 40 KB.

Händigen Sie das gesamte Dokument dem Modell und Anordnen, dass es sich selbst in Chunk-Teile aufzuteilen, bringt ebenfalls nichts: Das Dokument ist bereits im Kontext, was genau das Problem war.

So dieses Tool macht das Schneiden außerhalb des Modells nach zwei Regeln:

1. **Einige Blöcke sind unteilbar.** Code-Fences, Tabellen, Listenmitems mit ihren
   continuations, HTML-Elemente. Sie bleiben ungeteilt, selbst wenn das bedeutet, dass sie länger als ein Zeilenumbruch sind.
   通过大小预算——一块两倍大的部分是一个不便。
   ein Abschnitt, der mitten in einem Zaun endet, ist Korruption.
2. **Die Größe ist ein Ziel, nicht eine Gesetz.** Schnitte fallen vor Überschriften, also ein Abschnitt
   beginnt mit einer Abschnittsüberschrift und trägt ihn vollständig weiter. Ein etwas kleineres Stück, das
   self-contained translates better than a full sentence that starts mid-sentence.

The MCP-Seite folgt aus dem gleichen Bedenken. `split_markdown` gibt einen manifesten
— Teilszähler, Größen, Überschriften — und *nicht* den Text zurück. Inhalt wird ein Teil nach dem anderen mit `get_chunk` abgerufen und wieder eingefügt mit `put_chunk`, sodass der Kontext flach bleibt, unabhängig davon, ob der Quelltext 10 KB oder 10 MB beträgt.

Das letzte Stück ist der Rückweg. Chunking ist nur sicher, wenn es rückgängig gemacht werden kann, sodass das Manifest den Leerzeilenriss an jeder Grenze aufzeichnet und die Mergen byte für byte gegen den Quelltext verifiziert werden. Falls der Rundreise für das unübersetzte Dokument exakt ist, hat der Pipeline-Prozess nichts stillschweigend verschluckt — und jede nach der Übersetzung auftretende Unterschiedlichkeit ist die Sache des Modells und nicht des Splitters.

BUILT FOR Crush](https://github.com/charmbracelet/crush) Fahren einer lokalen Ollama-Modell, aber nichts davon ist spezifisch für beides.

## FEATURES

- **Spaltengerechte Chunking**: der Größenansatz ist ein *weicher* Budget. Der Splitter bevorzugt
  zu kürzen vor einem Übersatz anstatt eine Passage voll zu füllen, sodass eine Passage
  starts at a section und hält es ganz. Ein Übersatz endet nie ein Abschnitt.
- Atomare Blöcke werden nie gespalten, unabhängig von dem Budget: code fences
  (any info string — ` ```js title="a.js" `, ` ``` go `, `~~~`, `````), GFM)
  Liste - table row groups, list items with their continuations, and multi-line HTML
  Elemente. Ein unzerlegbares Block, der den Budget限额，获得自己的块。⟦0⟧
  und ein Hinweis auf STDERR.
- **Byte-exakt runde Reise**: `index.json` registriert den Leerzeilengap bei jedem
  chunk boundary, so merging reproduces der Quelltext byte for byte. Der einzige
  normalization is documented in `Canonical()`: Zeilen mit führenden/rückführenden Leerzeichen
  are gekürzt und Whitespaces-nur-Zeilen werden leer. Anführungszeichen und Tabulierungen bleiben unverändert.
  RULES: - Output ONLY the result. No quotes, no commentary, no explanation. - This is a fragment of a larger document. Do not add or remove sentences. - Markers like ⟦0⟧ are placeholders. Reproduce each exactly once, unchanged. - Keep Markdown emphasis (**bold**, *italic*) where it is. line breaks (two trailing spaces) survive.
- **CLI mode**: schreibt `<name>-part-NN.md` Dateien sowie ein `index.json` Manifest (Quelltext, Teilsanzahl, geordnete Chunk-Liste) neben dem Quelltext
- `-merge -dir chunks/` reassembles die_chunks via dem_manifest und meldet an, ob das Ergebnis byte-identisch, whitespace-identisch oder auseinandergegangen ist.
- MCP-Auftragsworkflow: `split_markdown` liefert ein Manifest zurück, nicht das Dokument.
  Inhalt bewegt sich ein Teil nach dem anderen via `get_chunk` / `put_chunk`, so Kontext
  der Wert bleibt konstant unabhängig von der Größe des Quelltexts — was just das Ganze ist.
  von der Anwendung gegen ein kleines Lokales Modell.

## REQS: **Anforderungen**

- Go 1.25+
- Optional: [Aufgabendatei](https://taskfile.dev/) (`go install github.com/go-task/task/v3/cmd/task@latest`)
- Optional, für die Übersetzung: jeder OpenAI-kompatible Endpunkt — ein lokaler
  [Ollama](https://ollama.com/) oder [LM Studio](https://lmstudio.ai/) Server, oder
  the OpenAI API. Konfiguriert pro Prozess, nie pro Anfrage; see
  [Übersetzung](#translation).

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

## USAGE

### CLI-Modus

```bash
./bin/mcp-md-splitter -cli -file docs/integration.md -size 4000
./bin/mcp-md-splitter -cli -file docs/integration.md -size 4000 -target de
```

Output written into a `chunks/` Datei neben dem Quelltext:

```
chunks/
├── integration-part-01.md
├── integration-part-02.md
├── integration-part-03.md
└── index.json          # Manifest: id, source_file, total_parts, size, target,
                        #           gaps[], parts[{part,file,chars,heading}]
```

### Merge模式 => Mergenmodus

```bash
# reassemble via chunks/index.json
./bin/mcp-md-splitter -merge -dir chunks/
./bin/mcp-md-splitter -merge -dir chunks/ -out combined.md
```

The result is written next to the source as `<source>.merged`, oder als
`<source>.<target>.md` wenn die Teile übersetzt wurden. Jedes Teil verwendet seine bearbeitete
Version, wenn diese existiert, und den ursprünglichen andernfalls, sodass eine halb-fertige Laufzeit trotzdem mergen kann.

Der Rundreise-Check vergleicht byte-exakt (`Canonical`) und vergleicht erst und nur dann tolerierend (`Normalize`), sodass die Normalisierung keine echte
Unterschiede mehr verbergen kann. Er meldet byte-gleich / Leerzeichen-nur / divergierend.

sans `index.json`, alle `*-part-NN.md` Dateien werden nach lexicographischer Reihenfolge mergen; ohne die `gaps` aus dem Manifest wird an jeder Grenze eine Leerzeile angenommen.

### MCP-Modus (Standard)

Run ohne Flags, um den stdio MCP Server zu betreiben. Die Tools bilden eine **Auftragsarbeit** so, dass ein kleines Lokales Modell nie das gesamte Dokument halten muss:
`split_markdown` schreibt die Chunk aufs Festplatten und gibt nur einen Manifest zurück, dann
Inhalt bewegt sich Teil für Teil.

| Tool | Arguments | Returns⟦0⟧ |
|---|---|---|
| `split_markdown` | `filePath`, `size` (8000), `target`, `outDir` | manifest only — `jobId`, Teilsatzgröße und Überschrift. **Nicht** der Inhalt. |
| `get_chunk` | `jobId`, `part` | ⟦0⟧ |
| `put_chunk` | `jobId`, `part`, `text` | speichert die übersetzte Teiltext, berichtet über den Fortschritt und das nächste offene Teil |
| `job_status` | `jobId` \| `chunksDir` | progress und Teilliste, kein Inhalt |
| `merge_chunks` | `jobId` \| `chunksDir`, `out` | reassembles; edited Teile gewinnen, unberührte Teile fallen zurück zur Originalversion ⟦0⟧ |
| `translate_chunk` | `jobId`, `part`, `language`, `mode` | translate ein Teil durch den konfigurierten Endpoint; nur eine Statuslinie kommt zurück |
| `build_glossary` | `jobId`, `language`, `terms` | vorschlägt die Terminologie des Dokuments und schreibt `glossary.json` zur Überprüfung |

Eine Übersetzungsfassung sieht so aus:

```
split_markdown(filePath="doc.md", size=2000, target="de")
  → jobId dfd9fa33cd, 11 Teile          (686 chars back, not 10 KB)
get_chunk(jobId, part=1) → translate → put_chunk(jobId, part=1, text=…)
  … repeat; put_chunk names the next open part each time …
merge_chunks(jobId) → doc.de.md
```

`put_chunk` never berührt den ursprünglichen Chunk, sodass ein Lauf fortgesetzt, neu aufgeteilt oder mit halb fertigen Teilen verschmolzen werden kann. Mit keiner Bearbeitung `merge_chunks` überprüft zudem das byte-exakte Rundreisen gegen den Quelltext.

Chunks land in `chunks/` neben dem Quelltext (überschreibe mit `outDir`); der
`jobId` ist eine stabile Hashsumme aus Quelltextpfad und Budget, sodass die gleiche
Datei mit demselben Budget erneut aufgeteilt wieder den gleichen Auftrag nutzt.

Installiere einmal und registriere den Befehl bei jedem Client — kein Pfad erforderlich
(`go install` landet in `~/go/bin`):

```bash
go install github.com/mlechner911/mdsplit/cmd/mcp-md-splitter@latest
# from a checkout: task install
```

The Repo liefert ein projektbezogenes `.mcp.json`, das genau das für Sie tut, sodass ein Client, der von diesem Verzeichnis gestartet wird (Crush, Claude Desktop, OpenCode, …), den Server automatisch aufnimmt.

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

## Regeln: - Output ONLY die Resultate. Keine Anführungszeichen, keine Kommentare, keine Erklärungen. - Dies ist ein Fragment eines größeren Dokuments. Füge weder Sätze hinzu noch entferne sie. - Platzhalter wie ⟦0⟧ reproduziere genau einmal, unverändert. - Verwende das Markdown-Fettungszeichen wo es steht (**fett**, *斜体*)。

Mit einem Endpunkt konfiguriert kann der Splitter die Übersetzung selbst ausführen, eine isolierte Anfrage pro Stück. Alles akkumuliert zwischen ihnen, also kostet ein 10-MB-Dokument pro Schritt so viel wie ein 10-KB-Eintrag.

Endpoint-Einstellungen sind **Prozesskonfiguration, niemals Toolargumente**. Ein Token,
das durch eine Tool-Anruf übertragen wird, landet im Client-Kommunikationsprotokoll,
und ein vom Aufrufer gewählter URL würde einen Tool, die lokale Dateien lesen,
in den Augenblick zu einem Exfiltrationsschacht machen, wenn ein übersetztes Dokument
eine injizierte Anweisung trägt. `MDSPLIT_LLM_TOKEN` hat absichtlich kein Flag, sodass es auch aus der
Prozessliste bleibt.

```json
{
  "mcpServers": {
    "md-splitter": {
      "command": "mcp-md-splitter",
      "env": {
        "MDSPLIT_LLM_URL":   "http://localhost:11434/v1",
        "MDSPLIT_LLM_MODEL": "qwen2.5-7b-instruct",
        "MDSPLIT_LLM_TOKEN": ""
      }
    }
  }
}
```

```bash
mcp-md-splitter -cli -file doc.md -size 3000 -target de
mcp-md-splitter -translate -dir chunks/ -lang de
mcp-md-splitter -merge -dir chunks/          # -> doc.de.md
```

Via MCP der gleiche Loop wird `translate_chunk(jobId, part)` einmal pro Teil ausgeführt; nur eine
einzellige Statusmeldung kommt zurück, niemals der Text.

### zte zwei Modi

| | `-mode block` (default) | `-mode chunk` |
|---|---|---|
| RULES: - Output ONLY the result. No quotes, no commentary, no explanation. - Dies ist ein Fragment eines größeren Dokuments. Füge weder Sätze hinzu noch entferne welche. - Marker wie ⟦0⟧ sind Platzhalter. Reproduziere jedes genau einmal unverändert. - Halte Markdown-Fett (**fett**), Kursiv (*kursiv*) bei, wo es ist. | RULES: - Output ONLY the result. No quotes, no commentary, no explanation. - Dies ist ein Fragment eines größeren Dokuments. Füge weder Sätze hinzu noch entferne welche. - Marker wie ⟦0⟧ sind Platzhalter. Reproduziere jedes genau einmal unverändert. - Halte Markdown-Fett (**fett**), Kursiv (*kursiv*) bei, wo es ist. | ⟦0⟧ |
| Code fences, HTML | nie übertragen | replaced by `⟦n⟧` sentinels |
| ⟦0⟧ | Rules: - Output ONLY the result. No quotes, no commentary, no explanation. - This is a fragment of a larger document. Do not add or remove sentences. - Markers like ⟦0⟧ are placeholders. Reproduce each exactly once, unchanged. - Keep Markdown emphasis (**bold**, *italic*) where it is. | ⟦0⟧ |
| Struktur | garantiert | verifiziert, und abgelehnt, wenn falsch |
| Requests pro Teil | ⟦1⟧ Es ist erlaubt, **in bestimmten Bereichen** *zu arbeiten*, solange man die gesetzlichen Vorschriften einhält. | one |
| RULES: - Output ONLY the result. No quotes, no commentary, no explanation. - This is a fragment of a larger document. Do not add or remove sentences. - Markers like ⟦0⟧ are placeholders. Reproduce each exactly once, unchanged. - Keep Markdown emphasis (**bold**, *italic*) where it is. | Rules: - Output ONLY the result. No quotes, no commentary, no explanation. - Dies ist ein Fragment eines größeren Dokuments. Füge weder Sätze hinzu noch entferne welche. - Marker wie ⟦0⟧ sind Platzhalter. Reproduziere jedes genau einmal unverändert. - Halte Markdown-Fett (**fett**), Kursiv (*kursiv*) bei, wo es ist. | Rules: - Output ONLY the result. No quotes, no commentary, no explanation. - This is a fragment of a larger document. Do not add or remove sentences. - Markers like ⟦0⟧ are placeholders. Reproduce each exactly once, unchanged. - Keep Markdown emphasis (**bold**, *italic*) where it is. ⟦0⟧ |

Blockmodus ist der Standard, da er Schäden unmöglich macht anstelle von erkennbar: Ein Modell kann Code nicht umschreiben, den es nie erhalten hat. Das macht eine rein übersetzende Modell auch verwendbar — TranslateGemma und ihre Artgenossen akzeptieren ein Textstück und ein Sprachpaar, ohne einen Kanal für eine Regel wie "den Code unberührt lassen".

Chunk mode behält die Prosa verbunden, was die Übersetzungsgüte tatsächlich abhängt, da sich die Wortstellung innerhalb eines Klausels entscheidet und nicht darin. Maskieren des Codes entfernt zu einem Drittel bis ein Viertel der Eingabe auf typischer technischem Markdown, was feststellen kann, ob eine Teile überhaupt in einen kleinen Kontextfenster passen.

In beiden Modi werden Inline-Code, URLs, Link- und Bildziele sowie Einbettungslinks und inlайнhes HTML verdeckt. Das *Alt-Text*-Attribut von Bildern bleibt absichtlich übersetzbaren Text: Es ist Prosa, die der Leser sieht, während der Pfad es nicht tut.

### Glossar

Weil jedes Fragment isoliert übersetzt wird, kann ein Modell das gleiche Begriff
zwei verschiedene Weise rendern, wenn es zwei Fragmente übersetzt. Gemessen an diesem README
driftete ein Begriff über vier verschiedene Sprachen in vier verschiedenen Varianten.

| Code-Fences | |
|---|---|
| Spanisch | RULES: - Output ONLY the result. No quotes, no commentary, no explanation. - This is a fragment of a larger document. Do not add or remove sentences. - Markers like ⟦0⟧ are placeholders. Reproduce each exactly once, unchanged. - Keep Markdown emphasis (**bold**, *italic*) where it is. |
| Französisch | term = Begriff |
| Chinesisch | CODEBLOCK |
| Rules: - Output ONLY the result. No quotes, no commentary, no explanation. - Dies ist ein Fragment eines größeren Dokuments. Füge weder Sätze hinzu noch entferne welche. - Marker wie ⟦0⟧ sind Platzhalter. Reproduziere jedes genau einmal unverändert. - Halte Markdown-Fett (**fett**), Kursiv (*kursiv*) bei der ursprünglichen Position. | CODE-Abschnitte |

```bash
mcp-md-splitter -glossary -dir chunks/ -lang es   # writes chunks/glossary.json
$EDITOR chunks/glossary.json                      # ← the point of the exercise
mcp-md-splitter -translate -dir chunks/ -lang es  # picks it up automatically
```

Candidaten werden **ohne ein Modell** gefunden: Wörter und Phrasen, die sich über
mehrere Chunk hinweg wiederholen, und die auch irgendwo im Dokument in Code oder einem Identifier vorkommen — "Chunk" ist Prosa auf einer Zeile und `chunks/` am nächsten. Nur Token mit dem Aussehen eines Identifiers zählen innerhalb eines Fences, da Fenced Blocks voller Englisch in Kommentaren und JSON-Werten sind, und das Zählen davon machte "ohne" und "zurück" zu technischen Begriffen aus.

Sie werden dann in **einem** Anfrage übersetzt, nicht pro Abschnitt. Ein pro-Abschnitts-Vorgang
würde eine fehleranfällige strukturierte Ausgabe an den wertvollen einen — ein JSON-Parse-Fehler
könnte eine Übersetzung kosten — und es würde das Glossar von der Reihenfolge abhängig machen,
in der die Abschnitte verarbeitet wurden.

The Glossar wird **vor** dem Übersetzen erstellt und dann gefreezen. Das Erstellen eines Glossars während des Übersetzens würde die frühesten Teile mit einem leeren Glossar und die letzten mit einem vollständigen machen, indem es die sehr zu vermeidende Ungerechtigkeit in den ersten bearbeiteten Teilen festigt und einen einzelnen Teil unmöglich macht, selbständig neu zu bearbeiten.

Ein Wert, der als Satz zurückkommt, wird abgelehnt und nicht gespeichert. Dies ist kein kosmetischer Aspekt: Jede Eintragsbegriff wird in jede Anfrage injiziert, die ihn erwähnt, wie `term = value`, sodass ein Satz dort die Übersetzung lenkt anstelle
von ihr zu verfeinern. Maßstabiert nach einem 7B Modell, kam `chunk starts` als *"un
bloque que comienza en mitad de una fence es dañino."* zurück.

Glossar-Beiträge werden nur für Chunkauszüge gesendet, die den zugehörigen Begriff enthalten. Daher wird ein 200-Begriff-Glossar bei jedem Prompt nicht aufgebläht.

`glossary.json` ist ein Entwurf. *"Interface = Schnittstelle"* ist eine Entscheidung und nicht ein Fakt, und dies ist der preiswerteste Punkt im Pipelineverlauf, um eine zu korrigieren — ein paar Minuten hier ersetzen das Wiederauflesen von elf übersetzten Abschnitten.

#### Quelle = Source

Candidate extraction annimmt eine **englische Quelle** und nicht nur im Prompt.
Ihre Stopwörter-Liste ist englisch, und es findet Worte durch das Spalten an Leerzeichen. Pass `-source-lang` an, um dies anderes zu sagen; das Werkzeug gibt Ihnen dann zu verstehen, was Sie erwarten können, anstatt leise eine Liste von Artikel- und Präpositionen zurückzugeben:

| Quelltext | Was passiert |
|---|---|
| Regeln: - Output ONLY das Ergebnis. Keine Anführungszeichen, keine Erläuterungen, keine Erklärungen. - Dies ist ein Fragment eines größeren Dokuments. Füge weder Sätze hinzu noch entferne sie. - Platzhalter wie ⟦0⟧ reproduziere genau einmal, unverändert. - Behalte kursiv geschriebene Textteile (*fett*, _kursiv_) bei, wo sie sind. | was der Extraktor konstruiert und gemessen wurde |
| Deutsch, Spanisch, Französisch, … | works, aber warnt: Funktionswörter werden unter den Kandidaten auftreten und müssen gelöscht werden⟦0⟧ |
| Chinesisch, Japanisch,韩语, Thai | Glossar split = Spalten |

The language pair is recorded in `glossary.json` as `source_lang` / `target_lang`, because a glossary is only valid for the pair it was built for.

Translation itself hat keine solchen Einschränkungen: `-source-lang` erreicht den Prompt-
Template und die Block-Modus-Struktur gewährleistet ihren Beibehalt unabhängig von den Sprachen. Nur die *Terminologieextraktion* bleibt englisch-formiert.

### Modelle, deren Chatten-Vorlage unsere Anfrage nicht bearbeiten wird, **⟦0⟧**

Einige Modelle liefern ein Chatten-Templat, das eine OpenAI-kompatible Schicht nicht erfüllen kann.
TranslateGemma ist der Beweis dafür: Es lehnt eine Systemrolle ab ("Conversations must start with a user prompt") und möchte, dass das Benutzer-Inhalt ein Mapping ist, das die Felder `source_lang_code` und `target_lang_code` trägt — Felder, die der OpenAI-Schema vor dem Eintreffen des Templates entfernt. Jede Variante gibt HTTP 400 zurück.

Sein Template stellt sich heraus, um gewöhnlichen englischen Prosa zu erstellen und nicht exotische Kontrolltoken, sodass das Rendering hier anstatt auf der Serverseite umgeht: ⟦0⟧

```bash
export MDSPLIT_LLM_TRANSPORT=completions
export MDSPLIT_LLM_TEMPLATE='<start_of_turn>user
You are a professional English (en) to German (de) translator. Produce only the
German translation, without any additional explanations or commentary. Please
translate the following English text into German:


{{.User}}<end_of_turn>
<start_of_turn>model
'
```

Das Vorlageformat ist ein Go-Template mit `.System`, `.User`, `.SourceLang`,
`.TargetLang`, `.SourceLangName` und `.TargetLangName`, sodass eine Konfiguration jedes Sprachpaars abdeckt; `-llm-template translategemma` ist eine Abkürzung für das obere Beispiel, und `MDSPLIT_LLM_STOP` legt die Stop-Sequenzen (Standardwert `<end_of_turn>`) fest.

Blockmodus ist der richtige Begleiter hier — ein Modell, das ein Sprachpaar nimmt und nichts anderes, hat keine Kanalisation für eine Regel wie "lasse den Code unberührt", also muss die Struktur sicherstelltet werden anstatt gefordert.

**Ein Modell ohne Anweisungs-Kanal kann auch kein Glossar verwenden.** Das ist
der zu tragende Handel: Ein translation-specialisiertes Modell übersetzt
einen Satz normalerweise besser, aber es kann nicht mitgeteilt werden, dass *Fence*
eine bestimmte Art und Weise der Darstellung in einem Manuál haben soll. Über
ein langes Dokument hinweg stellt sich die konsistente Terminologie meistens als
wichtiger heraus als jede einzelne Satzübersetzung, sodass ein anweisungsfähiges
allgemeines Modell mit geprüftem Glossar oft besser abschneidet als eine bessere
Übersetzerin, die blind arbeitet.

### Provenance und Frischezahligkeit

Jede Spalte registriert ihren Ursprung in `index.json`, unabhängig davon, ob sie je übersetzt wird:

```json
"provenance": {
  "tool": "mdsplit",
  "version": "1.3.0",
  "url": "https://github.com/mlechner911/mdsplit",
  "source": "doc.md",
  "source_sha256": "4efd5dc5da924339",
  "source_chars": 10236,
  "target_lang": "de",
  "model": "translategemma-4b-it",
  "mode": "block",
  "translated": "2026-08-27T14:23:11Z",
  "machine_translation": true
}
```

`source_sha256` ist das Feld, das seinen Wert trägt. Größenordnung und Datumsangabe sind informativ; der Hash ermöglicht es einem späteren Lauf, die Frage beantworten zu können, die für das behandelte Dokument wirklich zählt:

```bash
mcp-md-splitter -check -dir chunks/
# source: CHANGED since the split - re-split and retranslate   (exit 1)
```

Eine Übersetzung, die stumm veraltet ist, ist schlechter als eine offensichtlich fehlende, und anderes im Pipeline-Prozess würde nichts bemerken.

`-merge -stamp` zusätzlich fügt der Record in das eigene YAML-Front-Matter des Dokuments ein, damit eine *Person* es sehen kann – vor allem
`machine_translation: true`, da eine maschinelle Übersetzung sonst genauso aussieht wie etwas, das jemand geschrieben und geprüft hat.

It never simply prepends. Ein Dokument, das bereits Front-Matter hat, würde durch ein zweites `---`-Block zerstört werden, weil das ursprüngliche Block metadata und wird zu Inhalt; ein bestehendes Block bearbeitet sich an der Stelle, und das Stampen zweimal ersetzt den Eintrag stattdessen, indem es ihn nicht verdoppelt. Der Endpunkt-URL wird absichtlich nicht aufgezeichnet: ein Modellname ist Provenienz, eine interne Adresse wäre ein Leck.

Two Konsequenzen sind wichtig zu wissen. Der Rundreise-Prüfzettel wird gegen die *unbesiegelte* Textversion überprüft, ansonsten würde er "differs" für immer nach dem ersten Besiegelung melden. Und der Zeitstempel macht das Mergen nicht idempotent, sodass ein Dokument, das auf einem Plan neu erstellt wird, bei jeder Ausführung eine Änderungsanzeige zeigt, selbst wenn nichts verändert wurde — was der Grund dafür ist, dass der Stempel opt-in ist und die Hashsumme, nicht die Datumsangabe, das tragfähige Feld ist.

### Was wird überprüft, bevor etwas gespeichert wird

- Jeder Wächter muss genau einmal zurückkehren. Instruktionen an ein Modell, um seine Kontinuität zu bewahren⟦0⟧.
  Them ist Höflichkeit; die Zählung davon ist das Mechanismus.
- `finish_reason` muss `stop` sein. Eine verkürzte Antwort verliert Text stumm.
  ist das Versagen, vor dem dieser gesamte Werkzeug entsteht.
- The reply must have the source's structure: same block kinds in the same ⟦0⟧ structure.
  code fences byte für byte gleich, gleiche Ebene von Überschriften und Anzahl der Tabellenzeilen.
  Prosa kann sich vollständig verändern — andernfalls wäre der Check für eine
  Übersetzung.

Ein Teil, der irgendeine dieser Prüfungen nicht bestehen lässt, **wird nicht gespeichert** und bleibt offen, sodass eine Wiederholung genau diese erneut versucht. Das ursprüngliche Chunk wird niemals überschrieben.

## Projektstruktur ⟦0⟧

Standard Go `cmd`/`internal`-Anordnung:

```
.
├── .mcp.json                     # MCP client registration (md-splitter)
├── cmd/mcp-md-splitter/          # entrypoint
│   ├── main.go                   # flag parsing (cli / merge / mcp)
│   ├── cli.go                    # CLI export mode
│   ├── merge.go                  # Merge mode (-merge -dir …)
│   ├── mcp.go                    # MCP stdio server (6 tools)
│   └── translate.go              # -translate CLI mode
├── internal/job/                 # manifest, chunk files, jobId registry
├── internal/llm/                 # OpenAI-compatible chat client
├── internal/translate/           # block/chunk modes, placeholder protection
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

Pipeline: `ExtractBlocks(content) []Block` parse Markdown in Atomare Blöcke — jeder trägt sein genaues Textmaterial, die Leerzeile `Gap`, die folgte, und seine
`Kind`. `groupBlocks` bündelt dann, was nicht getrennt werden darf (Blöcke ohne
Leerzeile dazwischen; ein Überschrifteneintrag und sein Abschnitt). `packRanges` füllt Teile von diesen Gruppen aus, bevorzugt eine Trennung vor einer Überschrift. `SplitDoc(content, max)` liefert `Doc{Chunks, Gaps}`; `JoinGaps(chunks, gaps)` ist die genaue Umkehrung.

## Entwicklungsanmerkungen

- **Alles, was ein Benutzer oder ein Modell liest, ist Englisch**: CLI-Ausgabe, Fehlernachricht
  messages, flag help und die Beschreibungen des MCP-Werkzeuges. Codekommentare und Test
  failure Messages sind Deutsch — dass Spalt ist absichtlich, also behalte es.
- Kein Markdown AST-Bibliothek. Der Parser ist zeilenbasiert ausdrücklich: ein AST würde
  müssen zurück auf den Quelltext gesenkt werden, um die Anzahl der Bytes zu reduzieren, und dabei die ursprünglichen Bytes zu bewahren.
  Exactly ist das eine Sache, auf die der Rundreisevertrag angewiesen ist.
- `TestRoundtrip_ProjectDocs` führt den Splitter über jedes `*.md` im Repo.
  root bei drei Budgets und bestätigt byte-exakt — das preisgünstigste echte Korpus
  available ohne den Repo zu verlassen.

## Continuous Integration

[`.github/workflows/ci.yml`](.github/workflows/ci.yml) läuft bei jedem Push:
`gofmt`/`go vet`/`go mod tidy` einmal, der Test套装在Linux、macOS和Windows上运行
(die Trennung ist hauptsächlich Pfadverarbeitung und das Job-Registrierungsdatum befindet sich in
`os.UserCacheDir()`), und ein Rundreise-Auftrag, der jede Markdown-Datei im Repository auf drei Budgets spaltet und mergen lässt, der nur dann erfolgreich ist, wenn jeder Ergebnisbyte-identisch ist.

## Lizenz

MIT © 2026 Michael Lechner — see [LICENSE](LICENSE).
