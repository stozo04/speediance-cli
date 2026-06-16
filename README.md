# Speediance CLI

Pull your **completed** Speediance Gym Monster workouts and auto-write them into your
weekly training sheet (`WEEKS/Week-XX.md`), **and** build workout programs and push them
to your machine so there's nothing to reference mid-session.

> Unofficial. Uses the Speediance cloud API (reverse-engineered from the Android app).
> Personal use with your own account. A future Speediance update could change the API;
> if a command breaks, the endpoints in `speediance/client.py` are where to look.
> Built on the MIT-licensed `UnofficialSpeedianceWorkoutManager` (hbui3) and
> `speediance-influx` (gavinmcfall).

## Setup

```bash
pip install -r requirements.txt
cp config.example.json config.json   # then edit it with your login
```

`config.json` (gitignored - your password never leaves this machine):

| key | meaning |
|-----|---------|
| `email` / `password` | your Speediance account login |
| `region` | `Global` (Americas/APAC) or `EU` |
| `unit` | label used in the sheet, `lb` or `kg` |
| `weeks_dir` | folder holding your `Week-XX.md` sheets |

You can also pass secrets as env vars instead of the file:
`SPEEDIANCE_EMAIL`, `SPEEDIANCE_PASSWORD`, `SPEEDIANCE_REGION`, `SPEEDIANCE_WEEKS_DIR`.

## Use

```bash
# confirm login works (caches a token)
python -m speediance login

# list recent completed sessions
python -m speediance workouts --days 3

# sync a completed session into the current week sheet
python -m speediance sync --dry-run
python -m speediance sync
python -m speediance sync --date 2026-06-15
```

Run `sync` **after** you finish and save the workout in the Speediance app.

## Building programs (so there's nothing to reference mid-workout)

Freestyle "Free Lift" sessions record only totals (time/calories/volume) - no per-exercise
detail. Sessions run from a **custom program** record everything. So author the workout as
a program, push it to your account, do it, then `sync` pulls full per-set results back.

```bash
# 1) cache YOUR exercise catalog (ids differ per device) and search it
python -m speediance library
python -m speediance library --search "chest press"

# 2) preview the payload a plan would create (no network write)
python -m speediance push plans/example-push.json --dry-run

# 3) create the program on your account, then open the app to run it
python -m speediance push plans/my-week-push.json
```

### Plan JSON

A *plan* is just JSON - the planner that writes it can be a human, a coach, or an LLM:

```json
{
  "name": "Week 1 - Push",
  "exercises": [
    {"id": 304, "title": "Standing Dual-Handle Chest Press",
     "sets": [{"reps": 15, "weight": 18, "mode": 1, "rest": 75}]}
  ]
}
```

- `id` - exercise id from `speediance library`
- `weight` - **kilograms** (stored as kg x 2.2 internally; verify on the machine on your first push and adjust if your display unit differs)
- `mode` - 1 Standard, 2 Eccentric, 3 Isokinetic, 4 Constant, 5 Spotter
- `rest` - seconds

## How sync matching works

Each exercise from a completed session is fuzzy-matched to a row in your week sheet (names
don't have to be identical). Matched rows get weights filled and the box checked. Anything
that doesn't match is still captured in full in the notes block, so no data is lost.

## Files

- `speediance/client.py`    - API auth + endpoints
- `speediance/templates.py` - fetch library; build & create programs from a plan JSON
- `speediance/sheet.py`     - writes completed sessions into Week-XX.md
- `speediance/cli.py`       - `login` / `workouts` / `sync` / `library` / `push`
- `plans/`                  - workout plan JSON files
- `tests/`                  - offline tests (no network)

## License

MIT - see `LICENSE`.
