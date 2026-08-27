---
translation:
  tool: mdsplit
  version: 1.4.0
  url: "https://github.com/mlechner911/mdsplit"
  source: README.md
  source_sha256: 4b8a7030553edadb
  source_chars: 25144
  target_lang: es
  model: qwen2.5-7b-instruct
  mode: block
  parts: 17/17
  translated: "2026-08-27T17:08:19Z"
  machine_translation: true
---

# Split = división

Divide documentos Markdown en trozos de tamaño limitado que permanecen seguros para la traducción o procesamiento por LLM. Los bloques atómicos (bloques de código, tablas, listas, HTML multi línea) nunca se dividen a través de las fronteras de los trozos — solo se mueven bloques completos entre ellos.

Implementado en Go con tres modos de tiempo de ejecución: división CLI, fusión CLI (caminata circular), y un servidor MCP (Protocolo de Contexto del Modelo) expuesto un flujo de trabajo de tarea por trozos de texto.

![La pipeline: un documento fuente se divide en chuncks de tamaño limitado sin romper bloques de código, tablas o HTML; el splitter devuelve solo un manifiesto, nunca el texto; cada parte viaja a un LLM local como una solicitud estatal con historial de chat y regresa a través de put_chunk.](docs/pipeline.svg)

## Motivación

Translating un documento largo con un modelo local es un problema de contexto antes que uno lingüístico. Un manual de 60 KB no encaja en una ventana de 8k tokens, así que tiene que ser cortado —y los cortes ingenuos son exactamente los dañinos ones.
Divide cada N caracteres y un fence de código termina medio en una pieza y medio en la siguiente; el modelo "traduce" diligentemente la mitad huérfana, renombra los identificadores, y el bloque nunca cierra nuevamente. Divide en líneas en blanco y una tabla pierde su fila de encabezados. Divide solo en subtítulos estrictamente y una sección es de 200 bytes mientras que la siguiente es de 40 KB.

Haciendo que el modelo entregue el archivo completo y lo particione a sí mismo tampoco ayuda: el archivo ya está en el contexto en ese momento, lo cual era lo que se quería evitar.

Así que esta herramienta realiza el corte fuera del modelo, según dos reglas:

1. Some blocks are indivisible. Code fences, tables, list items with their ⟦0⟧(blocks de código), ⟦0⟧(tablas), ⟦0⟧(listas con sus elementos).
   continuaciones, elementos HTML. Se mantienen enteros incluso cuando eso significa expandirse.
   通过规模预算——一块大小是两倍的增量会带来不便，⟦0⟧
   una porción que termina en mitad de una cerca es corrupción.
2. **El tamaño es un objetivo, no una ley.** Cuts land antes que encabezados, así que un trozo
   comienza en una sección y la lleva enteramente. Un trozo ligeramente pequeño que es
   self-contenida traduce mejor que una completa que comienza en mitad de la frase.

The MCP side follows from the same concern. `split_markdown` returns un contenido
— parte count, tamaños, subtítulos — y *no* el texto. Se extrae el contenido una parte
a la vez con `get_chunk` y se escribe de vuelta con `put_chunk`, por lo que el contexto
se mantiene plano independientemente de si la fuente es 10 KB o 10 MB.

El último fragmento es el camino de vuelta. El chunking solo es seguro si es reversible, por lo que el manifiesto registra la separación por líneas en blanco en cada borde y la fusión se verifica byte a byte contra la fuente. Si el recorrido ida-vuelta es exacto para el documento sin traducir, el pipeline no devoró silenciosamente nada —y cualquier diferencia después de la traducción es debido al modelo, no al divisor.

Construido para el manejo de Crush](https://github.com/charmbracelet/crush) conduciendo un modelo local Ollama, pero nada en él está específico para ninguno de los dos.

## FEATURES

- **Secciones alineadas en bloques**: el tamaño es una *soft* división. El splitter prefiere
  para cortar antes de un encabezado en lugar de llenar una sección hasta el borde, así que una sección
  comienza en una sección y la mantiene intacta. Un encabezado nunca termina un bloque.
- **Los bloques atómicos nunca se dividen**, independientemente del presupuesto: bloques de código
  (any info string — ` ```js title="a.js" `, ` ``` go `, `~~~`, `````), GFM
  table row groups, listas items con sus continuaciones, y múltiples líneas de HTML
  elementos. Un bloque indivisible que excede el presupuesto obtiene su propia sección.
  y un mensaje en el stderr.
- **Byte-exact round-trip**: `index.json` registra el espacio en blanco entre líneas en cada
  chunk boundary, so merging reproduce la fuente byte for byte. El único
  normalización se documenta en `Canonical()`: líneas en blanco leading/trailing
  son eliminados y los líneas en blanco se convierten en vacías. La indentación y los espacios en blanco duros son mantenidos.
  Rules: - Output ONLY the result. No quotes, no commentary, no explanation. - This is a fragment of a larger document. Do not add or remove sentences. - Markers like ⟦0⟧ are placeholders. Reproduce each exactly once, unchanged. - Keep Markdown emphasis (**bold**, *italic*) where it is. line breaks (two trailing spaces) survive.
- **CLI mode**: escribe `<name>-part-NN.md` archivos más un `index.json` manifiesto (archivo fuente, cantidad de partes, lista ordenada de trozos) junto al archivo fuente
- **Modo de fusión**: `-merge -dir chunks/` reasembra los chunk mediante el manifiesto y reporta si el resultado es byte-identico, espacio-en blanco-identico o divergente.
- **Flujo de trabajo de la tarea MCP**: `split_markdown` devuelve un manifiesto, no el documento.
  contenido se mueve una parte a la vez mediante `get_chunk` / `put_chunk`, así que contexto
  stays constant no matter how large la fuente es — lo cual es el punto principal
  de ejecutar esto contra un pequeño modelo local.

## Requisitos

- Go 1.25+
- Opcional: [archivo de tarea](https://taskfile.dev/) (`go install github.com/go-task/task/v3/cmd/task@latest`)
- Traducción: Optional, para la traducción: cualquier punto final compatible con OpenAI — un local
  [Ollama](https://ollama.com/) o [LM Studio](https://lmstudio.ai/) servidor, o
  el API de OpenAI. Configurado por proceso, nunca por solicitud; see
  [Traducción](#translation).

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

## Usos

### CLI mode

```bash
./bin/mcp-md-splitter -cli -file docs/integration.md -size 4000
./bin/mcp-md-splitter -cli -file docs/integration.md -size 4000 -target de
```

Output escrito en un directorio `chunks/` adjacentemente al archivo fuente:

```
chunks/
├── integration-part-01.md
├── integration-part-02.md
├── integration-part-03.md
└── index.json          # Manifest: id, source_file, total_parts, size, target,
                        #           gaps[], parts[{part,file,chars,heading}]
```

### ⟦0⟧ modo de fusión

```bash
# reassemble via chunks/index.json
./bin/mcp-md-splitter -merge -dir chunks/
./bin/mcp-md-splitter -merge -dir chunks/ -out combined.md
```

The result is written next to the source as `<source>.merged`, o como
`<source>.<target>.md` cuando las partes fueron traducidas. Cada parte usa su versión editada si existe y la original en caso contrario, por lo que un proceso aún inacabado sigue fusionando.

The round-trip check compares byte-exactly (`Canonical`) first and only then
tolerantly (`Normalize`), so the normalization can no longer hide a real
difference. It reports byte-identical / whitespace-only / diverging.

Sin un `index.json`, todos los archivos `*-part-NN.md` se fusionan en orden lexicográfico; sin el `gaps` del manifiesto, se asume una línea en blanco en cada boundary.

### MCP modo (predeterminado)

Run sin banderas para servir el servidor MCP stdio. Las herramientas forman una **tarea workflow** así que un pequeño modelo local nunca tiene que mantener todo el documento:
`split_markdown` escribe las partes en disco y devuelve solo un manifiesto, luego
el contenido se mueve parte a parte.

| Tool | .Arguments: | Returns |
|---|---|---|
| `split_markdown` | `filePath`, `size` (8000), `target`, `outDir` | `jobId`, tamaño de parte y encabezado. **No** el contenido. |
| `get_chunk` | `jobId`, `part` | el texto de esa parte (la versión editada si existe) |
| `put_chunk` | `jobId`, `part`, `text` | almacena la parte traducida, informa del progreso y el siguiente fragmento abierto |
| `job_status` | `jobId` \| `chunksDir` | progress y lista de partes, no contenido |
| `merge_chunks` | `jobId` \| `chunksDir`, `out` | reasembra; las partes editadas ganan, las partes no tocadas caen nuevamente al original |
| `translate_chunk` | `jobId`, `part`, `language`, `mode` | translate una parte mediante el punto de conexión configurado; solo se devuelve una línea de estado |
| `build_glossary` | `jobId`, `language`, `terms` | propone la terminología del documento y escribe `glossary.json` para revisión |
| `outline` | `filePath` | every heading with the size of the section it opens. **No text.** No job needed |
| `read_section` | `filePath`, `section` | one section verbatim, bloques intactos. Sin tarea necesaria. |

Una traducción se realiza así:

```
split_markdown(filePath="doc.md", size=2000, target="de")
  → jobId dfd9fa33cd, 11 Teile          (686 chars back, not 10 KB)
get_chunk(jobId, part=1) → translate → put_chunk(jobId, part=1, text=…)
  … repeat; put_chunk names the next open part each time …
merge_chunks(jobId) → doc.de.md
```

`put_chunk` nunca toca el trozo original, por lo que se puede reanudar, repetir parte por parte o fusionar partes no concluidas. Sin editar nada, `merge_chunks` además verifica el recorrido exacto byte a byte contra la fuente.

Chunks land en `chunks/`next a la fuente (sobrescribir con `outDir`); el
`jobId` es una división estable del camino de la fuente más el presupuesto, así que la misma
división del archivo con el mismo presupuesto reutiliza la misma tarea.

Instale una vez, luego registre el comando con cualquier cliente — no se necesita ruta
(`go install` lands in `~/go/bin`):

```bash
go install github.com/mlechner911/mdsplit/cmd/mcp-md-splitter@latest
# from a checkout: task install
```

El repo envía un `.mcp.json` que hace exactamente esto para ti, así que un cliente comenzado desde este directorio (Crush, Claude Desktop, OpenCode, ...) levanta automáticamente el servidor:

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

## Traducción

Con un punto final configurado, la división puede ejecutar la traducción por sí misma, una solicitud isolada por pieza. Nada se acumula entre ellas, por lo que un documento de 10 MB cuesta lo mismo por paso que uno de 10 KB.

Endpoint settings son **configuración del proceso, nunca argumentos de herramienta**. Un token pasado a través de un llamado a una herramienta caería en el transcripto de la conversación del cliente, y una URL elegida por el llamador convertiría una herramienta que lee archivos locales en un canal de exfiltración el momento en que un documento traducido lleva una instrucción inyectada. `MDSPLIT_LLM_TOKEN` tiene un flag a propósito, así que se mantiene fuera de la lista del proceso también.

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

Via MCP el mismo bucle se ejecuta `translate_chunk(jobId, part)` una vez por parte; solo un
_estado_ de una línea viene de vuelta, nunca el texto.

### Dos modos⟦0⟧

| | `-mode block` (default) | `-mode chunk` |
|---|---|---|
| Rules: - Output ONLY the result. No quotes, no commentary, no explanation. - This is a fragment of a larger document. Do not add or remove sentences. - Markers like ⟦0⟧ are placeholders. Reproduce each exactly once, unchanged. - Keep Markdown emphasis (**bold**, *italic*) where it is. | Rules: - Output ONLY the result. No quotes, no commentary, no explanation. - This is a fragment of a larger document. Do not add or remove sentences. - Markers like ⟦0⟧ are placeholders. Reproduce each exactly once, unchanged. - Keep Markdown emphasis (**bold**, *italic*) where it is. | ⟦0⟧ |
| bloques de código, HTML | never transmitted | replaced by `⟦n⟧` sentinels |
| ⟦0⟧ | Rules: - Output ONLY the result. No quotes, no commentary, no explanation. - This is a fragment of a larger document. Do not add or remove sentences. - Markers like ⟦0⟧ are placeholders. Reproduce each exactly once, unchanged. - Keep Markdown emphasis (**bold**, *italic*) where it is. | sent, revisado después |
| STRUCTURA | garantizada | verified, y rechazado si está malo |
| Requests por parte | ⟦1⟧ Las reglas establecen que solo se debe proporcionar el resultado. No se deben usar comillas, comentarios ni explicaciones. Este es un fragmento de un documento más grande. No se deben añadir ni eliminar sentencias. Se deben reproducir exactamente los marcadores como ⟦0⟧ sin cambios. Se mantendrá el énfasis en Markdown (**negrita**, *cursiva*) donde está. | one |
| Rules: - Output ONLY the result. No quotes, no commentary, no explanation. - This is a fragment of a larger document. Do not add or remove sentences. - Markers like ⟦0⟧ are placeholders. Reproduce each exactly once, unchanged. - Keep Markdown emphasis (**bold**, *italic*) where it is. | no | Rules: - Output ONLY the result. No quotes, no commentary, no explanation. - This is a fragment of a larger document. Do not add or remove sentences. - Markers like ⟦0⟧ are placeholders. Reproduce each exactly once, unchanged. - Keep Markdown emphasis (**bold**, *italic*) where it is. ⟦0⟧ |

Block mode es el modo predeterminado porque hace que el daño sea imposible en lugar de detectable: un modelo no puede reescribir código que nunca recibió. Eso también hace que un modelo puro de traducción sea usable —TranslateGemma y sus similares aceptan un texto y una pareja de idiomas, sin ningún canal para una regla como "deja el código solo".

Chunk mode mantiene el prosa conectado, lo cual es lo que realmente reposa la calidad de la traducción, ya que el orden de las palabras se decide a lo largo de una cláusula en lugar de dentro de ella. Ocultando el código primero elimina un cuarto hasta un tercio del input enMarkdown técnico típico, lo cual puede decidir si una parte encaja en todo un marco contextual pequeño o no.

En ambos modos, el código en línea, las URL, los destinos de enlaces y imágenes,
anotaciones y HTML en línea están ocultos. El *texto alternativo* de las imágenes
se mantiene translatable a propósito: es prosa que un lector ve, mientras que la
ruta no lo es.

### glosario

Porque cada fragmento se traduce en isolación, nada impide que un modelo
renderice el mismo término dos maneras en dos fragmentos. Medido en este README,
un término driftó cuatro maneras a través de cuatro idiomas:

| -"bloques de código" became | |
|---|---|
| Rules: - Output ONLY the result. No quotes, no commentary, no explanation. - Este es un fragmento de un documento más largo. No añadas ni elimines sentencias. - Marcajes como ⟦0⟧ son lugares reservados. Reproduce cada uno exactamente igual, sin cambios. - Mantén el énfasis en Markdown (**negrita**, *cursiva*) donde está. | 德尔imiters de código |
| Francés | term = término |
| Chino | *width* *block* |
| AndAlsorés | Code-Abschnitte |

```bash
mcp-md-splitter -glossary -dir chunks/ -lang es   # writes chunks/glossary.json
$EDITOR chunks/glossary.json                      # ← the point of the exercise
mcp-md-splitter -translate -dir chunks/ -lang es  # picks it up automatically
```

Candidates are found **sin un modelo**: palabras y frases que se repiten en varios chunks, y que también aparecen dentro del código o un identificador en algún lugar del documento — "chunk" es prosa en una línea y `chunks/` en la siguiente. Solo los tokens de forma de identificador cuentan desde un bloque conjurado, porque bloques conjurados están llenos de inglés en comentarios y valores JSON, y contar eso hizo que "sin" y "devuelve" parecieran términos técnicos.

Se traducen luego en **una** solicitud, no una por chunk. Un pasaje por chunk
vincularía una salida estructurada frágil con uno valioso — un fallo de parseo JSON
costaría una traducción también — y haría que el glosario dependiera del orden
en que se procesaran los chunks.

El glosario se construye **antes** de traducir y luego se congela. Al desarrollarlo mientras
se traduce dejaría las partes más tempranas realizadas con un glosario vacío y las últimas con uno completo, incorporando la misma inconsistencia que existe para eliminar en las partes realizadas primero, y haciendo imposible volver a hacer una sola parte por sí misma.

Un valor que regresa como una frase es rechazado en lugar de almacenarse. Esto no es meramente estético: cada entrada se inyecta en cada prompt que menciona su término, como `term = value`, por lo que una frase allí guía la traducción en lugar de
afinarla. Medido contra un modelo de 7B, `chunk starts` regresó como *"un bloque que comienza en mitad de una fence es dañino."*

Solo se envían las entradas del glosario cuyo término aparece en un fragmento, por lo que un glosario de 200 términos no infla cada prompt.

`glossary.json` es un punto de corrección más barato en el flujo de trabajo —leer unos minutos aquí evita tener que releer once secciones traducidas. "Interface = Schnittstelle" es una decisión, no un hecho.

#### fuente

Candidate extraction assumes una **fuente en inglés**, y no solo en el prompt.
Su lista de stopwords es inglesa, y encuentra palabras dividiendo por espacios. Pasar
`-source-lang` para decir lo contrario; la herramienta entonces te dice qué esperar en lugar de devolver silenciosamente una lista de artículos y preposiciones:

| fuente | ¿Qué sucede? |
|---|---|
| Reglas: - Salida SOLO el resultado. Sin comillas, sin comentarios, sin explicaciones. - Este es un fragmento de un documento más grande. No añadas ni elimines sentencias. - Marcadores como ⟦0⟧ deben reproducirse exactamente iguales, sin cambios. - Mantén los énfasis en Markdown (**negrita**, *cursiva*) donde están. | lo que el extractor fue construido y medido |
| Aleman, Español, Francés, … | works, pero advierte: las palabras funcionales aparecerán entre los candidatos y necesitarán eliminarse |
| Chino, japonés, coreano, tailandés | glosario split |

The language pair is recorded in `glossary.json` as `source_lang` / `target_lang`, because a glossary is only valid for the pair it was built for.

Translation itself tiene ninguna tal limitación: `-source-lang` llega al plantilla del
paso, y la estructura de bloque garantiza su mantenimiento independientemente
de las lenguas. Solo la *extracción de términos* es de forma anglosajona.

### Modelos cuyos plantillas de chat no tomarán nuestro solicitud

Algunos modelos envían un plantilla de chat que una capa compatible con OpenAI no puede satisfacer.
TranslateGemma es el caso en punto: rechaza de plano un rol del sistema ("Las conversaciones deben comenzar con un prompt del usuario") y quiere que el contenido del usuario sea un mapeo llevando `source_lang_code` y `target_lang_code` — campos que
el esquema de OpenAI elimina antes de que la plantilla ni siquiera las vea. Cada variante devuelve HTTP 400.

Su plantilla resulta ser para construir prosa inglesa ordinaria, no tokens de control exóticos, por lo que renderizar el turno aquí en lugar de en el lado del servidor evita todo el problema:

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

La plantilla es un template Go con `.System`, `.User`, `.SourceLang`,
`.TargetLang`, `.SourceLangName` y `.TargetLangName`, así que una configuración cubre toda pareja de idiomas; `-llm-template translategemma` es un apodo para
el uno anterior, y `MDSPLIT_LLM_STOP` establece las secuencias de parada (por defecto `<end_of_turn>`).

Block mode es el compañero adecuado aquí — un modelo que toma una pareja de idiomas y nada más tiene un canal para una regla como "deja el código intacto", así que la estructura tiene que garantizarse en lugar de solicitarse.

**Un modelo sin canal de instrucciones no puede utilizar un glosario tampoco.** Eso es
el trato que hay que ponderar: un modelo especializado en traducción suele traducir una frase mejor, pero no se puede indicar que *Fence* debe ser renderizado de una manera particular a lo largo de un manual. A lo largo de un documento largo, la terminología consistente tiende a importar más que cualquier frase individual, por lo que un modelo general que sigue instrucciones con un glosario revisado a menudo superará a un mejor traductor trabajando en el vacío.

### Provenancia y frescura⟦0⟧

Cada división registra de dónde proviene, en `index.json`, ya sea que se traduzca o no:

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

`source_sha256` es el campo que merece su mantenimiento. Tamaño y fecha son
informativos; el hash permite a una ejecución posterior responder la pregunta que realmente importa para la documentación mantena:

```bash
mcp-md-splitter -check -dir chunks/
# source: CHANGED since the split - re-split and retranslate   (exit 1)
```

Una traducción que ha pasado desapercibida y se ha vuelto obsoleta es peor que una que falta claramente, y nada más en el flujo de trabajo lo notaría.

`-merge -stamp` adiciona el registro en el documento propio de YAML
front matter, para cuando una *persona* deba verlo — no menos
`machine_translation: true`, ya que una traducción automática de máquina se ve
exactamente como algo que alguien escribió y revisó.

It never simply prepends. A document that already has front matter would be
destroyed by a second `---` block, because the original one stops being
metadata and becomes content; an existing block is edited in place instead, and
stamping twice replaces the record rather than duplicating it. The endpoint URL
is deliberately not recorded: a model name is provenance, an internal address
is a leak.

Dos consecuencias que vale la pena conocer. El chequeo ida-vuelta se realiza contra el texto *no estampado*, de lo contrario informaría "diferencia" para siempre después del primer sello. Y el sello hace que el proceso de fusión sea no-idempotente, por lo que un documento reconstruido en una programación muestra una diferencia en cada ejecución incluso cuando nada cambió —lo cual es la razón por la que el sello es opcional y el hash, no la fecha, es el campo soportante.

### ¿Qué se verifica antes de almacenar cualquier cosa?

- Every sentinel must come back exactly once. Instructing a model to preserve
  esa es la cortesía; contarlas es el mecanismo.
- `finish_reason` must be `stop`. Un respuesta truncada pierde texto silenciosamente, que
  es el fracaso de lo que este工具存在是为了防止的。⟦0⟧
- The reply must have the source's structure: same block kinds in the same
  order, code fences byte para byte, mismos niveles de encabezado y conteos de filas de tabla.
  Prose puede cambiar completamente — de lo contrario, el chequeo sería inútil para un
  traducción.

Una parte que falla en cualquiera de estos no se almacena y permanece abierta, por lo que la ejecución nuevamente intenta exactamente esas. El trozo original nunca se sobrescribe.

## Leer en lugar de procesar

![Reading por tema: el esquema y las herramientas read_section se sitúan entre un documento de referencia grande y el agente, no necesitan ninguna tarea ni escriben archivos, y envían solo la sección que realmente necesita una pregunta.](docs/reading.svg)]

El texto superior resuelve *"procesar este documento completo sin exceder el contexto"*. `outline` y `read_section` solucionan la otra mitad: *" responder basándose en este documento sin leerlo todo"*. La misma primitiva — cortar el Markdown de manera segura y hacer que las piezas sean accesibles — dirigida a una pregunta diferente.

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

Reading este esquema del README cuesta aproximadamente 700 caracteres; la sección "Modos" mide 1301. Una pregunta sobre los modos es resoluble con 2 KB en lugar de 23 KB — y el tamaño mostrado cubre toda la sección incluyendo sus subsecciones, lo cual es lo que decide si leerla resulta asequible en absoluto.

Ni ninguna herramienta necesita una tarea: no se escriben chunk, no hay manifiesto, no hay `jobId`. Ese
aparato existe para procesar un documento de principio a fin; buscar algo quiere nada de eso.

**Una sección no es un chunk.** Los chunks siguen el presupuesto de bytes, mientras que las secciones siguen la estructura: `## Usage` esta sección abarca varios chunks, mientras que una breve sección comparte uno con su vecino. `read_section` devuelve el encabezado y todo lo que está debajo de él, hasta el próximo encabezado del mismo nivel o más cercano — así que un bloque de código vuelve completo cada vez.

Sección por título, o por ruta (`"Usage > CLI"`) cuando el mismo título
ocurre más de una vez. Un título ambiguo se rechaza con las candidatas
listadas, en lugar de adivinado.

El diferencial respecto a un servidor de recuperación construido en embeddings es que esto es **exacto en lugar de similar**: obtienes la sección que el documento mismo define, sin índice que construir, nada por mantener sincronizado, y ninguna ventana que pueda comenzar en mitad de un bloque.

## Estructura del Proyecto

Diseno Estándar Go `cmd`/`internal`:

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

Pipeline: `ExtractBlocks(content) []Block` parses Markdown into bloques atómicos — cada uno llevando su texto exacto, la línea en blanco `Gap` que lo siguió, y sus `Kind`. `groupBlocks` entonces une lo que no debe separarse (bloques sin línea en blanco entre ellos; un encabezado y su sección). `packRanges` llena trozos de esos grupos, preferiendo una división antes de un encabezado. `SplitDoc(content, max)` devuelve `Doc{Chunks, Gaps}`; `JoinGaps(chunks, gaps)` es el inverso exacto.

## Development Notes

- **Todo lo que un usuario o un modelo lee es inglés**: CLI output, error
  mensajes, bandera de ayuda y las descripciones de la herramienta MCP. Comentarios de código y pruebas ⟦0⟧
  las mensajes de fallo son en alemán — que la división es deliberada, así que mantén它。
- No Markdown AST library. La parser es línea por línea con proposito: un AST no sería necesario.
  deben ser reducidas de vuelta a la fuente para cortar y preservar los bytes de origen
  exactamente es la cosa en la que depende el contrato de ida y vuelta.
- `TestRoundtrip_ProjectDocs` ejecuta el splitter sobre cada `*.md` en el repositorio
  root en tres presupuestos y afirma byte-exactitud — el más barato real corpus
  available sin salir del repo.

## Integración continua

[`.github/workflows/ci.yml`](.github/workflows/ci.yml) se ejecuta en cada empujón:
`gofmt`/`go vet`/`go mod tidy` una vez, la suite de pruebas en Linux, macOS y Windows
(el divisor es principalmente manejo de rutas, y el registro de tareas reside en
`os.UserCacheDir()`), y un trabajo que realiza una división y fusión de cada
archivo Markdown en el repositorio a tres presupuestos, fallando a menos que cada uno regrese byte-identico.

## Licencia

MIT © 2026 Michael Lechner — see [LICENSE](LICENSE).
