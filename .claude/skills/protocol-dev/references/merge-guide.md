# 分支合并规范

适用：「合并 main」「同步 main 最新代码」「merge xxx 分支」等。

## 1. 合并前看分歧

```bash
git log --oneline --left-right <当前分支>...<目标分支>
```

向用户说明：目标分支有几个新 commit 要进来、当前分支有几个独有 commit、两边各自主要改了什么。

## 2. 合并，不自动提交

```bash
git merge <目标分支> --no-commit --no-ff
```

## 3. 无冲突

告诉用户无冲突，按 commit-guide「合并提交」生成提交信息。

## 4. 有冲突：先停下

1. `git diff --name-only --diff-filter=U` 列出冲突文件，逐处读冲突区域的上下文。
2. 每处说明：当前分支（HEAD）是什么、目标分支是什么、为什么冲突（同位置修改 / 相邻插入 / 逻辑冲突）。
3. 每处给解决策略（保留双方 / 取其一 / 手动融合）和理由，说「确认方案没问题的话我就执行解冲突」，等授权。
4. 授权后按方案解决，确认没有残留冲突标记（`grep -nE '^(<<<<<<<|=======|>>>>>>>)' <文件>`），`git add` 冲突文件，`git status --short` 确认状态。
5. 按 commit-guide「合并提交」生成提交信息。

生成型 / 清单类文件（`project.pbxproj`、`package-lock.json`、`yarn.lock`、`*.csproj`、`Package.resolved` 等）：

- 双方各自新增的条目互不冲突时一般都保留；分组段、配置段逐项对比，不能简单拼接。
- 锁文件优先用包管理器重新生成（`npm install`、`pod install` 等），不手工解。
- 解完检查条目成对完整（如 Xcode 每个 PBXBuildFile 都有对应的 PBXFileReference），没有悬挂引用。

## 不要做

- 合并后自己 `git commit`：只给提交信息，用户看过后再次明确要求才提交。
- 未经用户要求执行 `git merge --abort`。
- 不分析就用 `git checkout --ours` / `--theirs` 整文件取一边。
