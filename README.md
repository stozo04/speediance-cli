# speediance-cli

A tiny, **agent-friendly** CLI for the Speediance (Gym Monster) cloud API. Read your
completed workouts and **create programs that show up on the machine** - no photos to
reference mid-session. Every command speaks `--json`, and the tool owns no data layout:
it returns structured data and creates programs; *you* (or your agent) decide what to do
with it.

> Point an agent at this repo. See **[AGENTS.md](AGENTS.md)** for the full self-serve guide
> (setup, credentials, command surface, plan schema).

> Unofficial - uses the Speediance cloud API reverse-engineered from the Android app.
> Personal use, your own account. Built on the MIT-licensed
> `UnofficialSpeedianceWorkoutManager` (hbui3) and `speediance-influx` (gavinmcfall).

## Quickstart

```bash
git clone https://github.com/stozo04/speediance-cli && cd speediance-cli
pip install -r requirements.txt
cp config.example.json config.json     # add your email + password (gitignored)
python -m speediance login
```

Credentials can also come from env vars (`SPEEDIANCE_EMAIL`, `SPEEDIANCE_PASSWORD`,
`SPEEDIANCE_REGION`) - see `.env.example`. SSO/Google accounts: set a password in the
Speediance app once.

## Commands

```bash
python -m speediance workouts --days 7 --json      # recent sessions
python -m speediance session <training_id> --json  # full per-set detail
python -m speediance library --search "row"        # exercise catalog (ids/names/muscles)
python -m speediance push plan.json --dry-run      # build a program (preview)
python -m speediance push plan.json                # create it on your account
```

`sync` is an **optional** extra that writes a session into Markdown `WEEKS/Week-XX.md`
checklist files (the pattern this repo's author uses). It needs a path and nothing else
does:

```bash
python -m speediance sync --weeks-dir /path/to/WEEKS
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
`speediance library`. Full schema and field notes in [AGENTS.md](AGENTS.md).

## Notes

- "Free Lift" (freestyle) sessions return totals only - no per-set detail. Programs do.
- `config.json` / `.token.json` / `.env` are gitignored; never commit secrets.
- `main` is PR-protected; changes land via pull request.

## Files

- `speediance/client.py`    - API auth + endpoints
- `speediance/templates.py` - exercise library + create programs from a plan
- `speediance/sheet.py`     - (optional) write sessions into Markdown week sheets
- `speediance/cli.py`       - `login` / `workouts` / `session` / `library` / `push` / `sync`
- `plans/`                  - example plan JSON
- `tests/`                  - offline tests (no network)

## License

MIT - see `LICENSE`.
