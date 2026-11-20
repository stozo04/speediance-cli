#!/usr/bin/env bash
set -euo pipefail

readonly MAX_ATTEMPTS=5
readonly SLUG="$1"
readonly OWNER="$2"
readonly DISPLAY_NAME="$3"

next_patch() {
  local major minor patch
  IFS=. read -r major minor patch <<<"$1"
  echo "$major.$minor.$((patch + 1))"
}

taken_version() {
  sed -nE 's/.*Version ([0-9]+\.[0-9]+\.[0-9]+) already exists.*/\1/p' <<<"$1" | head -n 1
}

version_args=()
for ((attempt = 1; attempt <= MAX_ATTEMPTS; attempt++)); do
  if output=$(clawhub skill publish . --owner "$OWNER" --slug "$SLUG" --name "$DISPLAY_NAME" ${version_args[@]+"${version_args[@]}"} 2>&1); then
    echo "$output"
    exit 0
  fi
  echo "$output"
  taken=$(taken_version "$output")
  if [[ -z "$taken" ]]; then
    exit 1
  fi
  version_args=(--version "$(next_patch "$taken")")
  echo "Version $taken is held by a release still in ClawHub review; retrying as ${version_args[1]}." >&2
done
echo "Gave up after $MAX_ATTEMPTS attempts: every version tried was already taken." >&2
exit 1
