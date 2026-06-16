# speediance-cli

A tiny, **agent-friendly** CLI for the Speediance (Gym Monster) cloud API. Read your
completed workouts and **create programs that show up on the machine** - no photos to
reference mid-session. Every command speaks `--json`, and the tool owns no data layout:
it returns structured data and creates programs; *you* (or your agent) decide what to do
with it.

> ⚠️ **Device support:** built and tested for the **Gym Monster (v1)** (`device_type = 1`).
> A **Gym Monster 2** now exists and may use a different device type and exercise ids -
> it is currently **UNTESTED**. Set `SPEEDIANCE_DEVICE_TYPE` (or `device_type` in
> config.json) if you want to try another device.

> Point an agent at this repo. See **[AGENTS.md](AGENTS.md)** for the full self-serve guide
> (setup, credentials, command surface, plan schema).

> Unofficial - uses the Speediance cloud API reverse-engineered from the Android app.
> Personal use, your own account. Built on the MIT-licensed
> `UnofficialSpeedianceWorkoutManager` (hbui3) and `speediance-influx` (gavinmcfall).

## Install

**Via pip** (installs the `speediance-cli` command globally):

```bash
pip install git+https://github.com/stozo04/speediance-cli
speediance-cli login
```

**Clone and run** (no install needed):

```bash
git clone https://github.com/stozo04/speediance-cli && cd speediance-cli
pip install -r requirements.txt
python -m speediance login
```

`speediance-cli` (installed) and `python -m speediance` (cloned) are interchangeable.

Credentials via env vars (`SPEEDIANCE_EMAIL`, `SPEEDIANCE_PASSWORD`, `SPEEDIANCE_REGION`)
or a gitignored `config.json` — see `.env.example` and [AGENTS.md](AGENTS.md).
SSO/Google accounts: set a password in the Speediance app once.

## Commands

```bash
speediance-cli workouts --days 7 --json      # recent sessions
speediance-cli session <training_id> --json  # full per-set detail
speediance-cli library --search "row"        # exercise catalog (ids/names/muscles)
speediance-cli push plan.json --dry-run      # build a program (preview)
speediance-cli push plan.json                # create it on your account
```

`sync` is an **optional** extra that writes a session into Markdown `WEEKS/Week-XX.md`
checklist files. It needs a path and nothing else does:

```bash
speediance-cli sync --weeks-dir /path/to/WEEKS
```

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

## ClawHub skill

This tool is published as a public skill on [ClawHub](https://clawhub.ai/stozo04/speediance)
so any OpenClaw or compatible agent can install it. The skill definition lives in
[SKILL.md](SKILL.md). ClawHub is updated automatically on every merge to `main` via
GitHub Actions — no manual publish step needed.

## Notes

- Built/tested for **Gym Monster 1** only (see device note above). GM2 is untested.
- "Free Lift" (freestyle) sessions return totals only - no per-set detail. Programs do.
- `library.json` is a committed **snapshot** of the exercise catalog for convenience;
  regenerate it anytime with `speediance-cli library`.
- `config.json` / `.token.json` / `.env` / `plans/` are gitignored; never commit secrets
  or personal workout plans.
- `main` is PR-protected; changes land via pull request.

## Files

- `speediance/client.py`    - API auth + endpoints
- `speediance/templates.py` - exercise library + create programs from a plan
- `speediance/sheet.py`     - (optional) write sessions into Markdown week sheets
- `speediance/cli.py`       - `login` / `workouts` / `session` / `library` / `push` / `sync`
- `SKILL.md`                - ClawHub marketplace skill definition
- `pyproject.toml`          - pip packaging (`pip install git+...`)
- `library.json`            - committed snapshot of the exercise catalog (Gym Monster 1)
- `plans/`                  - personal workout plan JSONs (gitignored)
- `tests/`                  - offline tests (no network)

## License

MIT - see `LICENSE`.
