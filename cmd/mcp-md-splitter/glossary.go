package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mlechner911/mdsplit/internal/glossary"
	"github.com/mlechner911/mdsplit/internal/job"
	"github.com/mlechner911/mdsplit/internal/llm"
	"github.com/mlechner911/mdsplit/internal/translate"
)

// runGlossaryMode proposes terminology for a split and writes it for review.
//
// It runs before any translation and the result is then frozen. Growing a
// glossary while translating would leave the first parts done with an empty one
// and the last with a full one - baking the inconsistency it exists to remove
// into exactly the parts translated first, and making a single part impossible
// to redo on its own.
func runGlossaryMode(dir string, cfg llm.Config, language, sourceLang string, limit int) {
	if dir == "" {
		fmt.Println("error: pass the chunk directory with -dir (e.g. chunks/)")
		os.Exit(1)
	}
	abs, err := filepath.Abs(dir)
	if err != nil || !dirExists(abs) {
		fmt.Printf("error: directory not found: %s\n", dir)
		os.Exit(1)
	}
	m, err := job.Load(abs)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	if language == "" {
		language = m.Language
	}
	if language == "" {
		language = m.Target
	}
	if language == "" {
		fmt.Println("error: no target language - pass -lang (e.g. -lang de)")
		os.Exit(1)
	}

	source, err := os.ReadFile(m.SourceFile)
	if err != nil {
		fmt.Printf("error: cannot read the source: %v\n", err)
		os.Exit(1)
	}
	if level, note := glossary.Support(sourceLang); level != glossary.SupportGood {
		fmt.Printf("warning: source language %q - %s\n", sourceLang, note)
		if level == glossary.SupportNone {
			os.Exit(1)
		}
	}
	cands := glossary.Candidates(string(source), limit)
	if len(cands) == 0 {
		fmt.Println("no recurring terminology found - nothing to pin")
		return
	}
	fmt.Printf("%d candidate terms from %s\n", len(cands), filepath.Base(m.SourceFile))

	if !cfg.Ready() {
		fmt.Println("error: no model configured - pass -llm-model or set MDSPLIT_LLM_MODEL")
		os.Exit(1)
	}
	lang := translate.LanguageName(language)
	fmt.Printf("asking %s for %s -> %s equivalents (one request)\n\n",
		cfg.Model, translate.LanguageName(sourceLang), lang)

	terms, err := glossary.Build(context.Background(), llm.New(cfg),
		translate.LanguageName(sourceLang), lang, cands)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	f := &glossary.File{
		SourceLang: sourceLang,
		TargetLang: language,
		Model:      cfg.Model,
		Generated:  time.Now().UTC().Format(time.RFC3339),
		Terms:      terms,
		Notes:      glossary.Notes(cands),
	}
	if err := f.Save(abs); err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	for _, k := range glossary.Sorted(terms) {
		fmt.Printf("  %-24s %s\n", k, terms[k])
	}
	fmt.Printf("\n%d of %d terms decided -> %s\n", len(terms), len(cands), filepath.Join(abs, glossary.FileName))
	fmt.Println("Review and edit that file before translating: a glossary is a set of decisions,")
	fmt.Println("and this is the cheapest point in the pipeline to correct one.")
}
