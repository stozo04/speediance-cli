# Releasing & Versioning — Cheat Sheet

How to cut a release for `speediance-cli`, when to do it, and what happens under the
hood. Written for this exact repo's setup (GoReleaser + GitHub Actions + ClawHub).

> **The one rule that explains everything:**
> **A release is triggered by pushing a git tag (`vX.Y.Z`) — *not* by merging a PR.**
> Merging code updates `main`. Tagging is the deliberate "publish" button.

---

## TL;DR — cut a release

```bash
# 1. Get on a clean, up-to-date, GREEN main
git checkout main
git pull origin main
gh run list --branch main --limit 3        # confirm CI is green first

# 2. Tag the new version (annotated) and push the tag
git tag -a v1.0.1 -m "v1.0.1 — short summary of what changed"
git push origin v1.0.1                      # <-- THIS fires the release

# 3. Watch it build & publish (~2-3 min: cross-compiles 6 platforms)
gh run watch

# 4. Verify the published release
gh release view v1.0.1
```

That's it. The tag push triggers `.github/workflows/release.yml`, which runs
GoReleaser, builds binaries for all platforms, and posts them as a GitHub Release.

---

## Picking the version number (Semantic Versioning)

Format is `vMAJOR.MINOR.PATCH` (e.g. `v1.2.3`). Bump the part that matches the change:

| Bump | When | Example |
|------|------|---------|
| **PATCH** `v1.0.0 → v1.0.1` | Bug fixes, dependency bumps. Nothing about *how you use it* changes. | Fixed a token-refresh crash; merged Dependabot PRs |
| **MINOR** `v1.0.0 → v1.1.0` | New feature/command, **backward-compatible** (old usage still works). | Added a `speediance-cli export` command |
| **MAJOR** `v1.0.0 → v2.0.0` | **Breaking change** — existing usage would break. | Renamed/removed a command, changed `--json` output shape, changed config format |

Rule of thumb: ask *"would this surprise or break someone already using v1.0.0?"*
- Yes → **MAJOR**
- No, but it's new → **MINOR**
- No, it's just a fix → **PATCH**

---

## When SHOULD I cut a release?

Release when there are changes on `main` that you want **available as a downloadable
binary** (for a new machine, another person, or an agent). You do **not** release after
every merge — merge freely, let `main` accumulate, then release a meaningful unit.

**Good reasons to release:**
- 🐛 A bug fix users need now → PATCH
- ✨ A new feature/command worth shipping → MINOR
- 💥 A breaking change → MAJOR
- 📦 A batch of dependency updates (e.g. several Dependabot PRs) you want shipped → PATCH

**Don't bother releasing for:**
- Docs-only changes (README/AGENTS/this file)
- Work-in-progress on `main`
- Pure internal refactors with zero user-facing change (optional)

**One tag = one release** that bundles *everything currently on `main`*. If you merge
5 PRs and then tag once, all 5 ride along in that single release. You don't tag per PR.

---

## What triggers what (so nothing surprises you)

| You do this… | …this runs | Result |
|---|---|---|
| `git push origin vX.Y.Z` (a tag) | **Release** (`release.yml` → GoReleaser) | GitHub Release w/ 6 binaries + `checksums.txt` |
| Push / merge to `main` | **CI** (`ci.yml`) | build & test + lint (no release!) |
| Push / merge to `main` **that changes `SKILL.md`** | **Publish to ClawHub** | Skill republished to ClawHub |
| Open / update a PR | **CI** | build & test + lint |

Merging PRs **never** creates a release or bumps the version. The version lives in the
git tag, not in a file — the build stamps it into the binary at release time, so
`speediance-cli version` prints whatever tag it was built from.

---

## Test before you ship (optional but smart)

**Local dry run** — builds all the release artifacts on your machine, publishes nothing
(needs Go + `goreleaser` on PATH):

```bash
goreleaser check                       # validate .goreleaser.yaml
goreleaser release --snapshot --clean  # build everything into ./dist, no upload
```

**Full dress rehearsal** — a real but *deletable* pre-release. Because `.goreleaser.yaml`
has `prerelease: auto`, any tag with a `-rc.N` suffix publishes as a **Pre-release**
(not "Latest"), so you can test the whole publish pipeline, then bin it:

```bash
git tag -a v1.1.0-rc.1 -m "release candidate" && git push origin v1.1.0-rc.1
# ...inspect the pre-release on GitHub...
gh release delete v1.1.0-rc.1 --cleanup-tag --yes   # remove release + tag
git tag -a v1.1.0 -m "v1.1.0 — ..." && git push origin v1.1.0   # the real one
```

---

## Verify a published release

```bash
gh release view v1.0.0 --json tagName,isDraft,isPrerelease,assets

# Prove the published binary actually works + reports the right version:
gh release download v1.0.0 --pattern "*Windows_x86_64*" --dir /tmp/verify
cd /tmp/verify && unzip -o speediance-cli_Windows_x86_64.zip
./speediance-cli.exe version           # should print: 1.0.0 (commit ..., go...)

# Confirm the download isn't corrupted (fingerprint must match):
gh release download v1.0.0 --pattern "checksums.txt" --dir /tmp/verify
sha256sum speediance-cli_Windows_x86_64.zip && grep Windows_x86_64 checksums.txt
```

---

## Fixing a botched release

A published version number should be treated as permanent — **don't reuse it.** If a
release is broken, delete it and ship the next patch instead:

```bash
gh release delete v1.0.1 --cleanup-tag --yes   # deletes the release AND the tag
# fix the code on main, then cut v1.0.2
```

(Only reuse a tag if literally nobody could have downloaded it yet — otherwise people
end up with two different "v1.0.1"s, which is the thing versioning exists to prevent.)

---

## Known quirks / gotchas

- **Tags must start with `v`** — the release workflow only triggers on `v*`.
- **Lint Go-version pin:** `ci.yml`'s lint job is pinned to the module's Go version
  (`go-version-file: go.mod`) so the golangci-lint release binary doesn't panic. If you
  bump the `go` directive in `go.mod`, you'll likely need to bump golangci-lint too.
  See **issue #15** for the proper cleanup.
- **GoReleaser runs `go mod tidy`** before building — keep `go.mod`/`go.sum` tidy or it
  may complain.
- **ClawHub** publishes automatically on a `SKILL.md` change to `main`; you can also run
  it manually via the **"Publish to ClawHub"** Actions workflow (`workflow_dispatch`).

---

## Prerequisites (one-time)

- `gh` CLI authenticated with push access (`gh auth status`).
- For local dry runs only: Go + `goreleaser` on your PATH.
- The `CLAWHUB_TOKEN` repo secret must be set (it is) for the ClawHub publish step.
