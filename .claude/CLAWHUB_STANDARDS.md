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

- Scanners do keyword/heuristic matching. A comment like
  `// A missing .env is a silent no-op` next to credential code can itself trip a
  finding. Write security comments to **explain the safeguard** (what we
  deliberately do *not* do and why), not just to narrate behavior. A good comment
  doubles as the reviewer's answer.
- Don't try to "hide" from the scanner by deleting honest comments — fix the
  behavior; the clearer comment is a side effect.

---

## Pre-publish checklist

Run before merging anything that touches config, auth, file I/O, network, or
`SKILL.md`:

- [ ] No `godotenv.Load`/`Overload` — `.env` parsed via `Read` into a map, allowlisted keys only.
- [ ] No `os.Setenv` used to wire configuration.
- [ ] Every secret-bearing file written `0600` (dir `0700`), with the chmod re-tighten pattern.
- [ ] No secret printed to stdout/stderr/logs at any verbosity; passwords masked in `config show`.
- [ ] `SKILL.md` permissions/env/network block matches the code exactly.
- [ ] No new `os/exec`, shell-out, or non-HTTPS / user-supplied network target.
- [ ] `go build ./... && go vet ./... && go test ./...` all green; `gofmt -l` clean.
- [ ] New security behavior is pinned by an **immutable regression test** (below).

## Immutable tests for security behavior

Every security guard gets a test that **fails loudly if the safeguard is
removed** — that's what makes it immutable. Pattern set by
`internal/config/config_test.go`:

- `TestDotEnvDoesNotInjectForeignEnv` — a `.env` with `LD_PRELOAD`/`PATH`/sentinel
  keys must load the `SPEEDIANCE_*` value yet leak **none** of the foreign keys
  into `os.Environ`. Reintroducing `godotenv.Load()` fails this.
- `TestDotEnvNeverMutatesProcessEnv` — even a *consumed* key reaches `Config`
  without appearing in the real environment.

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
