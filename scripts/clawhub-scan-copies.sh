#!/usr/bin/env bash
set -euo pipefail

readonly OWNER="stozo04"
readonly LIVE_SLUG="speediance"
readonly SCAN_COPY_PREFIX="speediance-scan-"
readonly SCAN_WAIT_SECONDS="${SCAN_WAIT_SECONDS:-1800}"
readonly SCAN_POLL_SECONDS="${SCAN_POLL_SECONDS:-30}"
readonly SEARCH_LIMIT=200

usage() {
  echo "usage: $0 version <slug> < publish-output | await <slug> <version> | delete <slug> | sweep" >&2
  exit 64
}

is_scan_copy() {
  [[ "$1" == "$SCAN_COPY_PREFIX"* && "$1" != "$LIVE_SLUG" ]]
}

scan_finished() {
  jq -e '.security.checkedAt != null' <<<"$1" >/dev/null 2>&1
}

await_scan() {
  local slug="$1" version="$2" report="" deadline=$((SECONDS + SCAN_WAIT_SECONDS))
  while :; do
    report=$(clawhub skill verify "$slug" --version "$version" 2>/dev/null) || true
    scan_finished "$report" && break
    if ((SECONDS >= deadline)); then
      echo "ClawHub had not finished scanning \`$slug@$version\` after ${SCAN_WAIT_SECONDS}s."
      return 2
    fi
    sleep "$SCAN_POLL_SECONDS"
  done
  jq -r '
    "| Field | Value |",
    "| --- | --- |",
    "| Slug | `\(.slug)` |",
    "| Version | `\(.version)` |",
    "| Status | \(.security.status) |",
    "| Verdict | \(.security.verdict) (\(.security.confidence) confidence) |",
    "",
    (.security.summary // "")' <<<"$report"
  jq -e '.security.passed == true' <<<"$report" >/dev/null
}

delete_scan_copy() {
  local slug="$1" output
  if ! is_scan_copy "$slug"; then
    echo "Refusing to delete \`$slug\`: only slugs starting with $SCAN_COPY_PREFIX are scan copies." >&2
    return 1
  fi
  if output=$(clawhub delete "$slug" --yes --reason "Scan copy from the branch deploy workflow" 2>&1); then
    echo "$output"
    return 0
  fi
  if grep -qi "not found" <<<"$output"; then
    echo "\`$slug\` is already gone."
    return 0
  fi
  echo "$output" >&2
  return 1
}

listed_scan_copies() {
  local page cursor=""
  while :; do
    page=$(clawhub search --prefix "$SCAN_COPY_PREFIX" --limit "$SEARCH_LIMIT" ${cursor:+--cursor "$cursor"}) || return 1
    awk -v owner="$OWNER/" 'index($1, owner) == 1 { print substr($1, length(owner) + 1) }' <<<"$page"
    cursor=$(sed -n 's/^Next cursor: //p' <<<"$page")
    [[ -n "$cursor" ]] || return 0
  done
}

run_still_active() {
  local run_id="$1" status
  [[ "$run_id" =~ ^[0-9]+$ ]] || return 1
  if status=$(gh run view "$run_id" --json status --jq .status 2>&1); then
    [[ "$status" != "completed" ]]
    return
  fi
  ! grep -qi "not found" <<<"$status"
}

published_version() {
  local slug="$1" version
  version=$(sed -nE "s/.* ${slug}@([0-9]+\.[0-9]+\.[0-9]+).*/\1/p" | tail -n 1)
  if [[ -z "$version" ]]; then
    echo "Could not find the published version of \`$slug\` in the publish output." >&2
    return 1
  fi
  echo "$version"
}

sweep_scan_copies() {
  local slug run_id listing failures=0
  if ! listing=$(listed_scan_copies); then
    echo "Could not list scan copies on ClawHub." >&2
    return 1
  fi
  while read -r slug; do
    [[ -n "$slug" ]] || continue
    is_scan_copy "$slug" || continue
    run_id="${slug#"$SCAN_COPY_PREFIX"}"
    run_id="${run_id%%-*}"
    if run_still_active "$run_id"; then
      echo "Keeping \`$slug\`: workflow run $run_id is still going."
      continue
    fi
    delete_scan_copy "$slug" || failures=$((failures + 1))
  done <<<"$listing"
  ((failures == 0))
}

case "${1:-}" in
  version) [[ $# -eq 2 ]] || usage; published_version "$2" ;;
  await) [[ $# -eq 3 ]] || usage; await_scan "$2" "$3" ;;
  delete) [[ $# -eq 2 ]] || usage; delete_scan_copy "$2" ;;
  sweep) [[ $# -eq 1 ]] || usage; sweep_scan_copies ;;
  *) usage ;;
esac
