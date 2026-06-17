# GOAL — Rewrite `speediance-cli` in Go

> **One-line objective:** Reimplement the Python `speediance-cli` as a single, statically-compiled,
> idiomatic Go CLI that is a **drop-in replacement** for the existing tool — same commands, same
> credentials, same API behavior, same `--json` output — while modernizing the internals, error
> handling, and distribution so it's a high-quality, maintainable open-source project for the
> OpenClaw / agent community.

This document is the specification to build against. It is grounded in current (2025/2026) Go
best practices (sources in [§20](#20-references)). The author is **not** a Go developer, so the
resulting code and docs must be idiomatic and self-explanatory enough for a non-Go maintainer to
keep healthy.

---

## 1. Context

`speediance-cli` is a ~800-line Python package that wraps the **reverse-engineered** Speediance
(Gym Monster) cloud API. It lets a human or an AI agent read completed workouts and push custom
training programs that appear on the machine. It ships today as:

- a `pip install git+…` package (entry point `speediance-cli`, also `python -m speediance`), and
- a **ClawHub skill** (`SKILL.md`) auto-published by a GitHub Action — installed and driven by
  OpenClaw / Claude-style agents.

Because agents and the published skill consume this tool, **the external contract is load-bearing**:
other people's automation parses our `--json` stdout, and the Speediance servers reject anything
whose request payloads/headers aren't byte-for-byte what the Android app sends.

### Source of truth (the Python implementation)

| File | Responsibility |
|---|---|
| `speediance/cli.py` | argparse CLI: `login`, `workouts`, `session`, `library`, `push`, `sync` |
| `speediance/client.py` | HTTP client (`requests`): auth, token refresh, endpoints, region base URLs |
| `speediance/config.py` | config JSON + env-var overrides; token cache file (`0600`) |
| `speediance/models.py` | `Workout`, `SetData` dataclasses; record parsing; timestamp normalization |
| `speediance/templates.py` | build/push `customTrainingTemplate` payload; fetch exercise library; `KG_TO_API` |
| `speediance/sheet.py` | optional Markdown `WEEKS/Week-XX.md` sync with fuzzy exercise matching |
| `tests/` | two offline (no-network) assert scripts |
| `pyproject.toml`, `SKILL.md`, `.github/workflows/publish-clawhub.yml` | packaging + skill publish |

---

## 2. Guiding principles (the non-negotiable contract)

These hold regardless of how freely we "modernize" elsewhere.

1. **API wire format is frozen.** Every request URL, query param, JSON body field name, CSV-encoded
   value, and HTTP header must be **byte-identical** to the Python client. The server is an
   unofficial, brittle target; deviation breaks auth or program creation. See [§8](#8-api-client-spec)
   and [§10](#10-pushtemplate-payload-spec--fidelity-critical).
2. **`--json` stdout is backward-compatible.** Field names, nesting, and types of all `--json`
   output match the Python tool exactly (field *order* preserved as a courtesy). Existing agents and
   the ClawHub skill must keep working with **zero** changes. See per-command schemas in [§9](#9-commandbycommand-port-spec).
3. **stdout = data, stderr = everything else.** Machine-readable JSON only on stdout; all human
   hints, progress, warnings, and logs go to stderr. Never interleave. See [§13](#13-logging--output-discipline).
4. **Drop-in credentials.** Same env vars and the same `config.json` / `.token.json` discovery, so a
   current user (who already has a cached token) doesn't have to reconfigure. See [§7](#7-configuration--credentials).
5. **Exit codes preserved.** Auth failure → exit `2`; missing-credentials / missing-`--weeks-dir` →
   non-zero hard error; success → `0`. See [§12](#12-error-handling--exit-codes).

Everything *not* on this list — internal structure, error wording, retries, logging, new
convenience commands — is fair game to improve.

---

## 3. Non-goals

- **No new device support guarantees.** Gym Monster 1 (`device_type = 1`) is the only tested device.
  GM2 stays explicitly **untested**; carry the warning forward verbatim.
- **No re-engineering of the unofficial API** (no new endpoints, no scraping, no caching layer beyond
  the existing token cache).
- **No GUI/TUI, no daemon, no server mode.** It stays a one-shot CLI.
- **No telemetry / analytics.**
- **No breaking changes to the `--json` schemas** in v1 (per [§2](#2-guiding-principles-the-nonnegotiable-contract)).
- **Not a public Go library.** All non-entrypoint code lives under `internal/` so we can refactor
  freely without API-compatibility obligations.

---

## 4. Tech stack & key decisions

| Area | Decision | Rationale |
|---|---|---|
| Language / version | Go, `go 1.24` directive; CI builds with `stable` | Recent-but-not-bleeding-edge; `go` directive is a hard minimum, newer toolchains auto-resolve |
| Module path | `github.com/stozo04/speediance-cli` | Replaces the Python repo **in place**; same import/install path |
| Binary name | `speediance-cli` (entry `cmd/speediance-cli/main.go`) | True drop-in; existing docs/skill say `speediance-cli` |
| CLI framework | `spf13/cobra` | Standard for Go CLIs (gh, kubectl); clean subcommands, help, completions |
| Config | **stdlib** `encoding/json` + `os.LookupEnv` + Cobra flags (no Viper) | A 6-command CLI doesn't need Viper's transitive weight; full control of precedence |
| HTTP retries | `hashicorp/go-retryablehttp` (GET/idempotent only) | Sane default policy, honors `Retry-After`; exposes a plain `*http.Client` |
| Fuzzy match | `hbollon/go-edlib` + stdlib Sørensen-Dice token overlap | Actively maintained, normalized `[0,1]` ratio; "good-enough" parity for `sync` |
| JSON | stdlib `encoding/json` (Encoder, HTML-escape off) | Streaming, clean machine output |
| Logging | stdlib `log/slog` → **stderr** | No need for zap/zerolog at CLI scale |
| Tests | stdlib `testing` + `net/http/httptest` + `google/go-cmp`; `testify/require` only for guards | Idiomatic, great diffs, minimal deps |
| Lint/format | `golangci-lint` v2 (`default: standard` + a few extras) + `gofumpt`/`goimports` | Single meta-linter bundles vet/staticcheck/errcheck |
| Release | GoReleaser v2 + GitHub Actions on `v*` tags | Cross-platform binaries, checksums, changelog |
| Install paths | release binaries **and** `go install …/cmd/speediance-cli@latest` **and** ClawHub skill | All three distribution channels requested |

**Deliberately small dependency set:** `spf13/cobra`, `hashicorp/go-retryablehttp`, `hbollon/go-edlib`
(+ test-only `google/go-cmp`, optional `testify`). No Viper, no third-party logging.

---

## 5. Proposed repository layout

Thin `cmd/` entrypoint + closed `internal/` tree (the `gh` pattern, minus `pkg/` since we expose no
library). Do **not** adopt `golang-standards/project-layout`.

```
speediance-cli/
  go.mod                          # module github.com/stozo04/speediance-cli ; go 1.24
  go.sum
  LICENSE                         # MIT (carry over)
  README.md  AGENTS.md  SKILL.md  # rewritten for the Go tool (see §15)
  library.json                    # carry over the committed GM1 catalog snapshot
  .goreleaser.yaml
  .golangci.yml
  .gitignore                      # config.json, .token.json, .env, dist/, plans/
  .github/workflows/
    ci.yml                        # build + test -race + lint on push/PR
    release.yml                   # goreleaser on v* tags
    publish-clawhub.yml           # carry over; trigger on SKILL.md change
  cmd/speediance-cli/
    main.go                       # thin: NewRootCmd().ExecuteContext(); map error→exit code; os.Exit
  internal/
    cli/                          # cobra wiring — one file per command
      root.go                     # NewRootCmd(), App struct, PersistentPreRunE (load config)
      login.go workouts.go session.go library.go push.go sync.go
      config.go version.go        # new commands
      exit.go                     # ExitError{Code,Err} type
      output.go                   # writeJSON helper, stdout/stderr discipline
    api/
      client.go                   # *http.Client, headers, auth, token refresh (code 91), do/doJSON
      endpoints.go                # workouts/session/library/template calls
      types.go                    # request + response structs with JSON tags
    config/
      config.go                   # defaults → file → env → flags; discovery; validation
    auth/
      tokencache.go               # read/write .token.json at 0600
    template/
      template.go                 # build payload (KG_TO_API, unilateral, capacity), create
    workout/
      model.go                    # Workout/SetData equivalents, record parsing, ts normalization
    sheet/
      sheet.go                    # WEEKS/Week-XX.md writer
      match.go                    # fuzzy matching (go-edlib + token overlap)
    version/
      version.go                  # ldflags vars + debug.ReadBuildInfo fallback
  testdata/                       # golden files, sample WEEKS sheet, recorded API JSON, sample plans
```

> `cmd/speediance-cli/main.go` (vs a root `main.go`) gives `go install` a clean binary name and room
> for a second tool later (e.g. `cmd/gen-docs`). Cobra commands live in `internal/cli`, not `cmd/`.

---

## 6. Command surface (parity + additions)

| Command | Status | `--json` | Notes |
|---|---|---|---|
| `login` | port | — | authenticate, cache token |
| `workouts [--days N]` | port | yes | list recent completed sessions |
| `session <training_id>` | port | yes | per-set detail for one session |
| `library [--search X] [--out FILE]` | port | yes | dump/search exercise catalog |
| `push <plan.json> [--dry-run]` | port | yes | create program (dry-run previews payload) |
| `sync [--date D] [--days N] [--weeks-dir DIR]` | port | — | optional Markdown sheet write |
| `config [show\|set\|path]` | **new** | yes (`show`) | manage `config.json` without hand-editing |
| `version` | **new** | yes | build metadata; plus root `--version` |
| `completion [bash\|zsh\|fish\|powershell]` | **new** | — | Cobra built-in shell completions |

Global persistent flags on root: `--config PATH`, `--verbose/-v`. Each command keeps its existing
local flags. Use `ExecuteContext(ctx)` so a cancelable context reaches every command for HTTP
timeouts and Ctrl-C.

---

## 7. Configuration & credentials

### Inputs (names are part of the contract — do not rename)

| Source | Keys |
|---|---|
| Env vars | `SPEEDIANCE_EMAIL`, `SPEEDIANCE_PASSWORD`, `SPEEDIANCE_REGION`, `SPEEDIANCE_DEVICE_TYPE`, `SPEEDIANCE_WEEKS_DIR`, `SPEEDIANCE_CONFIG`, `SPEEDIANCE_TOKEN_CACHE` |
| `config.json` keys | `email`, `password`, `region`, `unit`, `device_type`, `weeks_dir` |
| Defaults | `region="Global"`, `unit="lb"` (label only), `device_type=1`, `weeks_dir=""` |

### Precedence (modernized)

`flags > env > config file > defaults`. (Python only did env > file; flags-win is the new, idiomatic
layer. A flag overrides only when actually *set* — check `cmd.Flags().Changed(...)`; env uses
two-return `os.LookupEnv` to distinguish set-empty from unset.)

### File discovery (drop-in, with an optional modern fallback)

1. `--config PATH` (new flag) or `SPEEDIANCE_CONFIG` env → explicit path.
2. else `config.json` in the current working directory (**preserves today's behavior** and the skill
   docs).
3. *(enhancement)* else `<os.UserConfigDir>/speediance/config.json` — documented as optional, never
   required. Keep CWD as the default so existing agents/skills are unaffected.

Token cache: `SPEEDIANCE_TOKEN_CACHE` or `.token.json` in CWD by default (same as Python).

### Validation / errors

- Missing `email` or `password` after resolution → friendly error to **stderr** + non-zero exit
  (preserve Python's message intent: tell the user to set env vars or `config.json`).
- `sync` without a `weeks_dir` (flag/env/config) → hard error explaining only `sync` needs it.
- `device_type` parsed as int; carry the GM2-untested warning.

### Token cache file (security)

- Write with `os.OpenFile(path, O_CREATE|O_WRONLY|O_TRUNC, 0o600)` (NOT `os.WriteFile`, which won't
  re-restrict perms on an existing file). `MkdirAll` parent `0700` if using a non-CWD location.
- On Windows, `0600` is best-effort — must **not** fail the command if chmod is unsupported (mirror
  Python's `try/except`).
- Contents unchanged: `{"token": "...", "user_id": "..."}`. After every successful API call the
  refreshed token/user_id is written back (as Python does).

> **Security note for the maintainer:** `config.json` stores the password in plaintext today and is
> gitignored. Keep it gitignored. Prefer env vars for agents/headless use. An OS-keychain backend is
> a possible *future* enhancement, **out of scope** for v1.

---

## 8. API client spec

`internal/api/client.go`. Build a **custom `*http.Client`** (never `http.DefaultClient`) with an
explicit timeout (~20–30s like Python), wrapped by `go-retryablehttp` for transient retries on
**idempotent GETs only**.

### Region base URLs (frozen)

```
Global → https://api2.speediance.com/api
EU     → https://euapi.speediance.com/api
```
Unknown region falls back to Global (as Python does). `Host` header derived from the base URL host.

### Headers (frozen — must match the Android app byte-for-byte)

```
Host: <derived from base>
User-Agent: Dart/3.9 (dart:io)
Content-Type: application/json
Timestamp: <current epoch milliseconds, as string>
Utc_offset: +0000
Timezone: GMT
Versioncode: 40304
Accept-Language: en
App_type: SOFTWARE
Mobiledevices: {"brand":"google","device":"emulator64","deviceType":"sdk_gphone64","os":"","os_version":"31","manufacturer":"Google"}
Token: <token>            # only when present
App_user_id: <user_id>    # only when present
```
`User-Agent` here is the spoofed Dart string — **do not** replace it with `speediance-cli/<ver>`.
(Our own version string belongs only in `--version`/logs.)

### Auth flow (two-step, frozen)

1. `POST /app/v2/login/verifyIdentity` body `{"type":2,"userIdentity":<email>}`.
   - `code != 0` → `AuthError(verifyIdentity failed: <message>)`.
   - `data.isExist == false` → `AuthError("Account does not exist…")`.
   - `data.hasPwd == false` → `AuthError("Account has no password set…")` (Google/SSO accounts).
2. `POST /app/v2/login/byPass` body `{"userIdentity":<email>,"password":<pw>,"type":2}`.
   - `code != 0` → `AuthError(Login failed: <message>)`.
   - on success: `token = data.token`, `user_id = str(data.appUserId)`.

### Token expiry refresh (frozen behavior)

Every GET/POST: if response `code == 91`, re-`login()` once, then retry the **same** request once.
This is an *application-level* retry distinct from transport retries; it applies to POSTs too
(safe — a 91 means the request was rejected unprocessed). Lazy login: if no token is set when a
request is made, log in first.

### Retry policy (new, additive)

- `go-retryablehttp`: retry on connection errors and 5xx; honor `Retry-After` on 429;
  `RetryMax ≈ 3`, exponential backoff.
- **Restrict transport retries to safe methods** (GET/HEAD). Do **not** auto-retry the `login` or
  `push`/`customTrainingTemplate` POSTs at the transport layer — only the deliberate code-91 single
  retry — to avoid duplicate logins or duplicate program creation.
- Set the retry client's logger to a slog adapter on **stderr** (or nil) so retries never pollute
  stdout.

### Decode discipline

- `defer resp.Body.Close()`, check status, cap reads with `io.LimitReader`.
- Decode the standard envelope `{ "code": int, "message": string, "data": <any> }`. Use
  `json.RawMessage` for `data` where the shape depends on the endpoint, and decode loosely — **do
  not** use `DisallowUnknownFields` on API responses (resilience to new server fields).

---

## 9. Command-by-command port spec

For each: behavior, flags, **exact stdout JSON shape** (the contract), stderr, exit. Use a shared
`writeJSON` (encoder, `SetEscapeHTML(false)`, `SetIndent("", "  ")`, trailing newline) so output
matches Python's `json.dumps(indent=2)`.

### 9.1 `login`
- Authenticate (no token cache used), then write `.token.json`.
- stdout (human): `Logged in (user <id>). Token cached to <path> — keep this file private.`
- stderr: the "do not share this file" note.
- No `--json`. Auth failure → exit 2.

### 9.2 `workouts [--days N] [--json]`  (default `--days 3`)
- `fetch_workouts(days)`: `start = (today+1d) - N days`, `end = today+1d`; GET
  `/mobile/v2/report/userTrainingDataRecord?startDate=<ISO>&endDate=<ISO>`. `code != 0` → empty list
  (+ stderr warning). Write back token cache.
- **stdout `--json`** — array of:
  ```json
  {"training_id": 0, "title": "", "date": "YYYY-MM-DD|null",
   "duration_secs": 0, "calories": 0, "volume": 0.0, "type": ""}
  ```
- stdout (human): `Found N session(s)…` then `- <date>  <title>  -  <min> min, <kcal> kcal  (id <id>)`;
  empty → `No completed workouts in the last N day(s).`

### 9.3 `session <training_id> [--json]`
- `fetch_detail`: GET `/app/trainingInfo/cttTrainingInfo/<id>` → `completionRate`; GET
  `/app/trainingInfo/cttTrainingInfoDetail/<id>` → list; per exercise: `actionLibraryName`,
  `maxWeight`, and `finishedReps[]` items `{finishedCount, targetCount, weight, capacity,
  maxHeartRate, leftRight}`. Group sets by exercise name, first-seen order.
- **stdout `--json`**:
  ```json
  {"training_id": 0, "completion_rate": 0.0,
   "exercises": [{"name": "", "sets": [
     {"set": 1, "reps": 0, "target_reps": 0, "weight": 0.0, "max_hr": 0.0, "left_right": 0}]}]}
  ```
- **Edge:** "Free Lift" / freestyle sessions return no per-set detail → `exercises: []`; human mode
  prints the "no per-set detail (freestyle…)" message.

### 9.4 `library [--search X] [--out FILE] [--json]`  (default `--out library.json`)
- `fetch_library(device_type)`:
  1. GET `/app/actionLibraryTab/list?deviceType=<dt>` → tabs; **skip** tabs where `isCustom` truthy.
  2. per tab: GET `/app/actionLibraryGroup/trainingPartGroup?tabId=<id>&deviceTypeList=<dt>`; collect
     each action `{id, name:title, muscle:"", tab:tabName}`, **dedupe by id**.
  3. enrich muscle: for ids in **chunks of 50**, GET `/app/actionLibraryGroup/list?ids=…&ids=…` →
     set `muscle = mainMuscleGroupName`.
- **Always writes** the full catalog to `--out` (stderr: `Saved N exercises to <file>`).
- `--search` filters by case-insensitive substring in name or muscle.
- **stdout `--json`** — array of `{"id":0,"name":"","muscle":"","tab":""}` (filtered if `--search`).
- Human + `--search`: `N match '<q>':` then `[id] name (muscle)` (first 60). Human without search:
  nothing on stdout (only the stderr save line).

### 9.5 `push <plan.json> [--dry-run] [--json]`
- `load_plan`: JSON must contain `name` and `exercises` (else error). Plan shape:
  ```json
  {"name":"Pull Day","exercises":[
    {"id":434,"title":"…","sets":[{"reps":12,"weight":20,"mode":1,"rest":75}]}]}
  ```
- stderr summary: `Plan: <name> - <nEx> exercises, <nSets> sets`.
- `--dry-run`: build payload, **do not POST**. `--json` prints the payload (see [§10](#10-pushtemplate-payload-spec--fidelity-critical));
  human prints `[dry-run] totalCapacity=…; not sent.` and per-action `groupId … reps … | weights …`.
- real: `POST /app/v2/customTrainingTemplate`; `code != 0` → error. `--json` prints response `data`;
  human prints `Created '<name>' on your Speediance account…`.

### 9.6 `sync [--date D] [--days N] [--weeks-dir DIR]`  (default `--date today`, `--days 3`)
See [§11](#11-sync-command--fuzzy-matching). Requires a weeks dir; no `--json`.

### 9.7 `config` (new)
- `config show [--json]` — print resolved effective config with **password masked** (`****`).
  `--json` emits a stable object (omit/blank the password). This is a convenience view; it is *not*
  a Python-parity surface.
- `config set <key> <value>` — write/update `config.json` (the discovered/`--config` path), creating
  it if absent; restrict perms to `0600` since it holds a secret.
- `config path` — print the resolved config + token-cache paths to stdout.

### 9.8 `version` (new) + `--version`
- `version [--json]` prints `{ "version", "commit", "date", "go" }`; root `--version` prints the short
  version string. Values via ldflags with `debug.ReadBuildInfo()` fallback (see [§14](#14-version-information)).

---

## 10. Push/template payload spec — *fidelity critical*

`internal/template`. This is the highest-risk parity surface: the produced JSON is the **API wire
body** and the weight math determines real loads on a physical machine.

### Constants & resolution
- `KG_TO_API = 2.2` (kg → internal API units).
- `group_ids` = unique int `id`s across plan exercises.
- **Variant id:** `GET /app/actionLibraryGroup/list?ids=…` → for each `d`, `actionLibraryId =
  d.actionLibraryList[0].id`. Unresolved id → error naming the id/title and telling the user to run
  `library`.
- **Unilateral:** per group, `GET /app/actionLibraryGroup/<id>?isDisplay=1` → `data.isLeftRight == 1`.

### Per-set encoding (0-based set index `i`)
- `reps_list[i] = str(reps)`; `break_list[i] = str(rest)` (default rest 60); `mode_list[i] = str(mode)`
  (default 1); `level_list[i] = "0"`.
- `leftRight[i] = ("1" if i%2==0 else "2")` when unilateral, else `"0"`.
- `completionMethod[i] = "1"`, `countType[i] = "1"`, `selectCompletionMethod[i] = "1"`.
- `api_weight = weight * 2.2`; `weights[i] = format(api_weight, 1 decimal)` → e.g. `44.0`
  (Go: `strconv.FormatFloat(api_weight, 'f', 1, 64)`).
- `set_capacity += reps * api_weight`.

### Action object (field names + CSV strings frozen)
```json
{"groupId": <int>, "actionLibraryId": <int>, "templatePresetId": -1,
 "setsAndReps": "12,10", "breakTime": "75,75", "breakTime2": "75,75",
 "sportMode": "1,1", "leftRight": "0,0", "selectCompletionMethod": "1,1",
 "completionMethod": "1,1", "countType": "1,1", "weights": "44.0,49.5",
 "counterweight2": "", "level": "0,0", "capacity": <float>}
```
Note `breakTime2` duplicates `breakTime`; `counterweight2` is `""`.

### Top-level payload (frozen)
```json
{"name": <string>, "actionLibraryList": [ …actions… ],
 "totalCapacity": <sum of set_capacity>, "deviceType": <int>, "bgColor": 0}
```

**Test obligation:** golden-file the payload for `plans/example-push.json` and `plans/week-01-legs.json`
and assert byte-equality with the Python output (e.g. `weights == "44.0,49.5"` for kg `20, 22.5`).

### mode reference (for docs)
`1=Standard, 2=Eccentric, 3=Isokinetic, 4=Constant, 5=Spotter`. `rest` seconds. `weight` kilograms.

---

## 11. `sync` command + fuzzy matching

Optional integration that writes a completed session into a Markdown `WEEKS/Week-XX.md` checklist.
Behavior to preserve; **matching may be "good enough" (not byte-identical to Python's `difflib`).**

### Flow
1. Resolve `weeks_dir` (flag → env → config) else hard error.
2. Resolve date: `today` | `yesterday` | `YYYY-MM-DD`.
3. `fetch_workouts(max(days,1))`; keep those whose `.date == target` (local-time interpretation —
   see [§12 edge](#workout-date-edge)). None → message + return.
4. `find_week_sheet`: glob `Week-*.md` sorted; pick the file containing a `^##.*\b<m/d>\b` header,
   else the highest-numbered file. None → message.
5. For each session: `fetch_detail`, then `write_session`.

### `write_session` (preserve structure)
- Date token: `"<month>/<day>"` with **no leading zeros** (e.g. `6/15`).
- Checkboxes: `☐` empty / `☑` done (exact Unicode).
- Locate the day's workout section (`##` line containing the token, not "Notes").
- Inside it, for each 4-column table row whose first cell is a checkbox: fuzzy-match the exercise cell
  against workout exercise names; on a match, set the box to `☑` and fill the weight cell with a
  compact `"<w><unit>×<reps>, …"` string. Each workout name used at most once.
- Flip the at-a-glance row's trailing checkbox for the date.
- Build a full "**Logged from Speediance — <title>** (<m/d>) - <min> - <kcal> [- <pct>% complete]"
  block listing every exercise (captures unmatched too); insert under the matching Notes bullet, else
  append a `## Logged Sessions` section.
- Return `{sheet, matched, unmatched, exercise_count}`; CLI prints matched/total and any unmatched.

### Fuzzy matcher (`internal/sheet/match.go`)
Python used `0.5*SequenceMatcher.ratio() + 0.5*token_overlap`, threshold **0.45**, after normalization
(lowercase, strip parentheticals, `dumbbell→db`, `barbell→bb`, synonyms `rdl→romanian deadlift`,
`ohp→overhead press`, stopword removal). Reproduce the normalization, and compute similarity as a
**weighted blend** using `hbollon/go-edlib` (e.g. Jaro-Winkler or Levenshtein ratio) + a stdlib
Sørensen-Dice over token sets:

```
score = 0.5*edlib.StringsSimilarity(a,b,algo) + 0.5*sorensenDice(tokens(a),tokens(b))
match if score >= threshold (start at 0.45, tune on fixtures)
```
Tune weights/threshold/algo against the real `tests/sample_week.md` + `make_workout()` fixtures so the
existing test's expectations (≥6 boxes flipped, weights written, glance checked) still pass. Document
that exact byte-parity with Python is **not** a goal here. If perfect parity were ever required, the
fallback is to vendor `pmezard/go-difflib`'s `Ratio()` (a real Ratcliff-Obershelp port, but
unmaintained) into `internal/sheet`.

---

## 12. Error handling & exit codes

- Wrap errors with `%w` at boundaries that add context; use sentinel errors (`var ErrAuth = …`) for
  parameterless conditions and typed errors where structured data (status code) matters; match with
  `errors.Is` / `errors.As`.
- **Single exit point.** `main()` calls `run() int` which executes Cobra with `ExecuteContext`; map
  errors to codes via a typed `ExitError{Code int; Err error}` checked with `errors.As`. No `os.Exit`
  inside command bodies (it skips deferred cleanup).
- Set root `SilenceUsage = true` and `SilenceErrors = true`; print the single error line to **stderr**
  yourself (keeps stdout clean and avoids dumping usage on runtime failures).

| Condition | Exit |
|---|---|
| Success | 0 |
| Auth error (`AuthError`) | **2** (preserve Python) |
| Missing credentials / missing `--weeks-dir` / bad plan / unresolved exercise id | non-zero (1, or a dedicated code) |
| Any other error | 1 |

<a name="workout-date-edge"></a>
**Edge — timestamp/date:** a record's date comes from `start_timestamp || end_timestamp`; if the value
is `> 1e12` treat it as **milliseconds** (`/1000`), else seconds; convert with **local time** (Python
`datetime.fromtimestamp`) — this determines which calendar day a session maps to. Decode the raw
number via `json.Number`/`UseNumber()` to avoid float lossiness, then normalize in a typed accessor.

---

## 13. Logging & output discipline

- **stdout:** machine output only — JSON under `--json`, or the specific human strings the Python
  commands print (those are the de-facto human UI; keep them on stdout to match behavior). Use the
  `writeJSON` encoder with HTML escaping **off**.
- **stderr:** all hints, warnings, progress, and `log/slog` diagnostics. Default level Warn; `--verbose`
  → Debug; optional `LOG_LEVEL` env override. Text handler for humans; a `--log-format=json` (stderr)
  is a nice-to-have, not required.
- Commands must write via `cmd.OutOrStdout()` / `cmd.ErrOrStderr()` (not `os.Stdout` directly) so tests
  can capture output.

---

## 14. Version information

Expose build metadata robustly whether built by GoReleaser or `go install`:
- ldflags `-X main.version=… -X main.commit=… -X main.date=…` (set by GoReleaser).
- Fallback to `runtime/debug.ReadBuildInfo()` for `bi.Main.Version` and `vcs.revision/time/modified`
  (populated by `go install pkg@vX` and `go build` in a repo).
- **Gotcha:** `-X main.version` only sets a var in `package main`. Either declare the ldflags vars in
  `cmd/speediance-cli/main.go` (package main) and pass them into `internal/version`, **or** point
  ldflags at the full path `…/internal/version.version`. Pick one and document it in `.goreleaser.yaml`.

---

## 15. Documentation updates

Rewrite the three docs so they describe the Go tool — **drop the `python3` requirement everywhere**:

- **README.md** — install via release binary / `go install github.com/stozo04/speediance-cli/cmd/speediance-cli@latest`; same command examples; keep the GM1/GM2 + unofficial-API + credits notes.
- **AGENTS.md** — same command surface and plan schema; replace pip/clone-Python setup with binary
  download / `go install`; keep the stdout-parseable + secrets-gitignored conventions.
- **SKILL.md** — update `requires.bins` from `python3` to none (bundled binary) or document the
  `go install`/download step; keep `envVars`, permissions, and emoji. The auto-publish Action stays.
- Carry over `library.json`, `LICENSE` (MIT), `config.example.json`, `.env.example`, and the
  `plans/` examples (still gitignore `plans/`).

---

## 16. Testing strategy

Target **behavioral parity**, verified offline (no network), mirroring and extending the two Python
tests.

- **Table-driven** tests with subtests; `t.TempDir()`, `t.Setenv()`. (Go 1.22+ needs no loop-var
  capture.)
- **API client** via `net/http/httptest`: stand up a server returning recorded Speediance JSON
  fixtures (store under `testdata/`), point the client base URL at `server.URL`; assert the client
  parses workouts/session/library correctly and handles `code == 91` re-login (server returns 91 once,
  then 0) and region selection.
- **Template payload** golden tests: build payloads for `plans/example-push.json` and
  `plans/week-01-legs.json`; assert byte-equality with checked-in golden JSON (the frozen wire format),
  including `weights == "44.0,49.5"`, alternating `leftRight` for a unilateral fixture, and
  `totalCapacity`.
- **Sheet/sync**: port `tests/test_sheet.py` — copy `sample_week.md` to a temp file, write a synthetic
  session, assert ≥6 boxes flipped, weights present, "Logged from Speediance" block, glance row checked,
  no blank weight cells before `## Notes`.
- **Fuzzy matcher**: unit-test normalization + scoring on the known sheet-vs-Speediance name pairs;
  assert the intended matches clear the threshold and bad pairs don't.
- **Config precedence**: defaults < file < env < flags, including unset-vs-empty.
- **Cobra commands**: `cmd.SetArgs`, capture `SetOut`/`SetErr` buffers, assert stdout JSON shape and
  that hints went to stderr; fresh command per test (no sticky global flags).
- Use stdlib `testing` + `google/go-cmp` for struct/JSON diffs; `testify/require` only for terse
  nil/err guards. Run `go test -race ./...` in CI.

---

## 17. Lint / format / CI

- **Formatting (enforced):** `gofumpt` + `goimports` (local-prefix `github.com/stozo04/speediance-cli`);
  CI fails if formatting drifts.
- **golangci-lint v2**, `.golangci.yml` with `linters.default: standard` plus `revive, gosec, misspell,
  bodyclose, errorlint, nilnil` (start conservative, not `all`). `bodyclose` guards the HTTP client.
  Don't separately run staticcheck (bundled); `go vet ./...` is fine as a free extra.
- **`ci.yml`** (push/PR): `actions/setup-go@v6` with `go-version: stable` → `go build ./...` →
  `go test -race ./...`; separate `lint` job with `golangci/golangci-lint-action`.
- **go.mod:** `go 1.24`, omit a `toolchain` directive.

---

## 18. Distribution & release

### GoReleaser v2 (`.goreleaser.yaml`, starts with `version: 2`)
- `builds`: `main: ./cmd/speediance-cli`, `binary: speediance-cli`, `CGO_ENABLED=0`,
  `goos: [linux, darwin, windows]`, `goarch: [amd64, arm64]`, `ldflags: -s -w -X main.version=… -X
  main.commit=… -X main.date=…`, `mod_timestamp`.
- `archives` tar.gz (zip override for Windows); `checksums` sha256; `changelog: use: github`.
- (Later, optional: Homebrew tap, cosign signing, SBOM.)

### Release workflow (`.github/workflows/release.yml`, on `v*` tags)
- `permissions: contents: write`; checkout with `fetch-depth: 0`; `setup-go` stable;
  `goreleaser-action@v7` with `args: release --clean`, `GITHUB_TOKEN` from secrets.

### `go install`
- Module path = repo path; `go install github.com/stozo04/speediance-cli/cmd/speediance-cli@latest`
  resolves the highest **semver** tag. Tag releases as `vMAJOR.MINOR.PATCH`. (These builds lack ldflags
  vars → version falls back to `ReadBuildInfo`.)

### ClawHub skill
- Keep `publish-clawhub.yml` (trigger on `SKILL.md` change). Update `SKILL.md` so the skill no longer
  requires `python3` — it downloads the release binary or uses `go install`. Verify the skill installs
  and runs end-to-end without a Python interpreter.

---

## 19. Phased milestones (Definition of Done per phase)

- [ ] **P0 — Scaffold.** `go.mod` (`github.com/stozo04/speediance-cli`, `go 1.24`), layout from
      [§5](#5-proposed-repository-layout), `cmd/speediance-cli/main.go` shim + `internal/cli/root.go`,
      `version` cmd, `.golangci.yml`, `ci.yml`. `go build`/`go test`/lint green on an empty skeleton.
- [ ] **P1 — Config + auth + client.** `internal/config`, `internal/auth`, `internal/api` (headers,
      regions, two-step login, code-91 refresh, retry policy). `login` command works against fixtures;
      token cache `0600`. Unit tests via httptest.
- [ ] **P2 — Read commands.** `workouts`, `session` with exact `--json` schemas + human output +
      stdout/stderr discipline. Tests assert schema parity.
- [ ] **P3 — Library.** `library` (tab walk, dedupe, 50-id muscle chunks, `--out`, `--search`, `--json`).
- [ ] **P4 — Push.** `internal/template` payload (KG×2.2, unilateral, capacity, frozen field names),
      `--dry-run`, real create. **Golden-file byte-parity** tests against the sample plans.
- [ ] **P5 — Sync.** `internal/sheet` writer + fuzzy matcher; port the sheet test; tune threshold.
- [ ] **P6 — Modern extras.** `config` command, `completion`, richer errors, HTTP retries, `--version`.
- [ ] **P7 — Test/parity pass.** Recorded-fixture comparison of all `--json` outputs vs the Python tool;
      `go test -race` green; lint clean.
- [ ] **P8 — Docs.** README / AGENTS / SKILL rewritten (no `python3`); examples verified.
- [ ] **P9 — Release.** `.goreleaser.yaml` + `release.yml`; cut `v1.0.0`; verify binaries for 3 OSes,
      `go install …@latest`, and ClawHub skill install.
- [ ] **P10 — Repo cutover.** Land the Go tree on a branch in `stozo04/speediance-cli`, PR-review,
      remove Python sources, merge, tag. (See [§21](#21-repo-cutover-plan).)

---

## 20. Acceptance criteria

1. Given identical API responses (recorded fixtures), every `--json` command emits JSON whose field
   names, nesting, and types match the Python tool exactly.
2. `push --dry-run --json` for the sample plans is **byte-identical** to the Python payload.
3. Exit codes match (auth → 2, success → 0, errors non-zero); stdout carries only machine output,
   stderr carries hints/logs.
4. Existing env vars and `config.json`/`.token.json` work unchanged (a user with a cached token isn't
   forced to re-login).
5. `go build` cross-compiles for linux/darwin/windows × amd64/arm64; `go test -race ./...` passes;
   `golangci-lint` is clean.
6. Tool runs with **no Python interpreter present**; ClawHub skill installs and works without `python3`.
7. New commands (`config`, `version`, `completion`) work and are documented.

---

## 21. Repo cutover plan

Because the Go tool **replaces** `github.com/stozo04/speediance-cli` in place:
1. Develop in this folder (`speediance-cli-go`) until P9 is green.
2. On a branch in the real repo: add the Go tree, `go.mod`, workflows; rewrite docs.
3. Remove the Python package (`speediance/`, `pyproject.toml`, `requirements.txt`, Python tests) in the
   **same** PR so `main` is never half-and-half. Preserve `LICENSE`, `library.json`, `config.example.json`,
   `.env.example`, `plans/` examples, and the (updated) `SKILL.md` + ClawHub Action.
4. Keep `main` PR-protected (existing convention). Merge, then tag `v1.0.0` to trigger the release.
5. Note in the README/release that v1.0.0 is the Go rewrite and is CLI-compatible with the Python
   versions (same commands, same `--json`).

---

## 22. Risks & open items

- **Unofficial API drift.** If Speediance updates the app, headers/endpoints may break; `internal/api`
  is the single patch point (mirror the Python "all calls live in client.py" convention).
- **Fuzzy-match tuning.** Weights/threshold/algorithm need empirical tuning on real sheets; flagged as
  "good enough," not byte-parity.
- **Windows file perms.** `0600` is best-effort on Windows — must not fail commands.
- **`go install` version strings.** Will be `ReadBuildInfo`-derived, not ldflags — acceptable; document.
- **GM2 still untested** — carry the warning, do not imply support.
- **Open choice (low stakes):** whether to also support the `os.UserConfigDir` config location in v1 or
  defer it. Default plan: keep CWD `config.json` as the only default; add XDG later if wanted.

---

## 23. References

Best-practice sources this spec relies on (verified current 2025/2026):

- Project layout — go.dev/doc/modules/layout · golang-standards/project-layout#117 · cli/cli
- Cobra — cobra.dev/docs · pkg.go.dev/github.com/spf13/cobra · spf13/cobra#2124 (exit codes)
- Config/paths — pkg.go.dev/os#UserConfigDir · #UserCacheDir · #WriteFile · #LookupEnv
- HTTP — "Don't use Go's default http client" (nate510) · pkg.go.dev/net/http#Client · hashicorp/go-retryablehttp
- Errors — go.dev/blog/go1.13-errors · pkg.go.dev/errors
- Testing — go.dev/blog/subtests · pkg.go.dev/net/http/httptest · pkg.go.dev/github.com/google/go-cmp/cmp · go.dev/doc/go1.22
- JSON — pkg.go.dev/encoding/json · go.dev/doc/go1.24 (omitzero)
- Logging — go.dev/blog/slog
- Release — goreleaser.com (v2 builds/ci) · go.dev/ref/mod#go-install · pkg.go.dev/runtime/debug#ReadBuildInfo
- Lint/CI — golangci-lint.run (v2 config + migration) · golangci/golangci-lint-action · go.dev/doc/toolchain
- Fuzzy match — github.com/hbollon/go-edlib · pkg.go.dev/github.com/pmezard/go-difflib/difflib

---

*Spec authored against the Python `speediance-cli` source (`speediance/*.py`, v0.1.0) on 2026-06-16.*
