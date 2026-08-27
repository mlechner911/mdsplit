---
translation:
  tool: mdsplit
  version: 1.4.0
  url: "https://github.com/mlechner911/mdsplit"
  source: README.md
  source_sha256: 5dfef16a27b78a9b
  source_chars: 28835
  target_lang: de
  model: "qwen3.5:35b"
  mode: block
  parts: 18/18
  translated: "2026-08-27T19:32:15Z"
  machine_translation: true
---

![Ein Detektiv untersucht ein Markdown-Dokument mit einer Lupe und zieht einen Abschnitt in den Fokus, während der Rest unberührt bleibt. ](docs/hero.jpg)

<sub>Abbildung erstellt mit Nano Banana 2</sub>

# Markdown-Splitter

<sub>Maschinell übersetzt von diesem Tool: [Deutsch](README.de.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文](README.zh.md) — jede trägt ihre Herkunft im vorderen Teil.</sub>

Teilt Markdown-Dokumente in größenbeschränkte Fragmente auf, die für die Übersetzung oder Verarbeitung durch LLMs sicher sind. Atomare Blöcke (Code-Fences, Tabellen, Listen, mehrzeiliges HTML) werden niemals über Chunk-Grenzen hinweg geteilt — nur ganze Blöcke werden zwischen Chunks verschoben.

Implementiert in Go. Sechs Laufzeitmodi — Spalten, Mergen, Übersetzen, Glossar, Prüfen und Gliederung — sowie ein MCP-Server (Model Context Protocol), der dieselbe Arbeit als Chunk-für-Chunk-Auftragsworkflow bereitstellt.

![Die Pipeline: Ein Quell-Dokument wird in größenbeschränkte Teile geschnitten, ohne Code-Fences, Tabellen oder HTML zu unterbrechen; der Splitter gibt nur ein Manifest zurück, niemals den Text; jeder Teil reist als zustandslose Anfrage ohne Chat-Verlauf zu einem lokalen LLM und kehrt über put_chunk zurück. ](docs/pipeline.svg)

## Motivation

Das Übersetzen eines langen Markdown-Dokuments mit einem lokalen Modell ist eher ein Kontextproblem als ein Sprachproblem. Ein 60 KB großes Handbuch passt nicht in ein 8k-Token-Fenster, sodass es zerschnitten werden muss — und die naiven Schnitte sind genau die schädlichen. Spaltet man alle N Zeichen, endet eine Code-Fence zur Hälfte in einem Stück und zur Hälfte im nächsten; das Modell übersetzt pflichtbewusst die verwaiste Hälfte, benennt die Bezeichner um, und der Block schließt sich nie wieder. Spaltet man an Leerzeilen, verliert eine Tabelle ihre Kopfzeile. Spaltet man nur an Überschriften, ist ein Abschnitt 200 Bytes groß, während der nächste 40 KB beträgt.

Das Übergeben der gesamten Datei an das Modell und die Aufforderung, sich selbst zu chunken, hilft ebenfalls nicht: Die Datei ist dann bereits im Kontext, was genau zu vermeiden war.

Also führt dieses Werkzeug den Schnitt außerhalb des Modells auf zwei Regeln durch:

1. **Einige Blöcke sind unteilbar.** Code-Fences, Tabellen, Listenelemente mit ihrer
   Fortsetzungen, HTML-Elemente. Sie bleiben ganz, selbst wenn dies bedeutet, dass sie aufgeblasen werden
   durch das Größenbudget — ein Stück, das doppelt so groß ist, ist eine Unannehmlichkeit,
   Ein Chunk, der mitten im Fence endet, ist eine Beschädigung.
2. **Die Größe ist ein Ziel, kein Gesetz.** Schnitte landen vor Überschriften, sodass ein Chunk
   beginnt bei einem Abschnitt und trägt ihn ganz mit sich. Ein etwas kleineres Stück, das
   selbstständig übersetzt besser als ein vollständiger, der mitten im Satz beginnt.

Die MCP-Seite folgt aus derselben Sorge. `split_markdown` gibt einen Manifestteil zurück — Teileanzahl, Größen, Überschriften — und *nicht* den Text. Der Inhalt wird mit `get_chunk` Teil für Teil abgerufen und mit `put_chunk` zurückschreiben, sodass der Kontext flach bleibt, unabhängig davon, ob der Quelltext 10 KB oder 10 MB groß ist.

Das letzte Stück ist der Rückweg. Chunking ist nur sicher, wenn es reversibel ist, daher zeichnet das Manifest die Leerzeile an jeder Grenze auf und das Mergen wird Byte für Byte gegen den Quelltext verifiziert. Wenn die Rundreiseprüfung für das unveränderte Dokument exakt ist, hat die Pipeline nichts stillschweigend verschluckt — und jede Differenz nach der Übersetzung ist dem Modell zu verdanken, nicht dem Splitter.

Entwickelt für Crush](https://github.com/charmbracelet/crush), der ein lokales Ollama-Modell fährt, aber nichts daran ist spezifisch für eines von beiden.

## Funktionen

- **Abschnittsbezogene Chunks**: Die Größe ist ein *weiches* Budget. Der Splitter bevorzugt
  um vor einer Überschrift zu schneiden, anstatt einen Chunk bis zum Rand zu füllen, sodass ein Chunk
  beginnt bei einem Abschnitt und hält ihn ganz. Eine Überschrift beendet niemals ein Chunk.
- **Atomare Blöcke werden niemals gespalten**, egal was das Budget sagt: Code-Fences
  (jede Infozeichenfolge — ` ```js title="a.js" `, ` ``` go `, `~~~`, `````), GFM
  Tabellenzeilengruppen, Listenelemente mit ihren Fortsetzungen und mehrzeiliges HTML
  Elemente. Ein unteilbarer Block, der das Budget überschreitet, erhält sein eigenes Chunk.
  und ein Hinweis auf stderr.
- **Byte-exakte Rundreiseprüfung**: `index.json` erfasst die Leerzeilenlücke an jeder
  Chunk-Grenze, sodass das Zusammenführen den Quelltext Byte für Byte reproduziert. Das einzige
  Normalisierung ist dokumentiert in `Canonical()`: führende/ende Leerzeilen
  werden abgeschnitten und Zeilen, die nur aus Leerzeichen bestehen, werden leer. Einrückung und harte
  Zeilenumbrüche (zwei Leerzeichen am Ende) bleiben erhalten.
- **CLI-Modus**: schreibt `<name>-part-NN.md` Dateien sowie eine `index.json`-Manifestdatei (Quelltext, Anzahl der Teile, sortierte Chunk-Liste) neben die Quelldatei.
- **Mergen-Modus**: `-merge -dir chunks/` setzt die Chunks über das Manifest zusammen und meldet, ob das Ergebnis byte-identisch, whitespace-identisch oder divergierend ist
- **MCP-Auftragsworkflow**: `split_markdown` gibt ein Manifest zurück, nicht das Dokument.
  Inhalt bewegt sich Teil für Teil über `get_chunk` / `put_chunk`, sodass Kontext
  bleibt konstant, unabhängig davon, wie groß der Quelltext ist – was den eigentlichen Punkt ausmacht
  des Ausführens gegen ein kleines lokales Modell.

## Anforderungen

- Go 1.25+
- Optional: [taskfile](https://taskfile.dev/) (`go install github.com/go-task/task/v3/cmd/task@latest`)
- Optional, für die Übersetzung: ein beliebiger mit OpenAI kompatibler Endpunkt — ein lokaler
  [Ollama](https://ollama.com/) oder [LM Studio](https://lmstudio.ai/) Server, oder
  die OpenAI-API. Pro Prozess konfiguriert, nie pro Anfrage; siehe
  [Übersetzung](#translation).

## Build & Test

```bash
# with Taskfile
task build     # -> bin/mcp-md-splitter
task test      # go test -v ./...
task vet
task check     # vet + test + build (CI-style)
task roundtrip # split the fixture, merge it back, fail unless byte-identical
task install   # go install -> ~/go/bin (global `mcp-md-splitter`)
task clean     # removes bin/ and chunks/

# or plain Go
go build -ldflags "-X main.version=$(cat VERSION)" -o bin/mcp-md-splitter ./cmd/mcp-md-splitter
go test ./...
```

## Verwendung

### CLI-Modus

```bash
./bin/mcp-md-splitter -cli -file docs/integration.md -size 4000
./bin/mcp-md-splitter -cli -file docs/integration.md -size 4000 -target de
```

Ausgabe in ein `chunks/` Verzeichnis neben der Quelldatei geschrieben:

```
chunks/
├── integration-part-01.md
├── integration-part-02.md
├── integration-part-03.md
└── index.json          # Manifest: id, source_file, total_parts, size, target,
                        #           gaps[], parts[{part,file,chars,heading}]
```

### Zusammenführungsmodus

```bash
# reassemble via chunks/index.json
./bin/mcp-md-splitter -merge -dir chunks/
./bin/mcp-md-splitter -merge -dir chunks/ -out combined.md
```

Das Ergebnis wird neben dem Quelltext als `<source>.merged` oder als `<source>.<target>.md` geschrieben, wenn die Teile übersetzt wurden. Jeder Teil verwendet seine bearbeitete Version, falls eine existiert, andernfalls die Originalversion, sodass ein halb abgeschlossener Durchlauf noch Mergen ermöglicht.

Die Rundreiseprüfung vergleicht zuerst byte-exakt (`Canonical`) und erst dann tolerant (`Normalize`), sodass die Normalisierung keine echten Unterschiede mehr verbergen kann. Sie meldet byte-identisch / nur Leerzeichen / abweichend.

Ohne eine `index.json` werden alle `*-part-NN.md`-Dateien in lexikographischer Reihenfolge zusammengeführt; ohne die `gaps` aus dem Manifest wird an jeder Grenze eine Leerzeile angenommen.

### MCP-Modus (Standard)

Ausführen ohne Flags, um den stdio MCP-Server zu bedienen. Die Tools bilden einen **Auftrag**Workflow, sodass ein kleines lokales Modell niemals das gesamte Dokument halten muss: `split_markdown` schreibt die Teile auf die Festplatte und gibt nur ein Manifest zurück, dann bewegt sich der Inhalt Teil für Teil.

| Werkzeug | Argumente | Rückgabe |
|---|---|---|
| `split_markdown` | `filePath`, `size` (8000), `target`, `outDir` | nur Manifest — `jobId`, Größe und Überschrift pro Teil. **Nicht** der Inhalt. |
| `get_chunk` | `jobId`, `part` | der Text dieses Teils (die bearbeitete Version falls vorhanden) |
| `put_chunk` | `jobId`, `part`, `text` | speichert den übersetzten Teil, meldet Fortschritt und das nächste offene Teil |
| `job_status` | `jobId` \| `chunksDir` | Fortschritt und Teileliste, kein Inhalt |
| `merge_chunks` | `jobId` \| `chunksDir`, `out` | reassembliert; bearbeitete Teile gewinnen, unberührte Teile fallen auf das Original zurück |
| `translate_chunk` | `jobId`, `part`, `language`, `mode` | übersetzt ein Teil über den konfigurierten Endpunkt; es kommt nur eine Statuszeile zurück |
| `build_glossary` | `jobId`, `language`, `terms` | schlägt die Terminologie des Dokuments vor und schreibt `glossary.json` zur Prüfung vor |
| `outline` | `filePath` | jeder Überschrift mit der Größe des Abschnitts, den sie öffnet. **Kein Text.** Kein Auftrag nötig |
| `read_section` | `filePath`, `section` | ein Abschnitt wortwörtlich, Blöcke intakt. Kein Job erforderlich |

Ein Übersetzungslauf sieht so aus:

```
split_markdown(filePath="doc.md", size=2000, target="de")
  → jobId dfd9fa33cd, 11 Teile          (686 chars back, not 10 KB)
get_chunk(jobId, part=1) → translate → put_chunk(jobId, part=1, text=…)
  … repeat; put_chunk names the next open part each time …
merge_chunks(jobId) → doc.de.md
```

`put_chunk` berührt den ursprünglichen Chunk nie, sodass ein Lauf fortgesetzt, Teil für Teil neu ausgeführt oder halb fertig zusammengeführt werden kann. Ohne Änderungen wird `merge_chunks` zusätzlich die byte-exakte Rundreiseprüfung gegen den Quelltext durchführen.

Chunks landen in `chunks/` neben dem Quelltext (überschreiben mit `outDir`); der `jobId` ist ein stabiler Hash aus Pfad des Quelltexts plus Budget, sodass eine erneute Aufteilung derselben Datei mit demselben Budget denselben Auftrag wiederverwendet.

Einmal installieren, dann den Befehl bei jedem Client registrieren – kein Pfad erforderlich (`go install` landet in `~/go/bin`):

```bash
go install github.com/mlechner911/mdsplit/cmd/mcp-md-splitter@latest
# from a checkout: task install
```

Das Repo stellt ein projektlokales `.mcp.json` bereit, das genau dies für Sie erledigt, sodass ein Client, der aus diesem Verzeichnis gestartet wird (Crush, Claude Desktop, OpenCode, …), den Server automatisch erkennt:

```json
{
  "mcpServers": {
    "md-splitter": {
      "command": "mcp-md-splitter"
    }
  }
}
```

Ein globaler `crushrc`-Eintrag funktioniert genauso: `mcp add md-splitter --command mcp-md-splitter`.

## Übersetzung

Mit einem konfigurierten Endpunkt kann der Splitter die Übersetzung selbst ausführen, einen isolierten Antrag pro Stück. Nichts sammelt sich zwischen ihnen an, sodass ein 10 MB-Dokument pro Schritt genauso viel kostet wie ein 10 KB-Dokument.

Endpoint-Einstellungen sind **Prozesskonfiguration, niemals Tool-Argumente**. Ein Token, das über einen Tool-Aufruf weitergegeben wird, würde in das Gesprächsprotokoll des Clients gelangen, und eine vom Aufrufer gewählte URL würde ein Tool, das lokale Dateien liest, zu einem Exfiltrationskanal machen, sobald ein übersetztes Dokument eine injizierte Anweisung enthält. `MDSPLIT_LLM_TOKEN` hat aus Absicht kein Flag, damit es auch nicht in der Prozessliste erscheint.

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

Via MCP the same loop is `translate_chunk(jobId, part)` once per part; only a
one-line status comes back, never the text.

### Zwei Modi

| | `-mode block` (Standard) | `-mode chunk` |
|---|---|---|
| Was gesendet wird | Nur Prosafragmente | Der gesamte Teil, Code maskiert |
| Code-Fences, HTML | Niemals übertragen | Ersetzt durch `⟦n⟧`-Sentinels |
| Aufzählungszeichen, Pipes, Einrückung | Wörtlich reproduziert | Gesendet, danach geprüft |
| Struktur | Garantiert | Verifiziert und bei Fehler abgelehnt |
| Anfragen pro Teil | Eine pro Fragment | Eine |
| Benötigt ein anweisungsfolgendes Modell | Nein | Ja |

Blockmodus ist der Standard, weil er Schäden unmöglich macht statt sie erkennbar zu machen: Ein Modell kann Code nicht umschreiben, den es nie erhalten hat. Das macht auch ein reines Übersetzungsmodell nutzbar — TranslateGemma und seine Artgenossen akzeptieren einen Text und ein Sprachpaar, ohne dass eine Regel wie „den Code in Ruhe lassen“ eingebracht werden kann.

Der Chunk-Modus hält den Prosa zusammenhängend, was darauf beruht, dass die Wortreihenfolge über einen Satz hinweg und nicht innerhalb davon entschieden wird. Das Maskieren des Codes entfernt zunächst ein Viertel bis ein Drittel der Eingabe bei typischem technischem Markdown, was entscheiden kann, ob ein Teil überhaupt in ein kleines Kontextfenster passt.

In beiden Modi werden Inline-Code, URL-, Link- und Bildziele, Referenzlinks, Fußnoten und Inline-HTML maskiert. Das *Alt-Text* von Bildern bleibt absichtlich übersetzbar: Es ist Prosa, die ein Leser sieht, während der Pfad nicht sichtbar ist.

### Glossar

Da jeder Chunk isoliert übersetzt wird, hindert nichts ein Modell daran, denselben Begriff auf zwei verschiedene Arten in zwei Chunks wiederzugeben. Gemessen an diesem README ist ein Begriff über vier Sprachen hinweg viermal abgewichen:

| "Code-Fences" wurden | |
|---|---|
| Spanisch | delimitador de código |
| Französisch | **un code** — der Begriff einfach weggelassen |
| Chinesisch | 代码块 |
| Deutsch | Code-Abschnitte |

```bash
mcp-md-splitter -glossary -dir chunks/ -lang es   # writes chunks/glossary.json
$EDITOR chunks/glossary.json                      # ← the point of the exercise
mcp-md-splitter -translate -dir chunks/ -lang es  # picks it up automatically
```

Kandidaten werden **ohne ein Modell** gefunden: Wörter und Phrasen, die sich über mehrere Chunks wiederholen und auch innerhalb eines Codes oder einer Kennung im Dokument vorkommen — „Chunk" ist Prosa in einer Zeile und `chunks/` in der nächsten. Nur kennungsähnliche Tokens zählen innerhalb eines Zauns, da umzäunte Blöcke voller englischer Kommentare und JSON-Werte sind, und das Zählen davon ließ „without" und „returns" wie technische Begriffe erscheinen.

Sie werden dann in **einer** Anfrage übersetzt, nicht pro Chunk. Ein Durchlauf pro Chunk würde eine fragile strukturierte Ausgabe mit der wertvollen verknüpfen — ein JSON-Parse-Fehler würde auch eine Übersetzung kosten — und es würde das Glossar von der Reihenfolge abhängig machen, in der die Chunks verarbeitet wurden.

Das Glossar wird **vor** der Übersetzung erstellt und dann eingefroren. Das Wachsenlassen eines Glossars während der Übersetzung würde dazu führen, dass die frühesten Teile mit einem leeren Glossar und die letzten mit einem vollständigen Glossar versehen werden, wodurch genau die Inkonsistenz, die es zu beseitigen gilt, in die zuerst bearbeiteten Teile eingebacken wird und ein einzelnes Teil unmöglich macht, es eigenständig neu zu bearbeiten.

Ein Wert, der als Satz zurückgegeben wird, wird abgelehnt statt gespeichert. Dies ist nicht kosmetisch: Jeder Eintrag wird in jedes Prompt injiziert, das seinen Begriff erwähnt, als `term = value`, sodass ein Satz dort die Übersetzung steuert statt sie zu schärfen. Im Vergleich zu einem 7B-Modell kam `chunk starts` als *"un bloque que comienza en mitad de una fence es dañino."* zurück.

Nur Einträge, deren Begriff tatsächlich in einem Chunk vorkommt, werden mit diesem gesendet, sodass ein Glossar mit 200 Einträgen nicht jeden Prompt aufbläht.

`glossary.json` soll bearbeitet werden. „Interface = Schnittstelle" ist eine Entscheidung, keine Tatsache, und dies ist der günstigste Punkt im Pipeline-Prozess, um dies zu korrigieren — ein paar Minuten hier sparen das erneute Lesen von elf übersetzten Abschnitten.

#### Ausgangssprache

Candidate extraction assumes an **English source**, und nicht nur im Prompt.
Seine Stopwortliste ist englisch, und es findet Wörter durch Aufteilung an Leerzeichen. Übergeben Sie `-source-lang`, um dies zu ändern; das Tool teilt Ihnen dann mit, was Sie erwarten können, anstatt stillschweigend eine Liste von Artikeln und Präpositionen zurückzugeben:

| Quelle | Was passiert |
|---|---|
| Englisch (Standard) | wofür der Extraktor entwickelt und gemessen wurde |
| Deutsch, Spanisch, Französisch, … | funktioniert, warnt jedoch: Funktionswörter werden unter den Kandidaten erscheinen und müssen gelöscht werden |
| Chinesisch, Japanisch, Koreanisch, Thai | lehnt ab — es gibt keine Leerzeichen zum Spalten, also schreiben Sie das Glossar von Hand |

Das Sprachpaar ist in `glossary.json` als `source_lang` / `target_lang` verzeichnet, da ein Glossar nur für das Paar gültig ist, für das es erstellt wurde.

Die Übersetzung selbst hat keine solche Einschränkung: `-source-lang` erreicht die Prompt-Vorlage, und die Blockmodus-Struktur garantiert gilt unabhängig von den Sprachen. Es ist nur die *Terminologieextraktion*, die englischgeprägt ist.

### Modelle, die ein Anforderungsformat vorschreiben

Ein Übersetzungsmodell möchte die Anfrage oft in einer bestimmten Form haben.
`zongwei/gemma3-translator` erwartet `Translate from English to German: <text>`
und hat keinen anderen Weg, die Zielsprache zu lernen — reiner Text enthält keine Informationen.

```bash
mcp-md-splitter -translate -dir chunks/ -lang de \
  -llm-model zongwei/gemma3-translator:1b -llm-user-template gemma3-translator
```

`-llm-user-template` nimmt eine Go-Vorlage mit denselben Feldern oder eine Abkürzung entgegen: `gemma3-translator` für das obige Format, `translategemma` für TranslateGemma's eigene Anweisung als reine Benutzer-Nachricht. Wenn sie festgelegt ist, **wird keine Systemnachricht gesendet**: Ein Aufrufer, der die Nachricht selbst gestaltet, entscheidet, ob die Regeln darin enthalten sein sollen. Genau das macht ein Modell mit einem eingebauten System-Prompt überhaupt nutzbar — eine Systemnachricht von uns würde ihn ersetzen.

Das gleiche Modell kann je nach Anbieter entweder einen Mechanismus benötigen.
Der Chat-Templat von TranslateGemma lehnt jede OpenAI-ähnliche Anfrage in LM Studio ab, daher ist dort `-llm-transport completions -llm-template
translategemma` erforderlich. Ollamas Dokumentation des Dokuments weist die Anweisung stattdessen dem Auftrag des Aufrufers zu, daher ist dort `-llm-user-template translategemma` ausreichend. Beide rendern denselben Text, einschließlich der zwei Leerzeilen vor dem Abschnitt, den seine Modellkarte hervorhebt.

### Modelle, deren Chat-Template unsere Anfrage nicht berücksichtigt

Einige Modelle liefern eine Chat-Vorlage mit, die von einer OpenAI-kompatiblen Schicht nicht erfüllt werden kann. TranslateGemma ist der Fall im Punkt: Es lehnt eine Systemrolle direkt ab ("Konversationen müssen mit einem Benutzer-Prompt beginnen") und erwartet, dass der Benutzer-Inhalt eine Abbildung ist, die `source_lang_code` und `target_lang_code` enthält — Felder, die das OpenAI-Schema entfernt, bevor die Vorlage sie überhaupt sieht. Jede Variante gibt HTTP 400 zurück.

Sein Template erweist sich als Mittel zur Erzeugung gewöhnlicher englischer Prosa und nicht exotischer Kontroll-Tokens, sodass die Darstellung des Turns hier statt auf dem Server das gesamte Problem umgeht:

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

Die Vorlage ist eine Go-Vorlage mit `.System`, `.User`, `.SourceLang`, `.TargetLang`, `.SourceLangName` und `.TargetLangName`, sodass eine Konfiguration jedes Sprachpaar abdeckt; `-llm-template translategemma` ist eine Abkürzung für das obige, und `MDSPLIT_LLM_STOP` legt die Stop-Sequenzen fest (Standard: `<end_of_turn>`).

Der Blockmodus ist hier der richtige Begleiter — ein Modell, das ein Sprachpaar und nichts anderes nimmt, hat keinen Kanal für eine Regel wie „den Code in Ruhe lassen“, daher muss die Struktur garantiert statt angefordert werden.

**Ein Modell ohne Anweisungskanal kann auch kein Glossar verwenden.** Das ist der zu abwägende Kompromiss: Ein auf Übersetzung spezialisiertes Modell übersetzt in der Regel einen Satz besser, aber es kann nicht angewiesen werden, dass *Fence* im gesamten Handbuch auf eine bestimmte Weise wiedergegeben wird. Bei einem langen Dokument ist eine konsistente Terminologie oft wichtiger als bei jedem einzelnen Satz, sodass ein anweisungsfolgendes Allgemeinmodell mit einem überprüften Glossar häufig ein besseres Ergebnis erzielt als ein besserer Übersetzer, der blind arbeitet.

### Herkunft und Staleness

Jede Spalte zeichnet auf, woher sie stammt, in `index.json`, ob sie jemals übersetzt wird oder nicht:

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

`source_sha256` ist das Feld, das seinen Wert beweist. Größe und Datum sind informativ; der Hash ermöglicht es einem späteren Lauf, die Frage zu beantworten, die für eine gepflegte Dokumentation tatsächlich von Bedeutung ist:

```bash
mcp-md-splitter -check -dir chunks/
# source: CHANGED since the split - re-split and retranslate   (exit 1)
```

Eine Übersetzung, die stillschweigend veraltet ist, ist schlimmer als eine, die offensichtlich fehlt, und nichts anderes in der Pipeline würde es bemerken.

`-merge -stamp` schreibt den Eintrag zusätzlich in die eigene YAML-Vorschrift des Dokuments, damit eine *Person* ihn sehen kann – nicht zuletzt `machine_translation: true`, da eine maschinelle Übersetzung sonst genau so aussieht wie etwas, das jemand geschrieben und geprüft hat.

Es fügt niemals einfach etwas voran. Ein Dokument, das bereits Frontmatter enthält, würde durch einen zweiten `---`-Block zerstört, da das ursprüngliche Frontmatter nicht länger als Metadaten gilt und zum Inhalt wird; stattdessen wird ein bestehender Block in place bearbeitet, und das zweimalige Stempeln ersetzt den Eintrag, anstatt ihn zu duplizieren. Die Endpunkt-URL wird absichtlich nicht aufgezeichnet: Ein Modellname ist Provenienz, eine interne Adresse ist ein Leck.

Zwei Konsequenzen, die man kennen sollte. Die Rundreiseprüfung wird gegen den *nicht gestempelten* Text ausgeführt, sonst würde sie nach dem ersten Stempel für immer „unterschiedlich" melden. Und der Zeitstempel macht das Zusammenführen nicht idempotent, sodass ein Dokument, das im Zeitplan neu erstellt wird, bei jeder Ausführung einen Unterschied anzeigt, auch wenn sich nichts geändert hat — weshalb das Stempeln optional ist und die Hashsumme, nicht das Datum, das tragende Feld darstellt.

### Was vor dem Speichern geprüft wird

- Jeder Sentinel muss genau einmal zurückkehren. Ein Modell dazu anweisen, zu bewahren
  Sie sind Höflichkeit; das Zählen ist der Mechanismus.
- `finish_reason` muss `stop` sein. Eine abgeschnittene Antwort verliert Text stillschweigend, was
  ist das Versagen, dessen Verhinderung dieses gesamte Werkzeug dient.
- Die Antwort muss die Struktur der Quelle aufweisen: gleiche Blockarten in derselben
  Reihenfolge, Code-Fences Byte für Byte, gleiche Überschriftenebenen und Tabellenzeilenzahlen.
  Der Text kann sich vollständig ändern – andernfalls wäre die Prüfung für einen
  Übersetzung.

Ein Teil, das eines dieser Kriterien nicht erfüllt, wird **nicht gespeichert** und bleibt offen, sodass ein erneutes Ausführen genau diese wiederholt. Der ursprüngliche Chunk wird niemals überschrieben.

## Lesen statt Verarbeiten

![Lesen nach Thema: Die Outline- und Read_Section-Tools sitzen zwischen einem großen Referenzdokument und dem Agenten, benötigen keinen Auftrag und schreiben keine Dateien, und senden nur den Abschnitt, den eine Frage tatsächlich benötigt. ](docs/reading.svg)

Die oben genannten Werkzeuge lösen „*dieses gesamte Dokument verarbeiten, ohne den Kontext zu verlieren*". `outline` und `read_section` lösen die andere Hälfte: „*aus diesem Dokument antworten, ohne es vollständig lesen zu müssen*". Dasselbe Primitive — Markdown sicher schneiden und die Teile adressierbar machen — auf eine andere Frage gerichtet.

```bash
mcp-md-splitter -outline -file README.md
#      1  22841  Markdown Splitter
#      9   2133    Motivation
#    194  10554    Translation
#    231   1301      Two modes
#    257   3425      Glossary
#    ...
mcp-md-splitter -section "Two modes" -file README.md
```

Das Lesen der Gliederung dieser README kostet etwa 700 Zeichen; der Abschnitt „Zwei Modi" umfasst 1301. Eine Frage zu den Modi lässt sich aus 2 KB statt 23 KB beantworten — und die angegebene Größe deckt den gesamten Abschnitt einschließlich seiner Unterabschnitte ab, was entscheidet, ob das Lesen überhaupt erschwinglich ist.

Weder Werkzeug benötigt einen Auftrag: Es werden keine Chunks geschrieben, kein Manifest erstellt, und auch kein `jobId`. Dieses Gerät dient der Verarbeitung eines Dokuments von Anfang bis Ende; das Nachschlagen erfordert keines davon.

**Ein Abschnitt ist kein Chunk.** Chunks folgen dem Byte-Budget, Abschnitte dem Gliederungspunkt: `## Usage` erstreckt sich über mehrere Chunks, während ein kurzer Abschnitt einen mit seinem Nachbarn teilt. `read_section` gibt die Überschrift und alles darunter zurück, bis zur nächsten Überschrift desselben oder eines flacheren Levels — so wird eine Code-Fence jedes Mal vollständig zurückgegeben.

Adressieren Sie einen Abschnitt nach Titel oder nach Pfad (`"Usage > CLI"`), wenn derselbe Titel mehrmals vorkommt. Ein mehrdeutiger Titel wird abgelehnt, wobei die Kandidaten aufgelistet werden, anstatt zu raten.

Der Unterschied zu einem Retrieval-Server, der auf Embeddings basiert, besteht darin, dass dies **exakt statt ähnlich** ist: Sie erhalten den Abschnitt, den das Dokument selbst definiert, ohne Index erstellen zu müssen, nichts synchronisieren zu müssen und kein Fenster, das mitten in einem Block beginnen kann.

## Ihrem Agenten erklären, wie er ihn verwendet

Das Registrieren des Servers reicht nicht aus. Die Tool-Beschreibungen erläutern, was jedes Tool tut, aber ein Modell liest die Beschreibung eines Tools erst dann, wenn es sich bereits entschieden hat, es aufzurufen — daher kommen die Reihenfolgeanweisungen zu spät. Fügen Sie dies in Ihr Projekt-`AGENTS.md`, `CLAUDE.md` oder System-Prompt ein:

```markdown
## Markdown documents

The `md-splitter` MCP server is available. Use it instead of reading Markdown
files directly.

- **Looking something up:** call `outline(filePath)` first, then
  `read_section` for the one section you need. Do not read a whole document
  to answer a question about part of it.
- **Translating or rewriting:** `split_markdown` → `build_glossary` once →
  `translate_chunk` per part → `merge_chunks`. Never read the document into
  the conversation first; that is the cost the tool exists to avoid.
- **Never paste a chunk's text into a message.** `put_chunk` and
  `translate_chunk` write to disk. The text is supposed to travel from file to
  model and back without passing through the conversation.
- **A rejected part is not a failure to work around.** It was not stored, so
  the document is intact. Call `translate_chunk` again for that part; the
  others are unaffected.
- **Do not loop `get_chunk` over every part.** Holding all of them at once is
  the situation the split was made to prevent.
```

Diese letzte Regel ist die einzige, die es wert ist, beibehalten zu werden. Einem Modell, das ein Chunking-Tool übergeben wird, wird oft jedes Chunk abgerufen und in seinem eigenen Kontext wieder zusammengesetzt, was mehr kostet als das Lesen der Datei — das Tool wurde verwendet und sein Zweck im selben Atemzug vereitelt.

Endpoint-Einstellungen gehören in `.mcp.json` unter `env`, niemals in einen Prompt: Ein Modell kann kein Token weitergeben, das es nie erhalten hat, und genau darum geht es.

Beachten Sie, dass das eigene `AGENTS.md` dieses Repos ein anderes Dokument ist — es informiert Agenten, die *an* dem Splitter arbeiten, nicht Agenten, die ihn *verwenden*.

## Projektstruktur

Standard-Go `cmd`/`internal` Layout:

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
└── testdata/sample.md            # a real README, used as a fixture
```

Pipeline: `ExtractBlocks(content) []Block` parst Markdown in atomare Blöcke — jeder trägt seinen exakten Text, die Leerzeile `Gap`, die ihm folgte, und seinen Inhalt `Kind`. `groupBlocks` verbindet dann das, was nicht getrennt werden darf (Blöcke ohne Leerzeile dazwischen; eine Überschrift und ihr Abschnitt). `packRanges` füllt Chunk aus diesen Gruppen und bevorzugt einen Schnitt vor einer Überschrift. `SplitDoc(content, max)` gibt `Doc{Chunks, Gaps}` zurück; `JoinGaps(chunks, gaps)` ist das genaue Inverse.

## Entwicklungshinweise

- **Alles, was ein Benutzer oder ein Modell liest, ist Englisch**: CLI-Ausgabe, Fehler
  Nachrichten, Flagge Hilfe und die MCP-Werkzeugbeschreibungen. Code-Kommentare und Tests
  Fehlermeldungen sind auf Deutsch — diese Trennung ist absichtlich, also behalten Sie sie bei.
- Keine Markdown-AST-Bibliothek. Der Parser ist absichtlich zeilenbasiert: Eine AST würde
  müssen wieder auf den Quelltext abgesenkt werden, um einzuschalten, und die Quelltextbytes zu erhalten
  genau ist das eine Ding, auf das der Round-Trip-Vertrag angewiesen ist.
- `TestRoundtrip_ProjectDocs` führt den Splitter über jedes `*.md` im Repository aus
  root bei drei Budgets und behauptet Byte-Exaktheit – das günstigste echte Korpus
  verfügbar ohne das Repo zu verlassen.

## Kontinuierliche Integration

[`.github/workflows/ci.yml`](.github/workflows/ci.yml) läuft bei jedem Push:
`gofmt`/`go vet`/`go mod tidy` einmal, die Testsuite auf Linux, macOS und Windows
(der Splitter ist hauptsächlich für Pfadbehandlung zuständig, und die Job-Registrierung befindet sich in
`os.UserCacheDir()`), sowie eine Rundreiseprüfung, die jede
Markdown-Datei im Repository bei drei Budgets spaltet und mergt, wobei der Vorgang fehlschlägt, wenn nicht jede Datei byte-identisch zurückkommt.

## Lizenz

MIT © 2026 Michael Lechner — siehe [LICENSE](LICENSE).
