#!/usr/bin/env bash
# Start the CUBRID exporter against the live local CUBRID server (read-only).
# Usage: run/start-live.sh [port] [dbs]
set -euo pipefail
HERE="$(cd "$(dirname "$0")/.." && pwd)"
PORT="${1:-9890}"
DBS="${2:-testdb,demodb}"

# CUBRID environment (inherited if already set by the login profile).
export CUBRID="${CUBRID:-$HOME/CUBRID-11.4.6.1953-c447ed0-Linux.x86_64}"
export PATH="$CUBRID/bin:$PATH"
export LD_LIBRARY_PATH="$CUBRID/lib:${LD_LIBRARY_PATH:-}"

exec "$HERE/cubrid_exporter" --port "$PORT" --source statdump --dbs "$DBS" --refresh 15000
