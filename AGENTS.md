# AGENTS.md - using speediance-cli from an agent

This repo is a small Python CLI for the Speediance (Gym Monster) cloud API. It's
built to be driven by an agent (OpenClaw, Claude, etc.): every command has a
`--json` mode, and the CLI **does not own any user data layout** - it returns
structured data and creates programs; the *caller* decides what to do with it
(write to a sheet, a database, a notebook, wherever).

## 1. Setup (do this once)

```bash
git clone https://github.com/stozo04/speediance-cli
cd speediance-cli
pip install -r requirements.txt
```

## 2. Credentials (find them, don't hardcode them)

The CLI needs the user's Speediance **email + password**. Resolve them in this order:

1. **Environment variables** (preferred): `SPEEDIANCE_EMAIL`, `SPEEDIANCE_PASSWORD`,
   optional `SPEEDIANCE_REGION` (`Global` default, or `EU`).
2. **`config.json`** in the repo root - copy `config.example.json` to `config.json`
   and fill it in. This file is gitignored; never commit it.
3. If neither is set, **ask the user** (or read from their secret store / password
   manager). Do not invent or guess credentials.

Google/SSO accounts: the user must set a password in the Speediance app once
(`verifyIdentity` reports `hasPwd:false` otherwise). Email stays their Google email.

Verify it works:

```bash
python -m speediance login        # caches a token in .token.json (gitignored)
```

## 3. Read workouts

```bash
# recent completed sessions (summaries)
python -m speediance workouts --days 7 --json

# full per-set detail for one session (reps, weight, HR per set)
python -m speediance session <training_id> --json
```

Note: freestyle **"Free Lift"** sessions return only totals - no per-set detail.
Sessions run from a **program** (see below) return everything.

## 4. Create a workout (so it appears on the machine)

```bash
# 1) cache the user's exercise catalog (ids differ per device/account)
python -m speediance library            # writes library.json: {id, name, muscle, tab}
python -m speediance library --search "row" --json

# 2) write a plan JSON (you, the agent, author this), then:
python -m speediance push plan.json --dry-run   # preview payload
python -m speediance push plan.json             # create it on the account
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
python -m speediance sync --weeks-dir /path/to/WEEKS --date today
```

If you don't use that sheet convention, ignore `sync` and consume `session --json`.

## Conventions

- **stdout is parseable** with `--json`; human hints go to stderr.
- **Secrets**: `config.json`, `.token.json`, `.env` are gitignored. Never commit them.
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
