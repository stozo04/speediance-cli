# AGENTS.md - using speediance-cli from an agent

This repo is a small Python CLI for the Speediance (Gym Monster) cloud API. It's
built to be driven by an agent (OpenClaw, Claude, etc.): every command has a
`--json` mode, and the CLI **does not own any user data layout** - it returns
structured data and creates programs; the *caller* decides what to do with it
(write to a sheet, a database, a notebook, wherever).

> **Device note:** built and tested for the **Gym Monster (v1)** (`device_type = 1`).
> A **Gym Monster 2** exists and may use a different device type and exercise ids -
> UNTESTED. Override via `SPEEDIANCE_DEVICE_TYPE` or `device_type` in config.json.

## 1. Setup (do this once)

**Option A — pip install** (gives you the `speediance-cli` command):

```bash
pip install git+https://github.com/stozo04/speediance-cli
```

**Option B — clone** (run as a module instead):

```bash
git clone https://github.com/stozo04/speediance-cli
cd speediance-cli
pip install -r requirements.txt
```

`speediance-cli <command>` (installed) and `python -m speediance <command>` (cloned) are
interchangeable throughout this guide.

## 2. Credentials (find them, don't hardcode them)

The CLI needs the user's Speediance **email + password**. Resolve them in this order:

1. **Environment variables** (preferred): `SPEEDIANCE_EMAIL`, `SPEEDIANCE_PASSWORD`,
   optional `SPEEDIANCE_REGION` (`Global` default, or `EU`) and
   `SPEEDIANCE_DEVICE_TYPE` (`1` = Gym Monster v1, the only tested device).
2. **`config.json`** in the repo root - copy `config.example.json` to `config.json`
   and fill it in. This file is gitignored; never commit it.
3. If neither is set, **ask the user** (or read from their secret store / password
   manager). Do not invent or guess credentials.

Google/SSO accounts: the user must set a password in the Speediance app once
(`verifyIdentity` reports `hasPwd:false` otherwise). Email stays their Google email.

Verify it works:

```bash
speediance-cli login        # caches a token in .token.json (gitignored)
# or: python -m speediance login
```

## 3. Read workouts

```bash
# recent completed sessions (summaries)
speediance-cli workouts --days 7 --json

# full per-set detail for one session (reps, weight, HR per set)
speediance-cli session <training_id> --json
```

Note: freestyle **"Free Lift"** sessions return only totals - no per-set detail.
Sessions run from a **program** (see below) return everything.

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
speediance-cli push plan.json --dry-run   # preview payload
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

- `id` - from `library.json`
- `weight` - **kilograms** (stored internally as `kg x 2.2`; confirm the displayed
  unit on the machine on first use and adjust if needed)
- `mode` - 1 Standard, 2 Eccentric, 3 Isokinetic, 4 Constant, 5 Spotter
- `rest` - seconds

## 5. Optional: Markdown sheet sync

`sync` is one specific integration (writing a session into `WEEKS/Week-XX.md`
checklist files). It is **opt-in** and requires a path - core commands never do:

```bash
speediance-cli sync --weeks-dir /path/to/WEEKS --date today
```

If you don't use that sheet convention, ignore `sync` and consume `session --json`.

## Conventions

- **stdout is parseable** with `--json`; human hints go to stderr.
- **Secrets**: `config.json`, `.token.json`, `.env` are gitignored. Never commit them.
- **Device**: tested for Gym Monster 1 only; GM2 untested.
- **Unofficial API**: endpoints live in `speediance/client.py`; if the Speediance app
  updates and something breaks, that's where to patch.
- Branch `main` is PR-protected - changes land via pull request.

## Command surface

| Command | Purpose | `--json` |
|---|---|---|
| `login` | authenticate, cache token | - |
| `workouts --days N` | list recent sessions | yes |
| `session <id>` | per-set detail for one session | yes |
| `library` | dump exercise catalog to `library.json` | yes |
| `push <plan.json>` | create a program (`--dry-run` to preview) | yes |
| `sync` | (optional) write a session into a Markdown sheet | - |
