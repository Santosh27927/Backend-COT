#!/bin/sh
set -eu
umask 077
cot_project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cot_env_file="$cot_project_dir/.env"
if [ -e "$cot_env_file" ]; then
  echo "Existing .env preserved."
  exit 0
fi
cot_database_password=$(openssl rand -hex 32)
cot_auth_token=$(openssl rand -hex 32)
set -C
cat > "$cot_env_file" <<EOF
COT_ENV=local
COT_AUTH_MODE=dev-token
COT_DEV_PLAYER_ID=local-player
COT_DEV_AUTH_TOKEN=$cot_auth_token
COT_DB_PORT=55432
COT_POSTGRES_PASSWORD=$cot_database_password
COT_DATABASE_URL=postgres://cot:$cot_database_password@127.0.0.1:55432/cot?sslmode=disable
COT_DB_MAX_CONNECTIONS=4
COT_LOG_LEVEL=info
EOF
echo "Created private .env; credentials are not printed."
