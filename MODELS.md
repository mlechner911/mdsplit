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

## The two machines

Latency figures mean nothing without them.

| | Ollama box | LM Studio box |
|---|---|---|
| CPU | Apple M2 Ultra | AMD Ryzen 7 2700X (8 cores, AVX2) |
| Memory | 64 GB unified | 64 GB system RAM |
| GPU | integrated, shares the unified memory | NVIDIA RTX 3060, **12 GB VRAM**, CUDA |
| Serves | `ollama` on `:11434` | LM Studio 0.4.2 on `:1234` |

The 12 GB is VRAM, not a memory ceiling. LM Studio's *Limit Model Offload to
Dedicated GPU Memory* is off here, so a model larger than 12 GB still loads —
it spills into the 64 GB of system RAM and runs slower. *Offload KV Cache to
GPU Memory* is on, which matters for this workload: our requests are many and
small, and the cache is what a large context slot allocates per request.

The two are not comparable as hardware and the table below does not try to.
What is comparable is a model against itself under two settings, or two models
on the same machine.

## What was measured

| Model | Endpoint | On disk | Requests | Short fragments usable | Left in English | Chars |
|---|---|---:|---:|---:|---:|---:|
| `qwen3.5:35b` | Ollama, M2 Ultra | 23.9 GB Q4 | 4 | 20/20 | 0 | 1501 |
| `gemma3:27b` | Ollama, M2 Ultra | 17.4 GB Q4 | 4 | 20/20 | 3 | 1335 |
| `qwen2.5-7b-instruct` | LM Studio, RTX 3060 | 4.7 GB Q4 | 4 | 20/20 | 0 | 1509 |
| `zongwei/gemma3-translator:1b` | Ollama, M2 Ultra | 0.8 GB Q4 | 4 | 20/20 | 0 | 1481 |
| `qwen2.5-7b-instruct` *(before batching)* | LM Studio, RTX 3060 | 4.7 GB Q4 | 23 | 13/23 | 10 | **half the table English** |

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

## Reasoning models: the setting that decides everything

A reasoning model spends its budget deliberating over a translation. The same
sentence, the same endpoint, the same answer:

| `reasoning_effort` | Output tokens | Steady state |
|---|---:|---:|
| not set | 2977 | 40.2 s |
| `"minimal"` / `"low"` / `"medium"` / `"high"` | 2977 each | 40.2 s |
| **`"none"`** | **24** | **0.56 s** |

**Seventy-two times faster for an identical translation.** The level does not
matter — four different values produced the same token count. Only `"none"`
does anything.

Once thinking is off, `qwen3.5:35b` is the fastest model measured here, ahead
of a specialised 27B translator at 1.42 s. It also reads a glossary, which a
specialised translator cannot. Both arguments point the same way, which is
rare.

### Four ways to turn it off that report success and do not work

This is the part worth remembering. Every one of these is accepted without
complaint, and only the last does anything.

| | Reported | Actual |
|---|---|---|
| `PARAMETER think false` in a Modelfile | `success` | 4716 tokens, unchanged. Thinking is a request field, not a model parameter |
| `PARAMETER stop "<think>"` | `success` | 4691 tokens generated, then the stop fires **before the answer**: empty reply with `finish_reason: "stop"` |
| `reasoning_effort: "low"` | accepted | reduces to 2977, but no further than any other level |
| `reasoning_effort: "off"` | **rejected** | not a valid value here, whatever a search result says. The server names the ones that are |
| `reasoning_effort: "none"` | accepted | **24 tokens** |

The stop-token variant is the dangerous one: full cost paid, nothing returned,
and `finish_reason: "stop"` is precisely the signal a client reads as
*complete*. Only an empty-content check catches it.

```bash
mcp-md-splitter -translate -dir chunks/ -lang de \
  -llm-model qwen3.5:35b -llm-reasoning none
```

### How not to measure this

Three of our own attempts produced numbers that meant nothing, and the failures
generalise.

**Probing an endpoint that is busy.** The first "77 s per request" was a probe
queued behind a running translation job.

**Switching models between measurements.** Changing model or options makes
Ollama load 17–24 GB from disk, and that load lands inside the measurement.
Early figures of 65 s were mostly that.

**One sample per configuration.** `"minimal"` measured 4716 tokens once and
2977 another time. A single reading turned into a claim that levels do nothing,
which was wrong.

What works: one model, `keep_alive` long enough that it is not evicted, two
warm-up requests, then four measured, with nothing else touching the server.
The result was 40.16 / 40.18 / 40.22 / 40.15 — that consistency is what a
usable measurement looks like.

## Context windows

`translategemma`'s model card states **2K tokens total input** for the whole
family, 4b through 27b — a bigger one buys quality, not room.

A large context slot is not the problem it looks like. The server log showed
`n_ctx_slot = 128000` and 62.8 MiB checkpoints for a 115-token prompt, which
looked like the cause of the slowness. Rebuilding the model with
`num_ctx 8192` changed the time by 0.1 s: 65.4 s against 65.4 s. The log was a
red herring; the reasoning tokens were the answer.

Chunk sizes matter less than they look. With `-size 8000`, section-aligned
cutting produces chunks averaging ~2000–2800 characters on heading-rich
Markdown, because a cut lands before a heading once a chunk has substance. A
document *without* headings does fill to 8000. On a small context window, pass
`-size 3000` and the worst case is covered too.

A 12 GB card is not the constraint it looks like either, as long as offload to
system RAM is allowed: a model that does not fit in VRAM still runs, just
slowly. What decides whether a part fits is the model's context length, which
is a separate setting from either.

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
