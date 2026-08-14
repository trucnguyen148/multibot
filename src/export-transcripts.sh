#!/usr/bin/env bash
# Pulls sessions.db out of the running backend container and exports every
# session's chat transcript as src/transcripts/<user_id>.txt.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CONTAINER_NAME="multibot-backend-1"
OUT_DIR="$REPO_ROOT/src/transcripts"
TMP_DB="$(mktemp -t sessions.XXXXXX.db)"
trap 'rm -f "$TMP_DB"' EXIT

if ! docker cp "$CONTAINER_NAME:/data/sessions.db" "$TMP_DB" 2>/dev/null; then
    echo "error: could not copy sessions.db from container '$CONTAINER_NAME'." >&2
    echo "Is it running? Check with: docker compose ps" >&2
    exit 1
fi

rm -rf "$OUT_DIR"
( cd "$REPO_ROOT/src/go" && go run ./cmd/export --db "$TMP_DB" --out "$OUT_DIR" )
