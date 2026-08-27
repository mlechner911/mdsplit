---
translation:
  tool: mdsplit
  version: 1.4.0
  url: "https://github.com/mlechner911/mdsplit"
  source: README.md
  source_sha256: 5dfef16a27b78a9b
  source_chars: 28835
  target_lang: es
  model: "qwen3.5:35b"
  mode: block
  parts: 18/18
  translated: "2026-08-27T19:34:28Z"
  machine_translation: true
---

![Un detective inspeccionando un documento Markdown con una lupa, enfocando una sección mientras el resto permanece intacto. ](docs/hero.jpg)

<sub>Ilustración creada con Nano Banana 2</sub>

# Divisor de Markdown

<sub>Traducido por esta herramienta: [Deutsch](README.de.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文](README.zh.md) — cada uno lleva su procedencia en la materia preliminar.</sub>

Divide documentos Markdown en fragmentos de tamaño limitado que permanezcan seguros para la traducción o el procesamiento por LLM. Los bloques atómicos (bloques de código, tablas, listas, HTML multilínea) nunca se dividen entre los límites de los fragmentos; solo los bloques completos se mueven entre fragmentos.

Implementado en Go. Seis modos de ejecución: división, fusión, traducción, glosario, verificación y esquema, además de un servidor MCP (Protocolo de Contexto del Modelo) que expone el mismo trabajo como un flujo de trabajo de tarea por fragmentos.

![El pipeline: un documento fuente se corta en fragmentos de tamaño limitado sin romper bloques de código, tablas o HTML; el divisor devuelve únicamente un manifiesto, nunca el texto; cada parte viaja a un LLM local como una solicitud sin estado sin historial de chat y regresa a través de put_chunk.](docs/pipeline.svg)

## Motivación

Traducir un documento Markdown largo con un modelo local es un problema de contexto antes que de idioma. Un manual de 60 KB no cabe en una ventana de 8k tokens, por lo que debe dividirse — y las divisiones ingenuas son exactamente las dañinas. Dividir cada N caracteres hace que un bloque de código termine a medias en un fragmento y a medias en el siguiente; el modelo "traduce" diligentemente la mitad huérfana, renombra los identificadores y el bloque nunca se cierra de nuevo. Dividir en líneas en blanco hace que una tabla pierda su fila de encabezado. Dividir solo en encabezados hace que una sección tenga 200 bytes mientras que la siguiente tiene 40 KB.

Entregar todo el archivo al modelo y pedirle que lo divida en fragmentos no ayuda tampoco: el archivo ya está en el contexto para entonces, que era lo que se quería evitar.

Así que esta herramienta realiza el corte fuera del modelo, sobre dos reglas:

1. **Algunos bloques son indivisibles.** Bloques de código, tablas, elementos de lista con su
   continuaciones, elementos HTML. Permanecen completos incluso cuando eso implica explotar
   a través del tamaño del presupuesto — un trozo que es el doble de grande es una molestia,
   Un fragmento que termina a mitad de la cerca es corrupción.
2. **El tamaño es un objetivo, no una ley.** Los cortes se aplican antes de los encabezados, por lo que un fragmento
   comienza en una sección y la lleva entera. Un trozo ligeramente pequeño que es
   self-contained se traduce mejor que una completa que comienza a mitad de oración.

El lado MCP sigue de la misma preocupación. `split_markdown` devuelve un manifiesto — conteo de partes, tamaños, encabezados — y no el texto. El contenido se extrae una parte a la vez con `get_chunk` y se escribe de nuevo con `put_chunk`, por lo que el contexto permanece plano sin importar si la fuente es de 10 KB o 10 MB.

La última pieza es el camino de regreso. El fragmentado solo es seguro si es reversible, por lo que el manifiesto registra la línea en blanco en cada límite y la fusión se verifica byte por byte contra la fuente. Si el viaje completo es exacto para el documento sin traducir, la tubería no comió silenciosamente nada — y cualquier diferencia después de la traducción es obra del modelo, no del divisor.

Construido para [Crush](https://github.com/charmbracelet/crush) conduciendo un modelo local de Ollama, pero nada en él es específico de ninguno de los dos.

## Características

- **Secciones alineadas**: el tamaño es un *presupuesto* blando. El divisor prefiere
  para cortar antes de un encabezado en lugar de llenar un fragmento hasta el borde, de modo que un fragmento
  comienza en una sección y la mantiene entera. Un encabezado nunca termina un fragmento.
- **Los bloques atómicos nunca se dividen**, independientemente del presupuesto que diga: bloques de código
  (cualquier cadena de información — ` ```js title="a.js" `, ` ``` go `, `~~~`, `````), GFM
  grupos de filas de tabla, elementos de lista con sus continuaciones y HTML de varias líneas
  elementos. Un bloque indivisible que excede el presupuesto obtiene su propio fragmento
  y una nota en stderr.
- **Redonda exacta de bytes**: `index.json` registra el espacio de línea en blanco en cada
  límite de fragmento, por lo que la fusión reproduce la fuente byte a byte. El único
  la normalización está documentada en `Canonical()`: líneas en blanco al principio y al final
  se recortan y las líneas que solo contienen espacios en blanco se vuelven vacías. La sangría y el endurecimiento
  los saltos de línea (dos espacios finales) se conservan.
- **Modo CLI**: escribe `<name>-part-NN.md` archivos más un manifiesto `index.json` (archivo fuente, recuento de partes, lista ordenada de fragmentos) junto al archivo fuente
- **Modo de fusión**: `-merge -dir chunks/` vuelve a ensamblar los fragmentos mediante el manifiesto e informa si el resultado es idéntico en bytes, idéntico en espacios en blanco o divergente.
- **Flujo de trabajo del MCP**: `split_markdown` devuelve un manifiesto, no el documento.
  El contenido se mueve una parte a la vez mediante `get_chunk` / `put_chunk`, por lo que el contexto
  permanece constante sin importar qué tan grande sea la fuente, que es todo el punto
  de ejecutar esto contra un modelo local pequeño.

## Requisitos

- Go 1.25+
- Opcional: [taskfile](https://taskfile.dev/) (`go install github.com/go-task/task/v3/cmd/task@latest`)
- Opcional, para la traducción: cualquier punto final compatible con OpenAI — un local
  [Ollama](https://ollama.com/) o servidor de LM Studio](https://lmstudio.ai/), o
  la API de OpenAI. Configurada por proceso, nunca por solicitud; véase
  [Traducción](#translation).

## Compilar y probar

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

## Uso

### Modo CLI

```bash
./bin/mcp-md-splitter -cli -file docs/integration.md -size 4000
./bin/mcp-md-splitter -cli -file docs/integration.md -size 4000 -target de
```

Se escribe en un directorio `chunks/` junto al archivo de fuente:

```
chunks/
├── integration-part-01.md
├── integration-part-02.md
├── integration-part-03.md
└── index.json          # Manifest: id, source_file, total_parts, size, target,
                        #           gaps[], parts[{part,file,chars,heading}]
```

### Modo de fusión

```bash
# reassemble via chunks/index.json
./bin/mcp-md-splitter -merge -dir chunks/
./bin/mcp-md-splitter -merge -dir chunks/ -out combined.md
```

El resultado se escribe junto a la fuente como `<source>.merged`, o como `<source>.<target>.md` cuando las partes fueron traducidas. Cada parte usa su versión editada si existe y la original de lo contrario, por lo que una ejecución a medias aún fusiona.

La verificación de viaje completo compara primero y únicamente de manera exacta en bytes (`Canonical`) y luego tolerante (`Normalize`), por lo que la normalización ya no puede ocultar una diferencia real. Informa sobre idénticos en bytes / solo espacios en blanco / divergentes.

Sin un `index.json`, todos los archivos `*-part-NN.md` se fusionan en orden lexicográfico; sin el `gaps` del manifiesto, se asume una línea en blanco en cada límite.

### Modo MCP (predeterminado)

Ejecutar sin flags para servir el servidor MCP de stdio. Las herramientas forman un **flujo de trabajo de tarea** para que un modelo local pequeño nunca tenga que contener todo el documento: `split_markdown` escribe los fragmentos en disco y devuelve solo un manifiesto, luego el contenido se mueve una parte a la vez.

| Herramienta | Argumentos | Retornos |
|---|---|---|
| `split_markdown` | `filePath`, `size` (8000), `target`, `outDir` | manifiesto solo — `jobId`, tamaño y encabezado por parte. **No** el contenido. |
| `get_chunk` | `jobId`, `part` | el texto de esa parte (la versión editada si existe) |
| `put_chunk` | `jobId`, `part`, `text` | almacena la parte traducida, informa del progreso y de la siguiente parte abierta |
| `job_status` | `jobId` \| `chunksDir` | progreso y lista de partes, sin contenido |
| `merge_chunks` | `jobId` \| `chunksDir`, `out` | reassemblies; partes editadas ganan, partes sin tocar vuelven al original |
| `translate_chunk` | `jobId`, `part`, `language`, `mode` | traduce una parte mediante el punto final configurado; solo regresa una línea de estado |
| `build_glossary` | `jobId`, `language`, `terms` | propone la terminología del documento y escribe `glossary.json` para revisión |
| `outline` | `filePath` | cada encabezado con el tamaño de la sección que abre. **No texto.** No se necesita tarea |
| `read_section` | `filePath`, `section` | una sección textualmente, bloques intactos. No se necesita trabajo |

Una ejecución de traducción se ve así:

```
split_markdown(filePath="doc.md", size=2000, target="de")
  → jobId dfd9fa33cd, 11 Teile          (686 chars back, not 10 KB)
get_chunk(jobId, part=1) → translate → put_chunk(jobId, part=1, text=…)
  … repeat; put_chunk names the next open part each time …
merge_chunks(jobId) → doc.de.md
```

`put_chunk` nunca toca el fragmento original, por lo que una ejecución puede reanudarse, rehacerse parte por parte o fusionarse a medias. Sin editar nada, `merge_chunks` además verifica el viaje completo byte-exacto contra la fuente.

Los fragmentos se colocan en `chunks/` junto a la fuente (sobrescribir con `outDir`); el `jobId` es un hash estable de la ruta de la fuente más el presupuesto, por lo que volver a dividir el mismo archivo con el mismo presupuesto reutiliza la misma tarea.

Instale una vez y luego registre el comando con cualquier cliente; no se necesita ruta
(`go install` se instala en `~/go/bin`):

```bash
go install github.com/mlechner911/mdsplit/cmd/mcp-md-splitter@latest
# from a checkout: task install
```

El repositorio envía un `.mcp.json` local al proyecto que hace exactamente esto por ti, de modo que un cliente iniciado desde este directorio (Crush, Claude Desktop, OpenCode, …) detecta el servidor automáticamente:

```json
{
  "mcpServers": {
    "md-splitter": {
      "command": "mcp-md-splitter"
    }
  }
}
```

Una entrada global `crushrc` funciona de la misma manera: `mcp add md-splitter --command mcp-md-splitter`.

## traducción

Con un punto final configurado, el divisor puede ejecutar la traducción por sí mismo, una solicitud aislada por pieza. Nada se acumula entre ellas, por lo que un documento de 10 MB cuesta lo mismo por paso que uno de 10 KB.

La configuración de los puntos finales es **configuración del proceso, nunca argumentos de la herramienta**. Un token pasado a través de una llamada a la herramienta terminaría en el registro de conversación del cliente, y una URL elegida por quien llama convertiría una herramienta que lee archivos locales en un canal de exfiltración en el momento en que un documento traducido lleve una instrucción inyectada. `MDSPLIT_LLM_TOKEN` no tiene bandera a propósito, para que también permanezca fuera de la lista de procesos.

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

A través de MCP el mismo bucle se `translate_chunk(jobId, part)` una vez por parte; solo regresa un estado de una línea, nunca el texto.

### Dos modos

| | `-mode block` (predeterminado) | `-mode chunk` |
|---|---|---|
| Lo que se envía | Solo fragmentos de prosa | Toda la parte, código enmascarado |
| Cajas de código, HTML | Nunca transmitido | Reemplazado por sentinelas `⟦n⟧` |
| Viñetas, barras verticales, sangría | Reproducido literalmente | Enviado, verificado después |
| Estructura | Garantizada | Verificada y rechazada si es incorrecta |
| Solicitudes por parte | Una por fragmento | Una |
| Requiere un modelo que siga instrucciones | No | Sí |

El modo bloque es el predeterminado porque hace que el daño sea imposible en lugar de detectable: un modelo no puede reescribir código que nunca recibió. Esto también hace que un modelo de traducción puro sea utilizable — TranslateGemma y sus similares aceptan un texto y una pareja de idiomas, sin canal para una regla como "dejar el código intacto".

El modo de fragmentos mantiene la prosa conectada, lo cual es en lo que realmente descansa la calidad de la traducción, ya que el orden de las palabras se decide a través de una cláusula y no dentro de ella. Ocultar primero el código elimina entre un cuarto y un tercio de la entrada en Markdown técnico típico, lo cual puede decidir si una parte cabe en una ventana de contexto pequeña o no.

En ambos modos, el código en línea, las URL, los objetivos de enlace e imagen, los enlaces de referencia, las notas al pie y el HTML en línea están enmascarados. El texto alternativo de la imagen *alt text* permanece traducible a propósito: es prosa que ve el lector, mientras que la ruta no lo es.

### Glosario

Como cada fragmento se traduce de forma aislada, nada impide que un modelo renderice el mismo término de dos maneras en dos fragmentos. Medido en este README, un término varió de cuatro formas a través de cuatro idiomas:

| "cercas de código" se convirtió en | |
|---|---|
| español | delimitador de código |
| francés | **un code** — el término simplemente desapareció |
| chino | bloque de código |
| alemán | segmentos de código |

```bash
mcp-md-splitter -glossary -dir chunks/ -lang es   # writes chunks/glossary.json
$EDITOR chunks/glossary.json                      # ← the point of the exercise
mcp-md-splitter -translate -dir chunks/ -lang es  # picks it up automatically
```

Los candidatos se encuentran **sin un modelo**: palabras y frases que se repiten en varios fragmentos y que también aparecen dentro de un código o identificador en algún lugar del documento — "fragmento" es prosa en una línea y `chunks/` en la siguiente. Solo los tokens con forma de identificador cuentan desde dentro de un bloque, porque los bloques cercados están llenos de inglés en comentarios y valores JSON, y contar eso hizo que "sin" y "devuelve" parecieran términos técnicos.

Luego se traducen en **una** solicitud, no una por fragmento. Un pase por fragmentos vincularía una salida estructurada frágil a la valiosa —un error de análisis JSON costaría también una traducción— y haría que el glosario dependiera del orden en que se procesaron los fragmentos.

El glosario se construye **antes** de traducir y luego se congela. Crear uno mientras se traduce dejaría las primeras partes con un glosario vacío y la última con uno completo, cocinando la misma inconsistencia que existe para eliminar en las partes hechas primero, y haciendo imposible rehacer una sola parte por sí misma.

Un valor que regresa como una oración se rechaza en lugar de almacenarse. Esto no es cosmético: cada entrada se inyecta en cada prompt que menciona su término, como `term = value`, por lo que una oración allí dirige la traducción en lugar de afilarla. Comparado con un modelo de 7B, `chunk starts` regresó como *"un bloque que comienza en mitad de una fence es dañino."*

Solo se envían con cada fragmento las entradas cuyo término aparece realmente en él, por lo que un glosario de 200 términos no infla cada solicitud.

`glossary.json` está destinado a ser editado. "Interface = Schnittstelle" es una decisión, no un hecho, y este es el punto más económico en la cadena de procesamiento para corregirlo: unos minutos aquí valen más que releer once fragmentos traducidos.

#### Idioma de origen

La extracción de candidatos asume una **fuente en inglés**, y no solo en el prompt. Su lista de palabras vacías es en inglés, y encuentra palabras dividiendo por espacios. Pase `-source-lang` para indicar lo contrario; la herramienta entonces le dice qué esperar en lugar de devolver silenciosamente una lista de artículos y preposiciones:

| Fuente | Qué ocurre |
|---|---|
| Inglés (predeterminado) | Para lo que se construyó y midió el extractor |
| Alemán, español, francés, … | funciona, pero advierte: las palabras funcionales aparecerán entre los candidatos y deberán eliminarse |
| Chino, japonés, coreano, tailandés | se niega — no hay espacios para la división, así que escriba el glosario a mano |

La pareja de idiomas se registra en `glossary.json` como `source_lang` / `target_lang`, porque un glosario solo es válido para la pareja para la cual fue construido.

La propia traducción no tiene tal limitación: `-source-lang` llega a la plantilla de solicitud, y las garantías de la estructura en modo bloque se mantienen sin importar los idiomas. Solo la extracción de terminología está conformada en inglés.

### Modelos que prescriben un formato de solicitud

Un modelo de traducción a menudo desea que la solicitud tenga una forma particular.
`zongwei/gemma3-translator` espera `Translate from English to German: <text>`
y no tiene otra manera de aprender el idioma objetivo: el texto plano no contiene nada.

```bash
mcp-md-splitter -translate -dir chunks/ -lang de \
  -llm-model zongwei/gemma3-translator:1b -llm-user-template gemma3-translator
```

`-llm-user-template` acepta una plantilla de Go con los mismos campos, o una abreviatura: `gemma3-translator` para el formato anterior, `translategemma` para la propia instrucción de TranslateGemma como un mensaje de usuario en texto plano. Cuando está configurado, **no se envía ningún mensaje del sistema**: quien llama y da forma al mensaje decide si las reglas pertenecen a él. Eso es lo que hace posible utilizar un modelo con su propio prompt del sistema integrado: un mensaje del sistema enviado por nosotros lo reemplazaría.

El mismo modelo puede necesitar uno u otro mecanismo dependiendo de quién lo sirva.
La plantilla de chat de TranslateGemma rechaza todas las solicitudes con forma de OpenAI en LM Studio, por lo que allí necesita `-llm-transport completions -llm-template
translategemma`. El empaquetado de Ollama documenta la instrucción como tarea del llamador, por lo que allí `-llm-user-template translategemma` es suficiente. Ambos renderizan el mismo texto, incluidas las dos líneas en blanco antes del pasaje que su tarjeta de modelo señala.

### Modelos cuya plantilla de chat no aceptará nuestra solicitud

Algunos modelos envían una plantilla de chat que una capa compatible con OpenAI no puede satisfacer. TranslateGemma es el caso en cuestión: rechaza directamente un rol de sistema ("Las conversaciones deben comenzar con un prompt de usuario") y desea que el contenido del usuario sea un mapeo que lleve `source_lang_code` y `target_lang_code` — campos que el esquema de OpenAI elimina antes de que la plantilla los vea. Cada variante devuelve HTTP 400.

Su plantilla resulta construir prosa inglesa ordinaria, no tokens de control exóticos, por lo que renderizar el turno aquí en lugar del servidor esquiva todo el problema:

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

La plantilla es una plantilla de Go con `.System`, `.User`, `.SourceLang`, `.TargetLang`, `.SourceLangName` y `.TargetLangName`, por lo que una configuración cubre cada pareja de idiomas; `-llm-template translategemma` es un atajo para la anterior, y `MDSPLIT_LLM_STOP` establece las secuencias de parada (predeterminado `<end_of_turn>`).

El modo bloque es el compañero adecuado aquí — un modelo que toma una pareja de idiomas y nada más no tiene canal para una regla como "dejar el código intacto", por lo que la estructura debe ser garantizada en lugar de solicitada.

**Un modelo sin canal de instrucciones tampoco puede utilizar un glosario.** Esa es la compensación que debe sopesarse: un modelo especializado en traducción suele traducir una oración mejor, pero no se le puede indicar que *Fence* debe renderizarse de una manera particular a lo largo de un manual. En un documento extenso, la terminología coherente tiende a ser más importante que cualquier oración individual, por lo que un modelo general que siga instrucciones y cuente con un glosario revisado a menudo superará a un mejor traductor que trabaja a ciegas.

### Procedencia y antigüedad

Cada división registra de dónde proviene, en `index.json`, si se traduce o no:

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

`source_sha256` es el campo que justifica su existencia. El tamaño y la fecha son informativos; el hash permite que una ejecución posterior responda a la pregunta que realmente importa para la documentación mantenida:

```bash
mcp-md-splitter -check -dir chunks/
# source: CHANGED since the split - re-split and retranslate   (exit 1)
```

Una traducción que ha ido envejeciendo silenciosamente es peor que una que está obviamente faltante, y nada más en la tubería lo notaría.

`-merge -stamp` además escribe el registro en la propia materia frontal YAML del documento, para cuando una *persona* deba verlo — no menos `machine_translation: true`, ya que una traducción de máquina de otro modo se ve exactamente como algo que alguien escribió y revisó.

Nunca simplemente se prefiere. Un documento que ya tiene metadatos frontales sería destruido por un segundo bloque `---`, porque el original deja de ser metadatos y se convierte en contenido; un bloque existente se edita in situ, y estampar dos veces reemplaza el registro en lugar de duplicarlo. La URL del endpoint no se registra deliberadamente: un nombre de modelo es procedencia, una dirección interna es una fuga.

Dos consecuencias que vale la pena conocer. La verificación de viaje completo se ejecuta contra el texto sin sellar; de lo contrario, informaría "difiere" para siempre después del primer sello. Y el marca temporal hace que la fusión no sea idempotente, por lo que un documento reconstruido en un horario muestra una diferencia en cada ejecución incluso cuando nada cambió — razón por la cual el sellado es opcional y el hash, no la fecha, es el campo portador de carga.

### ¿Qué se verifica antes de almacenar cualquier cosa?

- Cada centinela debe regresar exactamente una vez. Instruir a un modelo para preservar
  ellos es la cortesía; contarlos es el mecanismo.
- `finish_reason` debe ser `stop`. Una respuesta truncada pierde texto silenciosamente, lo que
  es el fallo para cuya prevención existe esta herramienta.
- La respuesta debe tener la estructura de la fuente: mismos tipos de bloques en el mismo
  orden, bloques de código byte por byte, mismos niveles de encabezado y conteos de filas de tabla.
  La prosa puede cambiar por completo — de lo contrario, la verificación sería inútil para un
  traducción.

Una parte que falla cualquiera de estas **no se almacena** y permanece abierta, por lo que volver a ejecutar los reintentos exactamente esas. El fragmento original nunca se sobrescribe.

## Lectura en lugar de procesamiento

![Lectura por tema: las herramientas de esquema y lectura de sección se sitúan entre un gran documento de referencia y el agente, no requieren tarea ni escriben archivos, y envían solo la sección que una pregunta realmente necesita. ](docs/reading.svg)

Las herramientas anteriores resuelven *"procesar este documento entero sin desbordar el contexto"*. `outline` y `read_section` resuelven la otra mitad: *"responder desde este documento sin leerlo todo"*. Misma primitiva — cortar Markdown de forma segura y hacer que las piezas sean direccionables — apuntando a una pregunta diferente.

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

Leer el índice de este README cuesta aproximadamente 700 caracteres; la sección "Dos modos" tiene 1301. Una pregunta sobre los modos se puede responder con 2 KB en lugar de 23 KB — y el tamaño mostrado cubre toda la sección, incluidas sus subsecciones, lo cual es lo que determina si leerla es asequible o no.

Ninguna herramienta necesita una tarea: no se escriben fragmentos, no hay manifiesto, ni `jobId`. Ese aparato existe para procesar un documento de principio a fin; buscar algo no requiere ninguno de ellos.

**Una sección no es un fragmento.** Los fragmentos siguen el presupuesto; las secciones siguen el esquema: `## Usage` abarca varios fragmentos, mientras que una sección corta comparte uno con su vecino. `read_section` devuelve el encabezado y todo lo que hay debajo, hasta el siguiente encabezado del mismo nivel o de un nivel más bajo — así, un bloque de código siempre se devuelve completo.

Diríjase a una sección por título o por ruta (`"Usage > CLI"`) cuando el mismo título aparezca más de una vez. Un título ambiguo se rechaza con los candidatos listados, en lugar de adivinarse.

La diferencia con un servidor de recuperación basado en incrustaciones es que esto es **exacto en lugar de similar**: obtienes la sección que el propio documento define, sin necesidad de construir un índice, nada que mantener sincronizado y ninguna ventana que pueda comenzar en medio de un bloque.

## Decir a tu agente cómo usarlo

Registrar el servidor no es suficiente. Las descripciones de las herramientas indican qué hace cada una, pero un modelo solo lee la descripción de una herramienta que ya ha decidido llamar; por lo tanto, las reglas de orden llegan demasiado tarde. Incluya esto en el `AGENTS.md`, `CLAUDE.md` o prompt del sistema de su proyecto:

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

Esa última regla es la que vale la pena mantener. Un modelo al que se le entrega una herramienta de fragmentación a menudo recuperará cada fragmento y los volverá a ensamblar en su propio contexto, lo cual cuesta más que leer el archivo —la herramienta ha sido utilizada y su propósito derrotado en el mismo aliento.

La configuración del endpoint pertenece en `.mcp.json` bajo `env`, nunca en un prompt: un modelo no puede pasar un token que nunca recibió, y ese es el punto.

Note que el propio `AGENTS.md` de este repositorio es un documento diferente: informa a los agentes *que trabajan en* el divisor, no a los agentes *que lo utilizan*.

## Estructura del proyecto

Disposición estándar de Go `cmd`/`internal`:

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

Pipeline: `ExtractBlocks(content) []Block` analiza Markdown en bloques atómicos — cada uno con su texto exacto, la línea en blanco `Gap` que lo siguió y su `Kind`. `groupBlocks` luego une lo que no debe separarse (bloques sin línea en blanco entre ellos; un encabezado y su sección). `packRanges` rellena fragmentos de esos grupos, prefiriendo un corte antes de un encabezado. `SplitDoc(content, max)` devuelve `Doc{Chunks, Gaps}`; `JoinGaps(chunks, gaps)` es el inverso exacto.

## Notas de desarrollo

- **Todo lo que lee un usuario o un modelo es inglés**: salida de la CLI, error
  mensajes, ayuda de bandera y descripciones de herramientas MCP. Comentarios de código y pruebas
  los mensajes de error están en alemán — esa división es deliberada, así que manténgala.
- No hay biblioteca de análisis sintáctico abstracto (AST). El analizador es basado en líneas a propósito: un AST sería
  deben volver a reducirse a la fuente para activarse y preservar los bytes de la fuente
  exactamente es lo único en lo que depende el contrato de ida y vuelta.
- `TestRoundtrip_ProjectDocs` ejecuta el divisor sobre cada `*.md` en el repositorio
  root en tres presupuestos y afirma la exactitud de los bytes — el corpus real más económico
  disponible sin salir del repositorio.

## Integración continua

[`.github/workflows/ci.yml`](.github/workflows/ci.yml) se ejecuta en cada push:
`gofmt`/`go vet`/`go mod tidy` una vez, el conjunto de pruebas en Linux, macOS y Windows
(el divisor es principalmente manejo de rutas, y el registro de tareas vive en
`os.UserCacheDir()`), y una tarea de viaje completo que divide y fusiona cada
archivo Markdown en el repositorio en tres presupuestos, fallando a menos que cada uno regrese
byte-identical.

## Licencia

MIT © 2026 Michael Lechner — ver [LICENSE](LICENSE).
