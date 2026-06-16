---
name: speediance
description: Read completed workouts and push custom training programs to your Speediance (Gym Monster) smart cable machine via its cloud API.
metadata:
  openclaw:
    emoji: 🏋️
    homepage: https://github.com/stozo04/speediance-cli
    primaryEnv: SPEEDIANCE_EMAIL
    requires:
      bins:
        - python3
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

Install the CLI:

```bash
pip install git+https://github.com/stozo04/speediance-cli
speediance login   # authenticates and caches a token
```

Or clone and run as a module (no install needed):

```bash
git clone https://github.com/stozo04/speediance-cli && cd speediance-cli
pip install -r requirements.txt
python -m speediance login
```

Once installed, `speediance` and `python -m speediance` are interchangeable.

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

## Commands

### Read workouts

```bash
speediance workouts --days 7 --json      # recent sessions (summaries)
speediance session <training_id> --json  # full per-set detail for one session
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
speediance library --search "chest" --json   # filter by name or muscle
speediance library                           # save full catalog to library.json
```

Returns `[{id, name, muscle, tab}]`. The `id` is required for plan JSON.
A committed `library.json` snapshot ships with the repo (Gym Monster v1) for offline
browsing — regenerate with `speediance library` to get the freshest catalog or a
different device's exercises.

### Create a training program

Author a plan JSON, then push it — the program appears on the machine immediately:

```bash
speediance push plan.json --dry-run   # preview payload, no network write
speediance push plan.json             # create it on the account
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
| `id` | int | From `speediance library` — IDs differ per account/device |
| `weight` | float | **Kilograms** |
| `mode` | int | 1=Standard, 2=Eccentric, 3=Isokinetic, 4=Constant, 5=Spotter |
| `rest` | int | Seconds between sets |

### Optional: Markdown week-sheet sync

If you keep workout logs as `WEEKS/Week-XX.md` Markdown checklists, `sync` writes a
completed session into the matching file automatically:

```bash
speediance sync --weeks-dir /path/to/WEEKS --date today
speediance sync --weeks-dir /path/to/WEEKS --date 2025-06-10
```

This is entirely opt-in — ignore it if you don't use that convention. Core commands
(`login`, `workouts`, `session`, `library`, `push`) never need a sheets folder.

## Full command reference

| Command | What it does | `--json` |
|---|---|---|
| `login` | Authenticate and cache a token in `.token.json` | — |
| `workouts [--days N]` | List recent completed sessions | ✓ |
| `session <training_id>` | Full per-set detail for one session | ✓ |
| `library [--search X] [--out FILE]` | Dump or search exercise catalog | ✓ |
| `push <plan.json> [--dry-run]` | Create a training program on the account | ✓ |
| `sync [--date DATE] [--weeks-dir DIR]` | (Optional) Write session into Markdown sheet | — |

## Conventions

- **stdout is parseable** with `--json`; all human-readable hints go to **stderr**.
- **Secrets**: `config.json`, `.token.json`, `.env` are gitignored — never commit them.
- **Token caching**: after the first login, the token is cached in `.token.json` and
  refreshed automatically on expiry.
- **Dry-run first**: always use `--dry-run` before `push` when authoring new programs to
  confirm exercise IDs resolved correctly.
- If an endpoint breaks after a Speediance app update, `speediance/client.py` is where
  all API calls live.
