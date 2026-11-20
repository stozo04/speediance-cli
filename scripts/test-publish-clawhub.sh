#!/usr/bin/env bash
set -euo pipefail

readonly SCRIPT="$(cd "$(dirname "$0")" && pwd)/publish-clawhub.sh"
readonly SANDBOX_ROOT="$(mktemp -d)"
trap 'rm -r -- "$SANDBOX_ROOT"' EXIT
failures=0

write_fake_clawhub() {
  local bin="$1" public_latest="$2" taken="$3" fail_mode="$4" attempts="$5"
  cat >"$bin" <<FAKE
#!/usr/bin/env bash
version=""
while [[ \$# -gt 0 ]]; do
  if [[ "\$1" == "--version" ]]; then version="\$2"; shift; fi
  shift
done
if [[ -z "\$version" ]]; then
  IFS=. read -r a b c <<<"$public_latest"
  version="\$a.\$b.\$((c + 1))"
fi
echo "\$version" >>"$attempts"
if [[ "$fail_mode" == "other" ]]; then echo "Error: HTTP 500: upstream unavailable"; exit 1; fi
for t in $taken; do
  if [[ "\$t" == "\$version" ]]; then echo "Error: Version \$version already exists. Increment the version number and try again."; exit 1; fi
done
echo "Update submitted for demo@\$version; pending security scans before it becomes public."
FAKE
  chmod +x "$bin"
}

run_case() {
  local name="$1" public_latest="$2" taken="$3" fail_mode="$4" want_exit="$5" want_version="$6"
  local sandbox="$SANDBOX_ROOT/case$((++case_number))"
  mkdir -p "$sandbox/bin"
  touch "$sandbox/attempts"
  write_fake_clawhub "$sandbox/bin/clawhub" "$public_latest" "$taken" "$fail_mode" "$sandbox/attempts"
  local got_exit=0
  PATH="$sandbox/bin:$PATH" "$SCRIPT" demo owner Demo >"$sandbox/out" 2>&1 || got_exit=$?
  local got_version
  got_version=$(sed -nE 's/.*submitted for demo@([0-9.]+);.*/\1/p' "$sandbox/out")
  if [[ "$got_exit" != "$want_exit" || "$got_version" != "$want_version" ]]; then
    echo "FAIL $name: exit=$got_exit version='$got_version' (want exit=$want_exit version='$want_version'); attempts: $(tr '\n' ' ' <"$sandbox/attempts")"
    failures=$((failures + 1))
  else
    echo "  ok  $name"
  fi
}

case_number=0
run_case "publishes on the first try" 1.0.11 "" none 0 1.0.12
run_case "skips a version still in review" 1.0.11 "1.0.12" none 0 1.0.13
run_case "skips several versions in review" 1.0.11 "1.0.12 1.0.13 1.0.14" none 0 1.0.15
run_case "gives up after five taken versions" 1.0.11 "1.0.12 1.0.13 1.0.14 1.0.15 1.0.16" none 1 ""
run_case "does not retry other errors" 1.0.11 "" other 1 ""

if ((failures > 0)); then
  echo "$failures case(s) failed"
  exit 1
fi
echo "all cases passed"
