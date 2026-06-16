"""Speediance CLI - read and create workouts via the Speediance cloud API.

stdout is kept machine-parseable with `--json`; human hints go to stderr.
Commands: login | workouts | session | library | push | sync
"""
import sys
import json
import argparse
import logging
from datetime import date, datetime, timedelta

from . import config as cfgmod
from .client import SpeedianceClient, AuthError
from . import sheet as sheetmod
from . import templates as tpl


def _err(*a):
    print(*a, file=sys.stderr)


def _client(cfg, use_cache=True):
    cache = cfgmod.load_token_cache() if use_cache else {}
    return SpeedianceClient(cfg["email"], cfg["password"], cfg["region"],
                            token=cache.get("token", ""), user_id=cache.get("user_id", ""))


def cmd_login(args):
    cfg = cfgmod.load_config()
    c = _client(cfg, use_cache=False)
    token, uid = c.login()
    cfgmod.save_token_cache(token, uid)
    print(f"Logged in (user {uid}). Token cached to {cfgmod.TOKEN_PATH} — keep this file private.")
    _err("Note: the token grants access to your Speediance account. Do not share or sync this file.")


def cmd_workouts(args):
    cfg = cfgmod.load_config()
    c = _client(cfg)
    workouts = c.fetch_workouts(days=args.days)
    cfgmod.save_token_cache(c.token, c.user_id)
    rows = [{
        "training_id": w.training_id, "title": w.title,
        "date": w.date.isoformat() if w.date else None,
        "duration_secs": w.duration_secs, "calories": w.calories,
        "volume": w.volume_kg, "type": w.workout_type,
    } for w in workouts]
    if args.json:
        print(json.dumps(rows, indent=2))
        return
    if not rows:
        print(f"No completed workouts in the last {args.days} day(s).")
        return
    print(f"Found {len(rows)} session(s) in the last {args.days} day(s):\n")
    for r in rows:
        print(f"  - {r['date']}  {r['title']}  -  {r['duration_secs']//60} min, "
              f"{r['calories']} kcal  (id {r['training_id']})")


def cmd_session(args):
    """Full per-set detail for one completed session (for the caller to consume)."""
    from .models import Workout
    cfg = cfgmod.load_config()
    c = _client(cfg)
    w = Workout(training_id=args.training_id, title="", workout_type="")
    c.fetch_detail(w)
    cfgmod.save_token_cache(c.token, c.user_id)
    out = {
        "training_id": w.training_id,
        "completion_rate": w.completion_rate,
        "exercises": [
            {"name": name, "sets": [
                {"set": s.set_index, "reps": s.finished_reps, "target_reps": s.target_reps,
                 "weight": s.max_weight, "max_hr": s.max_heart_rate, "left_right": s.left_right}
                for s in sets]}
            for name, sets in w.exercises().items()
        ],
    }
    if args.json:
        print(json.dumps(out, indent=2))
        return
    if not out["exercises"]:
        print(f"No per-set detail for training {args.training_id} "
              f"(freestyle 'Free Lift' sessions return none).")
        return
    for ex in out["exercises"]:
        sets = ", ".join(f"{s['weight']:g}x{s['reps']}" for s in ex["sets"])
        print(f"  {ex['name']}: {sets}")


def cmd_library(args):
    cfg = cfgmod.load_config()
    c = _client(cfg)
    lib = tpl.fetch_library(c, device_type=int(cfg.get("device_type", 1)))
    cfgmod.save_token_cache(c.token, c.user_id)
    with open(args.out, "w", encoding="utf-8") as f:
        json.dump(lib, f, ensure_ascii=False, indent=2)
    _err(f"Saved {len(lib)} exercises to {args.out}")
    hits = lib
    if args.search:
        q = args.search.lower()
        hits = [e for e in lib if q in e["name"].lower() or q in e.get("muscle", "").lower()]
    if args.json:
        print(json.dumps(hits, ensure_ascii=False, indent=2))
    elif args.search:
        print(f"{len(hits)} match '{args.search}':")
        for e in hits[:60]:
            print(f"  [{e['id']}] {e['name']} ({e.get('muscle','')})")


def cmd_push(args):
    cfg = cfgmod.load_config()
    plan = tpl.load_plan(args.plan)
    c = _client(cfg)
    n_ex = len(plan["exercises"])
    n_sets = sum(len(e.get("sets", [])) for e in plan["exercises"])
    _err(f"Plan: {plan['name']} - {n_ex} exercises, {n_sets} sets")
    if args.dry_run:
        payload = tpl.build_payload(c, plan["name"], plan["exercises"],
                                    device_type=int(cfg.get("device_type", 1)))
        cfgmod.save_token_cache(c.token, c.user_id)
        if args.json:
            print(json.dumps(payload, indent=2))
            return
        print(f"[dry-run] totalCapacity={payload['totalCapacity']:.0f}; not sent.")
        for a in payload["actionLibraryList"]:
            print(f"    groupId {a['groupId']}: reps {a['setsAndReps']} | weights {a['weights']}")
        return
    data = tpl.create_template(c, plan["name"], plan["exercises"],
                               device_type=int(cfg.get("device_type", 1)))
    cfgmod.save_token_cache(c.token, c.user_id)
    if args.json:
        print(json.dumps(data, indent=2))
    else:
        print(f"Created '{plan['name']}' on your Speediance account. Open the app to run it.")


def _resolve_date(s):
    if not s or s == "today":
        return date.today()
    if s == "yesterday":
        return date.today() - timedelta(days=1)
    return datetime.strptime(s, "%Y-%m-%d").date()


def cmd_sync(args):
    """OPTIONAL integration: write a completed session into a Markdown WEEKS/Week-XX.md sheet."""
    cfg = cfgmod.load_config()
    weeks_dir = args.weeks_dir or cfg.get("weeks_dir")
    if not weeks_dir:
        raise SystemExit("sync needs a sheets folder: pass --weeks-dir PATH, set weeks_dir in "
                         "config.json, or SPEEDIANCE_WEEKS_DIR. (Other commands don't need it.)")
    target = _resolve_date(args.date)
    c = _client(cfg)
    workouts = c.fetch_workouts(days=max(args.days, 1))
    cfgmod.save_token_cache(c.token, c.user_id)
    todays = [w for w in workouts if w.date == target]
    if not todays:
        print(f"No completed Speediance session found for {target}.")
        return
    sheet_path = sheetmod.find_week_sheet(weeks_dir, target)
    if not sheet_path:
        print(f"No Week-XX.md sheet found in {weeks_dir}")
        return
    print(f"Syncing {len(todays)} session(s) for {target} -> {sheet_path}\n")
    for w in todays:
        c.fetch_detail(w)
        res = sheetmod.write_session(sheet_path, w, target, unit=cfg["unit"])
        print(f"{w.title}: matched {len(res['matched'])}/{res['exercise_count']} exercises")
        if res["unmatched"]:
            print(f"    not matched (logged in notes): {', '.join(res['unmatched'])}")


def main(argv=None):
    logging.basicConfig(level=logging.WARNING, format="%(levelname)s %(message)s")
    p = argparse.ArgumentParser(prog="speediance", description="Read & create Speediance workouts.")
    sub = p.add_subparsers(dest="cmd", required=True)

    sp = sub.add_parser("login", help="Authenticate and cache a token")
    sp.set_defaults(func=cmd_login)

    sp = sub.add_parser("workouts", help="List recent completed sessions")
    sp.add_argument("--days", type=int, default=3)
    sp.add_argument("--json", action="store_true", help="machine-readable output")
    sp.set_defaults(func=cmd_workouts)

    sp = sub.add_parser("session", help="Full per-set detail for one training id")
    sp.add_argument("training_id", type=int)
    sp.add_argument("--json", action="store_true")
    sp.set_defaults(func=cmd_session)

    sp = sub.add_parser("library", help="Fetch your exercise catalog (id/name/muscle)")
    sp.add_argument("--out", default="library.json")
    sp.add_argument("--search", help="filter by name/muscle")
    sp.add_argument("--json", action="store_true", help="print results as JSON")
    sp.set_defaults(func=cmd_library)

    sp = sub.add_parser("push", help="Create a Speediance program from a plan JSON")
    sp.add_argument("plan", help="path to plan .json")
    sp.add_argument("--dry-run", action="store_true", help="build payload, do not send")
    sp.add_argument("--json", action="store_true", help="print payload/result as JSON")
    sp.set_defaults(func=cmd_push)

    sp = sub.add_parser("sync", help="(optional) write a session into a Markdown week sheet")
    sp.add_argument("--date", default="today", help="today | yesterday | YYYY-MM-DD")
    sp.add_argument("--days", type=int, default=3)
    sp.add_argument("--weeks-dir", dest="weeks_dir", help="folder with Week-XX.md sheets")
    sp.set_defaults(func=cmd_sync)

    args = p.parse_args(argv)
    try:
        args.func(args)
    except AuthError as e:
        _err(f"Auth error: {e}")
        sys.exit(2)


if __name__ == "__main__":
    main()
