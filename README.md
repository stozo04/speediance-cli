# Speediance CLI

Pull your **completed** Speediance Gym Monster workouts and auto-write them into your
weekly training sheet (`WEEKS/Week-XX.md`) — weights filled in, the day checked off,
and a full per-set log dropped into the notes.

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

`config.json` (gitignored — your password never leaves this machine):

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

# preview today's sync without changing anything
python -m speediance sync --dry-run

# sync today's session into the current week sheet
python -m speediance sync

# sync a specific day
python -m speediance sync --date 2026-06-15
```

Run `sync` **after** you finish and save the workout in the Speediance app.

## How matching works

Each exercise from Speediance is fuzzy-matched to a row in your week sheet (names don't
have to be identical — "Chest Press" matches "Cable chest press (Gym Monster)"). Matched
rows get their weights filled and box checked. **Anything that doesn't match is still
captured** in full in the notes block, so no data is ever lost.

## Note on weight units

The weight number comes straight from the API and is labeled with your `unit` setting.
On your first sync, eyeball one exercise against what the Speediance app shows and confirm
the unit looks right; tell the coach if it's off and we'll adjust.

## Files

- `speediance/client.py` — API auth + endpoints
- `speediance/sheet.py`  — writes sessions into Week-XX.md (the matching logic)
- `speediance/cli.py`    — the `login` / `workouts` / `sync` commands
- `tests/test_sheet.py`  — offline test of the sheet writer

## License

MIT — see `LICENSE`.
