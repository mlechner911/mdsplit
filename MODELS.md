# Models: what works, and how they fail

Notes from running this tool against real endpoints. Not a benchmark — a log of
what broke and why, kept because the failure modes turned out to be more useful
than any score.

Every measurement below is one section of this repo's own README: the **"Two
modes"** table, 1301 characters, 23 translatable fragments, most of them
two- or three-word table cells. It was chosen because it is the hardest thing
in the document: short labels carry almost no context, and that is where small
models come apart.

English → German, `-mode block`, with the reviewed `i18n/de/glossary.json`.

## What was measured

| Model | Endpoint | On disk | Requests | Short fragments usable | Left in English | Chars |
|---|---|---:|---:|---:|---:|---:|
| `qwen3.5:35b` | Ollama, M2 Ultra 64 GB | 23.9 GB Q4 | 4 | 20/20 | 0 | 1501 |
| `gemma3:27b` | Ollama, M2 Ultra 64 GB | 17.4 GB Q4 | 4 | 20/20 | 3 | 1335 |
| `qwen2.5-7b-instruct` | LM Studio, 12 GB GPU | 4.7 GB Q4 | 4 | 20/20 | 0 | 1509 |
| `zongwei/gemma3-translator:1b` | Ollama, M2 Ultra 64 GB | 0.8 GB Q4 | 4 | 20/20 | 0 | 1481 |
| `qwen2.5-7b-instruct` *(before batching)* | LM Studio | 4.7 GB Q4 | 23 | 13/23 | 10 | **half the table English** |

The last row is the same model on the same text a day earlier, one request per
fragment. The difference is not the model.

### The measurable columns stopped discriminating

Four models spanning **thirty-fold** in size produced the same structural
result: four requests, twenty of twenty short fragments accepted, nothing
corrupted. Once the guards are in place and short fragments travel together,
structural safety is no longer what separates a 0.8 GB model from a 24 GB one.

What separates them is only visible by reading. The same three cells:

| | "Bullets, pipes, indentation" | "Needs an instruction-following model" | "reproduced literally" |
|---|---|---|---|
| `qwen3.5:35b` | Aufzählungszeichen, Pipes, Einrückung | Benötigt ein instruktionsfolgendes Modell | wörtlich reproduziert |
| `gemma3:27b` | Aufzählungszeichen, Pipes, Einrückung | Benötigt ein Anweisungs-Folge-Modell | wörtlich wiedergegeben |
| `qwen2.5-7b` | **Punkte, Striche, Einrückungen** | **Anweisungslaufendes Modell** | wörtlich wiederholt |
| `gemma3-translator:1b` | **Bullet, Pipe, Abstand** | **Benötigt ein Modell nachweislich** | Literal reproduziert |

The 7B turned bullets and pipes into "dots and dashes" — plausible German for
the wrong meaning, which is the kind of error that survives review. The 1B left
them untranslated and produced a sentence that means nothing. Both cleared
every automated check.

So: the guards make a small model **safe**, not good. If the German has to be
right, size still buys something — and on a 64 GB box it costs nothing but
minutes.

### The specialised 1B is still worth noting

A 0.8 GB translation model matched a 4.7 GB general one on the structural
columns, at a sixth of the size, and beat it on nothing. Its German is the
weakest of the four. A specialised model also cannot read a glossary unless it
follows instructions — see below — and over a whole manual that trade usually
decides it.

## How they fail

The failures are worth more than the table. Each was found by running the tool
on its own documentation, and each is now a guard with a test.

### Prompt echo — small models, short fragments

`qwen2.5-7b` returned table cells filled with **our own system prompt**,
translated:

```
| RULES: - Output ONLY the result. No quotes, no commentary, no explanation.
  - Dies ist ein Fragment eines größeren Dokuments... |
```

A cell reading "never transmitted" is three imperative words. Sent alone under
a prompt full of rules, a 7B cannot tell content from instruction. The part
grew from 1301 to 3266 characters and **passed structure verification** — the
table still had six rows and the same pipes. Structure intact, content
nonsense.

Two guards now catch it: a reply carrying our instruction text, and a reply
several times longer than its fragment. Batching short fragments into one
numbered request removed the cause.

### Invented markers

The same model returned `⟦1⟧ Es ist erlaubt, in bestimmten Fällen Ausnahmen zu
machen.` for a cell with no protected spans at all. With an empty token list
there was nothing to substitute, so the marker went straight into the document.
`restore` now verifies the markers it *finds*, not the ones it expects.

### Modified code fences

In `-mode chunk` without masking, `qwen2.5-7b` altered code fences in **three
of five** parts of the same document. All three were rejected and left open, so
nothing was corrupted — but nothing progressed either. Chunk mode now masks
code behind sentinels; block mode never sends it.

### A chat template that refuses every request

`translategemma-4b-it` returns HTTP 400 to every shape the OpenAI chat schema
can produce:

| Request | Answer |
|---|---|
| system + user | `Conversations must start with a user prompt.` |
| user only, plain text | `User role must provide content as an iterable with exactly one item…` |
| user with the mapping its own model card documents | same — the OpenAI layer strips `source_lang_code` before the template runs |

Its chat template turns out to build ordinary English prose rather than control
tokens, so rendering the turn ourselves works:

```bash
-llm-transport completions -llm-template translategemma
```

Verified end to end: the code fence came back byte-identical, and the `⟦n⟧`
sentinels survived although the model was never told about them.

**It cannot read a glossary.** Its template accepts a text and a language pair
and has no slot for a rule. That is the trade with any specialised translator:
a better sentence, no terminology control.

### A prescribed request format

`zongwei/gemma3-translator` needs `Translate from English to German: <text>`
and has no other way to learn the target language. Its own system prompt lives
in the Modelfile, so sending ours would replace the part worth keeping:

```bash
-llm-user-template gemma3-translator     # and no system message is sent
```

### Wrong direction

`mlc-deenes-expert` (494M) and `mlc-deesen-expert` (8.5B) both carry
*"Your ONLY task is to translate German or Spanish text into English."* They
are not candidates for English → German whatever the request format. Worth
checking a specialised model's direction before its size.

## Context windows

`translategemma`'s model card states **2K tokens total input** for the whole
family, 4b through 27b — a bigger one buys quality, not room.

Chunk sizes matter less than they look. With `-size 8000`, section-aligned
cutting produces chunks averaging ~2000–2800 characters on heading-rich
Markdown, because a cut lands before a heading once a chunk has substance. A
document *without* headings does fill to 8000. On a small context window, pass
`-size 3000` and the worst case is covered too.

## Reproducing this

```bash
mcp-md-splitter -cli -file <doc>.md -size 4000 -target de -out-dir /tmp/cmp
cp i18n/de/glossary.json /tmp/cmp/
MDSPLIT_LLM_URL=http://<host>:11434/v1 \
  mcp-md-splitter -translate -dir /tmp/cmp -lang de -mode block -llm-model <model>
mcp-md-splitter -merge -dir /tmp/cmp
```

The status line reports requests, how many short fragments were batched and
accepted, and how many fragments were left in the source language.

## Two things this does not measure

**Structure is not quality.** Every guard here checks that a translation is not
*damage*. A model can pass all of them and still produce clumsy German —
`### zte zwei Modi` cleared structure verification. Nothing automated will tell
you the prose is bad.

**One section is a sample, not a statistic.** These numbers come from a single
hard passage. They are enough to rule a model out, not to rank two that both
pass.
