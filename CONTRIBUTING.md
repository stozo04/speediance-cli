# Contributing

Thanks for your interest in `speediance-cli`. It's a small, single-binary Go CLI; the bar is
"idiomatic, well-tested, and a non-Go maintainer can keep it healthy."

## Ground rules (the load-bearing ones)

This tool wraps an **unofficial, brittle** API and is consumed by other people's automation, so two
things are frozen — breaking them breaks real users:

1. **`--json` stdout is a contract.** Field names, nesting, types, and order must stay
   backward-compatible. stdout is machine output only; **all** hints/warnings/logs go to stderr.
2. **Request wire format is frozen.** Every URL, query param, JSON body field, and header value must
   match what the Android app sends. All API calls live in `internal/api` — that's the single patch
   point if the upstream app changes.

See `GOAL.md` for the full specification and rationale.

## Development

Requires Go 1.24+.

```bash
make check        # tidy + fmt + vet + lint + test -race  (run before every PR)
make test-race    # tests with the race detector
make lint         # golangci-lint
make fmt          # gofumpt + goimports
make snapshot     # cross-compile a local release build
```

CI runs build, `go test -race`, and `golangci-lint` on every PR; formatting is enforced
(`gofumpt` + `goimports`).

## Conventions

- **Tests are the parity oracle.** Wire-format and `--json` changes need a test. The push payload is
  golden-file tested for byte-equality (`testdata/golden/`); regenerate goldens only with care.
- Wrap errors with `%w`; use a typed `ExitError` for process exit codes (auth failure → 2).
- Keep the dependency set small and justify additions.
- Never commit secrets — `config.json`, `.token.json`, and `.env` are gitignored. See `SECURITY.md`.
- `main` is protected; land changes via pull request.
