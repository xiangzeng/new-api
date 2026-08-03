# 任务档案：Phase 3 — 定制 UI 在上游新前端重做

> 状态：进行中｜分支：`merge/upstream-20260803`（worktree `../new-api--merge-upstream-20260803/`）｜创建：2026-08-03｜更新：2026-08-03

## 0. 新会话启动指令（AI 必读）

1. 读 `@AGENTS.md` 开发协议（含 RelayTeam fork 扩展段 + 上游约定）与 `@web/AGENTS.md` 前端规范
2. 读本档案全文
3. 复述「当前阶段 + 上次停点 + 下一步」，等授权，禁止直接动手

## 1. 目标与验收标准

- 目标：把 fork 的定制 UI 在上游全新前端（React 19 + TS + TanStack 文件路由 + rsbuild + Base UI + Tailwind + Bun）上重做。后端 API 已全部就绪（Phase 2 完成），本任务纯前端。
- 验收：P0 三项功能可用且 `bun run build` 绿；P1/P2 按批次验收；文案全部走 i18n（英文 key）。

## 2. 分阶段计划

- [x] **P0-1 千人千面管理页** — 完成（本次提交）：`web/src/features/custom-pricing/`（页面 + 列定义 + 添加用户弹窗）+ 路由 `web/src/routes/_authenticated/custom-pricing/index.tsx`（`role >= ADMIN` 守卫）；列表展示已配置分组徽章（分组名 · 倍率）；侧边栏 Users 之后注册「千人千面定价」+ `admin.custom_pricing` 模块开关；后端 `controller/custom_pricing.go` 列表项补 `groups[{name,ratio}]`
- [x] **P0-2 用户表千人千面入口** — 完成（本次提交）：`data-table-row-actions.tsx` 行菜单加「启用/编辑千人千面」、`users-columns.tsx` 用户名旁加 Custom Pricing 徽章；配置弹窗 `users/components/dialogs/user-custom-pricing-dialog.tsx` 与管理页复用同一个
- [x] **P0-4 使用日志用户弹窗近 24h 消耗** — 完成（本次提交，计划外补做）：`usage-logs/components/dialogs/user-info-dialog.tsx` 重构，加近 24h 总额度/请求数 + 分组明细表（配额、请求数、占比）、用户名可复制、分组走 StatusBadge
- [x] **P0-3 邀请返利管理页** — 完成（会话3）：`web/src/features/invitations/`（types + api + 页面 + 表格 + 列定义 + 共用单元格 + 受邀人 Sheet）+ 路由 `/invitations`（`role >= ADMIN` 守卫 + `validateSearch`）；关键词 500ms 防抖即时生效、时间段走 `CompactDateTimeRangePicker`（跨 feature 复用 usage-logs 的），两者均写 URL；受邀人改用右侧 Sheet + `StaticDataTable`（不复刻旧版展开行，移动端天然可用）；侧边栏 4 处注册 + `admin.invitation` 模块开关。后端零改动
- [x] **P1-1 藏充值价切换器** — 完成（会话3）：`pricing-toolbar.tsx` 删掉 'standard'/'recharge' `SegmentedControl` 及其 props/handler（同容器的 `/1M`·`/1K` 保留）；`use-filters.ts` 新增模块常量 `SHOW_RECHARGE_PRICE = false` 并删掉 `rechargePrice` 的 state/setter；`index.tsx` 停止传参；`model-details.tsx:1340` 把详情页直读 URL 的 `search.rechargePrice ?? false` 改成硬编码 `false`（堵住绕行）。下游参数链、`applyRechargeRate`、路由 search schema、三个 i18n key 全部保留，恢复只需改 `use-filters.ts` 一行
- [x] **P1-2 充值卡/折扣回归验证** — 完成（会话3，验证 + 修复）：Bug A（Semi `Tag` 未导入致钱包页崩溃）**已 obsolete**，旧 `RechargeCard.jsx` 随上游重写删除，新 `wallet/components/recharge-form-card.tsx:264` 用普通 div 渲染折扣、`use-topup-info.ts:128` 的 `parseDiscountMap` 另有防御。Bug B（清空配置无法保存）**仍存在**，已修 `payment-settings-section.tsx:428-429`
- [ ] **P2-1 Logo 上传控件**：`web/src/features/system-settings/site/` 加图片上传（后端 `POST /api/option/upload/logo` 已就绪，≤2MB，png/jpg/jpeg/gif/svg/ico/webp）
- [ ] **P2-2 清理错误日志按钮**：usage-logs 功能区加管理员按钮调 `DELETE /api/log/errors`
- [ ] **P2-3 收尾**：i18n 六语言补全（`bun run i18n:sync`）、`bun run build` 全绿、整体走查

## 3. 架构与上下文

- **后端 API 已就绪（Phase 2 重放完成，全部在本分支）**：
  - 千人千面：`GET /api/user/custom-pricing/list`、`GET/PUT/DELETE /api/user/:id/custom-pricing`（AdminAuth；见 `router/api-router.go` adminRoute 段、`controller/custom_pricing.go`）
  - 邀请返利：`GET /api/invitation/summary`、`GET /api/invitation/invitees`（AdminAuth；`controller/invitation.go`）
  - Logo：`POST /api/option/upload/logo`；错误日志清理：`DELETE /api/log/errors`
  - 用户级定价对普通接口的影响：`/api/user/groups`、`/api/pricing` 已返回覆盖后的倍率，前端零改动即正确显示
- **旧前端参考实现（已删，从 git 历史取）**：`git show pre-upstream-merge-20260803:web/src/pages/CustomPricing/index.jsx`（503 行）、`...:web/src/pages/Invitation/index.jsx`、`...:web/src/components/table/invitations/`（5 组件）、`...:web/src/hooks/invitations/useInvitationsData.jsx`——交互逻辑照抄，UI 按新架构重写
- **同源姐妹 fork 参考实现（最高价值参考）**：`/Users/longshun/Desktop/Program/00_use/longjinApi`（main 分支）。它的新前端在 `web/default/src/`，与本仓 `web/src/` 是同一套上游（React 19 + TanStack + Base UI），组件签名完全一致，可直接对照移植而非从旧 Semi 版重写。查改动用 `git -C .../longjinApi diff upstream/main -- <路径>`。
  - **两仓能力差异（移植时必须区分）**：本仓独有 `extra_groups`/`hide_groups` 分组可见性覆盖（`service/group.go:50-61` 真实生效，longjin 的 dto 里没有这两个字段，照抄会丢功能）；longjin 独有千人千面站内通知（pending notice + 历史）
  - **P0-3 邀请返利 longjin 侧无对应实现**，仍需按本仓后端接口从零写
- **新前端关键约定**：文件路由自动生成 routeTree（`bun run build` 再生成）；i18n 英文 key 扁平 JSON，新键先写 `en.json` 再 `bun run i18n:sync` 对齐其余六语言；组件风格看同类 feature（users/pricing/wallet）；详细规范 `web/AGENTS.md`
- **技术决策**：前端整体采用上游实现，定制只做增量；不复活任何旧 JSX/Semi 组件

## 4. 进度台账（每次会话末追加，倒序）

### 2026-08-03（会话3 = P0-3 + P1 全批）

- 做了：P0-3 邀请返利管理页、P1-1 藏充值价切换器、P1-2 充值卡/折扣回归验证（含一处真实 bug 修复）
- **P0-3**：longjin 无对应实现，按本仓 `controller/invitation.go` + `model/invitation.go` 契约从零写；交互参照旧 Semi 版，UI 按上游新架构重做。前端新增 7 文件（`features/invitations/` 6 个 + 路由 1 个）、修改 6 文件（侧边栏 4 处 + i18n 七语言 + routeTree 自动生成）；七语言各补 17 个 key（自译）。后端零改动
  - 结构决策：受邀人明细用右侧 Sheet + `StaticDataTable`（旧版是展开行）——新前端 `MobileCardList` 无展开概念，且 `features/subscriptions` 的 `user-subscriptions-dialog` 已是「某用户的明细列表」既有解法；汇总表与受邀人表共用 `components/invitation-cells.tsx`（身份格 / 额度+请求数格 / 期间消耗格）
- **P1-1**：改 4 文件（`pricing-toolbar.tsx` / `use-filters.ts` / `pricing/index.tsx` / `model-details.tsx`），净 +10/−30 行。发现工具栏之外还有第二个入口——详情页 `/pricing/$modelId` 的 `model-details.tsx:1340` 直读 `search.rechargePrice`，只删切换器挡不住老书签，一并改成硬编码 `false`
- **P1-2**：Bug A 已 obsolete；**Bug B 仍存在且后果比原来更隐蔽**。后端 `setting/config/config.go` 的 `updateConfigFromMap` 对 Map/Slice 字段 `json.Unmarshal` 失败即 `continue`（静默），空串必然失败。实测（临时测试跑完已 trash）：`""` → 旧值原封不动；`"[]"`/`"{}"` → 正常清空。前端可视化编辑器删到最后一条发的是 `"{}"`/`"[]"`（正确），但 JSON 文本框模式整个清空会发 `""` → 保存提示成功但运行时不变、DB 写入 `""`；重启后 `LoadFromDB` 同样失败并回落到**结构体默认值**，`amount_options` 的默认是 `[10,20,50,100,200,500]`，即**被清空的预设充值金额会自己长回来**。修法照搬 4db82f4b 的形状：`payment-settings-section.tsx:428-429` 改成 `.trim() || '[]'` / `.trim() || '{}'`
- 验证：`bun run typecheck` 通过；涉及文件 oxlint 零 error（`pricing-toolbar.tsx:135` 那条 `self-closing-comp` warning 是上游遗留，位于未触碰的 `SegmentedControl` 内）；`format:check` / `copyright:check` 本次文件全绿；`i18n:sync` 七语言 missing/extras/untranslated 均为 0；`bun run build` 成功且 routeTree 已注册 `/invitations`
- commit：尚未提交
- 下一步：P2-1 Logo 上传控件（先给方案再动手）
- 遗留/坑：① 会话1、会话2 遗留项全部仍然有效；② 后端 `GET /api/invitation/summary|invitees` 的 `period_*` 走 LOG_DB 聚合，`start/end` 传 0 时**不加时间过滤 = 全表扫 logs**，已由前端默认时间窗兜住（见下方决策）；③ 后端不支持排序参数，汇总表固定「邀请人数降序」，前端未提供排序 UI；④ **`updateConfigFromMap` 的静默跳过是通病**，同样波及 `PayMethods`、`CreemProducts` 等所有 JSON 型设置项——治本要改上游核心文件、影响全部已注册配置模块，属独立课题，本次只在支付设置这一处做了前端兜底；⑤ 若生产 DB 里 `payment_setting.amount_options` 已被历史 bug 写成 `""`，管理员下次保存支付设置会把它自动纠正成 `"[]"`（UI 显示与运行时行为本就不一致，纠正后一致）

### 2026-08-03（会话2 = Phase 3 前端首轮）

- 做了：P0-1 + P0-2 + P0-4 全部完成。发现同源姐妹 fork `longjinApi` 已在同一套上游新前端做完千人千面，改为「移植 + 合并本仓独有能力」而非从旧 Semi 版重写
- 改动面：后端 1 文件（`controller/custom_pricing.go` 列表补 `groups[]`，`total_groups`/`missing_groups` 保留并存）；前端新增 13 文件、修改 12 文件；七语言各补 45 个 key（41 个取自 longjin 现成译文，4 个可见性覆盖文案自译）
- 验证：`go build ./...` + `go vet ./controller/` 通过；`bun run typecheck` 通过；涉及文件 oxlint 零 error/warning；`bun run format:check` 本次文件全绿；`bun run build` 成功且 routeTree 已注册 `/custom-pricing`；`i18n:sync` 七语言 missing/untranslated 均为 0
- commit：本次提交（前端批 A+B 与本档案更新同提交）
- 下一步：P0-3 邀请返利管理页（longjin 无参考实现，按本仓 `controller/invitation.go` 从零写；先给方案再动手）
- 遗留/坑：① 会话1 遗留项全部仍然有效（worktree 未推送、main 冻结、Phase 5 部署需授权且 compose 补 `MAX_REQUEST_BODY_MB=200`/`STREAMING_TIMEOUT=600`；上游会话体系上线后用户需重新登录；dd74cceb 可见性覆盖在上游新增绕行路径未补全，不阻塞前端）；② `web/` 仓库既有 3 个文件 format 不合规、2 个文件 copyright 待更新（`channel-mutate-drawer.tsx`、`channel-form.ts`、`api-key-group-cell.tsx`、`oauth-callback-mode.ts`、`channel-field-update.ts`），均与本次改动无关，未触碰

### 2026-08-03（会话1 = Phase 0-2/4 主会话）

- 做了：上游合并 + 后端全部重放（13 commit，HEAD `40e55f78`）；迁移安全评估通过（AutoMigrate 5s、零 DROP、custom_pricing 完好）；移植 agent 协作协议
- commit：`74107c66`(merge) … `27a2c06b`(gpt-5 倍率) + `40e55f78`(协议移植)
- 下一步：从 P0-1 千人千面管理页开始（先给方案再动手）
- 遗留/坑：① worktree 未推送、main 冻结，Phase 3 完成后再谈合回/部署（Phase 5 需用户授权 + 部署 compose 补 `MAX_REQUEST_BODY_MB=200`、`STREAMING_TIMEOUT=600`）；② 上游会话体系上线后用户需重新登录（低峰部署）；③ dd74cceb 可见性覆盖在上游新增绕行路径（GetRequestAutoGroups/controller/token.go:172 等）未补全——当前与旧版行为等价，做完整覆盖是后端可选增强，不阻塞前端

## 5. 决策与坑记录

- 已确认（会话2 末）：档案中剩余全部项目（P0-3 邀请返利、P1-1 藏充值价切换器、P1-2 充值卡回归验证、P2-1 Logo 上传、P2-2 清理错误日志按钮、P2-3 收尾走查）均在交付范围内，按 P0-3 → P1 → P2 顺序推进
- 已拍板：gpt-5 无点号家族补全倍率保持 6（让利，代码已改 `setting/ratio_setting/model_ratio.go`）；充值价切换器藏掉只显标准价
- 已拍板（会话2）：千人千面列表接口 `groups[]` 与原有 `total_groups`/`missing_groups` **并存**（前端只消费 `groups[]`，对既有调用零破坏）；配置弹窗**保留**本仓独有的 `extra_groups`/`hide_groups` 可见性覆盖区块
- 已拍板（会话3）：藏充值价切换器采用「只切断入口、下游参数链恒 false」而非连根拔除——`showRechargePrice` 贯穿 8 文件几十处，全删会大幅抬高上游合并冲突面；代价是几处死参数
- 已拍板（会话3）：P1-2 的 Bug B 只做前端兜底（发 `[]`/`{}`），不改 `setting/config/config.go` 的静默跳过逻辑——后者影响全部已注册配置模块，是独立课题
- 已拍板（会话3）：邀请返利页默认时间窗 = 「今天 00:00 ~ 现在+1h」（复用 usage-logs 的 `getDefaultTimeRange()`），保护生产 12GB logs 库。**代价**：清空日期输入并确认会回落到今天，前端不再有「不加时间过滤 = 全量历史」的入口；要看全量需手动把开始时间拉到足够早的日期
- 跨 feature 复用采用「直接 import，不搬到 `src/components/`」：invitations 引用 `@/features/usage-logs/components/compact-date-time-range-picker` 与 `@/features/usage-logs/lib` 的 `getDefaultTimeRange`，与「usage-logs 引用 dashboard/lib」同一先例，零上游文件改动、合并冲突面最小
- 分组消耗聚合统一放 `web/src/features/dashboard/lib/group-usage.ts`（`aggregateFlowGroupUsage`），千人千面弹窗的近 7 日 Top3 与使用日志弹窗的近 24h 明细共用同一份，避免 longjin 那边两处重复实现
- 侧边栏新增模块要改 4 处前端文件：`hooks/use-sidebar-data.ts`（导航项）、`hooks/use-sidebar-config.ts`（DEFAULT + URL 映射）、`system-settings/maintenance/config.ts`（SIDEBAR_MODULES_DEFAULT）、`maintenance/sidebar-modules-section.tsx`（moduleMeta 文案）。后端 `model/user.go` / `controller/user.go` 的默认边栏配置无需改：前端 `mergeWithDefaultSidebarModules` 会补齐缺失键
- 上游错误调试信息只入服务端日志不进客户端错误（上游测试契约）
- 回滚点：tag `pre-upstream-merge-20260803`；生产 DB 每日 04:00 备份在 database-newapi
