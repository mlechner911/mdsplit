---
translation:
  tool: mdsplit
  version: 1.5.0
  url: "https://github.com/mlechner911/mdsplit"
  source: README.md
  source_sha256: 04c5e5b962e49303
  source_chars: 29365
  target_lang: fr
  model: "qwen3.5:35b"
  mode: block
  parts: 18/18
  translated: "2026-08-27T20:11:43Z"
  machine_translation: true
---

![Un détective inspectant un document Markdown avec une loupe, mettant une section en focus tandis que le reste reste intact. ](docs/hero.jpg)

<sub>Illustration créée avec Nano Banana 2</sub>

# Découpeur Markdown

[![CI](https://github.com/mlechner911/mdsplit/actions/workflows/ci.yml/badge.svg)](https://github.com/mlechner911/mdsplit/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Release](https://img.shields.io/github/v/tag/mlechner911/mdsplit?label=release&color=blue)](https://github.com/mlechner911/mdsplit/releases)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![MCP](https://img.shields.io/badge/MCP-9%20tools-8957e5)](#mcp-mode-default)

<sub>Traduit par machine par cet outil : [Deutsch](README.de.md) · [Español](README.es.md) · [Français](README.fr.md) · [中文](README.zh.md) — chacune porte sa provenance dans la matière de front.</sub>

Sépare les documents Markdown en fragments de taille limitée qui restent sûrs pour la traduction ou le traitement par les LLM. Les blocs atomiques (barres de code, tableaux, listes, HTML multi-lignes) ne sont jamais séparés entre les limites des fragments — seuls des blocs entiers se déplacent entre les fragments.

Implémenté en Go. Six modes d'exécution — séparation, fusion, traduction, glossaire, vérification et plan — ainsi qu'un serveur MCP (Model Context Protocol) qui expose le même travail sous forme de flux de tâches fractionnées.

![Le pipeline : un document source est découpé en morceaux de taille bornée sans rompre les barres de code, les tableaux ou le HTML ; le séparateur ne retourne qu'un manifeste, jamais le texte ; chaque partie voyage vers un LLM local comme une requête sans état sans historique de chat, et revient via put_chunk. ](docs/pipeline.svg)

## Motivation

Traduire un long document Markdown avec un modèle local est avant tout un problème de contexte, pas un problème de langue. Un manuel de 60 Ko ne rentre pas dans une fenêtre de 8k tokens, il faut donc le couper — et les coupures naïves sont exactement celles qui causent des dommages. Séparer tous les N caractères fait qu'une barre de code se retrouve en partie dans un morceau et en partie dans l'autre ; le modèle traduit fidèlement la moitié orpheline, renomme les identifiants, et le bloc ne se ferme plus jamais. Séparer aux lignes blanches fait perdre à un tableau sa ligne d'en-tête. Séparer uniquement aux titres fait qu'une section ne fait que 200 octets tandis que la suivante fait 40 Ko.

Transférer l'intégralité du fichier au modèle et lui demander de le découper ne sert à rien non plus : le fichier est déjà dans le contexte à ce stade, ce qui était précisément à éviter.

Ainsi, cet outil effectue la découpe à l'extérieur du modèle, selon deux règles :

1. **Certains blocs sont indivisibles.** Les barres de code, les tableaux, les éléments de liste avec leur
   continuations, éléments HTML. Ils restent entiers même lorsque cela signifie souffler
   à travers le budjet de taille — un morceau deux fois plus grand est une gêne,
   Un fragment se terminant en cours de clôture est une corruption.
2. **La taille est un objectif, pas une loi.** Les coupures s'appliquent avant les en-têtes, donc un bloc
   commence à une section et la transporte en entier. Un petit morceau légèrement plus petit qui est
   une version autonome se traduit mieux qu'une version complète qui commence en plein milieu d'une phrase.

Le côté MCP découle de la même préoccupation. `split_markdown` retourne un manifeste — nombre de parties, tailles, titres — et non le texte. Le contenu est extrait une partie à la fois avec `get_chunk` et réécrit avec `put_chunk`, de sorte que le contexte reste plat que la source fasse 10 Ko ou 10 Mo.

La dernière pièce est le chemin du retour. Le découpage en blocs n'est sûr que s'il est réversible, de sorte que le manifeste enregistre l'écart des lignes blanches à chaque frontière et que la fusion est vérifiée octet par octet contre la source. Si l'aller-retour est exact pour le document non traduit, le pipeline n'a pas mangé silencieusement quoi que ce soit — et toute différence après la traduction est l'œuvre du modèle, pas de l'analyseur.

Conçu pour Crush](https://github.com/charmbracelet/crush) conduisant un modèle local Ollama, mais rien en lui n'est spécifique à l'un ou à l'autre.

## Fonctionnalités

- **Chunk alignés sur les sections** : la taille est un budjet *doux*. Le séparateur préfère
  de couper avant un titre plutôt que de remplir un bloc jusqu'à la limite, de sorte qu'un bloc
  commence à une section et la conserve dans son intégralité. Un titre ne termine jamais un bloc.
- **Les blocs atomiques ne sont jamais séparés**, quel que soit le budjet : barres de code
  (une chaîne d'informations — ` ```js title="a.js" `, ` ``` go `, `~~~`, `````), GFM
  groupes de lignes de tableau, éléments de liste avec leurs suites et HTML multiligne
  éléments. Un bloc indivisible qui dépasse le budjet obtient son propre morceau
  et une note sur stderr.
- **Aller-retour byte-exact** : `index.json` enregistre l'espace de lignes blanches à chaque
  limite de bloc, donc la fusion reproduit le byte par byte la source. Le seul
  la normalisation est documentée dans `Canonical()` : les lignes blanches de début et de fin
  sont éliminées et les lignes ne contenant que des espaces deviennent vides. L'indentation et le durcissement
  les sauts de ligne (deux espaces à la fin) sont conservés.
- **Mode CLI** : écrit `<name>-part-NN.md` fichiers ainsi qu'un manifeste `index.json` (fichier source, nombre de parties, liste ordonnée des parties) à côté du fichier source
- **Mode de fusion** : `-merge -dir chunks/` réassemble les fragments via le manifeste et signale si le résultat est identique au niveau des octets, identique au niveau des espaces blancs ou divergent.
- **Flux de travail MCP** : `split_markdown` retourne un manifeste, pas le document.
  Le contenu se déplace une partie à la fois via `get_chunk` / `put_chunk`, donc le contexte
  reste constante quelle que soit la taille de la source — ce qui est tout l'enjeu
  d'exécution contre un petit modèle local.

## Exigences

- Go 1.25+
- Optionnel : [taskfile](https://taskfile.dev/) (`go install github.com/go-task/task/v3/cmd/task@latest`)
- Facultatif, pour la traduction : n'importe quel point de terminaison compatible avec OpenAI — un local
  [Ollama](https://ollama.com/) ou [LM Studio](https://lmstudio.ai/) serveur, ou
  l'API OpenAI. Configurée par processus, jamais par requête ; voir
  [Traduction](#translation).

## Construction et test

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

## Utilisation

### Mode CLI

```bash
./bin/mcp-md-splitter -cli -file docs/integration.md -size 4000
./bin/mcp-md-splitter -cli -file docs/integration.md -size 4000 -target de
```

Sorti dans un répertoire `chunks/` à côté du fichier source :

```
chunks/
├── integration-part-01.md
├── integration-part-02.md
├── integration-part-03.md
└── index.json          # Manifest: id, source_file, total_parts, size, target,
                        #           gaps[], parts[{part,file,chars,heading}]
```

### Mode fusion

```bash
# reassemble via chunks/index.json
./bin/mcp-md-splitter -merge -dir chunks/
./bin/mcp-md-splitter -merge -dir chunks/ -out combined.md
```

Le résultat est écrit à côté de la source comme `<source>.merged`, ou comme `<source>.<target>.md` lorsque les parties ont été traduites. Chaque partie utilise sa version modifiée si elle existe et l'originale sinon, de sorte qu'une exécution à moitié terminée fusionne toujours.

La vérification aller-retour compare d'abord de manière strictement identique au niveau des octets (`Canonical`) et uniquement ensuite de manière tolérante (`Normalize`), de sorte que la normalisation ne peut plus masquer une différence réelle. Elle signale : identique au niveau des octets / uniquement des espaces / divergent.

Sans `index.json`, tous les fichiers `*-part-NN.md` sont fusionnés dans l'ordre lexicographique ; sans le `gaps` du manifeste, une ligne blanche est supposée à chaque frontière.

### Mode MCP (par défaut)

Exécutez sans drapeaux pour servir le serveur MCP stdio. Les outils forment un flux de travail de **tâche** afin qu'un petit modèle local n'ait jamais à contenir l'intégralité du document : `split_markdown` écrit les fragments sur disque et ne retourne qu'un manifeste, puis le contenu se déplace une partie à la fois.

| Outil | Arguments | Retourne |
|---|---|---|
| `split_markdown` | `filePath`, `size` (8000), `target`, `outDir` | manifeste uniquement — `jobId`, taille et titre par partie. **Non** le contenu. |
| `get_chunk` | `jobId`, `part` | le texte de cette partie (version modifiée si elle existe) |
| `put_chunk` | `jobId`, `part`, `text` | stocke la partie traduite, fait un rapport sur l'avancement et indique la prochaine partie ouverte |
| `job_status` | `jobId` \| `chunksDir` | progression et liste des parties, sans contenu |
| `merge_chunks` | `jobId` \| `chunksDir`, `out` | rassemblés ; les parties modifiées gagnent, les parties intactes reviennent à l'original |
| `translate_chunk` | `jobId`, `part`, `language`, `mode` | traduit une partie via le point de terminaison configuré ; seule une ligne de statut revient |
| `build_glossary` | `jobId`, `language`, `terms` | propose la terminologie du document et écrit `glossary.json` pour examen |
| `outline` | `filePath` | chaque titre avec la taille de la section qu'il ouvre. **Aucun texte.** Aucune tâche n'est nécessaire |
| `read_section` | `filePath`, `section` | une section verbatim, blocs intacts. Aucun travail requis |

Une exécution de traduction ressemble à ceci :

```
split_markdown(filePath="doc.md", size=2000, target="de")
  → jobId dfd9fa33cd, 11 Teile          (686 chars back, not 10 KB)
get_chunk(jobId, part=1) → translate → put_chunk(jobId, part=1, text=…)
  … repeat; put_chunk names the next open part each time …
merge_chunks(jobId) → doc.de.md
```

`put_chunk` ne touche jamais le bloc original, de sorte qu'une exécution peut être reprise, refaite partie par partie, ou fusionnée à moitié terminée. Sans aucune modification, `merge_chunks` vérifie également l'aller-retour byte-exact par rapport à la source.

Les chunks atterrissent dans `chunks/` à côté de la source (à remplacer par `outDir`); le `jobId` est un hachage stable du chemin de la source plus budjet, donc en fractionnant à nouveau le même fichier avec le même budjet, on réutilise la même tâche.

Installez-le une fois, puis enregistrez la commande avec n'importe quel client — aucun chemin n'est nécessaire (`go install` se place dans `~/go/bin`) :

```bash
go install github.com/mlechner911/mdsplit/cmd/mcp-md-splitter@latest
# from a checkout: task install
```

Le dépôt fournit un `.mcp.json` local au projet qui fait exactement cela pour vous, de sorte qu'un client lancé depuis ce répertoire (Crush, Claude Desktop, OpenCode, …) détecte automatiquement le serveur :

```json
{
  "mcpServers": {
    "md-splitter": {
      "command": "mcp-md-splitter"
    }
  }
}
```

Une entrée globale `crushrc` fonctionne de la même manière : `mcp add md-splitter --command mcp-md-splitter`.

## traduction

Avec un point de terminaison configuré, le séparateur peut exécuter la traduction lui-même, une requête isolée par élément. Rien ne s'accumule entre eux, donc un document de 10 Mo coûte autant par étape qu'un document de 10 Ko.

Les paramètres de point de terminaison sont **la configuration du processus, jamais les arguments de l'outil**. Un jeton transmis via un appel d'outil atterrirait dans le transcript de conversation du client, et une URL choisie par l'appelant transformerait un outil qui lit des fichiers locaux en un canal d'exfiltration dès qu'un document traduit porte une instruction injectée. `MDSPLIT_LLM_TOKEN` n'a pas de flag exprès, afin de rester hors de la liste des processus également.

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

Via MCP, la même boucle est `translate_chunk(jobId, part)` une fois par partie ; seul un statut d'une ligne revient, jamais le texte.

### Deux modes

| | `-mode block` (par défaut) | `-mode chunk` |
|---|---|---|
| Ce qui est envoyé | fragments de prose uniquement | la partie entière, code masqué |
| balises de code, HTML | jamais transmis | remplacé par des sentinelles `⟦n⟧` |
| puces, barres verticales, indentation | reproduit littéralement | envoyé, vérifié ensuite |
| Structure | garantie | vérifiée et rejetée si incorrecte |
| Requêtes par partie | une par fragment | une |
| Nécessite un modèle suivant les instructions | non | oui |

Le mode bloc est le défaut car il rend les dommages impossibles plutôt que détectables : un modèle ne peut pas réécrire du code qu'il n'a jamais reçu. Cela rend également un modèle de traduction pure utilisable — TranslateGemma et ses semblables acceptent un texte et une paire de langues, sans canal pour une règle telle que « laissez le code intact ».

Le mode chunk maintient la prose connectée, ce sur quoi repose réellement la qualité de la traduction, car l'ordre des mots est décidé à travers une clause plutôt qu'à l'intérieur de celle-ci. Masquer d'abord le code élimine un quart à un tiers de l'entrée dans les Markdown techniques typiques, ce qui peut décider si une partie s'adapte ou non à une fenêtre de contexte petite.

Dans les deux modes, le code en ligne, les cibles d'URL, de lien et d'image, les liens de référence, les notes de bas de page et le HTML en ligne sont masqués. Le texte *alt* des images reste traduisable exprès : il s'agit d'une prose que le lecteur voit, tandis que le chemin ne l'est pas.

### Glossaire

Parce que chaque fragment est traduit isolément, rien n'empêche autrement un modèle de rendre le même terme de deux manières dans deux fragments. Mesuré sur ce README, un terme a dérivé de quatre façons à travers quatre langues :

| "code fences" est devenu | |
|---|---|
| Espagnol | délimiteur de code |
| Français | **un code** — le terme a simplement disparu |
| Chinois | Bloc de code |
| Allemand | Sections de code |

```bash
mcp-md-splitter -glossary -dir chunks/ -lang es   # writes chunks/glossary.json
$EDITOR chunks/glossary.json                      # ← the point of the exercise
mcp-md-splitter -translate -dir chunks/ -lang es  # picks it up automatically
```

Les candidats sont trouvés **sans modèle** : mots et expressions qui se répètent à travers plusieurs blocs et qui apparaissent également dans du code ou un identifiant quelque part dans le document — « bloc » est une prose sur une ligne et `chunks/` sur la suivante. Seuls les tokens de forme d'identifiant comptent à l'intérieur d'une clôture, car les blocs encadrés sont remplis d'anglais dans les commentaires et les valeurs JSON, et compter cela a fait que « sans » et « retourne » semblaient être des termes techniques.

Ils sont ensuite traduits en **une** seule requête, et non une par bloc. Une passe par bloc lierait une sortie structurée fragile à la précieuse — un échec d'analyse JSON entraînerait également une perte de traduction — et rendrait le glossaire dépendant de l'ordre dans lequel les blocs sont traités.

Le glossaire est établi **avant** la traduction puis figé. Le faire croître pendant la traduction laisserait les premières parties réalisées avec un glossaire vide et la dernière avec un glossaire complet, incorporant ainsi l'incohérence même qu'il existe pour éliminer dans les parties faites en premier, et rendant une partie unique impossible à refaire de manière autonome.

Une valeur qui revient sous forme de phrase est rejetée plutôt que stockée. Ce n'est pas cosmétique : chaque entrée est injectée dans chaque prompt qui mentionne son terme, comme `term = value`, de sorte qu'une phrase là-bas oriente la traduction au lieu de l'affiner. Comparé à un modèle de 7B, `chunk starts` est revenu sous forme de *"un bloque que comienza en mitad de una fence es dañino."*

Seules les entrées dont le terme apparaît réellement dans un fragment sont envoyées avec celui-ci, de sorte qu'un glossaire de 200 entrées n'engorge pas chaque requête.

`glossary.json` est destiné à être modifié. « Interface = Schnittstelle » est une décision, pas un fait, et c'est le point le moins coûteux dans le pipeline pour la corriger — quelques minutes ici valent mieux que de relire onze segments traduits.

#### Langue source

L'extraction de candidats suppose une **source anglaise**, et pas seulement dans l'instruction. Sa liste de mots vides est en anglais, et elle trouve des mots en les séparant par des espaces. Passez `-source-lang` pour indiquer le contraire ; l'outil vous indique alors à quoi vous attendre plutôt que de vous retourner silencieusement une liste d'articles et de prépositions :

| Source | Ce qui se produit |
|---|---|
| Anglais (par défaut) | Pour quoi l'extraction a été conçue et mesurée |
| Allemand, espagnol, français, … | fonctionne, mais avertit : les mots fonctionnels apparaîtront parmi les candidats et devront être supprimés |
| Chinois, japonais, coréen, thaï | refuse — il n'y a pas d'espaces pour la séparation, donc rédigez le glossaire à la main |

La paire de langues est enregistrée dans `glossary.json` comme `source_lang` / `target_lang`, car un glossaire n'est valide que pour la paire pour laquelle il a été construit.

La traduction elle-même n'a pas une telle limitation : `-source-lang` atteint le modèle de prompt et les garanties de la structure en mode bloc tiennent quelles que soient les langues. Seule l'extraction de terminologie est de forme anglaise.

### Modèles prescrivant un format de requête

Un modèle de traduction souhaite souvent que la requête soit structurée d'une manière particulière.
`zongwei/gemma3-translator` attend `Translate from English to German: <text>`
et n'a aucun autre moyen d'apprendre la langue cible — le texte brut ne porte aucune information.

```bash
mcp-md-splitter -translate -dir chunks/ -lang de \
  -llm-model zongwei/gemma3-translator:1b -llm-user-template gemma3-translator
```

`-llm-user-template` prend un modèle Go avec les mêmes champs, ou une abréviation : `gemma3-translator` pour le format ci-dessus, `translategemma` pour la propre instruction de TranslateGemma en tant que message utilisateur brut. Lorsqu'il est défini, **aucun message système n'est envoyé** : celui qui façonne le message lui-même décide si les règles doivent y figurer. C'est ce qui rend un modèle avec son propre prompt système intégré utilisable — un message système provenant de nous le remplacerait.

Le même modèle peut nécessiter l'un ou l'autre mécanisme selon qui le sert.
Le modèle de chat de TranslateGemma refuse toutes les requêtes de forme OpenAI dans LM Studio, donc là il a besoin de `-llm-transport completions -llm-template
translategemma`. La documentation d'emballage d'Ollama considère l'instruction comme la tâche de l'appelant, donc là `-llm-user-template translategemma` suffit. Les deux rendent le même texte, y compris les deux lignes blanches avant le passage que sa fiche modèle met en évidence.

### Modèles dont le modèle de conversation ne prendra pas en charge notre demande

Certains modèles livrent un modèle de chat qu'une couche compatible OpenAI ne peut satisfaire. TranslateGemma en est l'exemple : il rejette purement et simplement un rôle système (« Les conversations doivent commencer par une invite utilisateur ») et souhaite que le contenu de l'utilisateur soit une mappage portant `source_lang_code` et `target_lang_code` — des champs que le schéma OpenAI supprime avant que le modèle ne les voie. Chaque variante renvoie un HTTP 400.

Son modèle s'avère produire du prose anglais ordinaire et non des jetons de contrôle exotiques, ce qui fait que le rendu du tour ici plutôt que sur le serveur contourne tout le problème :

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

Le modèle est un modèle Go avec `.System`, `.User`, `.SourceLang`, `.TargetLang`, `.SourceLangName` et `.TargetLangName`, de sorte qu'une seule configuration couvre chaque paire de langues ; `-llm-template translategemma` est une abréviation pour ce qui précède, et `MDSPLIT_LLM_STOP` définit les séquences d'arrêt (par défaut `<end_of_turn>`).

Le mode bloc est le bon compagnon ici — un modèle qui prend une paire de langues et rien d'autre n'a pas de canal pour une règle comme « laissez le code seul », donc la structure doit être garantie plutôt que demandée.

**Un modèle sans canal d'instruction ne peut pas non plus utiliser un glossaire.** C'est le compromis à peser : un modèle spécialisé dans la traduction traduit généralement mieux une phrase, mais il ne peut pas être informé que *Fence* doit être rendu d'une manière particulière tout au long d'un manuel. Sur un document de longue haleine, une terminologie cohérente a souvent plus d'importance qu'une seule phrase, de sorte qu'un modèle général suivant les instructions et disposant d'un glossaire révisé battra souvent un meilleur traducteur travaillant à l'aveugle.

### Provenance et fraîcheur

Chaque séparation enregistre d'où elle provient, dans `index.json`, qu'elle soit ou non traduite :

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

`source_sha256` est le champ qui justifie son existence. La taille et la date sont informatives ; l'empreinte permet à une exécution ultérieure de répondre à la question qui compte vraiment pour une documentation maintenue :

```bash
mcp-md-splitter -check -dir chunks/
# source: CHANGED since the split - re-split and retranslate   (exit 1)
```

Une traduction qui est devenue silencieusement obsolète est pire qu'une qui manque manifestement, et rien d'autre dans la chaîne ne le remarquerait.

`-merge -stamp` écrit également l'enregistrement dans les métadonnées YAML propres du document, pour qu'une *personne* puisse le voir — notamment `machine_translation: true`, car une traduction automatique ressemble autrement exactement à quelque chose que quelqu'un aurait écrit et révisé.

Il ne se contente pas de préfixer simplement. Un document qui possède déjà des métadonnées serait détruit par un second bloc `---`, car le premier cesse d'être des métadonnées et devient du contenu ; un bloc existant est modifié in situ, et l'application de deux timbres remplace l'enregistrement au lieu de le dupliquer. L'URL de point de terminaison n'est délibérément pas enregistrée : un nom de modèle est une provenance, une adresse interne est une fuite.

Deux conséquences à connaître. La vérification aller-retour s'exécute sur le texte non timbré, sinon elle signalerait « diffère » indéfiniment après le premier timbre. Et l'horodatage rend la fusion non idempotente, de sorte qu'un document reconstruit selon un calendrier affiche une différence à chaque exécution même lorsque rien n'a changé — c'est pourquoi le timbre est optionnel et que le hachage, et non la date, est le champ porteur.

### Ce qui est vérifié avant tout stockage

- Chaque sentinelle doit revenir exactement une fois. Instruire un modèle pour préserver
  eux sont la politesse ; les compter est le mécanisme.
- `finish_reason` doit être `stop`. Une réponse tronquée perd du texte silencieusement, ce qui
  est l'échec que cet outil entier existe pour prévenir.
- La réponse doit avoir la structure de la source : mêmes types de blocs dans le même
  ordre, barres de code octet par octet, mêmes niveaux de titres et mêmes nombres de lignes de tableau.
  La prose peut changer complètement — sinon le contrôle serait inutile pour un
  traduction.

Une partie qui échoue à l'un de ces tests n'est **pas stockée** et reste ouverte, de sorte que la réexécution ne tente que celles-ci. Le bloc d'origine n'est jamais écrasé.

## Lire au lieu de traiter

![Lecture par sujet : les outils outline et read_section se situent entre un grand document de référence et l'agent, ne nécessitent aucune tâche et n'écrivent aucun fichier, et envoient uniquement la section dont la question a réellement besoin. ](docs/reading.svg)

Les outils ci-dessus résolvent *"traiter ce document entier sans dépasser la limite de contexte"*. `outline` et `read_section` résolvent l'autre moitié : *"répondre à partir de ce document sans le lire en entier"*. Même primitive — découper le Markdown en toute sécurité et rendre les morceaux adressables — appliquée à une question différente.

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

Lire le sommaire de ce README coûte environ 700 caractères ; la section « Deux modes » en compte 1301. Une question sur les modes peut être traitée à partir de 2 Ko au lieu de 23 Ko — et la taille indiquée couvre l'ensemble de la section, y compris ses sous-sections, ce qui détermine si sa lecture est abordable ou non.

Aucun outil n'a besoin d'une tâche : aucun chunk n'est écrit, aucun manifeste, ni `jobId`. Cet appareil existe pour traiter un document de bout en bout ; consulter quelque chose ne nécessite aucune de ces choses.

**Une section n'est pas un chunk.** Les chunks suivent le budjet en octets, les sections suivent la structure : `## Usage` ci-dessus s'étend sur plusieurs chunks, tandis qu'une courte section partage un chunk avec son voisin. `read_section` retourne le titre et tout ce qui suit, jusqu'au prochain titre de même niveau ou plus bas — ainsi une barre de code revient toujours entière.

Adressez une section par son titre ou par son chemin (`"Usage > CLI"`) lorsque le même titre apparaît plusieurs fois. Un titre ambigu est refusé avec la liste des candidats, plutôt que deviné.

La différence avec un serveur de récupération basé sur des embeddings est que celui-ci est **exact plutôt que similaire** : vous obtenez la section que le document définit lui-même, sans index à construire, rien à synchroniser et aucune fenêtre qui peut commencer au milieu d'un bloc.

## Indiquer à votre agent comment l'utiliser

Enregistrer le serveur ne suffit pas. Les descriptions des outils indiquent ce que fait chaque outil, mais un modèle ne lit la description d'un outil qu'il a déjà décidé d'appeler — les règles d'ordre arrivent donc trop tard. Intégrez ceci dans le `AGENTS.md`, le `CLAUDE.md` ou le prompt système de votre projet :

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

Cette dernière règle est celle qu'il vaut mieux conserver. Un modèle auquel on remet un outil de découpage va souvent récupérer chaque fragment et les réassembler dans son propre contexte, ce qui coûte plus cher que la lecture du fichier — l'outil a été utilisé et sa finalité battue en même temps.

Les paramètres de point de terminaison appartiennent à `.mcp.json` sous `env`, jamais dans une invite : un modèle ne peut pas transmettre un jeton qu'il n'a jamais reçu, et c'est là tout le point.

Notez que le `AGENTS.md` propre à ce dépôt est un document différent — il informe les agents *travaillant sur* le séparateur, et non les agents *l'utilisant*.

## Structure du projet

Disposition standard Go `cmd`/`internal` :

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

Pipeline: `ExtractBlocks(content) []Block` analyse le Markdown en blocs atomiques — chacun portant son texte exact, les lignes blanches `Gap` qui le suivent, et son `Kind`. `groupBlocks` ensuite lie ce qui ne doit pas être séparé (les blocs sans ligne blanche entre eux ; un titre et sa section). `packRanges` remplit des chunks à partir de ces groupes, en privilégiant une coupure avant un titre. `SplitDoc(content, max)` retourne `Doc{Chunks, Gaps}` ; `JoinGaps(chunks, gaps)` est l'inverse exact.

## Notes de développement

- **Tout ce qu'un utilisateur ou un modèle lit est en anglais** : sortie CLI, erreur
  messages, aide de drapeau et les descriptions des outils MCP. Commentaires de code et tests
  les messages d'échec sont en allemand — cette séparation est délibérée, donc conservez-la.
- Aucune bibliothèque d'AST Markdown. Le parseur est basé sur des lignes par principe : un AST serait
  doivent être ramenés à la source pour s'activer, tout en préservant les octets de la source
  exactement est la seule chose sur laquelle le contrat aller-retour dépend.
- `TestRoundtrip_ProjectDocs` exécute le séparateur sur chaque `*.md` du dépôt
  root à trois budjets et affirme la byte-exactitude — le corpus réel le moins cher
  disponible sans quitter le dépôt.

## Intégration continue

[`.github/workflows/ci.yml`](.github/workflows/ci.yml) s'exécute à chaque push :
`gofmt`/`go vet`/`go mod tidy` une fois, la suite de tests sur Linux, macOS et Windows
(le séparateur gère principalement les chemins, et le registre des tâches réside dans
`os.UserCacheDir()`), ainsi qu'une tâche aller-retour qui effectue une séparation et une fusion de chaque
fichier Markdown du dépôt à trois budjets, échouant sauf si chacun revient
byte-identique.

## Licence

MIT © 2026 Michael Lechner — voir [LICENSE](LICENSE).
