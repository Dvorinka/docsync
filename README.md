<h1 align="center">docsync</h1>

<p align="center">
  Documentation drift detector.<br>
  Your README is a contract. Enforce it.
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> ·
  <a href="#check-catalog">Checks</a> ·
  <a href="#configuration">Configuration</a> ·
  <a href="CONTRIBUTING.md">Contributing</a>
</p>

<p align="center">
  <a href="https://github.com/Dvorinka/docsync/actions/workflows/ci.yml"><img src="https://github.com/Dvorinka/docsync/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/Dvorinka/docsync/releases"><img src="https://img.shields.io/github/v/release/Dvorinka/docsync" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/Dvorinka/docsync" alt="License"></a>
</p>

## What is docsync?

docsync is a single-binary Go CLI that verifies documentation matches
codebase reality. It is a linter, not a test runner: it cross-references
`.env.example`, README files, `AGENTS.md`, and other markdown docs against
the actual code to detect drift — env vars documented but unused, env vars
used but undocumented, file paths referenced that no longer exist, commands
documented that no longer resolve, API routes documented that don't match
route definitions, and build pins that disagree.

It exists because documentation rots silently. Developers lose an hour
discovering the documented commands don't work; agents lose entire sessions
because they trust docs as ground truth. `markdown-link-check` does URLs
only, `dotenv-linter` does `.env` syntax only — nothing checks "does the
README describe this repo accurately." That gap is the product.

## Features

- **Env var cross-reference** — `.env.example` keys vs `os.Getenv`,
  `process.env`, `$VAR`, compose `environment:` blocks, and `env:"KEY"`
  struct tags. Flags missing keys (critical) and orphans (warning).
- **File path check** — backtick paths, code-block tree lines, and markdown
  links resolved against the repo root. URLs, API routes, package paths,
  keyboard combos, and `cp`-style create targets are filtered out.
- **Command check** — `npm/pnpm/yarn/bun run <script>` vs `package.json`
  scripts, `make`/`just` targets vs the build file, `./script.sh` existence,
  `npx` tools vs `node_modules/.bin`.
- **API route check** (opt-in) — `GET /api/...` mentions in docs vs
  Gin/Echo/Chi/net-http and Express route definitions.
- **Version check** — "Requires Node 24" vs `package.json` engines,
  `go.mod`, CI matrices.
- **Pin cross-check** — `FROM node:26-alpine` vs `node-version: 22` vs
  engines, across Dockerfiles, workflows, and manifests. `expo install
  --check` shells out when a package depends on Expo.
- **`docsync fix`** — appends missing keys to `.env.example` with a TODO
  comment. Never edits prose. `--dry-run` supported.
- **SARIF output** — `--format sarif` feeds GitHub code scanning;
  findings land as PR annotations.
- **Baseline adoption** — `--baseline-out .docsync-baseline` records
  today's drift; `--baseline .docsync-baseline` suppresses it so
  legacy repos gate only *new* drift.
- **Agent-ready** — `--json` on every command; stable snake_case keys.

## Quick Start

```bash
go install github.com/Dvorinka/docsync/cmd/docsync@latest
```

Or build from source (Go 1.23+):

```bash
git clone https://github.com/Dvorinka/docsync.git && cd docsync
go build -o docsync ./cmd/docsync
```

Run it in any repo:

```bash
docsync check            # gate: exit 2 on critical drift
docsync report           # human-readable report, always exit 0
docsync fix --dry-run    # preview env additions
docsync fix              # append missing keys to .env.example
```

## Commands

### `docsync check [--json | --format sarif] [--baseline FILE] [--baseline-out FILE]`

The CI gate. Reads `.docsync.yml` if present (sensible defaults without
it), scans the repo, prints findings grouped by category.

Exit codes: `0` clean · `1` warnings only · `2` critical findings ·
`5` malformed config or unreadable input.

`--format sarif` emits SARIF 2.1.0 — pipe it to
`github/codeql-action/upload-sarif` and drift findings become PR
annotations.

`--baseline-out FILE` writes the current finding set as a baseline;
`--baseline FILE` then suppresses exactly those findings (matched by
`category|key`, immune to line-number churn). The JSON summary reports
`suppressed: N`.

### `docsync report [--json]`

Same data as `check`, formatted for reading. Always exits `0`.

### `docsync version`

Prints the binary version (`dev` for source builds; release binaries are
stamped).

### `docsync fix [--dry-run] [--json]`

Adds missing env keys to `.env.example` with empty values and a
`# TODO: document this var` comment. Never removes or modifies existing
entries, never touches markdown.

## Check catalog

| Category | Default severity | What it catches |
|---|---|---|
| `env_missing_in_example` | critical | code/composes uses a var `.env.example` lacks |
| `env_orphan_in_example` | warning | `.env.example` declares a var nothing reads |
| `path_not_found` | critical | doc references a file that doesn't exist |
| `command_not_found` | critical | doc references a script/target that doesn't exist |
| `route_not_found` | warning | doc lists a route no code defines |
| `route_undocumented` | warning | code defines a route docs don't mention |
| `version_mismatch` | warning | "Requires Node 24" vs engines/CI disagreeing |
| `pin_mismatch` | critical | Dockerfile vs workflow vs manifest pin conflict |
| `pin_unpinned` | warning | `FROM image:latest`-style floating tags |

Suppress noise per-repo:

```yaml
# .docsync.yml
ignore:
  - "path_not_found:docs/drafts/**"
  - "env_orphan_in_example:LEGACY_*"
```

Glob syntax: `*` matches one segment, `**` any depth, bare strings match
exactly.

## Configuration

All optional — `.docsync.yml` at the repo root:

```yaml
docs:            # markdown files to scan
  - "README.md"
  - "AGENTS.md"
  - "docs/**/*.md"

env_file: ".env.example"

code_dirs:       # where env-var references are scanned
  - "apps/api"
  - "apps/web"

languages:       # env-access patterns per language
  - go           # os.Getenv("KEY"), env:"KEY" struct tags
  - typescript   # process.env.KEY, import.meta.env.KEY
  - bash         # $KEY, ${KEY} in .sh/Makefile/Dockerfile
  - yaml         # compose environment:, env_file, ${KEY}

api_routes:      # opt-in route cross-reference
  - dir: "apps/api"
    framework: "go"       # gin/echo/chi/net-http
  - dir: "apps/web/src"
    framework: "express"  # app.get, router.post

dependency_files:
  - "package.json"
  - "go.mod"
  - ".github/workflows/*.yml"

severity:
  env_missing_in_example: critical
  env_orphan_in_example: warning
  path_not_found: critical
  command_not_found: critical
  route_not_found: warning
  route_undocumented: warning
  version_mismatch: warning
  pin_mismatch: critical
  pin_unpinned: warning
```

## JSON output

`--json` on `check`/`report`:

```json
{
  "findings": [
    {
      "severity": "critical",
      "category": "env_missing_in_example",
      "key": "INTAKE_TOKEN",
      "detail": "referenced in apps/api/intake.go:15 but missing from .env.example",
      "file": "apps/api/intake.go",
      "line": 15
    }
  ],
  "summary": { "critical": 1, "warning": 0, "total": 1 }
}
```

`--json` on `fix`:

```json
{
  "fixed": [{"action": "env_added", "key": "INTAKE_TOKEN", "file": ".env.example"}],
  "unfixed": [{"severity": "critical", "category": "path_not_found",
               "key": "apps/mobile/plugins/x.js", "detail": "requires manual fix"}]
}
```

Clean runs emit `"findings": []`, never `null`.

## What it is not

- **Not a test runner.** It verifies references resolve, not that commands
  execute. Running README code blocks end-to-end is a literate testing
  framework — different product.
- **Not a prose generator.** `fix` only appends env keys. It never writes
  or rewrites markdown.
- **Not a link checker.** External URLs are `markdown-link-check`'s job.
- No daemon, no server, no web UI, no MCP. One-shot CLI.

## Development

```bash
go build ./...
go test ./...      # fixture suites under testdata/fixtures/
go vet ./...
gofmt -l .
```

Single Go module, stdlib first; the only dependency is `gopkg.in/yaml.v3`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

See [SECURITY.md](SECURITY.md).

## License

[Apache-2.0](LICENSE)
