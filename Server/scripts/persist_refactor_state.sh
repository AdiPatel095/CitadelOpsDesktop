#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE_DIR="$ROOT_DIR/Server/refactor_state"
LATEST_FILE="$STATE_DIR/latest_snapshot.md"
PID_FILE="$STATE_DIR/persist_refactor_state.pid"
STRUCTURE_FILE="$ROOT_DIR/Server/MODEL_REFACTOR_STRUCTURE.md"
INTERVAL_SECONDS="${1:-15}"

if ! [[ "$INTERVAL_SECONDS" =~ ^[0-9]+$ ]] || [ "$INTERVAL_SECONDS" -lt 1 ]; then
  echo "Interval must be a positive integer (seconds)."
  exit 1
fi

mkdir -p "$STATE_DIR"

if [ -f "$PID_FILE" ]; then
  EXISTING_PID="$(cat "$PID_FILE" 2>/dev/null || true)"
  if [ -n "${EXISTING_PID}" ] && kill -0 "$EXISTING_PID" 2>/dev/null; then
    echo "persist_refactor_state is already running with PID $EXISTING_PID"
    exit 0
  fi
fi

echo "$$" > "$PID_FILE"
trap 'rm -f "$PID_FILE"' EXIT

while true; do
  TS_ISO="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  TS_FILE_SAFE="$(date -u +"%Y%m%dT%H%M%SZ")"
  SNAPSHOT_FILE="$STATE_DIR/snapshot_${TS_FILE_SAFE}.md"

  {
    echo "# Refactor Snapshot"
    echo
    echo "- Timestamp (UTC): $TS_ISO"
    echo "- Root: $ROOT_DIR"
    echo "- IntervalSeconds: $INTERVAL_SECONDS"
    echo
    echo "## Git Status"
    echo
    echo '```'
    git -C "$ROOT_DIR" status --short --branch || true
    echo '```'
    echo
    echo "## Diff Stat"
    echo
    echo '```'
    git -C "$ROOT_DIR" diff --stat || true
    echo '```'
    echo
    echo "## Structure And Planned Changes"
    echo
    if [ -f "$STRUCTURE_FILE" ]; then
      sed -n '1,250p' "$STRUCTURE_FILE"
    else
      echo "Structure file not found: $STRUCTURE_FILE"
    fi
  } > "$SNAPSHOT_FILE"

  cp "$SNAPSHOT_FILE" "$LATEST_FILE"
  sleep "$INTERVAL_SECONDS"
done
