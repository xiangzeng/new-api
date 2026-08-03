# 版本发布规范（骨架）

> 本文件是**通用骨架**，请按你项目的实际发布渠道改写；若项目暂无发布流程，删除本文件并去 `protocol-dev/SKILL.md` 和 `AGENTS.md` 移除对应索引。

## 触发条件

用户说"准备发布新版本"、"我要发布新版本"、"出个新版本"等。

## 核心原则

1. **先谋后动**：查完信息、生成日志后**展示**，等用户确认再改文件 / 打 tag。
2. **不自动发布**：未经用户明确授权，不执行 `git tag` 推送、上传、部署等对外动作。

## 工作流程

### 第一步：确定版本号与构建号

- 版本号来源：`common/constants.go` 的 `Version` 为构建时注入占位（`-ldflags`），实际版本 = git tag / GHCR 镜像 tag，不手改代码
- 必要时遍历分支 / tag 找最新版本，避免版本回退
- 按语义化版本（MAJOR.MINOR.PATCH）或项目约定决定下一个版本号

### 第二步：生成更新日志

- **技术日志**：`git log <上一个 tag>..HEAD --oneline`，按 type 归类（feat / fix / perf ...）
- **面向用户的发布说明**：从技术日志提炼用户可感知的变化，用产品语言重写（隐藏内部重构 / 调试细节）
- 如渠道需要审核备注 / 迁移说明，一并生成

### 第三步：展示并等待确认

- 展示：新版本号、技术日志、发布说明草稿
- 等用户确认后，再执行版本号写入、changelog 更新、打 tag 等

### 第四步：发布动作（按渠道）

本项目渠道 = **容器 / 服务（GHCR + 香港自动部署）**：

- push 到 origin main → `.github/workflows/docker-image-main.yml` 构建镜像推 GHCR → `deploy-hk.yml` 部署 shan-dmit-hk
- 部署前置：确认 GitHub Actions 构建绿；生产 DB 全量备份（database-newapi 侧 systemd timer 或手动 `sudo systemctl start newapi-backup.service`）
- ⚠️ 严禁触发上游（原作者命名空间）的构建/同步 workflow；本 fork 已删除 docker-build.yml / docker-image-branch.yml / sync-release-to-gitcode.yml，勿复活
- 回滚：git 侧回滚 tag + DB 侧恢复备份（恢复命令见 RelayTeamVps/database-newapi/README.md）

## 禁止事项

- ❌ 未经确认直接改版本号 / 打 tag / 上传 / 部署
- ❌ 版本号回退或跳号而不说明
- ❌ 把内部调试 / 重构细节直接写进面向用户的发布说明
