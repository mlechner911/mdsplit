package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mlechner911/mdsplit/internal/job"
	"github.com/mlechner911/mdsplit/internal/llm"
	"github.com/mlechner911/mdsplit/internal/split"
	"github.com/mlechner911/mdsplit/internal/translate"
)

// runTranslateMode walks every part that has not been written back yet and
// sends each one as an isolated request. Nothing accumulates between parts, so
// a 10 MB document costs the same context per step as a 10 KB one.
func runTranslateMode(dir string, cfg llm.Config, language, mode string) {
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
	if !cfg.Ready() {
		fmt.Println("error: no model configured - pass -llm-model or set MDSPLIT_LLM_MODEL")
		os.Exit(1)
	}

	client := llm.New(cfg)
	opts := translate.Options{
		Language: language,
		Glossary: m.Glossary,
		Mode:     translate.Mode(mode),
	}
	if opts.Mode != translate.ModeBlock && opts.Mode != translate.ModeChunk {
		fmt.Printf("error: unknown -mode %q (use block or chunk)\n", mode)
		os.Exit(1)
	}
	_, missing := m.Progress()
	if len(missing) == 0 {
		fmt.Printf("nothing to do: all %d parts are already translated\n", m.TotalParts)
		return
	}

	fmt.Printf("translating %d of %d parts into %s (%s mode)\n", len(missing), m.TotalParts, translate.LanguageName(language), opts.Mode)
	fmt.Printf("endpoint: %s\n\n", cfg.Describe())

	failed := 0
	for _, n := range missing {
		res, err := translate.Part(context.Background(), client, m, n, opts)
		switch {
		case err == nil:
			note := ""
			if res.Mode == translate.ModeBlock {
				note = fmt.Sprintf(", %d fragments sent", res.Requests)
				if res.Reused > 0 {
					note += fmt.Sprintf(", %d reused", res.Reused)
				}
				if res.Kept > 0 {
					note += fmt.Sprintf(", %d kept as-is", res.Kept)
				}
			}
			if res.Glossary > 0 {
				note += fmt.Sprintf(", %d glossary terms", res.Glossary)
			}
			fmt.Printf("  ok   part %2d/%d  %d -> %d chars%s\n", n, m.TotalParts, res.InChars, res.OutChars, note)
		default:
			failed++
			var se *split.StructureError
			if errors.As(err, &se) {
				fmt.Printf("  FAIL part %2d/%d  not stored, structure drifted:\n", n, m.TotalParts)
				for _, r := range se.Reasons {
					fmt.Printf("         - %s\n", r)
				}
				continue
			}
			fmt.Printf("  FAIL part %2d/%d  %v\n", n, m.TotalParts, err)
		}
	}

	if err := m.RecordRun(language, cfg.Model, string(opts.Mode), time.Now().UTC().Format(time.RFC3339)); err != nil {
		fmt.Printf("warning: could not record provenance: %v\n", err)
	}

	done, still := m.Progress()
	fmt.Printf("\n%d/%d parts translated", done, m.TotalParts)
	if len(still) > 0 {
		fmt.Printf("; still open: %v", still)
	}
	fmt.Println()
	if failed > 0 {
		fmt.Printf("%d part(s) failed - rerun to retry only those\n", failed)
		os.Exit(1)
	}
	fmt.Printf("next: %s -merge -dir %s\n", filepath.Base(os.Args[0]), dir)
}
