#!/usr/bin/env bash
set -euo pipefail

APP_NAME="trade-go"
SERVICE_NAME="trade-go-backend"
INSTALL_DIR="/opt/trade-go"
RUN_USER="tradego"
DOMAIN="_"
WEB_PORT="80"
API_PORT="8080"
NODE_MAJOR="20"
GO_VERSION="1.22.12"
SKIP_DEPS="false"
ORIGINAL_ARGS=("$@")

usage() {
  cat <<USAGE
用法: sudo bash scripts/install_linux.sh [选项]

选项:
  --install-dir <dir>      安装目录（默认: /opt/trade-go）
  --service-name <name>    systemd 服务名（默认: trade-go-backend）
  --run-user <user>        运行用户（默认: tradego）
  --domain <domain>        Nginx server_name（默认: _）
  --web-port <port>        前端端口（默认: 80）
  --api-port <port>        后端 API 端口（默认: 8080）
  --node-major <version>   Node.js 主版本（默认: 20）
  --go-version <version>   Go 版本（默认: 1.22.12）
  --skip-deps              跳过系统依赖安装
  -h, --help               显示帮助

示例:
  sudo bash scripts/install_linux.sh
  sudo bash scripts/install_linux.sh --domain trade.example.com --web-port 80 --api-port 8080
USAGE
}

log() {
  printf "\n[INFO] %s\n" "$*"
}

warn() {
  printf "\n[WARN] %s\n" "$*"
}

err() {
  printf "\n[ERROR] %s\n" "$*" >&2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --install-dir)
      INSTALL_DIR="$2"
      shift 2
      ;;
    --service-name)
      SERVICE_NAME="$2"
      shift 2
      ;;
    --run-user)
      RUN_USER="$2"
      shift 2
      ;;
    --domain)
      DOMAIN="$2"
      shift 2
      ;;
    --web-port)
      WEB_PORT="$2"
      shift 2
      ;;
    --api-port)
      API_PORT="$2"
      shift 2
      ;;
    --node-major)
      NODE_MAJOR="$2"
      shift 2
      ;;
    --go-version)
      GO_VERSION="$2"
      shift 2
      ;;
    --skip-deps)
      SKIP_DEPS="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      err "未知参数: $1"
      usage
      exit 1
      ;;
  esac
done

if [[ "$(uname -s)" != "Linux" ]]; then
  err "该脚本仅支持 Linux"
  exit 1
fi

if [[ ${EUID:-0} -ne 0 ]]; then
  warn "需要 root 权限，尝试使用 sudo 重新执行..."
  exec sudo -E bash "$0" "${ORIGINAL_ARGS[@]}"
fi

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"

if [[ ! -f "$REPO_DIR/go.mod" || ! -d "$REPO_DIR/frontend" ]]; then
  err "请在项目仓库内执行该脚本（未找到 go.mod 或 frontend/）"
  exit 1
fi

if ! [[ "$WEB_PORT" =~ ^[0-9]+$ ]] || ! [[ "$API_PORT" =~ ^[0-9]+$ ]]; then
  err "端口必须为数字"
  exit 1
fi
if (( WEB_PORT < 1 || WEB_PORT > 65535 || API_PORT < 1 || API_PORT > 65535 )); then
  err "端口范围必须在 1-65535"
  exit 1
fi

APT_UPDATED="false"
apt_update_once() {
  if [[ "$APT_UPDATED" == "false" ]]; then
    apt-get update
    APT_UPDATED="true"
  fi
}

ensure_apt_pkg() {
  local pkg="$1"
  if dpkg -s "$pkg" >/dev/null 2>&1; then
    return 0
  fi
  apt_update_once
  apt-get install -y "$pkg"
}

ensure_command() {
  local cmd="$1"
  local hint="$2"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    err "缺少命令: $cmd。$hint"
    exit 1
  fi
}

install_go_if_needed() {
  local need_install="false"
  if ! command -v go >/dev/null 2>&1; then
    need_install="true"
  else
    local gov
    gov="$(go version | awk '{print $3}')"
    local major minor
    major="$(echo "$gov" | sed -E 's/go([0-9]+)\.([0-9]+).*/\1/')"
    minor="$(echo "$gov" | sed -E 's/go([0-9]+)\.([0-9]+).*/\2/')"
    if [[ -z "$major" || -z "$minor" ]]; then
      need_install="true"
    elif (( major < 1 || (major == 1 && minor < 21) )); then
      need_install="true"
    fi
  fi

  if [[ "$need_install" == "true" ]]; then
    log "安装 Go ${GO_VERSION}..."
    local tarball="go${GO_VERSION}.linux-amd64.tar.gz"
    curl -fL "https://go.dev/dl/${tarball}" -o "/tmp/${tarball}"
    rm -rf /usr/local/go
    tar -C /usr/local -xzf "/tmp/${tarball}"
    ln -sf /usr/local/go/bin/go /usr/local/bin/go
    ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
    rm -f "/tmp/${tarball}"
  fi
}

install_node_if_needed() {
  local need_install="false"
  if ! command -v node >/dev/null 2>&1; then
    need_install="true"
  else
    local node_major
    node_major="$(node -v | sed -E 's/^v([0-9]+).*/\1/')"
    if [[ -z "$node_major" ]] || (( node_major < 18 )); then
      need_install="true"
    fi
  fi

  if [[ "$need_install" == "true" ]]; then
    log "安装 Node.js ${NODE_MAJOR}.x..."
    curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | bash -
    apt-get install -y nodejs
  fi
}

install_system_deps() {
  if [[ "$SKIP_DEPS" == "true" ]]; then
    log "跳过依赖安装"
    return
  fi

  if ! command -v apt-get >/dev/null 2>&1; then
    err "当前脚本仅支持 Debian/Ubuntu（apt-get）。请手动安装 Go/Node/Nginx 后使用 --skip-deps。"
    exit 1
  fi

  log "安装系统依赖..."
  ensure_apt_pkg ca-certificates
  ensure_apt_pkg curl
  ensure_apt_pkg git
  ensure_apt_pkg build-essential
  ensure_apt_pkg nginx

  install_go_if_needed
  install_node_if_needed
}

prepare_user_and_dirs() {
  log "准备安装目录与运行用户..."
  mkdir -p "$INSTALL_DIR" "$INSTALL_DIR/bin" "$INSTALL_DIR/data" "$INSTALL_DIR/logs" "$INSTALL_DIR/frontend-dist"

  if ! id -u "$RUN_USER" >/dev/null 2>&1; then
    local nologin_bin
    nologin_bin="$(command -v nologin || true)"
    if [[ -z "$nologin_bin" ]]; then
      nologin_bin="/usr/sbin/nologin"
    fi
    useradd --system --home-dir "$INSTALL_DIR" --shell "$nologin_bin" "$RUN_USER"
  fi

  if [[ ! -f "$INSTALL_DIR/.env" ]]; then
    if [[ -f "$REPO_DIR/.env" ]]; then
      cp "$REPO_DIR/.env" "$INSTALL_DIR/.env"
    else
      cp "$REPO_DIR/.env.example" "$INSTALL_DIR/.env"
    fi
  fi

  for f in integrations.json generated_strategies.json skill_workflow.json; do
    if [[ -f "$REPO_DIR/data/$f" && ! -f "$INSTALL_DIR/data/$f" ]]; then
      cp "$REPO_DIR/data/$f" "$INSTALL_DIR/data/$f"
    fi
  done
}

build_backend() {
  log "构建后端..."
  export PATH="/usr/local/go/bin:${PATH}"
  cd "$REPO_DIR"
  go mod download
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$INSTALL_DIR/bin/trade-go" .
}

build_frontend() {
  log "构建前端..."
  cd "$REPO_DIR/frontend"
  if [[ -f package-lock.json ]]; then
    npm ci
  else
    npm install
  fi
  npm run build

  rm -rf "$INSTALL_DIR/frontend-dist"/*
  cp -R "$REPO_DIR/frontend/dist/." "$INSTALL_DIR/frontend-dist/"
}

write_systemd_service() {
  log "写入 systemd 服务..."
  local service_file="/etc/systemd/system/${SERVICE_NAME}.service"
  cat > "$service_file" <<SERVICE
[Unit]
Description=${APP_NAME} backend service
After=network.target

[Service]
Type=simple
User=${RUN_USER}
Group=${RUN_USER}
WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=-${INSTALL_DIR}/.env
Environment=MODE=web
Environment=HTTP_ADDR=:${API_PORT}
Environment=TRADE_DB_PATH=${INSTALL_DIR}/data/trade.db
ExecStart=${INSTALL_DIR}/bin/trade-go
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
SERVICE

  systemctl daemon-reload
  systemctl enable --now "$SERVICE_NAME"
}

write_nginx_config() {
  log "写入 Nginx 配置..."
  local nginx_conf
  if [[ -d /etc/nginx/sites-available ]]; then
    nginx_conf="/etc/nginx/sites-available/${APP_NAME}.conf"
    cat > "$nginx_conf" <<NGINX
server {
  listen ${WEB_PORT};
  server_name ${DOMAIN};

  root ${INSTALL_DIR}/frontend-dist;
  index index.html;

  location /api/ {
    proxy_pass http://127.0.0.1:${API_PORT};
    proxy_http_version 1.1;
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;
  }

  location / {
    try_files \$uri \$uri/ /index.html;
  }
}
NGINX
    ln -sf "$nginx_conf" "/etc/nginx/sites-enabled/${APP_NAME}.conf"
  else
    nginx_conf="/etc/nginx/conf.d/${APP_NAME}.conf"
    cat > "$nginx_conf" <<NGINX
server {
  listen ${WEB_PORT};
  server_name ${DOMAIN};

  root ${INSTALL_DIR}/frontend-dist;
  index index.html;

  location /api/ {
    proxy_pass http://127.0.0.1:${API_PORT};
    proxy_http_version 1.1;
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;
  }

  location / {
    try_files \$uri \$uri/ /index.html;
  }
}
NGINX
  fi

  nginx -t
  systemctl enable --now nginx
  systemctl restart nginx
}

main() {
  install_system_deps

  ensure_command go "请安装 Go 1.21+，或不要使用 --skip-deps"
  ensure_command npm "请安装 Node.js/npm，或不要使用 --skip-deps"
  ensure_command nginx "请安装 Nginx，或不要使用 --skip-deps"

  prepare_user_and_dirs
  build_backend
  build_frontend

  chown -R "$RUN_USER:$RUN_USER" "$INSTALL_DIR"

  write_systemd_service
  write_nginx_config

  log "安装完成"
  echo "后端服务: systemctl status ${SERVICE_NAME}"
  echo "后端日志: journalctl -u ${SERVICE_NAME} -f"
  echo "访问地址: http://<你的服务器IP>:${WEB_PORT}"
  echo "后端 API: http://<你的服务器IP>:${WEB_PORT}/api/status"
  echo "配置文件: ${INSTALL_DIR}/.env"
}

main
