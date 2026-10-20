#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 || ! "$1" =~ ^[A-Za-z0-9_-]+$ ]]; then
  echo "usage: $0 VIDEO_ID STEP_NAME" >&2
  exit 2
fi

video_id=$1
step_name=$2
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
  --execute="UPDATE tb_task_steps SET status='failed', error_msg='本地媒体完整性校验发现损坏，需要重新下载', progress=0, progress_msg='等待重新下载' WHERE video_id='${video_id}' AND step_name='${step_name}' AND status='completed'; SELECT video_id, step_name, status, progress_msg FROM tb_task_steps WHERE video_id='${video_id}' AND step_name='${step_name}';"
