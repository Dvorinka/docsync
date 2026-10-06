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

const usage = `docsync — documentation drift detector

Usage:
  docsync check   [--json] [--root DIR] [--config FILE]
  docsync report  [--json] [--root DIR] [--config FILE]
  docsync fix     [--dry-run] [--json] [--root DIR] [--config FILE]

Exit codes: 0 clean, 1 warnings only, 2 critical drift, 5 error.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(5)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	root := fs.String("root", ".", "repo root to scan")
	config := fs.String("config", "", "path to .docsync.yml")
	dryRun := fs.Bool("dry-run", false, "show what fix would change")
	if err := fs.Parse(os.Args[2:]); err != nil {
		os.Exit(5)
	}

	abs, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(5)
	}

	cfg, err := internal.LoadConfig(abs, *config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(5)
	}

	switch cmd {
	case "check":
		res, err := internal.RunCheck(abs, cfg)
		if err != nil {
			fail(err)
		}
		if *jsonOut {
			internal.WriteJSON(os.Stdout, res)
		} else {
			internal.Report(os.Stdout, res)
		}
		os.Exit(internal.ExitCode(res))
	case "report":
		res, err := internal.RunCheck(abs, cfg)
		if err != nil {
			fail(err)
		}
		if *jsonOut {
			internal.WriteJSON(os.Stdout, res)
		} else {
			internal.Report(os.Stdout, res)
		}
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
