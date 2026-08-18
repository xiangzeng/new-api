# 任务档案：使用日志仅错误筛选 + 渠道用量可视 + RPM 水位线

> 状态：**已完成**（六个阶段全部落地，已推 main 并部署生产，历史回填已执行）｜分支：`main`（本仓惯例直接 main）｜创建：2026-08-18｜更新：2026-08-18

## 0. 新会话启动指令（AI 必读）

1. 会话工作目录必须是本仓库根 `RsTroubleDebug/new-api/`，先读 `@AGENTS.md` 开发协议
   （含 protocol-dev / task-dossier 触发规则、三库兼容红线、commit 规范）
2. 读本档案全文，再读对标实现索引（第 3 节）里列出的对标 commit
3. **复述**「当前阶段 + 上次停点 + 下一步」，等用户授权，禁止直接动手
4. 每完成一个阶段：编译 + 测试 + 更新本档案第 4 节台账；不主动 commit，等用户点头

## 1. 目标与验收标准

对标 longjinApi 站点补齐三块能力，全部沿用其已验证实现，不自创设计。

- **需求1 使用日志「仅错误」**：通用日志筛选栏加一键切换，点一下只看失败请求（`type=5`），
  再点回全部类型。
- **需求2 渠道用量可视**：渠道列表「已使用」徽标 hover 出精确总量 + 最近 3 天分日消耗；
  编排页渠道卡片同样有「已使用量」行与同款 hover。数据来自新表 `channel_daily_usages`，
  并**一次性回填最近 30 天历史**，避免上线头两天全是 0。
- **需求3 RPM 水位线**：编排页卡片显示「RPM 当前/水位线」+ 负载条，可就地设定上限；
  级联选路支持「压满即溢出」。总开关默认关闭（只统计不限流）。
- **需求4（随需求2 对标 commit 捆绑）**：编排页底部「分组显示顺序」拖拽表，调整泳道上下顺序。

**整体验收**
- `go build ./...` + `go test ./...` 全绿；`cd web && bun run typecheck && bun run build` 通过
- 新增/改动的前端文件 oxlint 零新增问题、oxfmt 已格式化（**不要整仓 format**）
- 本地 SQLite 冒烟：仅错误筛选可切换、Tooltip 出 3 天数据、回填后昨天/前天非零、
  水位线打满后流量溢出到下一渠道、分组顺序保存后重启仍生效
- 三库兼容：新表与回填 SQL 在 SQLite/MySQL/PostgreSQL 语义下都成立

## 2. 分阶段计划

- [x] **阶段1 使用日志「仅错误」**：纯前端，最小闭环，先做完先验证｜无依赖（2026-08-18 完成，未 commit）
- [x] **阶段2 渠道日用量后端**：建表 + 落库 + 清理 + 查询 API + 历史回填｜无依赖（2026-08-18 完成，未 commit）
- [x] **阶段3 渠道列表 Tooltip**：共享 hook + BalanceCell 受控 Tooltip｜依赖：阶段2（2026-08-18 完成，未 commit；直接落对标最终态）
- [x] **阶段4 RPM 水位线**：RPM 计数 + 三轮选路 + 水位线配置/API + 编排页卡片与设置区｜无依赖（2026-08-18 完成，未 commit）
- [x] **阶段5 编排页分组顺序 + 卡片已用量行**：对标 `b67d96392` + `1952b75db` + `22dc378f4` 三连最终态｜依赖：阶段3（共享 hook）、阶段4（卡片布局）（2026-08-18 完成，未 commit）
- [x] **阶段6 发布验证**：全量构建 + 本地冒烟 + commit → 推 main → 部署 → 生产回填（2026-08-18 完成）

## 3. 架构与上下文

### 3.1 对标实现索引（仓库 `/Users/longshun/Desktop/Program/00_use/longjinApi`，分支 main）

- `544aaf32e` — 通用日志「仅错误」快捷筛选（**需求1 全部**，纯前端）
- `5222e2389` — 渠道每日用量追踪（**需求2 后端起源**：表/落库/清理/API；注意该 commit 的前端是老版
  `web/src/components/table/`，已废弃，不要参考其前端部分）
- `7f6d4d819` — 补回 `go model.UpdateChannelDailyUsage()` 启动逻辑（**必须一起带上**，
  漏了会导致只写内存、重启即丢，Tooltip 数字与使用日志严重不符）
- `2912c1f4a` — 渠道列表「已使用」Tooltip 展示近 3 天（**需求2 前端**）
- `76d5f3598` — 渠道编排 RPM 水位线，压满即溢出（**需求3 全部**）
- `b67d96392` — 编排页分组排序 + 卡片显示已使用量（**需求4 + 需求2 编排页部分**；
  其中把 `useChannelRecentUsageQuery` 从 BalanceCell 抽成共享 hook，两页共用 query key 与缓存）
- `1952b75db` — 编排卡片 tooltip 秒开：整卡包 `TooltipProvider`（delay 0），已用量改「悬停即预取」
  （**阶段5 一并落**，2026-08-18 新增）
- `22dc378f4` — 卡片已使用量并入 `#渠道号` 行右对齐、触发区收窄到金额本身 + `disableHoverablePopup`；
  「错 x% · 延迟」的 1h/24h 明细弹窗移进健康时间线弹窗顶部；`formatMs` 下沉 `features/cascade/lib/format.ts`，
  `MetricsDetailLine` 抽独立组件（**阶段5 一并落**，2026-08-18 新增）
- 对标侧任务档案可直接读：`longjinApi/docs/tasks/cascade-group-order-usage.md`、
  `cascade-rpm-watermark.md`（含他们的决策与踩坑记录）

### 3.2 两仓差异（移植时必须适配）

- 前端根目录：对标 `web/default/src/` → 本仓 `web/src/`
- 错误类型包：对标 `github.com/QuantumNous/new-api/types` → 本仓已拆独立 module
  `github.com/QuantumNous/new-api/relaykit/types`（dto 同理 `relaykit/dto`）
- auto 分组解析：对标 `service.GetUserAutoGroup(userGroup)` → 本仓 `service.GetRequestAutoGroups(c, userGroup)`
- 本仓已有而对标没有的：`RetryParam.ExcludeChannelIDs` 请求级排除、主循环 `c.Writer.Written()` 守卫
- 本仓已落地的级联基座（2026-08-17，commit `d9203068`）对应对标 `6b0ce9200` 之前的状态，
  即 RPM 水位线（`76d5f3598`）与分组顺序（`b67d96392`）是**我们尚未跟进的两个增量**

### 3.3 关键文件与落点

**需求1**
- `web/src/features/usage-logs/constants.ts` — 加 `LOG_TYPE_ERROR_VALUE`（`LOG_TYPE_ENUM.ERROR = 5`）
- `web/src/features/usage-logs/components/common-logs-filter-bar.tsx` — 抽 `applyWithType`，
  类型下拉旁加切换按钮（选中 destructive、未选中红描边）

**需求2 后端**
- 新增 `model/channel_daily_usage.go` — 表结构 + 内存累计 `LogChannelDailyUsage` +
  周期落库 `SaveChannelDailyUsageCache` + 区间查询（查询时合并未落库内存增量，保证「今天」实时）
- 新增 `model/channel_data_cleanup.go` — 每小时清理 30 天前数据（`IsMasterNode` 才跑）
- 新增 `controller/channel_usage.go` — `GET /api/channel/:id/daily_usage`
- 改 `model/log.go` — `RecordConsumeLog`（约 L426 `LogQuotaData` 之后）+ 任务计费日志挂钩，异步 `gopool.Go`
- 改 `model/main.go` — `&ChannelDailyUsage{}` 加入 `migrateDB` 与 `migrateDBFast` 两处清单
- 改 `main.go` — `go model.UpdateChannelDailyUsage()`、`model.StartChannelDataCleanupTask()`、
  优雅退出处 `model.SaveChannelDailyUsageCache()`
- 改 `router/api-router.go` — 注册路由（AdminAuth）

**需求2 历史回填（本仓自研，对标没有）**
- 数据源 `logs` 表：`type=2`(LogTypeConsume)、`channel_id`、`created_at`(unix)、`quota`、
  `prompt_tokens`、`completion_tokens`；日志库句柄是 `model.LOG_DB`（本仓生产与主库同一 PG）
- 三库兼容做法：**不写 DB 方言的日期函数**，在 Go 里按天循环，每天用 `created_at BETWEEN`
  区间 + `GROUP BY channel_id` 聚合（`SUM(quota)`、`COUNT(*)`、`SUM(prompt_tokens+completion_tokens)`），
  共 30 条可移植查询
- 日期口径必须与 `LogChannelDailyUsage` 一致：服务器本地时区的 `2006-01-02`
- 幂等：回填某天前先删该天已有行再插，重复执行结果一致
- 触发方式：管理员接口 `POST /api/channel/daily_usage/backfill`（参数 `days`，默认 30，上限 90），
  AdminAuth + 记管理审计日志；不做自动回填，避免每次重启重跑
- ClickHouse 日志库分支未覆盖（本仓不用），回填前判断 `common.UsingLogDatabase` 时直接拒绝并提示

**需求2 前端**
- 新增 `web/src/features/channels/lib/channel-recent-usage.ts` — 日期区间构造 + 按日期合并（缺失补 0）
- 新增 `web/src/features/channels/lib/use-channel-recent-usage.ts` — 共享查询 hook
  （query key `['channels','daily-usage',id,start,end]`，`enabled` 只在 tooltip 打开时为真，
  `staleTime 60s`；渠道列表与编排页共用同一 key → 悬停一处预热另一处）
- 改 `web/src/features/channels/{api.ts,types.ts,lib/index.ts}`
- 改 `web/src/features/channels/components/channels-columns.tsx` — `BalanceCell`（约 L328）
  的「已使用」Tooltip 改受控，额度隐藏（`sensitiveVisible` 为假）时不请求不展示

**需求3**
- 新增 `model/channel_rpm.go` — 每渠道 60 个 1 秒桶滚动窗口，`RecordChannelRequest` 选中即记账，
  快照惰性剔除 5 分钟无流量的环；`IsChannelOverWatermark` / `channelRpmLoadRatio`
- 新增 `setting/operation_setting/cascade_watermark.go` — `cascade_watermark.channel_rpm`（渠道 ID → 上限）
- 改 `model/channel_cascade.go` — 选路改三轮：正常轮（健康 + 未达线）→ 打满轮（健康渠道全达线时
  交负载率最低者接住）→ 熔断兜底轮（忽略健康与水位线按顺序选）
- 改 `service/channel_select.go` — `CacheGetRandomSatisfiedChannel` 外层包一层记账，
  本体拆为 `selectSatisfiedChannel`
- 改 `middleware/distributor.go` — 亲和可用条件加 `!IsChannelOverWatermark`；亲和命中处补记账
  （否则粘住的渠道 RPM 恒为 0，水位线形同虚设）
- 改 `setting/operation_setting/cascade_setting.go` — 加 `WatermarkEnabled`，**默认 false**
- 改 `controller/cascade.go` + `router/api-router.go` — overview 返回 `rpm`/`rpm_watermark`，
  新增 `POST /api/cascade/watermark`
- 改 `web/src/features/cascade/{api.ts,types.ts,components/channel-card.tsx,components/settings-card.tsx}`

**需求4**
- 改 `setting/operation_setting/cascade_order.go` — 加 `GroupSequence` + `GetCascadeGroupSequence(Positions)`
- 改 `controller/cascade.go` — 抽 `sortCascadeGroups`（孤儿恒沉底 → 在役按 `group_sequence` → 未入列按组名升序），
  `POST /api/cascade/order` 请求体由 `{orders}` 扩为 `{orders?, group_sequence?}`，purge 同步摘除组名
- 新增 `web/src/features/cascade/components/group-order-card.tsx`、`used-quota-row.tsx`
- 改 `web/src/features/cascade/index.tsx` — 本地分组顺序状态，dirty = 组内脏泳道 ∪ 分组顺序变化，
  统一保存栏一次提交两块
- 改 `channel-card.tsx` — `w-56 → w-64`、`p-3 → p-3.5`，已用量行插在指标行下

### 3.4 已定技术决策（用户已拍板，勿再问）

- 历史回填：**做**，只回填最近 30 天
- RPM 水位线总开关：**默认关闭**（RPM 照常统计、编排页可见，但不参与选路）
- 编排页卡片已使用量 Tooltip：**做**，随对标 `b67d96392` 一起（含分组显示顺序表）
- 未入列渠道语义沿用现状：编排顺序里没有的渠道按优先级降序追加队尾

### 3.5 参考文档

- `docs/channel/cascade-failover.md` — 级联母功能设计（需求3/4 需同步补 2.10 RPM 水位线等章节）
- `AGENTS.md` + `.claude/skills/protocol-dev/references/` — 开发/提交/兼容性规范
- `web/AGENTS.md` — 前端规范（i18n 键即英文原文；组件必须自带 `useTranslation()`）

## 4. 进度台账（每次会话末追加，倒序）

### 2026-08-18（会话2）

- 做了：**阶段6 发布验证 + 上线**。全量 `go build ./...`（含 `relaykit` 独立构建）、`go test ./...`
  零失败；前端 typecheck/build/oxlint(改动 19 文件)/format:check/copyright:check 全过
  （`cascade/lib/format.ts` 缺 AGPL 头已补，注意本仓 copyright 脚本要求头与正文之间无空行）；
  本地 SQLite 冒烟四项验收全绿（仅错误筛选取到真实失败日志、近 3 天分日昨天/前天非零、
  水位线打满即溢出、分组顺序重启后仍生效）
- commit：`f642cb54` feat（51 文件 +2635/-303）、`009ea689` docs(tasks)；已推 origin/main
- 部署：Build and Push Docker Image ✅ 3m50s → Deploy new-api to Hong Kong ✅ 33s，
  容器 03:04:20Z 重启，healthy，`/api/status` 200
- **生产回填已执行**：`POST /api/channel/daily_usage/backfill {"days":30}` →
  `2026-07-20 ~ 2026-08-18` 共 134 行，耗时 2.0s（先用 days=1 预演，45ms/8 行）。
  交叉核对渠道 28 昨天：日用量表 quota=2783390750 / 请求数=24766，与 logs 按
  **Asia/Shanghai 自然日**聚合逐字相等
- 上线后状态核查：`cascade_setting.watermark_enabled` 与 `cascade_watermark.channel_rpm`
  在 options 里无记录 ⇒ 走默认（关闭 + 空）＝ 水位线只统计不选路，选路行为与上线前一致；
  周期落库已在跑（`保存渠道日用量数据成功`）；overview 已返回 `rpm`/`rpm_watermark`
- 阶段6 坑记：**核对回填别用 PG 默认会话时区**——PG 是 `Etc/UTC`、应用容器是 `Asia/Shanghai`，
  用 UTC 切天核对会凭空差出一截（渠道 28 昨天差 4.3 亿额度 / 2814 次请求），
  换成 `(now() at time zone 'Asia/Shanghai')` 口径后完全对上
- 后续可选：需要用水位线时，在编排页给渠道逐个设上限并打开总开关；上线首日不建议直接开

- 做了：**阶段5 落地，一步到位落三连最终态**。后端：`cascade_order.go` 加 `GroupSequence`
  + `GetCascadeGroupSequence(Positions)`；`controller/cascade.go` 抽 `sortCascadeGroups`
  （孤儿恒沉底 → 在役按 group_sequence → 未入列按组名升序）、`POST /api/cascade/order`
  请求体扩为 `{orders?, group_sequence?}`（两块可单独提交）、加 `normalizeGroupSequence`
  （去空白/丢空串/首次出现去重）、purge 时同步把组名从展示顺序摘掉。
  前端：新增 `lib/format.ts`、`components/{metrics-detail,used-quota-row,group-order-card}.tsx`，
  `channel-card.tsx` / `index.tsx` / `api.ts` 直接取对标最终态文件（移植前已逐字 diff 确认
  三者与对标 `76d5f3598` 态完全一致，可整文件替换），`health-events-dialog.tsx` 因本仓有
  `renderTimeline` 抽取（避免嵌套三元 lint）而分叉，改为手工打 22dc378f4 的补丁；
  7 locale 各补 3 键；docs 2.7/2.8 + Phase 5 同步
- 验证：`go build ./...`、`gofmt -l`、全量后端相关包 `go test` 绿；新增 `controller/cascade_test.go`
  6 例（排序四种场景 + normalize 两例，testify）；前端 typecheck / build / oxlint(cascade)
  / format:check 绿；`bun run i18n:sync` 无重排
- **SQLite 冒烟（真跑二进制）**：建 default/vip/svip 三组渠道 →
  ① 只提交 `group_sequence`（含空白、空串、重复）→ 落库清洗成 `["vip","svip","default"]`，
  overview 泳道顺序同步；② 只提交 `orders` → 组内顺序翻转、`group_sequence` 原样不动；
  ③ 两块一起提交 → 都生效；④ 空请求体被拒；
  ⑤ 把 vip 从分组倍率里删掉 → overview 标 orphan 且**无视 group_sequence 沉底**；
  ⑥ purge vip → 组名从 `group_sequence` 摘除（渠道因「唯一分组」安全阀被 skip，符合预期）
- commit：无（阶段1~5 全在工作区，34 个文件 +1166/-303）
- 下一步：**阶段6 发布验证**——全量构建 + 冒烟收尾 + 出 commit 信息给用户审核 → 推 main → 部署
- 阶段5 坑记：`health-events-dialog.tsx` 是本仓唯一与对标分叉的 cascade 文件（`renderTimeline`），
  整文件替换会丢掉本仓的 lint 适配，只能手工打补丁；其余三个文件移植前都先 diff 过对标前态，
  确认逐字一致才整文件替换

- 做了：**阶段4 落地**，对标 `76d5f3598` 全量移植。新增 `model/channel_rpm.go`
  （每渠道 60 个 1 秒桶滚动窗口、槽位按 ts 校验天然过期、快照惰性剔除 5 分钟无流量的环、
  `IsChannelOverWatermark` / `channelRpmLoadRatio`）、`setting/operation_setting/cascade_watermark.go`
  （`cascade_watermark.channel_rpm`，渠道 ID → 上限，0/未配置 = 不限流）；
  `cascade_setting.go` 加 `WatermarkEnabled`（**默认 false**）；
  `model/channel_cascade.go` 选路改三轮（正常轮 → 打满轮取负载率最低 → 熔断兜底轮忽略两者）；
  `service/channel_select.go` 把本体拆成 `selectSatisfiedChannel`，外层「选中即记账」；
  `middleware/distributor.go` 亲和可用条件加 `!IsChannelOverWatermark` 并在亲和命中处补记账；
  `controller/cascade.go` overview 返回 `rpm`/`rpm_watermark` + 新增 `UpdateCascadeWatermark`；
  路由挂 `POST /api/cascade/watermark`；前端 `api.ts`/`types.ts`/`settings-card.tsx`（总开关）
  /`channel-card.tsx`（RPM 行 + 三色负载条 + Popover 就地编辑）；7 locale 各补 4 键（fr/ru/ja/vi
  对标留的是英文回落，本仓给了真译文）；`docs/channel/cascade-failover.md` 补 2.10 章节与
  2.3/2.4/2.6/2.7/2.8、边界 2/3、Phase 4
- 验证：`go build ./...`、`gofmt -l`、`go test ./model ./controller ./middleware ./router ./service ./setting/...` 全绿；
  新增 `model/channel_rpm_test.go`（5 例：窗口求和/过期滑出/同余槽位清零复用/非法 ID 不建环/快照剔除）
  与 `channel_cascade_test.go` 追加 6 例（压满溢出、开关关闭走旧版、全员打满取负载率最低、
  全熔断兜底忽略水位线、健康压满优先于熔断、水位线 0 = 不限流）、
  `cascade_watermark_test.go` 2 例（option JSON ↔ map[int]int 往返 + 整体替换、负数按不限流），
  全部按本仓规范用 testify；前端 typecheck / build / oxlint(cascade) / format:check 绿；
  `bun run i18n:sync` 无重排（纯插入 11 行/locale）
- **SQLite 冒烟（真跑二进制 + 假 OpenAI 上游 + 真实 relay 请求）**：建 2 个同组同模型渠道，
  开级联 + 水位线，设渠道1 水位线 2 → 连发 4 条 `/v1/chat/completions`，
  消费日志显示前 2 条落渠道 1、后 2 条自动溢出到渠道 2；overview 显示 `1: rpm 2/2`、`2: rpm 2/∞`；
  关掉总开关后再发 2 条又回到渠道 1（旧版行为）；`rpm=0` 保存后 option 变 `{}`（不留 0 残渣）；
  空 watermarks / 非法渠道 ID 均被挡
- commit：无（阶段1~4 全在工作区）
- 下一步：**阶段5 编排页分组顺序 + 卡片最终态**（`b67d96392` + `1952b75db` + `22dc378f4` 三连）
- 阶段4 坑记：scratchpad 会被清理，冒烟中途 `smoke.db` 与假上游脚本被清掉过一次，重建即可；
  冒烟脚本别依赖上一轮残留

- 做了：**阶段3 落地，直接对标最终态**（跳过 `2912c1f4a` 的内联 query 中间态，因为它在 `b67d96392`
  里已被重构成共享 hook；先落最终态省得阶段5 再改一遍）。新增
  `web/src/features/channels/lib/channel-recent-usage.ts`（本地时区日期区间 + 稀疏行按日期补 0）、
  `lib/use-channel-recent-usage.ts`（共享 hook，query key `['channels','daily-usage',id,start,end]`、
  `enabled` 由调用方给、`staleTime 60s`）、`lib/__tests__/channel-recent-usage.test.ts`（对标回归测试，
  按本仓惯例挪进 `__tests__/` 并补 AGPL 头）；改 `api.ts`（`getChannelDailyUsage`，走
  `channelActionConfig({params, disableDuplicate:true})`）、`types.ts`、`lib/index.ts`、
  `channels-columns.tsx`（BalanceCell 受控 tooltip：精确总量 + 近 3 天分日，loading 转圈/失败出文案，
  `canLoadRecentUsage = sensitiveVisible && !isTagRow` ⇒ 隐藏额度或标签聚合行不请求不展示）；
  7 locale 各补 4 键（`Today` 已有，复用）
- 验证：typecheck 绿；`bun run build` 绿且产物里能 grep 到 `daily_usage`（确认接线进包）；
  单跑 `bun test <新测试>` 2 pass；改动 7 个文件 oxfmt 后 `bun run format:check` 未列入、
  `bunx oxlint` 这 7 个文件 exit 0（仓库级 lint 本就不绿，均为既有文件）
- commit：无（阶段1+2+3 全在工作区）
- 下一步：**阶段4 RPM 水位线**（对标 `76d5f3598`，纯新增，与阶段5 同改 `channel-card.tsx`，先做阶段4）
- 阶段3 坑记：`bun test` 全量跑时，Bun 对 `node:test` 的 `describe()` 兼容不全
  （"describe() inside another test()"），基线就有 9 fail / 6 errors，加我这个文件变 10/7——
  同一个既有毛病（35 个测试文件里 34 个用 describe，命中哪几个看加载顺序），单跑我的文件 2 pass。
  没为此改用 `bun:test`：全仓零个文件用它，偏离约定的代价大于收益

- 做了：**阶段2 落地**。移植对标 `5222e2389` 后端部分 + `7f6d4d819` 启动逻辑，并按本仓适配：
  新建 `model/channel_daily_usage.go`（表 + 内存累计 + 周期落库 + 区间查询**合并未落库增量**）、
  `model/channel_daily_usage_backfill.go`（本仓自研 30 天回填，Go 层按天循环 + `created_at` 区间
  + `GROUP BY channel_id`，无方言函数）、`model/channel_data_cleanup.go`（每小时清 30 天前，仅主节点）、
  `controller/channel_usage.go`（查询 + 回填两个接口，含日期格式/区间/天数上限校验）；
  挂钩 `RecordConsumeLog` 与 `RecordTaskBillingLog`（后者只在 `LogTypeConsume` 时计，口径与回填一致）；
  `migrateDB`/`migrateDBFast` 双清单登记；`main.go` 起落库协程 + 清理任务 + 退出前 flush；
  路由按本仓 `channelPermissionRoutes` 权限表登记（查询 ChannelRead / 回填 ChannelOperate）；
  审计 `middleware/audit.go` + 前端模板 `channel.daily_usage_backfill` + 7 locale 文案
- 验证：`go build ./...`、`go vet`、`gofmt -l` 全绿；新增 `model/channel_daily_usage_test.go` 三例
  （内存合并 / 回填聚合口径 + 幂等 / 保留期清理）通过；`go test ./model ./controller ./middleware ./router` 全绿；
  前端 typecheck + build + oxlint + format:check 绿。
  **SQLite 冒烟（真跑二进制）**：造 6 条日志 → 回填 3 天 → 分日数据完全正确（错误日志 type=5 与 channel_id=0 均未计入）；
  重复回填数值不变（幂等）；days=91 / 日期格式错 / 缺参 / 区间写反都被挡；审计日志落到 type=3 且 action 正确；
  再起假 OpenAI 上游建渠道跑 `/api/channel/test/1`，真实计费链路记 quota=49 tokens=18，
  查询接口立即可见（库中尚无行，走内存合并），SIGTERM 后日志出现「保存渠道日用量数据成功」且行已落库
- commit：无（阶段1+2 都还在工作区，等用户点头）
- 下一步：**阶段3 渠道列表 Tooltip**（对标 `2912c1f4a`）——新增 `channel-recent-usage.ts` 日期区间构造 +
  按日期合并补 0、共享 hook `use-channel-recent-usage.ts`（query key `['channels','daily-usage',id,start,end]`，
  仅 tooltip 打开时 enabled，staleTime 60s），改 `channels-columns.tsx` 的 `BalanceCell` 受控 Tooltip
- 阶段2 决策补记：
  1）回填窗口内的**内存增量必须丢弃**（这些请求已先写进 logs，回填重算已含，留着下次落库会二次累加），
     已在 `dropCachedChannelDailyUsage` 处理并有测试覆盖
  2）区间查询合并内存增量，否则「今天」永远慢一个落库周期（对标只在 today 汇总接口里合并，本仓在区间查询里合并）
  3）任务计费日志只在 `LogTypeConsume` 时计入，与回填只认 `type=2` 的口径对齐
  4）未移植对标的 `channel_health_log` 与 `/channel/today_usage`：前者本仓已有 `ChannelHealthEvent`，
     后者本阶段无消费方（列表「已使用」总量取 `channel.used_quota`），避免死代码

- 做了：**阶段1 落地**。移植对标 `544aaf32e`——`constants.ts` 加 `LOG_TYPE_ERROR_VALUE`；
  筛选栏抽 `applyWithType(nextLogType)`（写回 draft 含 sourceKey → navigate 带 `type:[next]`+`page:1`
  → 失效 `['logs']`/`['usage-logs-stats']`），`handleApply` 收敛为其调用，新增 `isErrorOnly` 与
  `handleToggleErrorsOnly`；类型字段改 `wide + sm:min-w-[16rem]`，内部 flex 装「下拉 + 仅错误按钮」
  （选中 destructive / 未选中玫红描边 outline，带 `aria-pressed` 与 title）；7 个 locale 各补
  `Errors only`、`Show failed request logs only`
- 验证：`bun run typecheck` 绿；`bun run build` 绿；改动 2 个 ts/tsx 跑 `bunx oxfmt --write` 后
  `bun run format:check` 未列入（仅 5 个历史文件）；`bunx oxlint` 两文件零问题；
  diffstat 与对标 commit 完全一致（139/3/2×7，108 insertions 48 deletions）；
  与对标 post-image 逐字 diff，仅差本仓独有的 `ClearErrorLogsButton`（既有功能，非本次引入）
- commit：无（待用户点头）
- 下一步：**阶段2 渠道日用量后端**——对标 `5222e2389` + `7f6d4d819`，建 `channel_daily_usage`
  表/落库/清理/查询 API，再加本仓自研的 30 天历史回填接口
- 遗留/坑：对标仓最新提交仍是 `b67d96392`，无新增量需跟进（会话1 的遗留项已核销）；
  阶段1 未做浏览器实机冒烟（纯前端，留到阶段6 统一冒烟或用户随时 `bun run dev` 目视）

### 2026-08-18（会话1）

- 做了：读完对标仓库全部相关 commit 与其任务档案，核实本仓 6 处适配差异，出三需求方案，
  用户拍板三项决策（回填 30 天 / 水位线默认关 / 编排页卡片也做），建立本档案
- commit：无（代码未动）
- 下一步：从**阶段1**开始——`web/src/features/usage-logs/constants.ts` 加 `LOG_TYPE_ERROR_VALUE`，
  照 `544aaf32e` 改筛选栏；完成后编译验证再进阶段2
- 遗留/坑：对标 `b67d96392` 是刚提交的新增量，落地前先 `git -C ../../../longjinApi log --oneline -3`
  确认没有更新的后续提交

## 5. 决策与坑记录

- **别整仓跑 `bun run format`**：本仓有 5 个历史未格式化文件，整仓格式化会污染无关文件。
  只对新增/改动文件跑 `bunx oxfmt -c .oxfmtrc.json --write <file>`，再用 `bun run format:check` 确认
  自己的文件不在列表里即可
- **lint 同理**：`bun run lint` 仓库级本就不绿，只需保证 `grep cascade|channels|usage-logs` 相关新文件零新增问题
- **i18n 合并姿势**：用 node 脚本把对标 7 个 locale 的键值合并进 `web/src/i18n/locales/*.json`
  （保持原有键序、追加到末尾），再跑 `bun run i18n:sync` 归一化；
  **不要用 Python 重排序**，会把 `footer.newapi...` 混淆键解码并打乱全文件顺序，产生上千行假 diff
- **routeTree.gen.ts** 由 `bun run build` 自动生成，不要手改
- **三库兼容**：`group` 是保留字，用 `commonGroupCol`；`DELETE ... LIMIT` PG 不支持；
  日期计算放 Go 层不写方言函数
- **commit 规范**：先出信息给用户审核再提交，禁止附加 Co-Authored-By 等署名
- **本地冒烟配方**（上次验证级联时跑通，可复用）：
  `go build -o <scratch>/new-api-smoke . && SQLITE_PATH="<scratch>/smoke.db" PORT=13999 GIN_MODE=release ./new-api-smoke`，
  `POST /api/setup` 建 root（密码 ≥8 位）→ `POST /api/user/login` 取 `access_token` →
  管理接口带 `Authorization: Bearer <token>`；API Key 需从 SQLite `select key from tokens` 取全量值
  （列表接口是打码的），调用时前缀 `sk-`；上游可用 python 起个假 OpenAI 服务模拟成功响应
- **探活/渠道测试也会计入渠道日用量**（都走 `RecordConsumeLog`），与对标行为一致；
  我们的探活日志令牌名是「熔断探活」，统计时可据此剔除
