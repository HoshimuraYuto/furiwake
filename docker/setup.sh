#!/usr/bin/env bash
# Run this on the HOST before starting the devcontainer.
# This starts the furiwake container and creates the furiwake-net network.
#
# Usage:
#   bash setup.sh          # initial start / apply .env changes
#   bash setup.sh update   # pull latest furiwake binary (no-cache rebuild)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

case "${1:-}" in
  update)
    echo "=== Updating furiwake (no-cache rebuild) ==="
    (cd "$SCRIPT_DIR" && docker compose build --no-cache && docker compose up -d)
    ;;
  *)
    echo "=== Starting furiwake ==="
    (cd "$SCRIPT_DIR" && docker compose up -d --build)
    ;;
esac

echo ""
echo "Health check: curl -s http://localhost:52860/health"
