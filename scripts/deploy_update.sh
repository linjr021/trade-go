#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="${PROJECT_DIR:-/opt/trade-go}"
BRANCH="${BRANCH:-main}"
NO_BUILD="${NO_BUILD:-false}"
WITH_TUNNEL="${WITH_TUNNEL:-auto}" # auto|true|false
ALLOW_DIRTY="${ALLOW_DIRTY:-false}"

log() {
  echo "[deploy-update] $*"
}

die() {
  echo "[deploy-update][ERROR] $*" >&2
  exit 1
}

usage() {
  cat <<'EOF'
用法:
  bash scripts/deploy_update.sh [选项]

选项:
  --project-dir <dir>    项目目录（默认: /opt/trade-go）
  --branch <name>        部署分支（默认: main）
  --no-build             跳过 docker compose --build
  --with-tunnel          强制启用 cloudflared profile
  --without-tunnel       强制不启用 cloudflared profile
  --allow-dirty          允许工作区有未提交改动（默认不允许）
  -h, --help             查看帮助
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
      --branch)
        shift
        [[ $# -gt 0 ]] || die "--branch 需要参数"
        BRANCH="$1"
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
      --allow-dirty)
        ALLOW_DIRTY="true"
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        die "未知参数: $1"
        ;;
    esac
    shift
  done
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

resolve_tunnel_enabled() {
  local with_tunnel
  with_tunnel="$(echo -n "${WITH_TUNNEL}" | tr '[:upper:]' '[:lower:]')"
  local token="${CF_TUNNEL_TOKEN:-}"
  if [[ -z "${token}" ]]; then
    token="$(env_file_value CF_TUNNEL_TOKEN .env)"
  fi
  local tunnel_enabled="${CF_TUNNEL_ENABLED:-}"
  if [[ -z "${tunnel_enabled}" ]]; then
    tunnel_enabled="$(env_file_value CF_TUNNEL_ENABLED .env)"
  fi
  tunnel_enabled="$(echo -n "${tunnel_enabled}" | tr '[:upper:]' '[:lower:]')"

  case "${with_tunnel}" in
    true)
      [[ -n "${token}" ]] || die "WITH_TUNNEL=true 但未找到 CF_TUNNEL_TOKEN"
      ENABLE_TUNNEL="true"
      ;;
    false)
      ENABLE_TUNNEL="false"
      ;;
    auto)
      if [[ -n "${token}" ]] && [[ "${tunnel_enabled}" != "false" && "${tunnel_enabled}" != "0" ]]; then
        ENABLE_TUNNEL="true"
      else
        ENABLE_TUNNEL="false"
      fi
      ;;
    *)
      die "WITH_TUNNEL 仅支持 auto|true|false"
      ;;
  esac
}

git_clean_guard() {
  if [[ "${ALLOW_DIRTY}" == "true" ]]; then
    return
  fi
  if ! git diff --quiet || ! git diff --cached --quiet; then
    die "检测到未提交改动，已中止部署（可加 --allow-dirty 跳过）"
  fi
}

main() {
  parse_args "$@"

  [[ -d "${PROJECT_DIR}" ]] || die "项目目录不存在: ${PROJECT_DIR}"
  [[ -f "${PROJECT_DIR}/docker-compose.yml" ]] || die "缺少 docker-compose.yml: ${PROJECT_DIR}"
  [[ -d "${PROJECT_DIR}/.git" ]] || die "目录不是 git 仓库: ${PROJECT_DIR}"

  cd "${PROJECT_DIR}"

  command -v git >/dev/null 2>&1 || die "未找到 git"
  command -v docker >/dev/null 2>&1 || die "未找到 docker"
  docker compose version >/dev/null 2>&1 || die "未找到 docker compose"

  git_clean_guard

  log "拉取代码: branch=${BRANCH}"
  git fetch origin "${BRANCH}" --prune
  local current_branch
  current_branch="$(git branch --show-current || true)"
  if [[ "${current_branch}" != "${BRANCH}" ]]; then
    git checkout "${BRANCH}"
  fi
  git pull --ff-only origin "${BRANCH}"

  resolve_tunnel_enabled

  local -a compose_args=()
  if [[ "${ENABLE_TUNNEL}" == "true" ]]; then
    compose_args+=(--profile tunnel)
  fi
  compose_args+=(up -d)
  if [[ "${NO_BUILD}" != "true" ]]; then
    compose_args+=(--build)
  fi

  log "启动服务: docker compose ${compose_args[*]}"
  docker compose "${compose_args[@]}"

  log "当前容器状态:"
  docker compose ps
  log "部署完成"
}

main "$@"

