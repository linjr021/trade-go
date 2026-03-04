#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

SKIP_DOCKER_INSTALL="false"
NO_BUILD="false"
WITH_TUNNEL="auto"
ENABLE_TUNNEL="false"
BACKEND_PORT_EFFECTIVE="8080"
FRONTEND_PORT_EFFECTIVE="5173"
APP_UID_EFFECTIVE="1000"
APP_GID_EFFECTIVE="1000"

log() {
  echo "[deploy-docker] $*"
}

warn() {
  echo "[deploy-docker][WARN] $*" >&2
}

die() {
  echo "[deploy-docker][ERROR] $*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
用法:
  sudo bash scripts/deploy_docker.sh [选项]

选项:
  --project-dir <dir>       项目目录（默认: 脚本上级目录）
  --skip-docker-install     跳过 Docker/Compose 安装，仅执行部署
  --no-build                启动时不强制 --build
  --with-tunnel             启用 cloudflared（要求配置 CF_TUNNEL_TOKEN）
  --without-tunnel          强制不启用 cloudflared
  -h, --help                查看帮助

示例:
  sudo bash scripts/deploy_docker.sh
  sudo bash scripts/deploy_docker.sh --project-dir /opt/trade-go --no-build
  sudo bash scripts/deploy_docker.sh --with-tunnel
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --project-dir)
        shift
        [[ $# -gt 0 ]] || die "--project-dir 需要参数"
        PROJECT_DIR="$1"
        ;;
      --skip-docker-install)
        SKIP_DOCKER_INSTALL="true"
        ;;
      --no-build)
        NO_BUILD="true"
        ;;
      --with-tunnel)
        WITH_TUNNEL="true"
        ;;
      --without-tunnel)
        WITH_TUNNEL="false"
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        die "未知参数: $1（使用 --help 查看可用参数）"
        ;;
    esac
    shift
  done
}

ensure_root() {
  if [[ "${EUID}" -eq 0 ]]; then
    return
  fi
  if command -v sudo >/dev/null 2>&1; then
    log "需要 root 权限，正在通过 sudo 重新执行..."
    exec sudo -E bash "$0" "$@"
  fi
  die "请使用 root 运行，或安装 sudo 后重试"
}

pkg_install() {
  local packages=("$@")
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -y
    DEBIAN_FRONTEND=noninteractive apt-get install -y "${packages[@]}"
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y "${packages[@]}"
  elif command -v yum >/dev/null 2>&1; then
    yum install -y "${packages[@]}"
  elif command -v pacman >/dev/null 2>&1; then
    pacman -Sy --noconfirm "${packages[@]}"
  elif command -v zypper >/dev/null 2>&1; then
    zypper --non-interactive install "${packages[@]}"
  else
    die "未识别的包管理器，请手动安装: ${packages[*]}"
  fi
}

install_docker_engine() {
  if command -v docker >/dev/null 2>&1; then
    log "已检测到 Docker: $(docker --version)"
    return
  fi

  log "未检测到 Docker，开始自动安装..."
  pkg_install ca-certificates curl

  # 优先使用 Docker 官方安装脚本，适配多发行版。
  curl -fsSL https://get.docker.com -o /tmp/get-docker.sh
  sh /tmp/get-docker.sh
  rm -f /tmp/get-docker.sh

  command -v docker >/dev/null 2>&1 || die "Docker 安装失败，请手动检查"
  log "Docker 安装完成: $(docker --version)"
}

install_compose_plugin() {
  if docker compose version >/dev/null 2>&1; then
    log "已检测到 Docker Compose 插件: $(docker compose version --short 2>/dev/null || docker compose version)"
    return
  fi

  log "未检测到 Docker Compose 插件，尝试通过系统包安装..."
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -y || true
    DEBIAN_FRONTEND=noninteractive apt-get install -y docker-compose-plugin || true
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y docker-compose-plugin || true
  elif command -v yum >/dev/null 2>&1; then
    yum install -y docker-compose-plugin || true
  elif command -v zypper >/dev/null 2>&1; then
    zypper --non-interactive install docker-compose || true
  fi

  if docker compose version >/dev/null 2>&1; then
    log "Docker Compose 插件安装完成"
    return
  fi

  log "系统包安装失败，改为下载官方 Compose 插件二进制..."
  local arch
  case "$(uname -m)" in
    x86_64|amd64) arch="x86_64" ;;
    aarch64|arm64) arch="aarch64" ;;
    armv7l|armv7) arch="armv7" ;;
    *)
      die "不支持的 CPU 架构: $(uname -m)"
      ;;
  esac

  local version
  version="$(curl -fsSL https://api.github.com/repos/docker/compose/releases/latest | sed -n 's/.*"tag_name":[[:space:]]*"\(v[0-9.]*\)".*/\1/p' | head -n1)"
  [[ -n "${version}" ]] || version="v2.29.7"

  local plugin_dir="/usr/local/lib/docker/cli-plugins"
  mkdir -p "${plugin_dir}"
  curl -fL "https://github.com/docker/compose/releases/download/${version}/docker-compose-linux-${arch}" -o "${plugin_dir}/docker-compose"
  chmod +x "${plugin_dir}/docker-compose"

  docker compose version >/dev/null 2>&1 || die "Docker Compose 插件安装失败"
  log "Docker Compose 插件安装完成: $(docker compose version --short 2>/dev/null || docker compose version)"
}

start_docker_service() {
  if command -v systemctl >/dev/null 2>&1; then
    systemctl enable docker >/dev/null 2>&1 || true
    systemctl restart docker
  elif command -v service >/dev/null 2>&1; then
    service docker start || true
  fi

  for _ in $(seq 1 20); do
    if docker info >/dev/null 2>&1; then
      log "Docker 服务已就绪"
      return
    fi
    sleep 1
  done
  die "Docker 服务未就绪，请检查守护进程状态"
}

grant_docker_group() {
  local target_user="${SUDO_USER:-}"
  if [[ -z "${target_user}" || "${target_user}" == "root" ]]; then
    return
  fi
  if ! getent group docker >/dev/null 2>&1; then
    groupadd docker || true
  fi
  if id -nG "${target_user}" | tr ' ' '\n' | grep -qx docker; then
    return
  fi
  usermod -aG docker "${target_user}" || true
  warn "已将用户 ${target_user} 加入 docker 组，重新登录后可免 sudo 执行 docker 命令"
}

prepare_project() {
  [[ -d "${PROJECT_DIR}" ]] || die "项目目录不存在: ${PROJECT_DIR}"
  [[ -f "${PROJECT_DIR}/docker-compose.yml" ]] || die "缺少 docker-compose.yml: ${PROJECT_DIR}"

  cd "${PROJECT_DIR}"
  mkdir -p data logs

  if [[ ! -f ".env" ]]; then
    if [[ -f ".env.example" ]]; then
      cp .env.example .env
      warn "检测到缺少 .env，已从 .env.example 初始化，请尽快填写真实参数后再重启服务"
    else
      die "缺少 .env 和 .env.example，无法继续"
    fi
  fi
}

env_file_value() {
  local key="$1"
  local file="${2:-.env}"
  [[ -f "${file}" ]] || return 0
  local line
  line="$(grep -E "^[[:space:]]*${key}=" "${file}" | tail -n1 || true)"
  line="${line#*=}"
  line="${line%\"}"
  line="${line#\"}"
  line="${line%\'}"
  line="${line#\'}"
  echo -n "${line}"
}

resolve_runtime_options() {
  local token_from_env="${CF_TUNNEL_TOKEN:-}"
  local token_from_file
  token_from_file="$(env_file_value CF_TUNNEL_TOKEN .env)"
  local token="${token_from_env:-${token_from_file}}"
  local tunnel_enabled_from_env="${CF_TUNNEL_ENABLED:-}"
  local tunnel_enabled_from_file
  tunnel_enabled_from_file="$(env_file_value CF_TUNNEL_ENABLED .env)"
  local tunnel_enabled="${tunnel_enabled_from_env:-${tunnel_enabled_from_file}}"
  tunnel_enabled="$(echo -n "${tunnel_enabled}" | tr '[:upper:]' '[:lower:]')"

  local frontend_from_env="${FRONTEND_PORT:-}"
  local frontend_from_file
  frontend_from_file="$(env_file_value FRONTEND_PORT .env)"
  FRONTEND_PORT_EFFECTIVE="${frontend_from_env:-${frontend_from_file:-5173}}"

  local backend_from_env="${BACKEND_PORT:-}"
  local backend_from_file
  backend_from_file="$(env_file_value BACKEND_PORT .env)"
  BACKEND_PORT_EFFECTIVE="${backend_from_env:-${backend_from_file:-8080}}"

  local app_uid_from_env="${APP_UID:-}"
  local app_uid_from_file
  app_uid_from_file="$(env_file_value APP_UID .env)"
  APP_UID_EFFECTIVE="${app_uid_from_env:-${app_uid_from_file:-1000}}"

  local app_gid_from_env="${APP_GID:-}"
  local app_gid_from_file
  app_gid_from_file="$(env_file_value APP_GID .env)"
  APP_GID_EFFECTIVE="${app_gid_from_env:-${app_gid_from_file:-1000}}"

  case "${WITH_TUNNEL}" in
    true)
      ENABLE_TUNNEL="true"
      ;;
    false)
      ENABLE_TUNNEL="false"
      ;;
    auto)
      case "${tunnel_enabled}" in
        true|1|yes|on)
          ENABLE_TUNNEL="true"
          ;;
        false|0|no|off)
          ENABLE_TUNNEL="false"
          ;;
        ""|auto)
          if [[ -n "${token}" ]]; then
            ENABLE_TUNNEL="true"
          else
            ENABLE_TUNNEL="false"
          fi
          ;;
        *)
          die "CF_TUNNEL_ENABLED 仅支持 auto/true/false（或等价值）"
          ;;
      esac
      ;;
    *)
      die "WITH_TUNNEL 参数非法: ${WITH_TUNNEL}"
      ;;
  esac

  if [[ "${ENABLE_TUNNEL}" == "true" && -z "${token}" ]]; then
    die "已启用 cloudflared，但未检测到 CF_TUNNEL_TOKEN（请在 .env 中配置）"
  fi
}

fix_runtime_permissions() {
  cd "${PROJECT_DIR}"
  mkdir -p data logs

  chown -R "${APP_UID_EFFECTIVE}:${APP_GID_EFFECTIVE}" data logs || warn "设置 data/logs 权限失败"
  chmod -R u+rwX,g+rwX data logs || true

  if [[ -f ".env" ]]; then
    chown "${APP_UID_EFFECTIVE}:${APP_GID_EFFECTIVE}" .env || warn "设置 .env 所有者失败"
    chmod u+rw,g+rw .env || true
  fi
}

deploy_compose() {
  local compose_args=()
  if [[ "${ENABLE_TUNNEL}" == "true" ]]; then
    compose_args+=(--profile tunnel)
  fi
  if [[ "${NO_BUILD}" == "true" ]]; then
    docker compose "${compose_args[@]}" up -d
  else
    docker compose "${compose_args[@]}" up -d --build
  fi
}

main() {
  local original_args=("$@")
  parse_args "$@"
  ensure_root "${original_args[@]}"

  log "项目目录: ${PROJECT_DIR}"
  if [[ "${SKIP_DOCKER_INSTALL}" != "true" ]]; then
    install_docker_engine
    install_compose_plugin
  else
    log "已跳过 Docker/Compose 安装步骤"
  fi

  start_docker_service
  grant_docker_group
  prepare_project
  resolve_runtime_options
  fix_runtime_permissions

  log "启动 Docker Compose 服务..."
  if [[ "${ENABLE_TUNNEL}" == "true" ]]; then
    log "cloudflared: 已启用（profile=tunnel）"
  else
    log "cloudflared: 未启用（可通过 --with-tunnel 或配置 CF_TUNNEL_TOKEN 启用）"
  fi
  deploy_compose

  log "部署完成。"
  log "前端: http://localhost:${FRONTEND_PORT_EFFECTIVE}"
  log "后端: http://localhost:${BACKEND_PORT_EFFECTIVE}"
  log "状态检查: curl http://localhost:${BACKEND_PORT_EFFECTIVE}/api/status"
}

main "$@"
