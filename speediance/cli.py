"""Speediance CLI - sync completed Gym Monster workouts into your training log."""
import sys
import argparse
import logging
from datetime import date, datetime, timedelta

from . import config as cfgmod
from .client import SpeedianceClient, AuthError
from . import sheet as sheetmod
from . import templates as tpl


def _client(cfg, use_cache=True):
    cache = cfgmod.load_token_cache() if use_cache else {}
    c = SpeedianceClient(cfg["email"], cfg["password"], cfg["region"],
                         token=cache.get("token", ""), user_id=cache.get("user_id", ""))
    return c


def cmd_login(args):
    cfg = cfgmod.load_config()
    c = _client(cfg, use_cache=False)
    token, uid = c.login()
    cfgmod.save_token_cache(token, uid)
    print(f"Logged in (user {uid}). Token cached.")


def cmd_workouts(args):
    cfg = cfgmod.load_config()
    c = _client(cfg)
    workouts = c.fetch_workouts(days=args.days)
    cfgmod.save_token_cache(c.token, c.user_id)
    if not workouts:
        print(f"No completed workouts in the last {args.days} day(s).")
        return
    print(f"Found {len(workouts)} session(s) in the last {args.days} day(s):\n")
    for w in workouts:
        d = w.date
        mins = w.duration_secs // 60
        print(f"  - {d}  {w.title}  -  {mins} min, {w.calories} kcal  (id {w.training_id})")


def _resolve_date(s):
    if not s or s == "today":
        return date.today()
    if s == "yesterday":
        return date.today() - timedelta(days=1)
    return datetime.strptime(s, "%Y-%m-%d").date()


def cmd_sync(args):
    cfg = cfgmod.load_config()
    target = _resolve_date(args.date)
    c = _client(cfg)
    workouts = c.fetch_workouts(days=max(args.days, 1))
    cfgmod.save_token_cache(c.token, c.user_id)

    todays = [w for w in workouts if w.date == target]
    if not todays:
        print(f"No completed Speediance session found for {target}.")
        print("Tip: finish & save the workout in the Speediance app, then re-run.")
        return

    sheet_path = sheetmod.find_week_sheet(cfg["weeks_dir"], target)
    if not sheet_path:
        print(f"No Week-XX.md sheet found in {cfg['weeks_dir']}")
        return

    print(f"Syncing {len(todays)} session(s) for {target} -> {sheet_path}\n")
    for w in todays:
        c.fetch_detail(w)
        if args.dry_run:
            print(f"[dry-run] {w.title}: {len(w.sets)} sets across {len(w.exercises())} exercises")
            for name, sets in w.exercises().items():
                print(f"    {name}: {sheetmod._sets_str(sets, cfg['unit'])}")
            continue
        res = sheetmod.write_session(sheet_path, w, target, unit=cfg["unit"])
        print(f"{w.title}: matched {len(res['matched'])}/{res['exercise_count']} exercises into the sheet")
        for sheet_ex, api_ex in res["matched"]:
            print(f"    {sheet_ex}  <-  {api_ex}")
        if res["unmatched"]:
            print(f"    not matched to a sheet row (still logged in notes): {', '.join(res['unmatched'])}")
    if not args.dry_run:
        print("\nDone. Open the sheet to review.")


def cmd_library(args):
    cfg = cfgmod.load_config()
    c = _client(cfg)
    lib = tpl.fetch_library(c, device_type=int(cfg.get("device_type", 1)))
    cfgmod.save_token_cache(c.token, c.user_id)
    import json as _json
    with open(args.out, "w", encoding="utf-8") as f:
        _json.dump(lib, f, ensure_ascii=False, indent=2)
    print(f"Saved {len(lib)} exercises to {args.out}")
    if args.search:
        q = args.search.lower()
        hits = [e for e in lib if q in e["name"].lower() or q in e.get("muscle", "").lower()]
        print(f"\n{len(hits)} match '{args.search}':")
        for e in hits[:40]:
            print(f"  [{e['id']}] {e['name']} ({e.get('muscle','')})")


def cmd_push(args):
    cfg = cfgmod.load_config()
    plan = tpl.load_plan(args.plan)
    c = _client(cfg)
    n_ex = len(plan["exercises"])
    n_sets = sum(len(e.get("sets", [])) for e in plan["exercises"])
    print(f"Plan: {plan['name']} - {n_ex} exercises, {n_sets} sets")
    if args.dry_run:
        payload = tpl.build_payload(c, plan["name"], plan["exercises"],
                                    device_type=int(cfg.get("device_type", 1)))
        cfgmod.save_token_cache(c.token, c.user_id)
        print(f"[dry-run] built payload, totalCapacity={payload['totalCapacity']:.0f}; not sent.")
        for a in payload["actionLibraryList"]:
            print(f"    groupId {a['groupId']}: reps {a['setsAndReps']} | weights {a['weights']}")
        return
    data = tpl.create_template(c, plan["name"], plan["exercises"],
                               device_type=int(cfg.get("device_type", 1)))
    cfgmod.save_token_cache(c.token, c.user_id)
    print(f"Created template '{plan['name']}' on your Speediance account. Open the app to run it.")


def main(argv=None):
    logging.basicConfig(level=logging.WARNING, format="%(levelname)s %(message)s")
    p = argparse.ArgumentParser(prog="speediance", description="Sync Speediance workouts into your training log.")
    sub = p.add_subparsers(dest="cmd", required=True)

    sp = sub.add_parser("login", help="Authenticate and cache a token")
    sp.set_defaults(func=cmd_login)

    sp = sub.add_parser("workouts", help="List recent completed sessions")
    sp.add_argument("--days", type=int, default=3)
    sp.set_defaults(func=cmd_workouts)

    sp = sub.add_parser("sync", help="Write a day's session into the current week sheet")
    sp.add_argument("--date", default="today", help="today | yesterday | YYYY-MM-DD")
    sp.add_argument("--days", type=int, default=3, help="lookback window to search")
    sp.add_argument("--dry-run", action="store_true", help="print what would be written, change nothing")
    sp.set_defaults(func=cmd_sync)

    sp = sub.add_parser("library", help="Fetch your exercise catalog (ids) to a file")
    sp.add_argument("--out", default="library.json")
    sp.add_argument("--search", help="filter printed results by name/muscle")
    sp.set_defaults(func=cmd_library)

    sp = sub.add_parser("push", help="Create a Speediance program from a plan JSON")
    sp.add_argument("plan", help="path to plan .json")
    sp.add_argument("--dry-run", action="store_true", help="build payload, do not send")
    sp.set_defaults(func=cmd_push)

    args = p.parse_args(argv)
    try:
        args.func(args)
    except AuthError as e:
        print(f"Auth error: {e}", file=sys.stderr)
        sys.exit(2)


if __name__ == "__main__":
    main()
