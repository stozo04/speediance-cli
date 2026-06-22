---
name: speediance
description: >
  Read completed workouts (summaries and full per-set detail), browse and export the
  exercise catalog, and push custom training programs to your Speediance (Gym Monster)
  smart cable machine via its cloud API. Authenticates with your account credentials,
  caches a session token in your OS user-cache directory (override with SPEEDIANCE_TOKEN_CACHE),
  and makes outbound HTTPS requests to the Speediance cloud API. Reads and emits structured
  data — the caller decides where to store it. Ships as a single static binary — no Python or
  other runtime required.
metadata:
  openclaw:
    emoji: 🏋️
    homepage: https://github.com/stozo04/speediance-cli
    primaryEnv: SPEEDIANCE_EMAIL
    permissions:
      network:
        - "Speediance cloud API (HTTPS) — authentication, workout history, exercise catalog, program creation"
      files.read:
        - "config.json — optional credential/config file (working directory, or your OS user-config dir)"
        - "token cache — cached session token in your OS user-cache dir by default (non-roaming); path overridable via SPEEDIANCE_TOKEN_CACHE or the token_cache_path config key. A legacy .token.json in the working directory is read once to migrate it."
        - "plan JSON files passed to the push command"
      files.write:
        - "token cache — session token written after login and refreshed automatically; in your OS user-cache dir by default (non-roaming; override via SPEEDIANCE_TOKEN_CACHE or token_cache_path). A legacy .token.json in the working directory is relocated here and then removed."
        - "library.json — exercise catalog dump (library command)"
    requires:
      bins: []
      env:
        - SPEEDIANCE_EMAIL
        - SPEEDIANCE_PASSWORD
    envVars:
      - name: SPEEDIANCE_EMAIL
        description: Your Speediance / Gym Monster account email address
        required: true
      - name: SPEEDIANCE_PASSWORD
        description: "Your account password. Google/SSO users: set a password once in the Speediance app (Profile → Settings) before using this skill."
        required: true
      - name: SPEEDIANCE_REGION
        description: "API region — Global (default) or EU"
        required: false
      - name: SPEEDIANCE_DEVICE_TYPE
        description: "Device type integer — 1 = Gym Monster v1 (default, tested). Gym Monster 2 is untested; try 2 if exercises look wrong."
        required: false
      - name: SPEEDIANCE_CONFIG
        description: "Path to config.json (overrides discovery: working dir, then OS user-config dir)"
        required: false
      - name: SPEEDIANCE_TOKEN_CACHE
        description: "Override the token cache file location (default: OS user-cache dir)"
        required: false
---

# Speediance — Gym Monster CLI Skill

Talk to your **Speediance (Gym Monster)** smart cable machine from any agent. Read
completed workouts and push custom programs that appear on the machine ready to run —
no app navigation mid-session.

> **Unofficial** — reverse-engineered from the Android app. Personal use, your own
> account only. Built on the MIT-licensed `UnofficialSpeedianceWorkoutManager` (hbui3)
> and `speediance-influx` (gavinmcfall).
>
> Tested on **Gym Monster v1** (`SPEEDIANCE_DEVICE_TYPE=1`). GM2 is untested.

## Setup (one time)

`speediance-cli` is a single static binary — **no Python or other runtime needed**.

Do these steps **in order**. Step 2 must come before step 3: `login` authenticates
with the credentials you supply in step 2 and exits with a config error if neither
the environment, a `.env`, nor `config.json` provides an email *and* password.

**1. Install** — one of two ways:

```bash
# A) Download a release binary for your OS/arch, extract, put it on your PATH:
#    https://github.com/stozo04/speediance-cli/releases

# B) Or build/install with Go (1.24+):
go install github.com/stozo04/speediance-cli/cmd/speediance-cli@latest
```

**2. Provide your credentials** — pick whichever fits how you run the tool
(full reference in [Credentials](#credentials) below):

```bash
# A) Environment variables — best for CI / one-off shells:
export SPEEDIANCE_EMAIL="you@example.com"
export SPEEDIANCE_PASSWORD="your-password"

# B) Or a gitignored .env in the working directory — recommended for OpenClaw /
#    agent workspaces, since a headless agent can't answer an interactive prompt.
#    Put this in <workspace>/.env:
#      SPEEDIANCE_EMAIL=you@example.com
#      SPEEDIANCE_PASSWORD=your-password

# C) Or write them into config.json (created 0600, owner-only) without hand-editing:
speediance-cli config set email "you@example.com"
speediance-cli config set password "your-password"
```

> **Heads-up on option C:** a value passed on the command line is visible in your
> shell history and process list. For interactive setup prefer A or B; if you use
> C, clear the history entry afterward.

**3. Authenticate** — verifies the credentials and caches a session token:

```bash
speediance-cli login   # run `speediance-cli config path` to see where the token is cached
```

**4. Read your data:**

```bash
speediance-cli today --json
```

## Credentials

Set as environment variables — the CLI reads them automatically:

| Variable | Required | Default | Notes |
|---|---|---|---|
| `SPEEDIANCE_EMAIL` | ✓ | — | Account email |
| `SPEEDIANCE_PASSWORD` | ✓ | — | Account password |
| `SPEEDIANCE_REGION` | — | `Global` | `Global` or `EU` |
| `SPEEDIANCE_DEVICE_TYPE` | — | `1` | `1` = Gym Monster v1 |

Alternatively, write a `config.json` in the working directory (gitignored by the repo):

```json
{
  "email": "you@example.com",
  "password": "yourpassword",
  "region": "Global"
}
```

You can also put these variables in a gitignored **`.env`** file in the working directory — the
`SPEEDIANCE_*` keys are read from it automatically (exported environment variables still take
precedence). Only those keys are read; the file is parsed into a map and any other keys are
ignored, so a stray `.env` can never inject unrelated variables into the process environment.

## Commands

### Read workouts

```bash
speediance-cli today --json                  # every session today, fully resolved (any type)
speediance-cli today --date 2026-06-17 --json # …or a specific day (today | yesterday | YYYY-MM-DD)
speediance-cli workouts --days 7 --json      # recent sessions (summaries, for picking)
speediance-cli session <training_id> --json  # full, verbatim detail for one session
```

**`today` is the one-shot, agent-friendly entry point.** When the client just says
"I did a workout," call `today` — you do **not** need to know whether it was a
program, free weights, or a rowing/ski session. The tool finds the day's session(s)
and returns each one fully resolved, as an array of the same `{training_id, kind,
info, detail}` shape that `session` emits. (`kind` is `"program"`, `"free"`, or
`""`.)

Sample `workouts --json` output (a digest for picking; `kind` lets you filter):

```json
[
  {
    "training_id": 123456,
    "title": "Upper Body",
    "date": "2025-06-15",
    "duration_secs": 2700,
    "calories": 320,
    "volume": 4200.0,
    "type": "Strength",
    "kind": "program"
  }
]
```

`session <id> --json` is **autonomous and a faithful, complete passthrough**. Given
only an id it figures out what the session was — a program/Coach session, free
weights, or a rowing/ski free session — and emits the verbatim Speediance payloads
under a uniform shape. The field names, nesting, and values are exactly what
Speediance returned (`leftWatts`, `forceControlScore`, `weights`, `leftBreakTimes`,
`totalDistance`, …); the CLI does not rename, reshape, compute, or fill gaps.

`kind` tells you which namespace answered, so a type-agnostic consumer always reads
the same two fields:

| `kind` | `info` | `detail` |
|---|---|---|
| `"program"` | `cttTrainingInfo` payload (incl. `completionRate`) | `cttTrainingInfoDetail` — per-exercise, per-rep arrays |
| `"free"` | `freeTraining` payload (totals: `totalCapacity`, `totalEnergy`, `totalDistance` for rowing/ski; `name` for guided sessions) | `freeTrainingDetail` — `[]` for a freestyle Free Lift, **populated** for a guided session (e.g. *Aerobic Rowing* → per-interval `finishedReps` with `distance`/`pace`/`spm` + per-stroke traces) |
| `""` | `null` | `null` (no session found in either namespace) |

> `kind:"free"` is the *free namespace*, not "freestyle". It spans both a
> freestyle **Free Lift** (no `info.name`, `detail: []`, aggregates only) and a
> **guided** free-namespace session (has `info.name`, often a populated `detail`) —
> guided cardio like **Aerobic Rowing** carries the full per-interval breakdown.
> Distinguish via `info.name` + whether `detail` has rows; don't assume `free` ⇒ empty.

A program session:

```json
{
  "training_id": 940759,
  "kind": "program",
  "info": {
    "completionRate": 100.0
    // … the verbatim GET /app/trainingInfo/cttTrainingInfo/<id> data payload
  },
  "detail": [
    {
      "actionLibraryName": "Standing Dual-Handle Hammer Curl",
      "maxWeight": 15.0, "maxWeightCount": 5,
      "score": 16, "completionScore": 5, "forceControlScore": 4,
      "bilateralBalanceScore": 4, "amplitudeStableScore": 3, "actionRating": 3,
      "finishedReps": [
        {
          "finishedCount": 14, "targetCount": 14, "capacity": 330.0, "leftRight": 0,
          "trainingInfoDetail": {
            "weights":      [15,15,15,15,15,10,10,10,10,10,10,10,10,10],
            "leftWeights":  [15,15,15,15,15,10,10,10,10,10,10,10,10,10],
            "rightWeights": [15,15,15,15,15,10,10,10,10,10,10,10,10,10],
            "leftWatts":    [41.65,51.84, "…"],
            "rightWatts":   [26.28,55.05, "…"],
            "leftAmplitudes": [0.46,0.68, "…"]
          }
        }
      ]
    }
  ]
}
```

Notes for consumers:

- **Auto-detection is built in** — no caller knowledge of the session type is
  needed. `session`/`today` probe the program namespace, then the free namespace.
- **`weight` is never invented.** There is no synthesized per-set weight. For a
  program, the real per-rep weights are in `trainingInfoDetail.weights[]` (already
  per attachment, so a single-handle average is just their mean); a mid-set drop
  (e.g. `15×5 → 10×9`) is therefore visible. Average or summarize as you see fit.
- **Free-namespace detail varies.** A *freestyle* Free Lift records session-level
  totals only (`info` aggregates, `detail: []`). A *guided* free-namespace session
  does more: **Aerobic Rowing** fills `detail` with per-interval rows
  (`distance`/`pace`/`spm`/`time`) and per-stroke rope-length traces. Always read
  `detail` rather than assuming `kind:"free"` is empty.
- **Absence is preserved.** A field or array Speediance omits is omitted in the
  output too (e.g. a sparse capture with only `weights`); nothing is back-filled.
- **Values are unvalidated passthrough.** Speediance's fields aren't guaranteed
  internally consistent, so derive the metric you want from raw values rather than
  trusting a single field — e.g. a rowing split is `distance / time`, not the
  per-interval `pace` field (which is an instantaneous sample). The CLI never
  "corrects" a value; that interpretation is yours.
- **Empty shape.** `info` is `object | null`; `detail` is `array | null`. These are
  the verbatim endpoint payloads (never normalized), so treat **both `null` and
  `[]`** as "no rows" — e.g. `if not detail`. In practice `detail` is a populated
  array for `kind:"program"`, `[]` for `kind:"free"`, and `null` only for `kind:""`.
- **No flag unlocks data** — the endpoints return it, so the CLI returns it. There
  is no `--telemetry`.

> A `trainingId` can identify *different* sessions in the program vs. free
> namespaces. Auto-detection prefers the program match; pass `--program` or
> `--free` to `session` to force a namespace when an id is ambiguous.

### Browse the exercise catalog

```bash
speediance-cli library --search "chest" --json   # filter by name or muscle
speediance-cli library                           # save full catalog to library.json
```

Returns `[{id, name, muscle, tab}]`. The `id` is required for plan JSON.
A committed `library.json` snapshot ships with the repo (Gym Monster v1) for offline
browsing — regenerate with `speediance-cli library` to get the freshest catalog or a
different device's exercises.

### Create a training program

Author a plan JSON, then push it — the program appears on the machine immediately:

```bash
speediance-cli push plan.json --dry-run   # preview payload, no network write
speediance-cli push plan.json             # create it on the account
```

**Plan JSON format:**

```json
{
  "name": "Pull Day",
  "exercises": [
    {
      "id": 434,
      "title": "Seated Dual-Handle Lat Pulldown",
      "sets": [
        {"reps": 12, "weight": 20, "mode": 1, "rest": 75},
        {"reps": 10, "weight": 22, "mode": 1, "rest": 75},
        {"reps": 8,  "weight": 25, "mode": 1, "rest": 90}
      ]
    },
    {
      "id": 291,
      "title": "Seated Row",
      "sets": [
        {"reps": 12, "weight": 18, "mode": 1, "rest": 60}
      ]
    }
  ]
}
```

| Field | Type | Notes |
|---|---|---|
| `id` | int | From `speediance-cli library` — IDs differ per account/device |
| `weight` | float | **Kilograms** |
| `mode` | int | 1=Standard, 2=Eccentric, 3=Isokinetic, 4=Constant, 5=Spotter |
| `rest` | int | Seconds between sets |

### Storing what you read

The CLI owns no log format. To keep a record of a session, pull it with `workouts --json`
and `session <id> --json`, then write it wherever you keep data (a Markdown sheet, a
database, a notebook). The tool reads and emits structured data; the caller decides the
layout.

## Full command reference

| Command | What it does | `--json` |
|---|---|---|
| `login` | Authenticate and cache a session token | — |
| `workouts [--days N]` | List recent completed sessions | ✓ |
| `today [--date D]` | Every session on a day, auto-resolved to type-correct detail (the one-shot entry point) | ✓ |
| `session <training_id> [--free\|--program]` | Full, verbatim detail for one session; auto-detects program/free/rowing | ✓ |
| `library [--search X] [--out FILE]` | Dump or search exercise catalog | ✓ |
| `push <plan.json> [--dry-run]` | Create a training program on the account | ✓ |
| `config show\|set\|path` | Manage `config.json` | ✓ (`show`) |
| `version` | Build metadata (also `--version`) | ✓ |
| `completion <shell>` | Shell completion (bash/zsh/fish/powershell) | — |

## Conventions

- **stdout is parseable** with `--json`; all human-readable hints go to **stderr**.
- **Exit codes**: `0` success, `2` authentication failure, non-zero for other errors.
- **Secrets**: `config.json`, `.token.json`, `.env` are gitignored — never commit them.
- **Token caching**: after the first login, the token is cached in your **OS user-cache
  directory** (e.g. `%LocalAppData%\speediance\token.json` on Windows, `~/.cache/speediance/token.json`
  on Linux, `~/Library/Caches/speediance/token.json` on macOS) and refreshed automatically on
  expiry — *not* in the working directory (so it can't be swept into a commit) and *not* in the
  roaming config dir (so a live credential isn't synced across machines). Override the location with
  `SPEEDIANCE_TOKEN_CACHE` or the `token_cache_path` config key; run `speediance-cli config path` to
  see where it resolved. A token left in a legacy `.token.json` by an older version is moved to the
  per-user location on first run.
- **Dry-run first**: always use `--dry-run` before `push` when authoring new programs to
  confirm exercise IDs resolved correctly.
- If an endpoint breaks after a Speediance app update, all API calls live in
  `internal/api` — that's the single place to patch.
