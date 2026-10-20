#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 || ! "$1" =~ ^[A-Za-z0-9_-]+$ ]]; then
  echo "usage: $0 VIDEO_ID" >&2
  exit 2
fi

video_id=$1
config_path=${CONFIG_PATH:-config.toml}

database_value() {
  local key=$1
  awk -F '=' -v wanted="$key" '
    /^\[database\][[:space:]]*$/ { in_database=1; next }
    /^\[/ { if (in_database) exit }
    in_database {
      current=$1
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", current)
      if (current == wanted) {
        value=$2
        sub(/^[[:space:]]+/, "", value)
        sub(/[[:space:]]+$/, "", value)
        gsub(/^"|"$/, "", value)
        print value
        exit
      }
    }
  ' "$config_path"
}

db_host=$(database_value host)
db_port=$(database_value port)
db_user=$(database_value username)
db_pass=$(database_value password)
db_name=$(database_value database)

MYSQL_PWD="$db_pass" mysql --protocol=TCP \
  --host="$db_host" --port="$db_port" --user="$db_user" "$db_name" \
  --execute="UPDATE tb_task_steps SET status='completed', progress=100, progress_msg='复用现有产物', error_msg='' WHERE video_id='${video_id}' AND step_order BETWEEN 2 AND 4 AND status='pending'; SELECT step_order, step_name, status, progress_msg FROM tb_task_steps WHERE video_id='${video_id}' ORDER BY step_order;"
