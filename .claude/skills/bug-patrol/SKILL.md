---
name: bug-patrol
description: Bug 排查追踪：按用户描述的问题排查，或按功能区系统巡查，把查过哪里、发现了什么记到 00_debug-notes/bug-patrol/，跨会话延续排查进度。
user-invocable: true
---

# Bug Patrol — 排查追踪

记录「查过哪里、发现了什么」，跨会话延续排查进度，看清哪些功能区查过、哪些没查、哪些有已知问题。这里只通过代码审查发现和记录问题；运行时调试和修复走 protocol-dev 的 debug-guide。

## 文件

- 总追踪：`00_debug-notes/bug-patrol/TRACKER.md`（不存在就初始化）
- 会话记录：`00_debug-notes/bug-patrol/sessions/`，模板见 [references/session-templates.md](references/session-templates.md)
- 功能区地图：[references/area-map.md](references/area-map.md)

## 触发

- 问题驱动：「XX 有问题」「我发现 XX 不对」「XX 好像有 bug」→ 模式 A
- 系统巡查：「开始排查」「bug patrol」「巡查 R1.2」→ 模式 B
- 查看状态：「排查进度」「排查状态」「哪些还没查」
- 结束：「结束排查」「收工」

## 模式 A：问题驱动（主要模式）

1. 读 TRACKER.md，用 `git rev-parse --short HEAD` 记下基线 commit，对照 area-map 判断涉及哪些功能区。
2. 向用户复述现象，说明要查的区域和核心文件，问有无补充（复现条件、频率）。确认后建会话文件 `YYYY-MM-DD_问题关键词.md`。
3. 从触发点沿执行路径读代码，重点看：与现象直接相关的分支、异步时序、状态一致性、边界条件、最近 commit 改过的相关代码。每查完一个文件或模块，汇报它做了什么、有无可疑点、下一条线索，并记进会话文件。
4. 给出结论：
   - 代码审查定位到根因 → 写根因分析，登记到 TRACKER 问题清单，告诉用户修复可走 debug-guide。
   - 需要运行时验证 → 记下可疑点和思路，标「待确认」，建议用 debug-guide 加日志验证。
   - 没发现问题 → 记下排查范围和结论，建议扩大范围或补充复现信息。

## 模式 B：系统巡查

1. 读 TRACKER.md 和 area-map，记下基线 commit。
2. TRACKER 里有上次基线时，用 `git diff --name-only <上次基线>..HEAD` 找出变更文件，相关的已查区域标为 🔄 需复查。
3. 展示状态摘要，建议优先方向：高风险未查 > 需复查 > 中风险未查。用户选定区域后建会话文件 `YYYY-MM-DD_区域编号_区域名.md`。
4. 按 area-map 的检查点逐项查，留意边界条件、异步时序、状态一致性、平台版本兼容。发现问题只记录不修复，标严重程度（P0–P3）和是否已确认。

## 结束排查

1. 补全会话文件的「发现」和「结论」。
2. 更新 TRACKER.md：区域排查状态、新问题加入问题清单、会话索引加本次链接。
3. 展示更新后的全局进度。

## 查看状态

读 TRACKER.md，按风险等级展示各区状态，按严重程度列出未修复问题，建议下一步排查方向。

## 规则

- 追踪文件和会话文件的改动，先展示内容，用户确认后再写。
- 会话索引只追加，不删历史会话记录；进度总览和问题清单可以改。
- 排查中只记录问题，不启动修复。用户决定修某个问题时切到 debug-guide；修好后在 TRACKER 把它移到「已修复」，记下修复 commit SHA。
