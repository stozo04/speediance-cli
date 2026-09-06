#!/usr/bin/env python3
"""Self-check for scripts/sync-harness-skills.py — the project skill synchronization gate.

    python scripts/test-sync-harness-skills.py

Every case builds a throwaway git repo in a temp directory and copies the real script into it,
so nothing here can touch the actual harness trees. That matters more than usual: this script's
`--fix` overwrites files, and a test that ran in-repo could leave `.cursor/skills/` dirty and red
the very gate it is checking.

No framework on purpose — asserts and a `main()`. What each case pins down is a way the gate has
already been wrong or could silently become useless:

  * in_sync           — the happy path returns 0, or the gate cries wolf on every commit.
  * content_drift     — the case the gate exists for.
  * missing_file      — a skill present in one harness and absent in another.
  * untracked_file    — a NEW skill not yet `git add`ed. This was a real hole: the file set came
                        from tracked files only, so writing a new skill into one harness left the
                        gate green and the other two harnesses permanently behind.
  * deleted_on_disk   — tracked by git but removed from the working tree; `git ls-files` still
                        lists it, so existence has to be checked separately (this one crashed).
  * fix_propagates    — --fix --from actually repairs, and leaves the source untouched.
  * fix_mirrors_delete— a delete in the source is mirrored, not silently re-created forever.
  * fix_needs_source  — bare --fix refuses rather than guessing, because a wrong guess reverts
                        the edit being propagated and then passes the check, hiding the loss.
  * self_reference    — the one allowed difference: each copy points at its OWN skills tree. The
                        three cases below pin all of it, because the normalization that permits it
                        is also the way real drift could start slipping past the gate.
"""
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / "sync-harness-skills.py"
HARNESSES = ("claude", "cursor", "codex")
SKILL = "skills/demo/SKILL.md"


def build_repo(tmp):
    """A minimal repo: three harness trees holding one identical skill, all committed."""
    subprocess.run(["git", "init", "-q"], cwd=tmp, check=True)
    (tmp / "scripts").mkdir()
    shutil.copyfile(SCRIPT, tmp / "scripts" / SCRIPT.name)
    for h in HARNESSES:
        dst = tmp / f".{h}" / SKILL
        dst.parent.mkdir(parents=True)
        dst.write_text("# demo skill\n", encoding="utf-8")
    # A gitignored file inside a harness tree must never read as drift.
    (tmp / ".gitignore").write_text(".claude/worktrees/\n", encoding="utf-8")
    (tmp / ".claude" / "worktrees").mkdir(parents=True)
    (tmp / ".claude" / "worktrees" / "junk.md").write_text("ignore me\n", encoding="utf-8")
    subprocess.run(["git", "add", "-A"], cwd=tmp, check=True)
    subprocess.run(["git", "-c", "user.email=t@t", "-c", "user.name=t",
                    "commit", "-qm", "init"], cwd=tmp, check=True)


def run(tmp, *args):
    p = subprocess.run([sys.executable, str(tmp / "scripts" / SCRIPT.name), *args],
                       cwd=tmp, capture_output=True, text=True)
    return p.returncode, p.stdout + p.stderr


def case(name):
    """Fresh repo per case, torn down after — cases must not inherit each other's drift."""
    def decorator(fn):
        with tempfile.TemporaryDirectory() as d:
            tmp = Path(d)
            build_repo(tmp)
            fn(tmp)
        print(f"  ok  {name}")
    return decorator


def main():
    print(f"testing {SCRIPT.name}")

    @case("in_sync — clean trees exit 0 and ignore gitignored files")
    def _(tmp):
        code, out = run(tmp)
        assert code == 0, out
        assert "in sync" in out, out
        assert "worktrees" not in out, f"gitignored file leaked into the comparison:\n{out}"

    @case("content_drift — one edited copy is reported, naming the harness")
    def _(tmp):
        (tmp / ".cursor" / SKILL).write_text("# demo skill\nedited\n", encoding="utf-8")
        code, out = run(tmp)
        assert code == 1, out
        assert SKILL in out and "differs" in out, out

    @case("missing_file — a skill absent from one harness is drift")
    def _(tmp):
        (tmp / ".codex" / SKILL).unlink()
        subprocess.run(["git", "add", "-A"], cwd=tmp, check=True)
        code, out = run(tmp)
        assert code == 1 and "missing" in out, out

    @case("untracked_file — a NEW skill not yet git-added is still drift")
    def _(tmp):
        new = tmp / ".claude" / "skills" / "fresh" / "SKILL.md"
        new.parent.mkdir(parents=True)
        new.write_text("# brand new\n", encoding="utf-8")
        code, out = run(tmp)
        assert code == 1, f"a new unsynced skill must not pass the gate:\n{out}"
        assert "fresh" in out, out

    @case("deleted_on_disk — tracked but removed still counts, and does not crash")
    def _(tmp):
        (tmp / ".codex" / SKILL).unlink()          # deliberately NOT git rm'd
        code, out = run(tmp)
        assert code == 1 and "missing" in out, out
        assert "Traceback" not in out, out

    @case("fix_propagates — --from repairs the others and leaves the source alone")
    def _(tmp):
        (tmp / ".cursor" / SKILL).write_text("# demo skill\nthe good edit\n", encoding="utf-8")
        code, out = run(tmp, "--fix", "--from", "cursor")
        assert code == 0, out
        for h in HARNESSES:
            assert "the good edit" in (tmp / f".{h}" / SKILL).read_text(encoding="utf-8"), h
        assert run(tmp)[0] == 0

    @case("fix_mirrors_delete — a delete in the source removes the copies")
    def _(tmp):
        (tmp / ".claude" / SKILL).unlink()
        code, out = run(tmp, "--fix", "--from", "claude")
        assert code == 0, out
        for h in HARNESSES:
            assert not (tmp / f".{h}" / SKILL).exists(), h
        assert run(tmp)[0] == 0

    @case("fix_infers_source — bare --fix propagates FROM the tree git says changed")
    def _(tmp):
        (tmp / ".cursor" / SKILL).write_text("# demo skill\nedited\n", encoding="utf-8")
        code, out = run(tmp, "--fix")
        assert code == 0, out
        assert ".cursor" in out, out
        # Aligning must carry the edit outward, never erase it to match the untouched majority.
        for h in HARNESSES:
            assert "edited" in (tmp / f".{h}" / SKILL).read_text(encoding="utf-8"), h

    @case("fix_refuses_contradiction — --from that git disagrees with is blocked, not obeyed")
    def _(tmp):
        (tmp / ".cursor" / SKILL).write_text("# demo skill\nedited\n", encoding="utf-8")
        code, out = run(tmp, "--fix", "--from", "claude")
        assert code == 2, out
        assert "delete" in out and ".cursor" in out, out
        # The whole point: the edit survives the refusal.
        assert "edited" in (tmp / ".cursor" / SKILL).read_text(encoding="utf-8"), out

    @case("fix_force_overrides — --force still allows a deliberate discard")
    def _(tmp):
        (tmp / ".cursor" / SKILL).write_text("# demo skill\nedited\n", encoding="utf-8")
        code, out = run(tmp, "--fix", "--from", "claude", "--force")
        assert code == 0, out
        assert "edited" not in (tmp / ".cursor" / SKILL).read_text(encoding="utf-8")

    @case("fix_refuses_when_ambiguous — two edited trees are reconciled by a human, not by us")
    def _(tmp):
        (tmp / ".cursor" / SKILL).write_text("# demo skill\ncursor edit\n", encoding="utf-8")
        (tmp / ".codex" / SKILL).write_text("# demo skill\ncodex edit\n", encoding="utf-8")
        code, out = run(tmp, "--fix")
        assert code == 2, out
        assert "cursor edit" in (tmp / ".cursor" / SKILL).read_text(encoding="utf-8")
        assert "codex edit" in (tmp / ".codex" / SKILL).read_text(encoding="utf-8")

    @case("self_reference — each copy pointing at its own skills tree is not drift")
    def _(tmp):
        for h in HARNESSES:
            (tmp / f".{h}" / SKILL).write_text(
                f"# demo skill\nrun .{h}/skills/demo/go.py and pwsh .{h}\\skills\\demo\\go.ps1\n",
                encoding="utf-8")
        code, out = run(tmp)
        assert code == 0, f"self-references must be allowed in both slash flavors:\n{out}"

    @case("self_reference_wrong_target — pointing at ANOTHER harness's tree is still drift")
    def _(tmp):
        # The hole this closes: normalizing every harness token to one placeholder would make
        # `.codex/skills` inside .cursor's copy compare equal, and Cursor would be sent to a
        # directory it cannot read with the gate green.
        (tmp / ".cursor" / SKILL).write_text("# demo skill\nrun .codex/skills/demo/go.py\n",
                                             encoding="utf-8")
        (tmp / ".claude" / SKILL).write_text("# demo skill\nrun .claude/skills/demo/go.py\n",
                                             encoding="utf-8")
        (tmp / ".codex" / SKILL).write_text("# demo skill\nrun .codex/skills/demo/go.py\n",
                                            encoding="utf-8")
        assert run(tmp)[0] == 1, "a copy pointing at someone else's tree must be reported"

    @case("fix_retargets — --fix rewrites the self-reference for each destination harness")
    def _(tmp):
        (tmp / ".claude" / SKILL).write_text(
            "# demo skill\nread .claude/skills/demo/note.md\n", encoding="utf-8")
        code, out = run(tmp, "--fix", "--from", "claude")
        assert code == 0, out
        for h in HARNESSES:
            body = (tmp / f".{h}" / SKILL).read_text(encoding="utf-8")
            assert f".{h}/skills/demo/note.md" in body, f".{h} was not retargeted: {body!r}"
        assert run(tmp)[0] == 0, "a retargeted tree must satisfy the check it was written for"

    @case("identical_wrong_targets — identical bytes must not hide foreign harness paths")
    def _(tmp):
        for h in HARNESSES:
            (tmp / f".{h}" / SKILL).write_text(
                "# demo skill\nread .claude/skills/demo/note.md and .claude\\skills\\demo\\go.ps1\n",
                encoding="utf-8")
        code, out = run(tmp)
        assert code == 1, f"identical foreign paths must fail the check:\n{out}"
        code, out = run(tmp, "--fix", "--from", "claude")
        assert code == 0, out
        for h in HARNESSES:
            body = (tmp / f".{h}" / SKILL).read_text(encoding="utf-8")
            assert f".{h}/skills/demo/note.md" in body, body
            assert f".{h}\\skills\\demo\\go.ps1" in body, body
        assert run(tmp)[0] == 0, "repair must satisfy the check"

    @case("fix_preserves_crlf — a CRLF skill is not silently rewritten to LF")
    def _(tmp):
        (tmp / ".claude" / SKILL).write_bytes(b"# demo skill\r\nedited\r\n")
        assert run(tmp, "--fix", "--from", "claude")[0] == 0
        for h in HARNESSES:
            assert (tmp / f".{h}" / SKILL).read_bytes() == b"# demo skill\r\nedited\r\n", h

    print("all cases passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
