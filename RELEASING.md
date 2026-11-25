# Releasing & Versioning — Cheat Sheet

How to cut a release for `speediance-cli`, when to do it, and what happens under the
hood. Written for this exact repo's setup (GoReleaser + GitHub Actions + ClawHub).

> **The one rule that explains everything:**
> **A release is cut by merging the open release PR (`chore(main): release X.Y.Z`).**
> Merging code updates `main`. Merging the release PR is the deliberate "publish" button.

---

## TL;DR — cut a release

1. Merge feature and fix PRs into `main` as usual, with Conventional Commit titles.
2. [release-please](https://github.com/googleapis/release-please-action) keeps one
   release PR open, titled `chore(main): release X.Y.Z`. Each push to `main` updates its
   version and notes. CI runs on it like any other PR.
3. When you want to ship, merge that release PR. The **Release Please** workflow tags
   `vX.Y.Z`, creates the GitHub Release, and runs GoReleaser in the same run to attach the
   binaries and write the release notes.

```bash
gh pr list --label "autorelease: pending"   # the open release PR
gh run watch                                  # after merging it (~2-3 min)
gh release view vX.Y.Z                        # verify the published release
```

The version comes from the commits since the last release: `fix:` bumps PATCH, `feat:`
bumps MINOR, `feat!:` or a `BREAKING CHANGE:` footer bumps MAJOR. Commits like `ci:`,
`docs:`, `test:` and `chore:` alone don't open a release PR. To force a specific version,
add a commit to `main` with a `Release-As: X.Y.Z` footer.

The current version lives in `.github/.release-please-manifest.json`; config is in
`.github/release-please-config.json` (under `.github/` so ClawHub never ships them). There is still no `CHANGELOG.md` (`skip-changelog`).

**Why GoReleaser runs inside the Release Please workflow:** a tag created with the
workflow's own `GITHUB_TOKEN` does not start other workflows, so the tag push would never
fire `release.yml` on its own. The Release Please workflow calls `release.yml` directly
(`workflow_call`) when a release is created. For the same reason, it starts `ci.yml` on
the release PR's branch with `workflow_dispatch`, so the required checks report on it.

### Manual fallback — push a tag yourself

Pushing a tag still runs `release.yml` exactly as before:

```bash
git checkout main && git pull origin main
git tag -a v1.0.1 -m "v1.0.1 — short summary of what changed"
git push origin v1.0.1
```

If you do this, also update `.github/.release-please-manifest.json` to the same version in a PR,
or release-please will propose a version based on its last release.

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

A **MAJOR** bump past v1 also renames the Go module: v2.x lives at
`github.com/stozo04/speediance-cli/v2`, so v3.0.0 needs `/v3` in `go.mod`, every import,
the `go install` commands and `SKILL.md`'s `metadata.openclaw.install` in the PR **before**
the release PR is merged. A release PR proposing 3.0.0 is the signal to do this first.
Without it the Go proxy rejects the tag ("module path must match major version") and
`go install …@latest` keeps serving the old major.

---

## When SHOULD I cut a release?

Release when there are changes on `main` that you want **available as a downloadable
binary** (for a new machine, another person, or an agent). You do **not** release after
every merge — merge freely, let the release PR accumulate changes, then merge it when
there's a meaningful unit.

**Good reasons to release:**
- 🐛 A bug fix users need now → PATCH
- ✨ A new feature/command worth shipping → MINOR
- 💥 A breaking change → MAJOR
- 📦 A batch of dependency updates (e.g. several Dependabot PRs) you want shipped → PATCH

**Don't bother releasing for:**
- Docs-only changes (README/AGENTS/this file)
- Work-in-progress on `main`
- Pure internal refactors with zero user-facing change (optional)

**One release PR = one release** that bundles *everything merged since the last one*. If
you merge 5 PRs and then merge the release PR once, all 5 ride along in that single release.

---

## What triggers what (so nothing surprises you)

| You do this… | …this runs | Result |
|---|---|---|
| Merge the release PR | **Release Please** → `release.yml` (GoReleaser) | Tag `vX.Y.Z` + GitHub Release w/ 6 binaries + `checksums.txt` |
| Push / merge to `main` | **Release Please** | Release PR opened or updated, and CI started on it |
| Push / merge to `main` | **CI** (`ci.yml`) | build & test + lint (no release!) |
| `git push origin vX.Y.Z` (a tag, manual fallback) | **Release** (`release.yml` → GoReleaser) | GitHub Release w/ 6 binaries + `checksums.txt` |
| Push / merge to `main` **that changes a shipped file** (`SKILL.md`, `SECURITY.md`, `config.example.json`, `docs/MACHINE_CONTRACT.md`), `.clawhubignore` or `.gitignore` | **Publish to ClawHub** | Skill republished to ClawHub |
| Run **Deploy to ClawHub (from branch)** by hand | `deploy-clawhub-from-branch.yml` | Branch scanned on a throwaway copy, then deleted |
| Monday 06:17 UTC | **Sweep ClawHub scan copies** | Leftover `speediance-scan-*` copies deleted |
| Open / update a PR | **CI** | build & test + lint |

Merging an ordinary PR **never** creates a release; only merging the release PR does. The
binary's version comes from the git tag — the build stamps it in at release time, so
`speediance-cli version` prints whatever tag it was built from.

---

## Release notes & changelog (auto-generated — no `CHANGELOG.md`)

The de-facto changelog is the **GitHub Releases page** — there is intentionally no
`CHANGELOG.md` to hand-maintain. The release PR body previews the notes; on every `vX.Y.Z`
tag, GoReleaser replaces the GitHub Release body (`release.mode: replace`) **automatically**,
with **zero manual step**:

- **Changelog** — generated from commit subjects since the previous tag, **grouped by
  [Conventional Commit](https://www.conventionalcommits.org) type**: `feat:` → **Features**,
  `fix:` → **Bug fixes**, everything else → **Other changes**. Commits prefixed `docs:`,
  `test:`, or `chore:` are **excluded** entirely.
- **Footer** — a static install snippet (`go install …@<tag>` + a pointer to
  `SKILL.md`/`AGENTS.md`), appended to every release.

Config: `.goreleaser.yaml` → `changelog.groups` and `release.footer`. (There is no
per-release `header`; release-specific narrative, if any, is added by hand on the GitHub
Release after it posts.)

**This is why commit subjects matter** — they *are* the release notes:

- Write Conventional Commits (`feat:`, `fix:`, `docs:`, `test:`, `chore:`) so the changelog
  groups cleanly and release-please picks the right version. A commit with no recognized
  prefix still ships — it just lands under "Other changes" and doesn't bump the version.
- **Squash-merge each PR with a clean Conventional-Commit title** so one PR = one changelog
  line (e.g. squashing a PR with a `rev-2…rev-4` commit trail into a single `fix: …`).
- Want a human highlight on a notable release (a behavior change, a removed command)? Land
  the key change as a clear `feat:`/`fix:` subject, and/or edit the release body on GitHub
  after GoReleaser posts it — the auto changelog + footer are a fine base to add a sentence to.

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

# Prove `go install …@latest` serves this release (the proxy can lag a few minutes):
GOPROXY=https://proxy.golang.org go list -m github.com/stozo04/speediance-cli/v2@latest

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
- **Release PR checks:** if the release PR shows required checks as "Expected — waiting",
  re-run the latest **Release Please** run on `main`; its last step starts CI on the release
  branch.
- **golangci-lint vs. toolchain:** golangci-lint embeds the Go type-checker of the Go
  version it was built with, so the linter must be built with a Go **≥** the toolchain
  it lints under. `ci.yml`'s lint job pins Go `1.26.x` with golangci-lint `v2.12.2`
  (built with go1.26); build/test still run on `stable`. Upgrade the lint
  toolchain and linter together. If you bump the `go` directive in `go.mod` past the linter's
  build version, bump `version:` in the lint job too, or it panics with
  `file requires newer Go version ... (built with ...)`.
- **GoReleaser runs `go mod tidy`** before building — keep `go.mod`/`go.sum` tidy or it
  may complain.
- **ClawHub** publishes automatically on a `SKILL.md` change to `main`; you can also run
  it manually via the **"Publish to ClawHub"** Actions workflow (`workflow_dispatch`).
  ClawHub hides a new version until its security scans finish (about 10 minutes), and the CLI
  picks the next version from the latest public one, so two publishes inside that window collide
  with `Version X already exists`. `scripts/publish-clawhub.sh`, which both ClawHub publish
  workflows call, retries as the next patch version (up to five attempts) and fails on any other error.
- **Scanning a branch without merging:** run **"Deploy to ClawHub (from branch)"** on the branch
  with `target=scan-copy`. It publishes to `speediance-scan-<run id>-<attempt>`, waits for the scan,
  puts the verdict in the run summary and the full report in the `clawhub-scan-report` artifact,
  then deletes the copy. If the report could not be saved it keeps the copy until the next weekly
  sweep, so download the report from ClawHub before then. `target=live`
  republishes the real `speediance` listing from that branch. Every live publish (a merge to
  `main` or a `target=live` branch run) waits in one `clawhub-publish-live` queue (`queue: max`,
  first in, first out), so none cancels or replaces another and a queued main publish is never
  dropped. The weekly **"Sweep ClawHub scan
  copies"** workflow deletes any copy a finished run left behind. Both use
  `scripts/clawhub-scan-copies.sh`, which refuses to delete anything outside the `speediance-scan-`
  prefix. Never publish scan tests by hand to other scratch slugs: nothing cleans those up.
  `make test-scripts` tests both scripts against a fake `clawhub`.

---

## Prerequisites (one-time)

- `gh` CLI authenticated with push access (`gh auth status`).
- Repo setting **Settings → Actions → General → Workflow permissions → "Allow GitHub
  Actions to create and approve pull requests"** must be on, or release-please can't open
  the release PR.
- For local dry runs only: Go + `goreleaser` on your PATH.
- The `CLAWHUB_TOKEN` repo secret must be set (it is) for the ClawHub publish step.
