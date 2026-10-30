---
name: sync-skills
description: 以当前项目的 skill 协议为源，按目标项目的技术栈适配后同步到本机其他 git 仓库；可同步指定条款，也可做全量差异分析。
user-invocable: true
---

# 同步 Skill 协议

以当前项目的 `.claude/skills/` 为源，适配后同步到本机其他仓库。这不是复制文件：先理解目标项目，再改写内容。

通用件（所有项目一字不差的文件，清单见模板仓库 `SETUP.md`「通用件与项目件」）不走本 skill，用模板仓库的 `scripts/sync-protocol.py` 直接同步。本 skill 负责两类事：项目件（`AGENTS.md`、`project-guide.md` 等）需要跟着改的地方，以及脚本报告「本地改过」、不能直接覆盖的文件的合并。

## 两种模式

- **指定内容**：如「把文件删除规范同步到其他项目」「把 commit 规范同步到其他项目的 protocol-dev」。
- **全量差异**：如「同步一下协议」「检查其他项目的协议差异」。

## 流程

1. **找目标仓库**：扫 `~/Desktop/Program/` 下带 `.claude/skills` 的 git 仓库（排除当前项目），列给用户确认范围。

   ```bash
   find ~/Desktop/Program -maxdepth 4 -name .git -prune | while read g; do
     d=$(dirname "$g")
     [ "$d" != "<当前项目绝对路径>" ] && [ -d "$d/.claude/skills" ] && echo "$d"
   done
   ```

2. **理解目标项目**：读它的 `AGENTS.md`（没有就读 `CLAUDE.md`）和 `.claude/skills/*/SKILL.md`，弄清技术栈和已有约束。
3. **出方案**：
   - 指定内容：找到源端对应章节和目标端对应位置，按目标技术栈改写，展示「改哪个文件、哪个位置、改成什么」。
   - 全量差异：逐项对比，列出不同之处和源端新增的内容，由用户挑要同步的，再按「指定内容」处理。
4. **写入**：每个项目单独展示方案、单独确认后再写，写完给出变更摘要。

## 适配原则

- 保留规范的核心意图（如「删除必须可恢复」），按目标技术栈改具体做法；不把 iOS 的细节原样搬进 Web 项目。
- 目标项目已有类似规范但措辞不同时，保留目标项目的写法。
- 目标项目自有、源端没有的规范，不删也不覆盖。
- 往各项目的项目件里机械插入同一段原文时，可以改用模板仓库的 `scripts/sync-protocol-snippet.py`。
