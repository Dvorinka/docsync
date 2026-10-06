// docsync — documentation drift detector.
// Verifies that docs match codebase reality: env vars, file paths,
// commands, API routes, and build pins.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Dvorinka/docsync/internal"
)

// version is stamped at release: -ldflags "-X main.version=v0.1.0".
var version = "dev"

const usage = `docsync — documentation drift detector

Usage:
  docsync check   [--json | --format sarif] [--baseline FILE]
                  [--baseline-out FILE] [--root DIR] [--config FILE]
  docsync report  [--json] [--baseline FILE] [--root DIR] [--config FILE]
  docsync fix     [--dry-run] [--json] [--root DIR] [--config FILE]
  docsync version

Exit codes: 0 clean, 1 warnings only, 2 critical drift, 5 error.

A baseline file suppresses known findings — generate once with
--baseline-out, then adopt incrementally.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(5)
	}
	cmd := os.Args[1]
	if cmd == "version" || cmd == "--version" || cmd == "-version" {
		fmt.Println("docsync", version)
		return
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	format := fs.String("format", "", "output format: sarif")
	root := fs.String("root", ".", "repo root to scan")
	config := fs.String("config", "", "path to .docsync.yml")
	dryRun := fs.Bool("dry-run", false, "show what fix would change")
	baseline := fs.String("baseline", "", "suppress findings listed in FILE")
	baselineOut := fs.String("baseline-out", "", "write current findings as a baseline file")
	if err := fs.Parse(os.Args[2:]); err != nil {
		os.Exit(5)
	}

	abs, err := filepath.Abs(*root)
	if err != nil {
		fail(err)
	}

	cfg, err := internal.LoadConfig(abs, *config)
	if err != nil {
		fail(err)
	}

	var bl map[string]bool
	if *baseline != "" {
		bl, err = internal.LoadBaseline(*baseline)
		if err != nil {
			fail(err)
		}
	}

	emit := func(res internal.Result, alwaysZero bool) {
		res = internal.ApplyBaseline(res, bl)
		if *baselineOut != "" {
			if err := internal.WriteBaseline(*baselineOut, res.Findings); err != nil {
				fail(err)
			}
		}
		switch {
		case *format == "sarif":
			if err := internal.WriteSARIF(os.Stdout, res, version); err != nil {
				fail(err)
			}
		case *jsonOut:
			internal.WriteJSON(os.Stdout, res)
		default:
			internal.Report(os.Stdout, res)
		}
		if !alwaysZero {
			os.Exit(internal.ExitCode(res))
		}
	}

	switch cmd {
	case "check":
		res, err := internal.RunCheck(abs, cfg)
		if err != nil {
			fail(err)
		}
		emit(res, false)
	case "report":
		res, err := internal.RunCheck(abs, cfg)
		if err != nil {
			fail(err)
		}
		emit(res, true)
	case "fix":
		fr, res, err := internal.Fix(abs, cfg, *dryRun)
		if err != nil {
			fail(err)
		}
		if *jsonOut {
			internal.WriteJSON(os.Stdout, fr)
		} else {
			for _, a := range fr.Fixed {
				verb := "Added"
				if *dryRun {
					verb = "Would add"
				}
				fmt.Printf("  %s %s= to %s\n", verb, a.Key, a.File)
			}
			fmt.Printf("%d env var(s) %s. %d other finding(s) require manual fixes — see `docsync report`.\n",
				len(fr.Fixed), map[bool]string{true: "would be added", false: "added"}[*dryRun],
				len(fr.Unfixed))
			_ = res
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(5)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(5)
}
