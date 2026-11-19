#!/usr/bin/env python3
"""Check complete project skill packages across Claude, Cursor, and Codex.

Based on OpenLoop PR #176. Only own-skill-tree paths may differ.
Run --check before a PR. Reconcile all copies before --fix --from <harness>.
Provider settings and adapters outside skills stay in their native formats.
Requires Python 3.10+ and Git. Repair preserves source bytes and retargets paths.
"""
import argparse
import filecmp
import re
import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
HARNESSES = ("claude", "cursor", "codex")
SUBTREE = "skills"

# A reference to something INSIDE the harness's own skills tree, in either slash flavor — the
# skills are written on Windows and quote both `.claude/skills/verify-openloop/...` and
# `pwsh <repo>\.claude\skills\run-e2e\...`. A deeper path is required so that prose enumerating
# the three trees ("`.claude/skills/`, `.cursor/skills/`, `.codex/skills/`", which harness-sync's
# own SKILL.md does) is left alone: that is a list of all three, not a pointer at one.
SELF_REF = re.compile(r"\.(claude|cursor|codex)([/\\]" + SUBTREE + r"[/\\][A-Za-z0-9_.-])")
SELF_TOKEN = ".<harness>"


def normalized(path, harness):
    """File text with a reference to `harness`'s own skills tree collapsed to one token.

    Only that harness's token is replaced, so each copy has to point at ITSELF to compare equal:
    `.cursor`'s copy naming `.codex/skills` normalizes to nothing and reads as drift, which is
    what it is. None means "not UTF-8 text" — those are compared byte-for-byte instead.
    """
    try:
        # newline="": no universal-newline translation. Without it a CRLF skill file would come
        # back with LF endings and --fix would rewrite every line of it as a "sync".
        with path.open(encoding="utf-8", newline="") as fh:
            text = fh.read()
    except (UnicodeDecodeError, OSError):
        return None
    return re.sub(rf"\.{harness}([/\\]{SUBTREE}[/\\][A-Za-z0-9_.-])", SELF_TOKEN + r"\1", text)


def same_content(a, a_harness, b, b_harness):
    """True when two copies differ only in which harness tree they point at."""
    na = normalized(a, a_harness)
    if na is not None:
        return na == normalized(b, b_harness)
    return filecmp.cmp(a, b, shallow=False)


def tracked(harness):
    """Relative paths under <harness>/skills that git can see, mapped to their absolute path.

    `--cached --others --exclude-standard` is tracked files PLUS untracked ones that are not
    gitignored. Both halves matter: a directory walk would drag in gitignored working files
    (`.claude/worktrees/` above all) and read them as drift, while tracked-only would miss a
    brand-new skill that has not been `git add`ed yet — the case where drift is most likely,
    since a new skill written into one harness is exactly what needs propagating.
    """
    top = f".{harness}/{SUBTREE}"
    out = subprocess.run(["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard", top],
                         cwd=ROOT, check=True, capture_output=True, text=True).stdout
    return {p[len(top) + 1:]: ROOT / p for p in out.split("\0") if p}


def edited_trees(rel_paths):
    """Harnesses whose copy of one of the DRIFTED paths changed — committed here, or still dirty.

    This is the safety net under `--fix`. Aligning three trees is destructive by nature: two of
    them get overwritten, and if the wrong one is named as the source the edit being propagated is
    silently deleted and the check then passes, so nothing ever reports the loss. The person or
    agent running the command is the least reliable source for "which one did I edit"; git knows.

    Scoped to the paths that actually differ, so an unrelated edit elsewhere in a tree cannot vote
    on the direction for this one. Git has nothing to say about a path that is untracked in all
    three harnesses (a brand-new skill and the copies a previous --fix wrote look identical to it),
    and the caller reports that case rather than picking a winner from no evidence.
    """
    default = subprocess.run(["git", "symbolic-ref", "refs/remotes/origin/HEAD"], cwd=ROOT,
                             capture_output=True, text=True).stdout.strip()
    base = subprocess.run(["git", "merge-base", default or "origin/main", "HEAD"], cwd=ROOT,
                          capture_output=True, text=True).stdout.strip()
    paths = [f".{h}/{SUBTREE}/{rel}" for h in HARNESSES for rel in rel_paths]
    changed = set()
    # Committed on this branch (skipped when there is no origin/main to compare against, e.g. a
    # fresh clone or a test fixture) plus anything uncommitted in the working tree.
    cmds = ([["git", "diff", "--name-only", base, "--", *paths]] if base else []) + \
           [["git", "status", "--porcelain", "--", *paths]]
    for cmd in cmds:
        out = subprocess.run(cmd, cwd=ROOT, capture_output=True, text=True).stdout
        for line in out.splitlines():
            line = line[3:] if cmd[1] == "status" else line          # strip porcelain XY prefix
            for h in HARNESSES:
                if line.strip().strip('"').startswith(f".{h}/{SUBTREE}/"):
                    changed.add(h)
    return changed


def drift():
    """(rel_path, {harness: 'same'|'differs'|'missing'}) for every path that is not identical."""
    trees = {h: tracked(h) for h in HARNESSES}
    base = HARNESSES[0]
    found = []
    for rel in sorted(set().union(*(t.keys() for t in trees.values()))):
        state = {}
        for h in HARNESSES:
            # `git ls-files` still lists a tracked file that has been deleted on disk, so
            # existence is checked separately — an unstaged delete is drift like any other.
            if rel not in trees[h] or not trees[h][rel].exists():
                state[h] = "missing"
            elif rel not in trees[base] or not trees[base][rel].exists() or h == base:
                state[h] = "same"
            else:
                state[h] = "same" if same_content(trees[base][rel], base, trees[h][rel], h) else "differs"
        # All-missing is consistent, not drift: git still lists a path deleted from every
        # harness (staged or not), and flagging that would leave --fix with nothing to do
        # and the gate permanently red until the delete was committed.
        if set(state.values()) not in ({"same"}, {"missing"}):
            found.append((rel, state))
    return trees, found


def main(argv):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--check", action="store_true", help="exit 1 if the trees differ (default)")
    ap.add_argument("--fix", action="store_true", help="copy --from over the other harnesses")
    ap.add_argument("--from", dest="source", choices=HARNESSES,
                    help="the harness that was edited; inferred from git when omitted")
    ap.add_argument("--force", action="store_true",
                    help="proceed even though --from contradicts git — discards that change")
    args = ap.parse_args(argv)

    trees, found = drift()
    total = len(trees[HARNESSES[0]])

    if not args.fix:
        if not found:
            print(f"harness skills in sync ({total} files x {len(HARNESSES)} harnesses)")
            return 0
        print(f"{len(found)} path(s) differ between {', '.join('.' + h for h in HARNESSES)}:")
        for rel, state in found:
            print(f"  {SUBTREE}/{rel}")
            for h in HARNESSES:
                print(f"      .{h}: {state[h]}")
        print("\nThe harness you edited is the source of truth. Propagate it with:")
        print("  python scripts/sync-harness-skills.py --fix --from <claude|cursor|codex>")
        return 1

    if not found:
        # Checked before any source is worked out: with nothing to propagate there is no source to
        # get wrong, and three freshly-synced trees would otherwise look like three separate edits.
        print(f"harness skills already in sync ({total} files x {len(HARNESSES)} harnesses)")
        return 0

    # Align means the edited harness wins and the other two catch up — never the reverse. Git is
    # the authority on which one was edited, so a declared --from that contradicts it is refused
    # rather than obeyed: that combination is precisely the one that deletes the change.
    edited = edited_trees([rel for rel, _ in found])
    if not args.source:
        if len(edited) == 1:
            args.source = next(iter(edited))
            print(f"source: .{args.source} (the only harness with changes; use --from to override)\n")
        else:
            if not edited:
                why = ("git sees no change to the drifted path(s) in any harness — nothing to "
                       "propagate, so the drift came from somewhere else")
            elif len(edited) == len(HARNESSES):
                why = ("every copy is new or modified, which is what a fresh skill plus the copies "
                       "a previous --fix wrote look like. Git cannot tell an edit from a sync here, "
                       "so name the harness you edited with --from")
            else:
                why = (f"{len(edited)} harnesses were edited separately "
                       f"({', '.join('.' + h for h in sorted(edited))}). Reconcile them into one "
                       "correct tree by hand, then propagate from that one")
            print(f"cannot infer the source: {why}.")
            return 2
    elif edited and args.source not in edited:
        print(f"refusing: you named .{args.source} as the source, but git says the change is in "
              f"{', '.join('.' + h for h in sorted(edited))}.")
        print(f"Syncing from .{args.source} would overwrite that change and delete it. If you really "
              f"mean to discard it, pass --force.")
        if not args.force:
            return 2
        print("--force given; continuing.\n")

    src_tree = trees[args.source]
    copied = removed = 0
    for rel, _ in found:
        for h in HARNESSES:
            if h == args.source:
                continue
            dst = ROOT / f".{h}" / SUBTREE / rel
            if rel not in src_tree or not src_tree[rel].exists():
                # Deleted in the source: mirror the delete, or the next --check flags it forever.
                if dst.exists():
                    dst.unlink()
                    removed += 1
                    print(f"  removed .{h}/{SUBTREE}/{rel}")
                continue
            dst.parent.mkdir(parents=True, exist_ok=True)
            # Retarget the source's self-reference at the harness being written, so its copy points
            # at a tree that LLM can actually read. Everything else is copied verbatim; a non-text
            # file (or one with no self-reference) round-trips byte-for-byte through the same path.
            text = normalized(src_tree[rel], args.source)
            if text is None:
                shutil.copyfile(src_tree[rel], dst)
            else:
                dst.write_text(text.replace(SELF_TOKEN, f".{h}"), encoding="utf-8", newline="")
            copied += 1
            print(f"  wrote   .{h}/{SUBTREE}/{rel}")
    print(f"\nsynced from .{args.source}: {copied} copied, {removed} removed — `git add` the result")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
