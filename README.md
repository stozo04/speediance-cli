# speediance-cli

[![CI](https://github.com/stozo04/speediance-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/stozo04/speediance-cli/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/stozo04/speediance-cli.svg)](https://pkg.go.dev/github.com/stozo04/speediance-cli)
[![Go Report Card](https://goreportcard.com/badge/github.com/stozo04/speediance-cli)](https://goreportcard.com/report/github.com/stozo04/speediance-cli)
[![Release](https://img.shields.io/github/v/release/stozo04/speediance-cli?sort=semver)](https://github.com/stozo04/speediance-cli/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A tiny, **agent-friendly** CLI for the Speediance (Gym Monster) cloud API. Read your
completed workouts and **create programs that show up on the machine** — no photos to
reference mid-session. Every command speaks `--json`, and the tool owns no data layout:
it returns structured data and creates programs; *you* (or your agent) decide what to do
with it.

A single, statically-compiled **Go** binary — no Python, no runtime, nothing to install
alongside it. CLI-compatible with the original Python `speediance-cli` (same commands,
same `--json`).

> ⚠️ **Device support:** built and tested for the **Gym Monster (v1)** (`device_type = 1`).
> A **Gym Monster 2** now exists and may use a different device type and exercise ids —
> it is currently **UNTESTED**. Set `SPEEDIANCE_DEVICE_TYPE` (or `device_type` in
> config.json) if you want to try another device.

> Point an agent at this repo. See **[AGENTS.md](AGENTS.md)** for the full self-serve guide
> (setup, credentials, command surface, plan schema).

> Unofficial — uses the Speediance cloud API reverse-engineered from the Android app.
> Personal use, your own account. Built on the MIT-licensed
> `UnofficialSpeedianceWorkoutManager` (hbui3) and `speediance-influx` (gavinmcfall).

## Install

**Pre-built binary** (recommended): download the archive for your OS/arch from the
[Releases](https://github.com/stozo04/speediance-cli/releases) page, extract, and put
`speediance-cli` on your `PATH`.

**With Go** (1.24+):

```bash
go install github.com/stozo04/speediance-cli/cmd/speediance-cli@latest
speediance-cli login
```

`go install` drops the binary in `$(go env GOPATH)/bin` — make sure that's on your `PATH`.

Credentials via env vars (`SPEEDIANCE_EMAIL`, `SPEEDIANCE_PASSWORD`, `SPEEDIANCE_REGION`),
a gitignored `config.json`, or a gitignored `.env` file in the working directory (auto-loaded;
real exported env vars take precedence over it) — see `.env.example` / `config.example.json` and
[AGENTS.md](AGENTS.md). SSO/Google accounts: set a password in the Speediance app once.

## Commands

```bash
speediance-cli today --json                  # every session today, auto-resolved (program/free/rowing)
speediance-cli workouts --days 7 --json      # recent sessions (digest, each with a kind)
speediance-cli session <training_id> --json  # full, verbatim detail for one session (auto-detects type)
speediance-cli library --search "row"        # exercise catalog (ids/names/muscles)
speediance-cli push plan.json --dry-run      # build a program (preview)
speediance-cli push plan.json                # create it on your account
```

The reader commands emit JSON and own no data layout — pipe `workouts`/`session --json`
into whatever stores or derives from it (a sheet, a database, an agent). Storing is the
consumer's job, not the CLI's.

Convenience commands: `config show|set|path` (manage `config.json` without hand-editing),
`version` (build metadata; also `--version`), and `completion bash|zsh|fish|powershell`.

## Create a workout

Author a plan (a human, a coach, or an LLM can write it), then `push` it:

```json
{
  "name": "Pull Day",
  "exercises": [
    {"id": 434, "title": "Seated Dual-Handle Lat Pulldown",
     "sets": [{"reps": 12, "weight": 20, "mode": 1, "rest": 75}]}
  ]
}
```

`weight` is kilograms; `mode` 1=Standard; `rest` in seconds. Get `id`s from
`speediance-cli library`. Full schema and field notes in [AGENTS.md](AGENTS.md).

## Conventions

- **stdout is parseable** with `--json`; all human hints, warnings, and logs go to **stderr**.
  Never interleaved — pipe stdout straight into a parser.
- **Exit codes:** `0` success, `2` auth failure, non-zero for other errors.
- **Secrets:** `config.json`, `.token.json`, `.env`, and `plans/` are gitignored. Never
  commit them. Prefer env vars for agents/headless use.

## Troubleshooting

There's intentionally **no `doctor` / health-check command** — with a single dependency
(the Speediance API), the diagnostics you'd want are already split across commands you
have. If setup isn't working, walk these in order:

```bash
speediance-cli version          # is the binary installed, and which build?
speediance-cli config show      # are email/region/device_type what you expect? (password masked)
speediance-cli config path      # where did config.json and the token cache resolve?
speediance-cli login            # does auth + connectivity actually work?  (exit code 2 = auth failure)
```

`login` is the real connectivity test: it forces a fresh authentication against the API
and exits `2` if the credentials are wrong (note it also rewrites the token cache). Together
these answer *what's installed, what config resolved, and does the account connect* —
everything a `doctor` command would have bundled into one report.

## ClawHub skill

This tool is published as a public skill on [ClawHub](https://clawhub.ai/stozo04/speediance)
so any OpenClaw or compatible agent can install it. The skill definition lives in
[SKILL.md](SKILL.md). ClawHub is updated automatically on every change to `SKILL.md` via
GitHub Actions — no manual publish step needed.

## Notes

- Built/tested for **Gym Monster 1** only (see device note above). GM2 is untested.
- The session **token cache** lives in your OS user-cache dir by default (e.g.
  `~/.cache/speediance/token.json`; `%LocalAppData%\speediance\token.json` on Windows),
  **not** the working directory — so it can't be swept into a commit — and in the
  **non-roaming** cache dir rather than the roaming config dir, so a live credential
  isn't synced across machines. Override with `SPEEDIANCE_TOKEN_CACHE` or the
  `token_cache_path` config key; `config path` shows where it resolved. An older
  `.token.json` in the working directory is migrated automatically on first run.
- "Free Lift" (freestyle) sessions return totals only — no per-set detail. Programs do.
- `library.json` is a committed **snapshot** of the exercise catalog for convenience;
  regenerate it anytime with `speediance-cli library`.
- `main` is PR-protected; changes land via pull request.

## Project layout

A thin `cmd/` entrypoint over a closed `internal/` tree (the `gh` pattern):

- `cmd/speediance-cli/`  — entrypoint (`main`); maps errors to exit codes
- `internal/api/`        — HTTP client: frozen headers, auth, token refresh, endpoints
- `internal/config/`     — config + credential discovery (env / file / flags)
- `internal/auth/`       — token cache (0600), per-user OS config dir by default
- `internal/template/`   — exercise library + build/create programs from a plan
- `internal/workout/`    — workout/session models, record parsing, timestamp handling
- `internal/cli/`        — Cobra command wiring (one file per command)
- `SKILL.md`             — ClawHub marketplace skill definition
- `library.json`         — committed catalog snapshot (Gym Monster 1)
- `plans/`               — personal workout plan JSONs (gitignored)

## Development

```bash
go build ./...
go test -race ./...
golangci-lint run ./...
```

## License

MIT — see `LICENSE`.
