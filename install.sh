#!/usr/bin/env bash
set -euo pipefail

REPO="${FURIWAKE_REPO:-HoshimuraYuto/furiwake}"
VERSION="${FURIWAKE_VERSION:-latest}"
BIN_DIR="${FURIWAKE_BIN_DIR:-/usr/local/bin}"
CONFIG_DIR="${FURIWAKE_CONFIG_DIR:-$HOME/.config/furiwake}"
SERVICE_MODE="${FURIWAKE_SERVICE_MODE:-auto}" # auto|yes|no
START_SERVICE="${FURIWAKE_START_SERVICE:-yes}" # yes|no

usage() {
  cat <<'USAGE'
Usage: install.sh [options]

Options:
  --repo <owner/repo>       GitHub repository (default: HoshimuraYuto/furiwake)
  --version <tag|latest>    Release tag (e.g. v1.2.3) or latest
  --bin-dir <path>          Install directory for binary (default: /usr/local/bin)
  --config-dir <path>       Config directory (default: ~/.config/furiwake)
  --service <auto|yes|no>   Install systemd user service (default: auto)
  --start-service <yes|no>  Start service after install (default: yes)
  --help                    Show this help

Environment variables:
  FURIWAKE_REPO, FURIWAKE_VERSION, FURIWAKE_BIN_DIR, FURIWAKE_CONFIG_DIR,
  FURIWAKE_SERVICE_MODE, FURIWAKE_START_SERVICE
USAGE
}

log() {
  echo "[install] $*"
}

warn() {
  echo "[install] WARN: $*" >&2
}

die() {
  echo "[install] ERROR: $*" >&2
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      REPO="${2:-}"
      shift 2
      ;;
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --bin-dir)
      BIN_DIR="${2:-}"
      shift 2
      ;;
    --config-dir)
      CONFIG_DIR="${2:-}"
      shift 2
      ;;
    --service)
      SERVICE_MODE="${2:-}"
      shift 2
      ;;
    --start-service)
      START_SERVICE="${2:-}"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      die "unknown option: $1"
      ;;
  esac
done

case "$SERVICE_MODE" in
  auto|yes|no) ;;
  *) die "--service must be auto|yes|no" ;;
esac

case "$START_SERVICE" in
  yes|no) ;;
  *) die "--start-service must be yes|no" ;;
esac

detect_os() {
  local uname_s
  uname_s="$(uname -s)"
  case "$uname_s" in
    Linux) echo "linux" ;;
    Darwin) echo "darwin" ;;
    *)
      die "unsupported OS for install.sh: $uname_s"
      ;;
  esac
}

detect_arch() {
  local uname_m
  uname_m="$(uname -m)"
  case "$uname_m" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *)
      die "unsupported architecture: $uname_m"
      ;;
  esac
}

resolve_tag() {
  if [[ "$VERSION" == "latest" ]]; then
    local api tag
    api="https://api.github.com/repos/${REPO}/releases/latest"
    tag="$(curl -fsSL "$api" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
    [[ -n "$tag" ]] || die "failed to resolve latest release tag from ${api}"
    echo "$tag"
    return
  fi

  if [[ "$VERSION" == v* ]]; then
    echo "$VERSION"
  else
    echo "v$VERSION"
  fi
}

download_asset() {
  local url="$1"
  local out="$2"
  curl -fsSL "$url" -o "$out"
}

install_file() {
  local src="$1"
  local dst="$2"
  local mode="$3"
  local dst_dir
  dst_dir="$(dirname "$dst")"

  mkdir -p "$dst_dir"
  if install -m "$mode" "$src" "$dst" 2>/dev/null; then
    return
  fi
  if command -v sudo >/dev/null 2>&1; then
    sudo install -m "$mode" "$src" "$dst"
    return
  fi
  die "permission denied while installing ${dst} (try --bin-dir \$HOME/.local/bin)"
}

resolve_bin_dir() {
  if [[ "$BIN_DIR" == "/usr/local/bin" && ! -w "$BIN_DIR" ]]; then
    echo "$HOME/.local/bin"
    return
  fi
  echo "$BIN_DIR"
}

install_systemd_service() {
  local bin_path="$1"
  local config_path="$2"

  if [[ "$SERVICE_MODE" == "no" ]]; then
    log "Skipping systemd user service (--service no)"
    return
  fi
  if [[ "$OS" != "linux" ]]; then
    if [[ "$SERVICE_MODE" == "yes" ]]; then
      warn "systemd user service is only supported on Linux"
    fi
    return
  fi
  if ! command -v systemctl >/dev/null 2>&1; then
    if [[ "$SERVICE_MODE" == "yes" ]]; then
      warn "systemctl not found; skipping service installation"
    fi
    return
  fi

  local service_dir service_file
  service_dir="$HOME/.config/systemd/user"
  service_file="$service_dir/furiwake.service"
  mkdir -p "$service_dir"

  cat > "$service_file" <<EOF
[Unit]
Description=furiwake routing proxy
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${bin_path} --config ${config_path}
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
EOF

  if ! systemctl --user daemon-reload >/dev/null 2>&1; then
    warn "could not reload systemd user daemon; login session may not support --user services"
    warn "service file installed at ${service_file}"
    return
  fi

  if ! systemctl --user enable furiwake.service >/dev/null 2>&1; then
    warn "could not enable furiwake.service automatically"
  fi

  if [[ "$START_SERVICE" == "yes" ]]; then
    if ! systemctl --user restart furiwake.service >/dev/null 2>&1; then
      warn "could not start furiwake.service automatically"
      return
    fi
  fi

  log "systemd user service installed: furiwake.service"
}

OS="$(detect_os)"
ARCH="$(detect_arch)"
TAG="$(resolve_tag)"
BIN_DIR="$(resolve_bin_dir)"
mkdir -p "$BIN_DIR"
mkdir -p "$CONFIG_DIR"

ASSET_BIN="furiwake-${OS}-${ARCH}"
ASSET_CFG="furiwake.yaml.example"
BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

log "Repository: ${REPO}"
log "Release: ${TAG}"
log "Platform: ${OS}/${ARCH}"
log "Download: ${ASSET_BIN}"

download_asset "${BASE_URL}/${ASSET_BIN}" "${TMP_DIR}/${ASSET_BIN}" || die "failed to download ${ASSET_BIN}"
install_file "${TMP_DIR}/${ASSET_BIN}" "${BIN_DIR}/furiwake" 0755
log "Installed binary to ${BIN_DIR}/furiwake"

CONFIG_PATH="${CONFIG_DIR}/furiwake.yaml"
if [[ ! -f "${CONFIG_PATH}" ]]; then
  download_asset "${BASE_URL}/${ASSET_CFG}" "${TMP_DIR}/${ASSET_CFG}" || die "failed to download ${ASSET_CFG}"
  install -m 0644 "${TMP_DIR}/${ASSET_CFG}" "${CONFIG_PATH}"
  log "Created config: ${CONFIG_PATH}"
else
  log "Config already exists, skipping: ${CONFIG_PATH}"
fi

install_systemd_service "${BIN_DIR}/furiwake" "${CONFIG_PATH}"

cat <<EOF

Install complete.

Run directly:
  ${BIN_DIR}/furiwake --config ${CONFIG_PATH}

Systemd user service commands (if installed):
  systemctl --user status furiwake
  systemctl --user restart furiwake
  systemctl --user stop furiwake
  journalctl --user -u furiwake -f
EOF
