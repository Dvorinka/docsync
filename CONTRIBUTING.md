# Contributing

Contributions welcome. The bar is a green pre-push gate and a test for any
non-trivial logic.

## Workflow

1. Fork, branch from `main`.
2. Make the change. Keep it small — one concern per PR.
3. Run the gate:

   ```bash
   gofmt -l .
   go vet ./...
   go test ./...
   go build ./...
   ```

4. Open a PR describing *why*, not just *what*.

## Conventions

- Single Go module, standard library first. New dependencies need
  justification in the PR.
- Checkers are pure functions returning findings — no shared state.
- `--json` output is a contract: stable snake_case keys, never `null`
  where an empty array fits.
- Fixture repos for checker tests live in `testdata/fixtures/`. A new
  check ships with a fixture case.

## Reporting bugs

Include the repo layout that triggered it (or a minimal fixture),
`docsync check --json` output, and the expected vs actual finding.
