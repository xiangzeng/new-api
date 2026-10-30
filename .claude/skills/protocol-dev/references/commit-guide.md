# Commit 信息规范

## 1. 先看实际改动

- 运行 `git status --short` 和 `git diff HEAD --stat`。未跟踪的新文件不会出现在 `git diff` 里，要从 `git status` 补上并读它的内容。
- 逐个看改动文件的实际内容，以 diff 为准，不凭对话内容推测；两者不一致时以 diff 为准。
- 范围是「上次提交到现在的全部改动」，不只是本次对话改的。工作区里混着本任务以外的改动（用户原先没提交的、别的任务的）时，只 `git add` 本任务的具体文件，再用 `git diff --cached --stat` / `git diff --cached --name-only` 核对暂存边界，按暂存区写提交信息。
- diff 里有新增的调试日志（如 `print("🔬")`、`debugLog`）时，先问用户是否清理。用户说「清理调试日志，给出提交信息」时，先清理，再按清理后的 diff 写。
- 合并状态下不用 `git diff HEAD`（HEAD 仍指向合并前，会混进当前分支自己的历史），改用 `git diff --cached --stat` / `git diff --cached --name-only`。

## 2. 格式

`<type>(<scope>): <emoji> <主题>`

- `scope` 可选，改动集中在某个模块时填，否则连括号一起省略。
- type 和 scope 用英文，主题和正文用简体中文。
- 改动较多时，Header 后空一行，用列表写 Body，按功能分组，不逐文件罗列。

type 与 emoji：`feat` ✨ · `fix` 🐛 · `docs` 📝 · `style` 💄 · `refactor` ♻️ · `perf` ⚡️ · `test` ✅ · `chore` 🔧 · `revert` ⏪

```
feat(reader): ✨ 添加阅读进度同步功能

- 新增 ReadingProgressService 处理进度同步逻辑
- 在 ReaderView 中集成进度保存触发点
- 支持 iCloud 跨设备同步
```

`perf`、明显改善体验的 `feat`、修复严重问题的 `fix`：Body 末尾加「优化效果：」小节，写可量化的前后对比，如「生词库加载从 7–8 秒降到 1 秒内」。

## 3. 合并提交

`chore: 🔀 合并 <源分支> 到 <目标分支>，<概括合并内容>`

Body 按功能模块概括合并引入的新功能和修复，不写工程配置文件（如 pbxproj、lock 文件）的变化。

## 4. 只写最终交付的东西

- 覆盖本次要提交的每一处功能性改动，不遗漏；也不把任务外的改动顺带暂存。
- 不写探索过程中的失败尝试、被放弃的方案、中间状态。
- 不写本次提交前就不存在的东西的「移除 / 删除」。
- 不写调试日志的增删和纯格式调整（空行、格式化）。

## 5. 输出

- 只输出一个代码块，内含完整信息（Header + Body）；不拆成 Summary / Description，不写成 `git commit -m "..."` 命令。
- 不加 Co-Authored-By 或任何 AI 工具署名。
- 输出后主动问「确认无误，是否执行提交？」。
