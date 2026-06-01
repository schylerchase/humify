#!/usr/bin/env bash
# Sync the shared Humify methodology into each adapter so every adapter is
# self-contained and installable on its own. Edit docs in shared/, then run this.
# The vendored copies under <adapter>/skills/humify/reference/ are committed.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SRC="$ROOT/shared"

DOCS=(
  GUIDED-RUN.md
  HUMIFY.md
  HUMIFY-VISION.md
  HUMIFY-OPERATOR.md
  HUMIFY-AI-INSTRUCTIONS.md
  EXAMPLES.md
  STELLAR-CODEBASES.md
  STEELMAN-PASS.md
  MODEL-CONTEXT-PACKET.md
  MASSIVE-CODEBASE-WORKFLOW.md
  REFACTOR-PLAN-PROTOCOL.md
)

for adapter in claude codex; do
  dest="$ROOT/$adapter/skills/humify/reference"
  rm -rf "$dest"
  mkdir -p "$dest/templates"
  for d in "${DOCS[@]}"; do
    cp "$SRC/$d" "$dest/$d"
  done
  cp "$SRC"/templates/*.template.md "$dest/templates/"
  cp -R "$SRC/examples/stellar-codebase" "$dest/examples"
  echo "synced -> $adapter/skills/humify/reference ($(find "$dest" -type f | wc -l | tr -d ' ') files)"
done

echo "done. Commit the updated reference/ dirs."
