"""Write a completed Speediance session into the current WEEKS/Week-XX.md sheet."""
import os
import re
import glob
import difflib

CHECK_EMPTY = "☐"   # empty checkbox
CHECK_DONE = "☑"    # checked box
GLANCE_CHECKS = (CHECK_EMPTY, CHECK_DONE)


def date_token(d):
    """6/15 style, no leading zeros — matches the sheet headers."""
    return f"{d.month}/{d.day}"


def _norm(name):
    s = name.lower()
    s = re.sub(r"\(.*?\)", " ", s)          # drop parentheticals e.g. (Gym Monster)
    s = s.replace("dumbbell", "db").replace("barbell", "bb")
    s = re.sub(r"[^a-z0-9 ]", " ", s)
    syn = {"rdl": "romanian deadlift", "ohp": "overhead press"}
    toks = [syn.get(t, t) for t in s.split()]
    stop = {"the", "a", "machine", "cable", "db", "press", "the"}
    core = [t for t in toks if t not in stop] or toks
    return " ".join(sorted(set(toks))), set(core)


def _match(sheet_exercise, workout_names):
    """Best fuzzy match of a sheet exercise to a workout exercise name."""
    s_full, s_core = _norm(sheet_exercise)
    best, best_score = None, 0.0
    for wn in workout_names:
        w_full, w_core = _norm(wn)
        ratio = difflib.SequenceMatcher(None, s_full, w_full).ratio()
        overlap = len(s_core & w_core) / max(1, len(s_core | w_core))
        score = 0.5 * ratio + 0.5 * overlap
        if score > best_score:
            best, best_score = wn, score
    return (best, best_score) if best_score >= 0.45 else (None, best_score)


def _fmt_weight(w):
    return str(int(w)) if float(w).is_integer() else f"{w:g}"


def _sets_str(sets, unit):
    """Compact 'w x r, w x r' string for a list of SetData."""
    parts = []
    for s in sets:
        w = _fmt_weight(s.max_weight)
        parts.append(f"{w}{unit}×{s.finished_reps}")
    return ", ".join(parts)


def find_week_sheet(weeks_dir, target_date):
    files = sorted(glob.glob(os.path.join(weeks_dir, "Week-*.md")))
    if not files:
        return None
    tok = date_token(target_date)
    for path in files:
        with open(path, encoding="utf-8") as f:
            text = f.read()
        # a section header containing the date means this week covers it
        if re.search(rf"^##.*\b{re.escape(tok)}\b", text, re.MULTILINE):
            return path
    return files[-1]  # fallback: highest-numbered week


def write_session(path, workout, target_date, unit="lb"):
    with open(path, encoding="utf-8") as f:
        lines = f.read().split("\n")

    tok = date_token(target_date)
    ex_map = workout.exercises()
    workout_names = list(ex_map.keys())
    matched, used_names = [], set()

    # locate the workout section for this date (e.g. "## PUSH - Mon 6/15 ...")
    sec_start = None
    for i, ln in enumerate(lines):
        if ln.startswith("##") and tok in ln and "Notes" not in ln:
            sec_start = i
            break

    # 1) fill weight cells + check boxes inside that section's exercise table
    if sec_start is not None:
        for i in range(sec_start + 1, len(lines)):
            ln = lines[i]
            if ln.startswith("##"):
                break
            if not ln.strip().startswith("|"):
                continue
            cells = [c.strip() for c in ln.strip().strip("|").split("|")]
            if len(cells) != 4 or cells[0] not in GLANCE_CHECKS:
                continue
            exercise = cells[1]
            name, score = _match(exercise, workout_names)
            if name and name not in used_names:
                cells[0] = CHECK_DONE
                cells[3] = _sets_str(ex_map[name], unit)
                lines[i] = "| " + " | ".join(cells) + " |"
                matched.append((exercise, name))
                used_names.add(name)

    # 2) flip the at-a-glance row for this date (its last table cell is the checkbox)
    for i, ln in enumerate(lines):
        if tok in ln and ln.strip().startswith("|"):
            cells = [c.strip() for c in ln.strip().strip("|").split("|")]
            if cells and cells[-1] == CHECK_EMPTY:
                cells[-1] = CHECK_DONE
                lines[i] = "| " + " | ".join(cells) + " |"
                break

    # 3) build a full "logged" block (captures everything, even unmatched)
    dur = f"{workout.duration_secs // 60} min" if workout.duration_secs else "-"
    cal = f"{workout.calories} kcal" if workout.calories else "-"
    comp = f"{workout.completion_rate:.0f}% complete" if workout.completion_rate else ""
    header = f"  - **Logged from Speediance - {workout.title}** ({tok}) - {dur} - {cal}"
    if comp:
        header += f" - {comp}"
    block = [header]
    for name, sets in ex_map.items():
        block.append(f"    - {name}: {_sets_str(sets, unit)}")
    if not ex_map:
        block.append("    - (no per-set detail returned)")

    # insert under the matching Notes bullet, else append a Logged Sessions section
    note_idx = None
    for i, ln in enumerate(lines):
        if re.match(rf"^\s*-\s.*{re.escape(tok)}", ln):
            note_idx = i
            break
    if note_idx is not None:
        lines[note_idx:note_idx + 1] = [lines[note_idx]] + block
    else:
        lines += ["", "## Logged Sessions", ""] + block

    with open(path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))

    return {
        "sheet": path,
        "matched": matched,
        "unmatched": [n for n in workout_names if n not in used_names],
        "exercise_count": len(workout_names),
    }
