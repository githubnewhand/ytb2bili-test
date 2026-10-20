#!/usr/bin/env bash
set -euo pipefail

# Keep the runtime and model files on the Windows drive to avoid WSL VHD growth.
default_root="/mnt/c/Users/WC/AppData/Local/OllamaWSLValidation"
ollama_bin="${OLLAMA_BIN:-$default_root/runtime/bin/ollama}"
export OLLAMA_MODELS="${OLLAMA_MODELS:-$default_root/models}"
export OLLAMA_HOST="${OLLAMA_HOST:-127.0.0.1:11434}"
export OLLAMA_NO_CLOUD=1

if [[ ! -x "$ollama_bin" ]]; then
  echo "Ollama executable not found: $ollama_bin" >&2
  exit 1
fi
if [[ ! -d "$OLLAMA_MODELS" ]]; then
  echo "Ollama model directory not found: $OLLAMA_MODELS" >&2
  exit 1
fi

# Ollama uses HTTPS_PROXY only for remote model pulls. Local /api/chat stays direct.
if [[ -z "${HTTPS_PROXY:-}" && -f config.toml ]]; then
  proxy="$(python3 - <<'PY'
import pathlib, tomllib
config = tomllib.loads(pathlib.Path("config.toml").read_text())
proxy = config.get("ProxyConfig", {})
if proxy.get("use_proxy"):
    print(proxy.get("proxy_host", ""))
PY
)"
  if [[ -n "$proxy" ]]; then export HTTPS_PROXY="$proxy"; fi
fi

exec "$ollama_bin" serve
