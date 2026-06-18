---
name: speediance
description: >
  Read completed workouts (summaries and full per-set detail), browse and export the
  exercise catalog, and push custom training programs to your Speediance (Gym Monster)
  smart cable machine via its cloud API. Authenticates with your account credentials,
  caches a session token in your OS user-config directory (override with SPEEDIANCE_TOKEN_CACHE),
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
        - "token cache — cached session token in your OS user-config dir by default; path overridable via SPEEDIANCE_TOKEN_CACHE or the token_cache_path config key. A legacy .token.json in the working directory is read once to migrate it."
        - "plan JSON files passed to the push command"
      files.write:
        - "token cache — session token written after login and refreshed automatically; in your OS user-config dir by default (override via SPEEDIANCE_TOKEN_CACHE or token_cache_path). A legacy .token.json in the working directory is relocated here and then removed."
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

Install it one of two ways:

```bash
# A) Download a release binary for your OS/arch, extract, put it on your PATH:
#    https://github.com/stozo04/speediance-cli/releases

# B) Or build/install with Go (1.24+):
go install github.com/stozo04/speediance-cli/cmd/speediance-cli@latest
```

Then authenticate:

```bash
speediance-cli login   # authenticates and caches a session token (run `config path` to see where)
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
speediance-cli workouts --days 7 --json      # recent sessions (summaries)
speediance-cli session <training_id> --json  # full per-set detail for one session
```

Sample `workouts --json` output:

```json
[
  {
    "training_id": 123456,
    "title": "Upper Body",
    "date": "2025-06-15",
    "duration_secs": 2700,
    "calories": 320,
    "volume": 4200.0,
    "type": "Strength"
  }
]
```

Sample `session <id> --json` output:

```json
{
  "training_id": 123456,
  "completion_rate": 0.95,
  "exercises": [
    {
      "name": "Seated Dual-Handle Lat Pulldown",
      "sets": [
        {"set": 1, "reps": 12, "target_reps": 12, "weight": 20.0, "max_hr": 148, "left_right": 0}
      ]
    }
  ]
}
```

> "Free Lift" (freestyle) sessions return totals only — no per-set detail.
> Sessions started from a **program** return full set data.

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
| `session <training_id>` | Full per-set detail for one session | ✓ |
| `library [--search X] [--out FILE]` | Dump or search exercise catalog | ✓ |
| `push <plan.json> [--dry-run]` | Create a training program on the account | ✓ |
| `config show\|set\|path` | Manage `config.json` | ✓ (`show`) |
| `version` | Build metadata (also `--version`) | ✓ |
| `completion <shell>` | Shell completion (bash/zsh/fish/powershell) | — |

## Conventions

- **stdout is parseable** with `--json`; all human-readable hints go to **stderr**.
- **Exit codes**: `0` success, `2` authentication failure, non-zero for other errors.
- **Secrets**: `config.json`, `.token.json`, `.env` are gitignored — never commit them.
- **Token caching**: after the first login, the token is cached in your **OS user-config
  directory** (e.g. `%AppData%\speediance\token.json` on Windows, `~/.config/speediance/token.json`
  on Linux, `~/Library/Application Support/speediance/token.json` on macOS) and refreshed
  automatically on expiry — *not* in the working directory, so it can't be swept into a commit.
  Override the location with `SPEEDIANCE_TOKEN_CACHE` or the `token_cache_path` config key; run
  `speediance-cli config path` to see where it resolved. A token left in a legacy `.token.json`
  by an older version is moved to the per-user location on first run.
- **Dry-run first**: always use `--dry-run` before `push` when authoring new programs to
  confirm exercise IDs resolved correctly.
- If an endpoint breaks after a Speediance app update, all API calls live in
  `internal/api` — that's the single place to patch.
