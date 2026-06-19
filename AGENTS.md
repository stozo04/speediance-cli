# AGENTS.md — using speediance-cli from an agent

This repo is a single-binary **Go** CLI for the Speediance (Gym Monster) cloud API. It's
built to be driven by an agent (OpenClaw, Claude, etc.): every command has a `--json`
mode, and the CLI **does not own any user data layout** — it returns structured data and
creates programs; the *caller* decides what to do with it (write to a sheet, a database, a
notebook, wherever). No Python or other runtime is required.

> **Device note:** built and tested for the **Gym Monster (v1)** (`device_type = 1`).
> A **Gym Monster 2** exists and may use a different device type and exercise ids —
> UNTESTED. Override via `SPEEDIANCE_DEVICE_TYPE` or `device_type` in config.json.

## 1. Setup (do this once)

**Option A — download a release binary** (no toolchain needed): grab the archive for your
OS/arch from the [Releases](https://github.com/stozo04/speediance-cli/releases) page,
extract, and put `speediance-cli` on your `PATH`.

**Option B — install with Go** (1.24+):

```bash
go install github.com/stozo04/speediance-cli/cmd/speediance-cli@latest
```

This installs `speediance-cli` into `$(go env GOPATH)/bin`; ensure that's on your `PATH`.

## 2. Credentials (find them, don't hardcode them)

The CLI needs the user's Speediance **email + password**. Resolution order
(highest precedence first): **command flags → environment variables → `config.json` →
built-in defaults**.

1. **Environment variables** (preferred for agents): `SPEEDIANCE_EMAIL`,
   `SPEEDIANCE_PASSWORD`, optional `SPEEDIANCE_REGION` (`Global` default, or `EU`) and
   `SPEEDIANCE_DEVICE_TYPE` (`1` = Gym Monster v1, the only tested device). A gitignored
   **`.env`** file in the working directory is read automatically for these `SPEEDIANCE_*`
   keys (real exported variables take precedence over it), so they can live in `.env` instead
   of being exported. Only the `SPEEDIANCE_*` keys are read from `.env`; the file is parsed
   into a map and any other keys are ignored — nothing is injected into the process environment.
2. **`config.json`** in the working directory — copy `config.example.json` to
   `config.json` and fill it in. This file is gitignored; never commit it. (You can also
   point `--config PATH` or `SPEEDIANCE_CONFIG` at an explicit file.)
3. If neither is set, **ask the user** (or read from their secret store / password
   manager). Do not invent or guess credentials.

Google/SSO accounts: the user must set a password in the Speediance app once
(`verifyIdentity` reports `hasPwd:false` otherwise). Email stays their Google email.

Verify it works:

```bash
speediance-cli login        # caches a session token (0600) in your OS user-cache dir
```

You can inspect the resolved configuration any time with `speediance-cli config show`
(the password is masked) or `speediance-cli config path` (file locations).

## 3. Read workouts

```bash
# every session today, fully resolved — the one-shot entry point (no type knowledge needed)
speediance-cli today --json
speediance-cli today --date 2026-06-17 --json   # today | yesterday | YYYY-MM-DD

# recent completed sessions (a digest, for picking); each row carries `kind`
speediance-cli workouts --days 7 --json

# full, verbatim detail for one session; auto-detects program/free/rowing
speediance-cli session <training_id> --json
```

**The tool auto-detects the session type — the agent stays dumb.** When the client
says they did a workout, call `today`: it finds the day's session(s) and returns
each fully resolved, without you knowing whether it was a program, free weights, or
rowing. `session <id>` does the same for one id (probes the program namespace, then
free).

Output is a uniform **`{training_id, kind, info, detail}`** (`today` returns an
array of these). `kind` is `"program"`, `"free"`, or `""`:

- `kind:"program"` → `info` = `cttTrainingInfo` (incl. `completionRate`); `detail` =
  per-exercise, per-rep arrays.
- `kind:"free"` → the free *namespace* (not "freestyle"): `info` = `freeTraining`
  totals (`totalCapacity`, `totalEnergy`, `totalDistance` for rowing/ski; `name` for
  guided). `detail` is `[]` for a freestyle Free Lift, but **populated** for a
  guided session — e.g. *Aerobic Rowing* fills it with per-interval
  `finishedReps` (`distance`/`pace`/`spm`) + per-stroke traces. Read `detail`;
  don't assume `free` ⇒ empty. (`info.name` present ⇒ a guided session.)

`info`/`detail` are the **verbatim** Speediance payloads — original field names and
values (`leftWatts`, `forceControlScore`, `weights`, `leftBreakTimes`, …). The CLI
never renames, reshapes, computes, or fabricates; there is no synthesized per-set
weight and no `--telemetry` flag. Absence is preserved (omitted fields stay
omitted). `info` is `object | null` and `detail` is `array | null` (never
normalized) — treat both `null` and `[]` as "no rows". Values are **unvalidated**:
Speediance's fields aren't always self-consistent, so derive what you need from raw
values (e.g. a rowing split = `distance / time`, not the per-interval `pace` field,
which is instantaneous) rather than trusting one field. A `trainingId` can mean
different sessions across namespaces; auto-detect prefers program, and
`--free`/`--program` force a namespace when an id is ambiguous.

## 4. Create a workout (so it appears on the machine)

```bash
# 1) cache the user's exercise catalog (ids differ per device/account)
speediance-cli library            # writes library.json: {id, name, muscle, tab}
speediance-cli library --search "row" --json
```

A committed `library.json` snapshot ships with the repo (Gym Monster 1); regenerate it
with the command above for the freshest catalog or a different device.

```bash
# 2) write a plan JSON (you, the agent, author this), then:
speediance-cli push plan.json --dry-run   # preview payload, no network write
speediance-cli push plan.json             # create it on the account
```

### Plan JSON

```json
{
  "name": "Pull Day",
  "exercises": [
    {"id": 434, "title": "Seated Dual-Handle Lat Pulldown",
     "sets": [{"reps": 12, "weight": 20, "mode": 1, "rest": 75}]}
  ]
}
```

- `id` — from `library.json`
- `weight` — **kilograms** (stored internally as `kg × 2.2`; confirm the displayed
  unit on the machine on first use and adjust if needed)
- `mode` — 1 Standard, 2 Eccentric, 3 Isokinetic, 4 Constant, 5 Spotter
- `rest` — seconds

Always `--dry-run` first when authoring new programs to confirm exercise ids resolved.

## 5. Storing what you read (it's the caller's job)

The CLI does not own a log format. To keep a record of a session, **pull** it with
`workouts --days N --json` and `session <training_id> --json`, then write it wherever you
keep data (a Markdown sheet, a database, a notebook). The tool reads and emits; the caller
decides the layout. Note that freestyle **"Free Lift"** sessions return totals only — no
per-set detail to store.

## Conventions

- **stdout is parseable** with `--json`; human hints, warnings, and logs go to **stderr**.
  They are never interleaved, so piping stdout into a parser is safe.
- **Exit codes:** `0` success, `2` auth failure, non-zero for other errors. Check them.
- **No `doctor`/health command — by design.** To diagnose setup programmatically, read
  `config show --json` (what resolved, where) and run `login` (exit `2` = auth/connectivity
  failure; it rewrites the token cache). Don't go looking for a single health command — chain
  those instead.
- **Token cache:** the session token is cached in the OS user-**cache** dir by default
  (non-roaming; `config path` shows the resolved location), **not** the working directory
  (so running the CLI from another repo can't drop a credential into it) and **not** the
  roaming config dir (so a live token isn't synced across machines). Override with
  `SPEEDIANCE_TOKEN_CACHE` or the `token_cache_path` config key.
- **Secrets:** `config.json`, `.token.json`, `.env` are gitignored. Never commit them.
- **Device:** tested for Gym Monster 1 only; GM2 untested.
- **Unofficial API:** all endpoints live in `internal/api`; if the Speediance app updates
  and something breaks, that's the single place to patch.
- Branch `main` is PR-protected — changes land via pull request.

## Command surface

| Command | Purpose | `--json` |
|---|---|---|
| `login` | authenticate, cache token | — |
| `workouts --days N` | list recent sessions | yes |
| `today [--date D]` | every session on a day, auto-resolved to type-correct detail | yes |
| `session <id> [--free\|--program]` | full, verbatim detail for one session; auto-detects type | yes |
| `library` | dump exercise catalog to `library.json` | yes |
| `push <plan.json>` | create a program (`--dry-run` to preview) | yes |
| `config show\|set\|path` | manage `config.json` | yes (`show`) |
| `version` | build metadata (also `--version`) | yes |
| `completion <shell>` | shell completion script | — |
