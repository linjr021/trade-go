#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "用法: bash scripts/release_tag.sh <version> [message]"
  echo "示例: bash scripts/release_tag.sh v1.0.0 \"first stable release\""
  exit 1
fi

VERSION="$1"
MESSAGE="${2:-release ${VERSION}}"

if [[ ! "${VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "[ERROR] 版本号必须是 vMAJOR.MINOR.PATCH 格式，例如 v1.2.3"
  exit 1
fi

BRANCH="$(git branch --show-current)"
if [[ "${BRANCH}" != "main" ]]; then
  echo "[ERROR] 请在 main 分支执行（当前: ${BRANCH}）"
  exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "[ERROR] 工作区不干净，请先提交或暂存改动"
  git status --short
  exit 1
fi

git pull --ff-only origin main

if git rev-parse "${VERSION}" >/dev/null 2>&1; then
  echo "[ERROR] 标签 ${VERSION} 已存在"
  exit 1
fi

git tag -a "${VERSION}" -m "${MESSAGE}"
git push origin "${VERSION}"

echo "[OK] 已发布标签 ${VERSION}"
