# Speediance project instructions

Developer-facing brief for working **on** `speediance-cli` (a single-binary Go
CLI for the unofficial Speediance / Gym Monster cloud API). For *using* the CLI
from an agent see `docs/MACHINE_CONTRACT.md`; for the design contract see `GOAL.md`.

## MANDATORY — ClawHub security standards

Before changing anything that touches **credential handling, configuration
resolution, environment/`.env` loading, file writes, network calls, logging
output, or `SKILL.md`**, and before **publishing the skill to ClawHub**, you MUST
read and follow `.claude/CLAWHUB_STANDARDS.md`.

Read the local standards in full; their rules and pre-publish checklist are binding.
Pin every new security behavior with an immutable regression test as described there.

Read [.claude/CLAWHUB_STANDARDS.md](../.claude/CLAWHUB_STANDARDS.md).

## MANDATORY — shared CLI conventions

`speediance-cli` shares its config/auth/credential layer design with its sibling
`google-health-cli`. The cross-repo invariants for that layer (per-user/non-roaming
secret locations, `0600`/`0700` perms, advertised==actual, conservative migration,
`.env` no-inject, …) live in `.claude/CLI_CONVENTIONS.md`, committed **byte-identical**
in both repos. Changes go through the shared agent process so both copies stay in sync —
do not edit one repo's copy unilaterally. Read the local copy in full in every harness.

Read [.claude/CLI_CONVENTIONS.md](../.claude/CLI_CONVENTIONS.md).

## Build & verify

- `go build ./... && go vet ./... && go test ./...` must pass; `gofmt -l` must be clean.
- `main` is PR-protected — land changes via pull request.

## Local reference data — `REAL-SPEEDIANCE-JSON/`

If present on the working machine, `REAL-SPEEDIANCE-JSON/` holds **real captured
Speediance session JSON** from a live account — the ground-truth shapes of actual
CLI output / API responses per session `kind`/`type` (program, freestyle Free Lift,
guided rowing, the `today` array, …). See its `README.md` for an index.

- It is **gitignored on purpose** — personal workout data in a public repo. **Never
  commit it, never move it back into a tracked path, and never paste its contents
  into a PR, issue, commit, or published doc.** It is local reference only.
- **Prefer it over fabricating samples.** When changing session/dispatch code or
  reasoning about the `--json` contract, read a real file here instead of inventing a
  shape. Save any new live captures into this folder.
- It may be **absent** (a fresh clone / CI / another machine) — that's expected; the
  test fixtures (e.g. the genuine `940759` data embedded in tests) are the committed,
  CI-visible source of truth. This folder is a convenience, never a dependency.

## PR gate — guard tests are mandatory and immutable

A PR may not be opened or merged unless `go build ./... && go vet ./... && go test -race ./...`
pass and `gofmt -l` is clean. This is **enforced, not advisory**: CI
(`.github/workflows/ci.yml`) runs build + `go test -race` + lint on every `pull_request`.

The **negative-assertion guard tests** — every test named in the SPD cells of
`.claude/CLI_CONVENTIONS.md` (§0, §1, §3, §5, §7, §9) **plus**
`internal/cli`'s `TestEndToEndMigratesLegacyTokenToCacheDir` — are **immutable**: each
asserts that a known bad thing does **not** happen (a secret in CWD, a token in the
roaming base, a `.env` mutating the process env, an advertised-but-unwired key, …). They
must never be skipped (`t.Skip`), deleted, or weakened to turn a PR green — a red guard
means **fix the code, not the test**. Any new credential / config / permission / network
behavior ships with its guard in the **same** PR (see `.claude/CLAWHUB_STANDARDS.md`).

## Commits & releases

Commit subjects follow [Conventional Commits](https://www.conventionalcommits.org)
(`feat:`, `fix:`, `docs:`, `test:`, `chore:`) — this is **load-bearing, not cosmetic**.
Releases are cut by pushing a `vX.Y.Z` git tag (never by merging), and GoReleaser
auto-builds the GitHub Release notes by **grouping commit subjects** (`feat:` → Features,
`fix:` → Bug fixes, else → Other changes; `docs:`/`test:`/`chore:` excluded) plus a static
install footer — there is **no `CHANGELOG.md`**. Use the right prefix so the changelog groups
cleanly, and squash-merge PRs with a clean Conventional-Commit title. Full release playbook
(versioning, tagging, dry-runs): `RELEASING.md`.

## Scope — don't add a `doctor`/health command

The diagnostic surface is **intentionally** spread across existing commands, not bundled
into a `doctor`: `version` (install/build), `config show` + `config path` (resolved config
and file locations), and `login` (auth + connectivity; exit `2` on failure). With a single
external dependency (the Speediance API) and an agent-first consumer that prefers `--json`
+ exit codes over a human-readable health report, a `doctor` aggregator is speculative
surface that cuts against the minimal-command philosophy (GOAL.md). Revisit only if
human-user support load makes a read-only aggregator clearly worth its weight.

## Shared harness maintenance

Run `make check-agents` for instruction and skill changes (Python 3 is a developer
check dependency only; the shipped CLI remains a single binary). Root `SKILL.md`
is the published artifact; mirror its complete package into each harness after
editing it. Reconcile all copies before choosing a repair source. Run
`python scripts/sync-harness-skills.py --fix --from claude`, or select `cursor`/`codex`,
then `make check-agents`. Preserve native settings and publishing triggers.

Read `CONTRIBUTING.md`, `SECURITY.md`, and `RELEASING.md` for their applicable gates.
`GOAL.md` retains historical rewrite decisions; its removed sync/sheet surface is
not a request to restore those features. Paths in this document are repository-relative.
