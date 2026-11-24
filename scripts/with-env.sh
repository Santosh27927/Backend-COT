#!/bin/sh
set -eu
cot_project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
if [ ! -f "$cot_project_dir/.env" ]; then
  echo "Run make init-local before starting local services." >&2
  exit 1
fi
# .env is a trusted, private developer file produced by init-local.sh.
set -a
. "$cot_project_dir/.env"
set +a
exec "$@"
