# Git 版本管理规范（新手友好版）

本项目推荐使用 `GitHub Flow + 语义化版本`，目标是：可追溯、可回滚、可稳定上线。

## 1. 分支策略

- `main`：可部署主线，禁止直接开发。
- `feat/*`：新功能分支，例如 `feat/asset-calendar-ui`。
- `fix/*`：缺陷修复分支，例如 `fix/sqlite-permission`。
- `chore/*`：工程维护分支（脚本、依赖、文档等）。
- `hotfix/*`：线上紧急修复，修完优先合并回 `main`。

建议一个需求一个分支，不要在同一分支混多个大需求。

## 2. 提交规范

提交消息格式：

```text
type(scope): summary
```

常用 `type`：

- `feat`：新功能
- `fix`：修复
- `refactor`：重构
- `chore`：维护
- `docs`：文档
- `style`：样式/UI 调整
- `test`：测试相关

示例：

- `feat(frontend): add cloudflare tunnel status card`
- `fix(trader): avoid duplicate order after restart`
- `chore(deploy): improve docker startup checks`

## 3. 日常开发流程

1. 更新本地主线

```bash
git checkout main
git pull --ff-only origin main
```

2. 新建分支

```bash
git checkout -b feat/your-feature-name
```

3. 开发 + 自检

```bash
go test ./...
cd frontend && npm run check
```

4. 提交并推送

```bash
git add .
git commit
git push origin feat/your-feature-name
```

5. 合并回主线（单人项目也建议走此步骤）

```bash
git checkout main
git pull --ff-only origin main
git merge --no-ff feat/your-feature-name
git push origin main
```

## 4. 发布版本（Tag）

使用语义化版本号：`vMAJOR.MINOR.PATCH`

- `MAJOR`：不兼容变更
- `MINOR`：向后兼容的新功能
- `PATCH`：向后兼容的修复

发布命令：

```bash
bash scripts/release_tag.sh v1.0.0
```

脚本会检查：

- 当前分支必须是 `main`
- 工作区必须干净
- 自动 `pull --ff-only`
- 打注释标签并推送到远端

## 5. 回滚流程

按 Tag 回滚最稳：

```bash
git fetch --tags
git checkout v1.0.0
bash scripts/deploy_docker.sh --skip-docker-install --with-tunnel
```

## 6. 上线前最小检查

- `go test ./...` 通过
- `cd frontend && npm run check` 通过
- Docker 可构建
- `.env` 已确认生产参数
- 备份 `data/` 与 `.env`

## 7. 不要提交的内容

- `.env`
- `data/`（数据库与运行态）
- 本地编辑器文件（如 `.vscode/`）

本仓库已通过 `.gitignore` 忽略以上内容。
