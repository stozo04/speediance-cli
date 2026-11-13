# ClawHub Inspection Standards

Rules for keeping `speediance-cli` clean through ClawHub's security inspection
(the scan that runs on publish — see `.github/workflows/publish-clawhub.yml`).
ClawHub grades the **skill** the way a reviewer would grade an agent tool: it
looks for credential mishandling, privilege escalation, excessive permissions,
and risky patterns. Follow these rules so a real change never trips a real
finding, and so the few false positives are easy to defend.

> **Golden rule:** a credential-collecting tool *will* draw scanner attention.
> Our job is to make every credential/permission/network behavior **minimal,
> documented, and provable with a test** — so any finding is either fixed or
> trivially shown to be a false positive.

---

## 1. Credential & environment loading (the lesson that wrote this file)

**Incident:** `godotenv.Load()` in `internal/config/config.go` exported **every**
key found in a working-directory `.env` into the live process — not just our
`SPEEDIANCE_*` keys. ClawHub flagged it **High / Privilege Escalation →
Credential Access**, and it was a genuine vector: an agent's CWD is
attacker-influenceable, so a stray/hostile `.env` could inject `PATH`,
`LD_PRELOAD`, `HTTP_PROXY`, etc. into the running tool.

**Rules:**

- **Never let untrusted input mutate the process environment.** Parse `.env`
  with `godotenv.Read()` (returns a map) — **never** `godotenv.Load()` /
  `godotenv.Overload()` (which call `os.Setenv` for every key). Apply only the
  keys we own.
- **Allowlist, don't passthrough.** Only the documented `SPEEDIANCE_*` keys may
  flow from a file into config. A new env-driven setting means a new explicit
  key, never a wildcard.
- **A working-directory file is never trusted as auth state.** A `.token.json`
  there is never read or migrated: importing it would let a planted token switch
  the user's account. With no user cache dir, the token isn't cached at all.
- **A working-directory file can't choose paths.** Keys that redirect where config
  is read or a credential is written (`SPEEDIANCE_CONFIG`, `SPEEDIANCE_TOKEN_CACHE`)
  are read from the real exported environment only (`exportedOnly`), never from
  `.env`. The `token_cache_path` config key counts only when the config file came
  from `--config`, an exported `SPEEDIANCE_CONFIG` or the user-config dir, never
  from an implicit `./config.json` (`configSource.mayChoosePaths`). Every ignored
  path setting adds a `Config.Warnings` entry printed to stderr. A new path-like
  key follows the same rule.
- **Do not call `os.Setenv` to wire config.** Resolve values into the `Config`
  struct; leave `os.Environ` untouched. (`os.Getenv`/`os.LookupEnv` for *reading*
  a real exported var is fine and expected.)
- **Keep the precedence contract:** flags > real exported env > `.env` > config.json
  > defaults. Document it where the resolution happens.
- The env var **names** and JSON keys are a frozen external contract (GOAL.md §7)
  — fix the *mechanism*, never rename the keys.

## 2. Secrets on disk (least-privilege file permissions)

- Any file that can hold a credential — `.token.json`, `config.json` — is written
  **`0600`** (owner-only); its parent dir, if created, **`0700`**. See
  `internal/auth/tokencache.go` and `internal/cli/config.go:setConfigKey` for the
  pattern: `os.OpenFile(..., 0o600)` **plus** a best-effort `f.Chmod(0o600)` to
  re-tighten a pre-existing file (`os.WriteFile` will *not* re-restrict an
  existing file).
- Chmod failures on Windows are advisory — ignore them (best-effort), never fail
  the command over them.
- Secret files stay gitignored (`config.json`, `.token.json`, `.env`) and are
  declared as such in `SKILL.md` Conventions. Never commit a real secret, even
  in `testdata/` or an example.

## 3. Never expose secrets in output or logs

- `config show` masks the password (`****`); `config show --json` emits an empty
  password string — **never** dump the secret. Keep it that way.
- No credential, token, `Authorization` header, or full request body containing
  one may reach stdout, stderr, or a log line (even at `--verbose`/`LOG_LEVEL=debug`).
- stdout is for parseable data (`--json`); human hints and logs go to stderr.
  Don't leak secrets across either.

## 4. Least privilege — permissions must match reality

- Keep the `SKILL.md` `metadata.openclaw.permissions` block **exactly** in sync
  with what the code does. Every declared `files.read`, `files.write`, and
  `network` entry must be real; remove anything the code no longer does, and add
  anything new **before** publishing.
- Keep the top-level `permissions` list (`network`, `env`, `file_read`,
  `file_write`) in sync too. SkillSpector's LP3 reads only that list (or
  `allowed-tools`), never the nested `metadata.openclaw` block. Never add
  `shell`: the binary doesn't shell out.
- The `description` must name every local write: the token cache, `config set`
  storing values (including a password) in `config.json`, and the `library`
  catalog file. SkillSpector's TP4 compares the description with the code.
- Agent instructions never treat a passing mention of exercise as a request to
  read the account. `today` runs on an explicit request or after the user
  confirms (SkillSpector SQP-1).
- Request the **narrowest** scope that works. No new file reads/writes, network
  hosts, env vars, or required binaries without updating `SKILL.md` and asking
  whether a narrower option exists.
- `requires.bins` stays `[]` — we ship a single static binary. Don't introduce a
  dependency on `sudo`, a shell, or an external tool. Never shell out to run
  privileged commands.

## 5. No code execution / injection surfaces

- Don't pass user/config/`.env` values into `exec.Command`, a shell, `eval`-like
  APIs, or template-to-code paths. (We have no `os/exec` usage today — keep it
  that way unless there's a strong, reviewed reason.)
- Treat plan JSON and any fetched API data as untrusted input: validate, don't
  execute. All outbound calls stay HTTPS to the documented Speediance API host
  (`internal/api`); no arbitrary or user-supplied URLs.

## 6. Comments and naming around security code

- Security code carries no explanatory comments (house rule). Names, small
  functions and the guard tests carry the meaning; the why lives in the commit
  message, the PR and this file. Doc comments stay only where revive requires
  them on exported identifiers, and say what the identifier is in plain words.
- Never rename, split or encode honest strings (`".env"`, user-facing messages)
  to dodge a scanner regex. Fix the behavior, and record genuine false positives
  below.

## 7. Community-friendly paths — never machine-specific

**Documentation paths must be community-friendly — never machine-specific.** Any
path in tracked docs, comments, examples, or commit messages must be **generic and
portable**: no absolute paths, no home directories, no usernames, no machine-local
install locations (e.g. `C:\Users\NAME\...`, `/home/NAME/...`, `%AppData%\...`).
Use repo-relative paths, placeholders (`<path-to-go>`, `$(go env GOPATH)`, `~`), or
environment variables instead. Leaking a local layout is noise to contributors and
a minor info-disclosure smell; treat the docs as if a stranger will read them,
because once published they will.

The same rule applies to **code, tests, scripts, CI, `Makefile`, and
`.goreleaser.yaml`** — resolve paths at runtime, never bake in a literal one:

- Go: `os.UserConfigDir()`, `os.UserHomeDir()`, `os.TempDir()`, `t.TempDir()` in
  tests, `filepath.Join`, or env lookups — never a literal home dir.
  `internal/config/config.go:discoverConfigPath` already models this with
  `os.UserConfigDir()`.
- Docs / scripts / CI: `$(go env GOPATH)/bin`, `$HOME`, `%USERPROFILE%`,
  `$GITHUB_WORKSPACE`, or repo-relative paths. README and AGENTS.md already use
  `$(go env GOPATH)/bin` — keep that style.
- Build paths with `filepath.Join` (not hardcoded `\` or `/`); tests write only
  under `t.TempDir()`. A fake path used purely as *test data* (e.g. an ignored
  `.env` value like `/tmp/evil.so`) is fine; an actual filesystem dependency on a
  hardcoded absolute path is not.
- Examples and sample configs use neutral placeholders — `you@example.com`,
  `/path/to/plan.json`, `<training_id>` — never a real local path or username.

---

## Pre-publish checklist

Run before merging anything that touches config, auth, file I/O, network, or
`SKILL.md`:

- [ ] No `godotenv.Load`/`Overload` — `.env` parsed via `Read` into a map, allowlisted keys only.
- [ ] No `os.Setenv` used to wire configuration.
- [ ] Path-redirecting keys are read with `exportedOnly`, never from `.env`.
- [ ] `make check-clawhub` passes: it lists the package with clawhub's own `forEachPublishFile` (pinned `CLAWHUB_VERSION`) and fails on any file outside the `SHIPPABLE` allowlist in `scripts/check-clawhub-package.mjs`. Tests, `testdata/`, `scripts/`, `library.json` and agent docs stay out via `.clawhubignore`. CI runs it on every PR and before every publish.
- [ ] Every secret-bearing file written `0600` (dir `0700`), with the chmod re-tighten pattern.
- [ ] No secret printed to stdout/stderr/logs at any verbosity; passwords masked in `config show`.
- [ ] `SKILL.md` permissions/env/network block matches the code exactly.
- [ ] No new `os/exec`, shell-out, or non-HTTPS / user-supplied network target.
- [ ] No machine-specific paths (home dirs, usernames, drive letters) in code, docs, comments, or commit messages — placeholders or runtime resolution only.
- [ ] `go build ./... && go vet ./... && go test ./...` all green; `gofmt -l` clean.
- [ ] New security behavior is pinned by an **immutable regression test** (below).

## Immutable tests for security behavior

Every security guard gets a test that **fails loudly if the safeguard is
removed** — that's what makes it immutable. Pattern set by
`internal/config/config_test.go`:

- `TestDotEnvDoesNotInjectForeignEnv` — a `.env` with `LD_PRELOAD`/`PATH`/sentinel
  keys must load the `SPEEDIANCE_*` value yet leak **none** of the foreign keys
  into `os.Environ`. Reintroducing `godotenv.Load()` fails this.
- `TestDotEnvCannotRedirectPaths` — a `.env` setting `SPEEDIANCE_CONFIG` or
  `SPEEDIANCE_TOKEN_CACHE` must change neither the config file nor the token
  cache path, and must warn. `TestExportedEnvStillRedirectsPaths` keeps the exported override working.
- `TestWorkingDirConfigCannotChooseTokenPath` — a `token_cache_path` in an implicit
  `./config.json` is ignored with a warning; `TestUserDirConfigMayChooseTokenPath`
  keeps it working from the user-config dir.
- `TestDotEnvNeverMutatesProcessEnv` — even a *consumed* key reaches `Config`
  without appearing in the real environment.
- `TestPlantedLegacyTokenIsNeverUsed` (in `internal/cli`) — a `.token.json` in
  the working directory is never read, imported, moved or deleted; the session
  comes from a fresh login and stderr warns (A.I.G T09).
  `TestNoUserCacheDirDisablesTokenCacheInsteadOfUsingWorkingDir` — with no user
  cache dir, caching turns off instead of falling back to the working directory.
- `TestConfigWarningsReachStderr` (in `internal/cli`) — ignored path settings
  print to stderr even with `LOG_LEVEL=error`.
  `TestConfigSetTokenCachePathRefusedInWorkingDir` — `config set` won't write a
  `token_cache_path` that would be ignored.
- `internal/config/skill_doc_test.go`: `TestSkillDocDeclaresTopLevelPermissions`
  (LP3), `TestSkillDescriptionDisclosesLocalWrites` (TP4) and
  `TestAgentDocsRequireExplicitRequestBeforeReadingWorkouts` (SQP-1, checks both
  `SKILL.md` and `docs/MACHINE_CONTRACT.md`).

When you add a safeguard, add the matching test in the same PR. Name it so the
guarantee is obvious, and assert the **negative** (the bad thing does NOT happen),
not just the happy path.

## Handling a finding that is a genuine false positive

If a finding can't be fixed because the behavior is essential and already minimal
(e.g. "this credential-collecting tool reads credentials"):

1. Confirm the behavior really is minimal and documented (permissions in
   `SKILL.md`, precedence in code comments).
2. Prefer a small **hardening** that lowers the finding's severity/confidence over
   doing nothing (that's how the `.env` High became a non-issue — we removed the
   broad env mutation even though *some* credential reading is unavoidable).
3. Record the rationale here and in the PR so the next reviewer doesn't re-litigate
   it. Never silence a scanner by obfuscating honest code.

Known false positives (SkillSpector 2.12.0, read from its source):

- **PE3 "Credential Access" at 60% on `.env`.** The rule is a regex,
  `(?<!\w)\.env(...)?(\s|$|['"])`, that fires on any literal `.env` mention. The
  tool really does read `.env` and declares it, so the remaining hits (SKILL.md
  permissions and env descriptions, the `.env` warning and the missing-credentials
  message in `config.go`) are accepted.
- **"shell" capability.** LP-family rules detect shell via `\bchmod\b`, which
  matches `f.Chmod(0o600)`, the owner-only permission re-tighten. Nothing runs a
  subprocess (`TestSkillDocPromisesNoShellOut`), so `shell` is not declared.
- **LP4 "file_read declared but not detected".** SkillSpector's `file_read`
  patterns are Python and JS shapes (`open(...'r')`, `read_text`, `os.listdir`);
  Go's `os.ReadFile` doesn't match. The binary does read config, plan and token
  files, so `file_read` stays declared.
- **A shipped baseline can't suppress these.** SkillSpector applies an author's
  `.skillspector-baseline.yaml` only with `--use-shipped-baseline`, and clawhub
  never ships dotfiles.
