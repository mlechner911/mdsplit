package main

import (
	"flag"

	"github.com/mlechner911/mdsplit/internal/llm"
	"github.com/mlechner911/mdsplit/internal/meta"
)

// version ist die Server-Version für MCP-Clients; wird bei Bedarf via ldflags gesetzt.
var version = "1.1.0"

func main() {
	meta.Version = version

	cliMode := flag.Bool("cli", false, "run the standalone CLI export instead of the MCP server")
	filePath := flag.String("file", "", "path to the Markdown file (CLI mode)")
	chunkSize := flag.Int("size", 8000, "soft character budget per chunk; indivisible blocks may exceed it")
	mergeMode := flag.Bool("merge", false, "reassemble a split back into one document")
	chunksDir := flag.String("dir", "", "chunk directory containing index.json (merge mode)")
	outFile := flag.String("out", "", "merge output path (default: <source>.merged)")
	target := flag.String("target", "", "suffix for edited parts, e.g. de (default: out)")
	language := flag.String("lang", "", "target language for -translate, e.g. de, es, fr, zh")
	sourceLang := flag.String("source-lang", "en", "language the document is written in")
	translateMode := flag.Bool("translate", false, "translate every open part of a split via the configured LLM")
	stamp := flag.Bool("stamp", false, "write provenance (tool, version, source hash, model) into the merged document's YAML front matter")
	checkMode := flag.Bool("check", false, "report whether a split is still current: progress, and whether the source changed since it was made")
	mode := flag.String("mode", "block", "translation granularity: block (code never sent, structure guaranteed) or chunk (whole chunk, needs an instruction-following model)")

	// Endpoint settings are process configuration, never tool arguments: a
	// token passed through a tool call would end up in the conversation
	// transcript, and a caller-chosen URL would turn a tool that reads local
	// files into an exfiltration channel.
	env := llm.ConfigFromEnv()
	llmURL := flag.String("llm-url", env.BaseURL, "OpenAI-compatible base URL (env MDSPLIT_LLM_URL; default "+llm.DefaultURL+")")
	llmModel := flag.String("llm-model", env.Model, "model name (env MDSPLIT_LLM_MODEL)")
	llmTimeout := flag.Duration("llm-timeout", env.Timeout, "per-request timeout (env MDSPLIT_LLM_TIMEOUT)")
	llmTransport := flag.String("llm-transport", string(env.Transport), "chat (default) or completions; completions renders the prompt here instead of relying on the server's chat template (env MDSPLIT_LLM_TRANSPORT)")
	llmTemplate := flag.String("llm-template", env.PromptTemplate, "Go template for the completions transport, or the shorthand \"translategemma\"; fields .System .User .SourceLang .TargetLang .SourceLangName .TargetLangName (env MDSPLIT_LLM_TEMPLATE)")
	flag.Parse()

	cfg := llm.Config{
		BaseURL:        *llmURL,
		Model:          *llmModel,
		Token:          env.Token, // deliberately env-only: never a flag, never an argument
		Timeout:        *llmTimeout,
		Transport:      llm.Transport(*llmTransport),
		PromptTemplate: *llmTemplate,
		Stop:           env.Stop,
	}

	switch {
	case *translateMode:
		runTranslateMode(*chunksDir, cfg, *language, *sourceLang, *mode)
	case *checkMode:
		runCheckMode(*chunksDir)
	case *mergeMode:
		runMergeMode(*chunksDir, *outFile, *stamp)
	case *cliMode:
		runCLIMode(*filePath, *chunkSize, *target)
	default:
		runMCPMode(cfg)
	}
}
