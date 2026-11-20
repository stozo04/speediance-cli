#!/usr/bin/env bash
set -euo pipefail

readonly SCRIPT="$(cd "$(dirname "$0")" && pwd)/clawhub-scan-copies.sh"
readonly SANDBOX_ROOT="$(mktemp -d)"
trap 'rm -r -- "$SANDBOX_ROOT"' EXIT
failures=0
case_number=0
sandbox=""

new_sandbox() {
  sandbox="$SANDBOX_ROOT/case$((++case_number))"
  mkdir -p "$sandbox/bin"
  touch "$sandbox/deleted" "$sandbox/verify_calls"
  cat >"$sandbox/bin/clawhub" <<FAKE
#!/usr/bin/env bash
case "\$1" in
  delete)
    if grep -qx "\$2" "$sandbox/missing" 2>/dev/null; then echo "Error: Skill not found"; exit 1; fi
    echo "\$2" >>"$sandbox/deleted"; echo "OK. Deleted \$2." ;;
  search)
    if [[ -e "$sandbox/search_fails" ]]; then echo "Error: HTTP 401: Unauthorized"; exit 1; fi
    if [[ " \$* " == *" --cursor page2 "* ]]; then cat "$sandbox/listing2"; else cat "$sandbox/listing"; fi ;;
  skill)
    echo "\$*" >>"$sandbox/verify_calls"
    calls=\$(wc -l <"$sandbox/verify_calls")
    if (( calls < \$(cat "$sandbox/ready_after") )); then
      echo "Error: Skill version not found"; exit 1
    fi
    cat "$sandbox/report"
    jq -e .ok "$sandbox/report" >/dev/null ;;
esac
FAKE
  cat >"$sandbox/bin/gh" <<FAKE
#!/usr/bin/env bash
state=\$(sed -n "s/^\$3 //p" "$sandbox/runs")
if [[ -z "\$state" ]]; then echo "HTTP 404: Not Found"; exit 1; fi
if [[ "\$state" == "error" ]]; then echo "HTTP 502: Bad Gateway"; exit 1; fi
echo "\$state"
FAKE
  chmod +x "$sandbox/bin/clawhub" "$sandbox/bin/gh"
  echo 1 >"$sandbox/ready_after"
  touch "$sandbox/listing" "$sandbox/runs"
}

run_script() {
  local got_exit=0
  PATH="$sandbox/bin:$PATH" SCAN_WAIT_SECONDS="${WAIT:-60}" SCAN_POLL_SECONDS=0 \
    "$SCRIPT" "$@" >"$sandbox/out" 2>&1 || got_exit=$?
  echo "$got_exit"
}

check() {
  local name="$1" got_exit="$2" want_exit="$3" want_deleted="$4"
  local got_deleted
  got_deleted=$(sort "$sandbox/deleted" | tr '\n' ' ')
  if [[ "$got_exit" != "$want_exit" || "$got_deleted" != "$want_deleted" ]]; then
    echo "FAIL $name: exit=$got_exit deleted='$got_deleted' (want exit=$want_exit deleted='$want_deleted')"
    sed 's/^/      /' "$sandbox/out"
    failures=$((failures + 1))
  else
    echo "  ok  $name"
  fi
}

report() {
  local status="$1" passed="$2"
  echo "{\"ok\":$passed,\"slug\":\"speediance-scan-7\",\"version\":\"1.0.0\",\"security\":{\"status\":\"$status\",\"passed\":$passed,\"verdict\":\"benign\",\"confidence\":\"high\",\"checkedAt\":1,\"summary\":\"done\"}}" >"$sandbox/report"
}

new_sandbox
check "delete removes a scan copy" "$(run_script delete speediance-scan-7)" 0 "speediance-scan-7 "

new_sandbox
check "delete refuses the live skill" "$(run_script delete speediance)" 1 ""

new_sandbox
check "delete refuses a slug outside the scan prefix" "$(run_script delete google-health-cli)" 1 ""

new_sandbox
echo speediance-scan-7 >"$sandbox/missing"
check "delete treats an already-deleted copy as done" "$(run_script delete speediance-scan-7)" 0 ""

new_sandbox
report clean true
check "await passes when the scan is clean" "$(run_script await speediance-scan-7 1.0.3)" 0 ""
if ! grep -q -- "--version 1.0.3" "$sandbox/verify_calls"; then echo "FAIL await did not verify the published version"; failures=$((failures + 1)); fi

new_sandbox
report suspicious false
check "await reports a flagged scan instead of polling past it" "$(run_script await speediance-scan-7 1.0.3)" 1 ""
if ! grep -q "suspicious" "$sandbox/out"; then echo "FAIL flagged scan verdict missing from the report"; failures=$((failures + 1)); fi

new_sandbox
report clean true
echo 3 >"$sandbox/ready_after"
check "await keeps polling while the scan is pending" "$(run_script await speediance-scan-7 1.0.3)" 0 ""

new_sandbox
report clean true
echo 99 >"$sandbox/ready_after"
check "await gives up after the wait limit" "$(WAIT=0 run_script await speediance-scan-7 1.0.3)" 2 ""

new_sandbox
cat >"$sandbox/listing" <<'LIST'
stozo04/speediance  v1.0.14  6d ago  Real skill
stozo04/speediance-scan-100  v1.0.0  1d ago  Finished run
stozo04/speediance-scan-200  v1.0.0  1m ago  Running run
stozo04/speediance-scan-200-2  v1.0.0  1m ago  Running rerun
stozo04/speediance-scan-100-2  v1.0.0  1d ago  Finished rerun
stozo04/speediance-scan-300  v1.0.0  9d ago  Deleted run
stozo04/speediance-scan-400  v1.0.0  1d ago  GitHub unreachable
stozo04/speediance-scan-pr21  v1.0.0  9d ago  Old naming
someone/speediance-scan-500  v1.0.0  1d ago  Not ours
Next cursor: page2
LIST
echo "stozo04/speediance-scan-600  v1.0.0  3d ago  Second page" >"$sandbox/listing2"
printf '100 completed\n200 in_progress\n400 error\n600 completed\n' >"$sandbox/runs"
check "sweep deletes only finished or orphaned scan copies, across pages" "$(run_script sweep)" 0 \
  "speediance-scan-100 speediance-scan-100-2 speediance-scan-300 speediance-scan-600 speediance-scan-pr21 "

new_sandbox
touch "$sandbox/search_fails"
check "sweep fails when ClawHub cannot list scan copies" "$(run_script sweep)" 1 ""

version_of() {
  "$SCRIPT" version "$1" <<<"$2" 2>/dev/null || echo "exit $?"
}
expect_version() {
  local name="$1" got="$2" want="$3"
  if [[ "$got" != "$want" ]]; then
    echo "FAIL $name: got '$got' (want '$want')"
    failures=$((failures + 1))
  else
    echo "  ok  $name"
  fi
}
expect_version "version reads a submitted update" \
  "$(version_of speediance-scan-7 "Update submitted for speediance-scan-7@1.0.3; pending security scans before it becomes public.")" 1.0.3
expect_version "version reads a first publish" \
  "$(version_of speediance-scan-7 "OK. Published speediance-scan-7@1.0.0 (abc123)")" 1.0.0
expect_version "version takes the last line after retries" \
  "$(version_of speediance "$(printf 'Error: Version 1.0.15 already exists.\nUpdate submitted for speediance@1.0.16; pending.')")" 1.0.16
expect_version "version fails without a matching slug" \
  "$(version_of speediance "Update submitted for speediance-scan-7@1.0.3; pending.")" "exit 1"

if ((failures > 0)); then
  echo "$failures case(s) failed"
  exit 1
fi
echo "all cases passed"
