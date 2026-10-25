# 渠道级联与熔断恢复（channel-cascade）设计与开发计划

> 状态：已落地（RelayTeam fork）

## 1. 背景与目标

现有优先级机制的问题：重试次数默认为 0 时，最高优先级渠道一旦报错，请求直接失败，
不会切换到低优先级渠道；现有「自动禁用/自动启用」直接改数据库渠道状态，过重且难以恢复，
与手动管理的启用/禁用状态互相污染。

目标：

1. **级联流向**：组内渠道按顺序严格遍历，前一个报错立即把原请求转发给下一个，
   全部试完才把错误返回给用户。
2. **运行时熔断**：报渠道故障类错误的渠道被打上内存标记（不改数据库状态），
   后续请求自然绕过。
3. **探活恢复**：只对被熔断的渠道每分钟探活，连续 N 次（默认 3）成功后恢复；
   健康渠道零探活成本。
4. **集中管理页**：新侧边栏页面「渠道编排」，画布式展示各分组级联顺序，
   拖拽换序，健康状态实时可见，本功能全部配置集中在该页。

## 2. 总体架构

```
请求 → distributor(亲和优先) → relay 重试循环
                                   │ 每次迭代
                                   ▼
                        级联选择器（cascade 开启时）
                        按优先级降序遍历，跳过：
                          - 本次请求已试过的渠道
                          - 熔断中的渠道（内存健康标记）
                                   │ 报错
                                   ▼
                        错误分类器 IsChannelFaultError
                          fault → 标记失败(可能触发熔断) + 清亲和 + 切下一个
                          非fault(如 400) → 直接返回用户
                                   
后台探活循环（每 probe_interval 秒）
  只探被熔断渠道 → 连续 N 次成功 → 恢复健康
```

### 2.1 健康状态层（`model/channel_health.go`）

- 纯内存注册表 `map[channelId]entry`，重启即全部恢复健康（自愈，无历史遗留）。
- 状态：`healthy` / `cooling`（熔断冷却）/ `probing`（探活恢复中，冷却期已过）。
- 触发：连续 `failure_threshold` 次故障类错误 → 熔断。
- 恢复：连续 `recovery_success_count` 次成功（探活或真实请求均计数），失败清零。
- 兜底：探活关闭时，冷却期满自动放行（半开），再失败立即重新熔断。
- **不写渠道表、不动启用/禁用状态**；手动禁用的渠道不参与探活。

### 2.2 错误分类（`service/channel_health.go`）

默认视为渠道故障（触发切换 + 熔断计数）：

- 网络/连接类错误（`types.IsChannelError`）
- HTTP 429、401、403、5xx

默认不处理（不切换、不标记，原样返回用户）：400 等其余状态码。

可配白名单：

- `extra_fault_status_codes`：额外视为渠道故障的状态码
- `extra_fault_keywords`：错误内容命中关键词则视为渠道故障（应对把渠道故障报成 400 的上游）
- `ignore_fault_keywords`：反向排除，命中则不视为渠道故障（优先级最高）

容量类错误（`capacity_detour_keywords`，`IsChannelCapacityError`）：上游明确报「本次请求超出该渠道
输入上限」（如部分 Kiro 号商的 `exceeding the native input budget 124000`）。这是请求级问题、渠道本身健康：

- 本次换下一个渠道重试，但计 attempt 不计故障、不熔断、不自动禁用、不改归属；
  判定优先于 `extra_fault_*` 白名单与 5xx（`ignore_fault_keywords` 不影响它）。
- 按「亲和键 + 渠道」记忆 10 分钟（`service/channel_affinity_capacity.go`）：期间同一亲和键的请求
  归属绕行（`detour_reason=capacity_exceeded`），级联选路排除该渠道；排除后无渠道可选时放回再选。
- 记忆固定时长、跳过不续期：到期回该渠道试一次，上下文已压缩即自然回归，仍超限则再记一轮。
- 背景（2026-10-17）：新渠道 #58 输入上限 124k，大上下文用户被归属钉在 #58，400 不切换，
  每次请求都失败且 Claude Code 自动压缩阈值更高、`/compact` 同样超限，用户无法自救。

### 2.3 级联选择器（`model/channel_cache.go` + `service/channel_select.go`）

- 新增 `GetCascadeSatisfiedChannel(group, model, path, excludeIds)`（已试渠道集 =
  `use_channel`（选中即写）∪ `RetryParam.ExcludeChannelIDs`（失败即写））：
  候选渠道按分组「编排顺序」（`cascade_order.group_orders` 配置，与渠道优先级解耦）
  严格依次遍历，取第一个可用（健康且未试过）的渠道；未配置顺序的分组、
  未入列的渠道按优先级降序、id 升序兜底（同一渠道可在不同分组排不同位置）。
- 三轮兜底：
  1. **正常轮**：健康、未试过、且 RPM 未达水位线（见 2.10）；
  2. **打满轮**：健康渠道全部达到水位线时，交给负载率（当前 RPM / 水位线）最低的
     渠道接住溢出——退回顺序第一个等于把溢出全砸在最满的渠道上；
  3. **熔断兜底轮**：全部熔断时忽略健康标记与水位线，仍按顺序选一个未试过的渠道
     （服务可用性优先于标记）。
- 例外：**今日额度已满**的渠道（见 2.11）三轮一律跳过，含熔断兜底轮——等同禁用，
  只剩它可选时宁可返回无可用渠道也不超支。
- 重试上限：级联开启时不再使用「最大重试次数」，上限 = 组内候选渠道数
  （可用 `max_attempts_per_request` 封顶）。
- auto 分组：组间顺序即级联外层，组内用级联选择器，当前组耗尽后切下一组。

### 2.4 渠道亲和耦合

按 `channel_affinity_setting.home_mode` 分两种模式：

**旧模式（home_mode 关闭，默认）**

- 粘住的渠道报故障类错误时：清除该会话亲和缓存（`ClearCurrentChannelAffinityCache`，
  同时重置 skip-retry 标记），级联接管选下一个；成功后由现有逻辑自动重绑到新渠道。
- 粘住的渠道 RPM 达到水位线时同样让位（`IsChannelOverWatermark`）——否则长会话客户端
  会绕开级联选择器把水位线整个架空。

**归属模式（home_mode 开启）**

- 亲和缓存记录的是「归属渠道」。归属渠道熔断中 → 本次请求走级联选择器**临时绕行**
  （`MarkChannelAffinityDetour`），不清除、不改写归属；成功后仍写回归属并刷新 TTL，
  渠道恢复后自动回归——上游 prompt cache 大概率仍在有效期内，避免「搬家式」重建缓存。
- 归属渠道达水位线**仍沿用**，否则会在阈值附近逐请求来回弹。可配硬顶倍数
  `home_hard_watermark_ratio`（>0 时 RPM ≥ 水位线 × 倍数才绕行，`IsChannelOverWatermarkRatio`；
  0 = 不设硬顶）。
- **首次落位与归属流量同用硬顶**：命中规则但尚无归属（缓存 miss / TTL 过期 / 永久失效后重落位，
  `IsChannelAffinityFirstPlacement`）时，级联选择器正常轮按 `水位线 × 倍数` 判定
  （`GetCascadeSatisfiedChannel` 的 `watermarkRatio` 参数），重试循环内每次重选同样沿用。
  若落位用软水位线而留驻用硬顶，主渠道稍忙就会把新用户落到备用渠道并永久归属（生产实测：
  老站用户被钉在无水位线的第三方渠道数小时）。倍数为 0 时首次落位退回软水位线（旧行为）。
  软水位线只约束绕行等临时流量。
- 故障类错误不再清亲和（`ClearCurrentChannelAffinityCacheOnFault` 在归属模式下只重置
  skip-retry）；仅渠道被禁用 / 删除 / 移出分组（永久失效）时清除并重新落位。
- 归属渠道仍在分组内、只是**不支持本次请求的模型 / 路径**时同样绕行（`detour_reason=model_unsupported`），
  不清归属——亲和键是用户级不含模型，否则同组渠道模型列表不齐时，用户发一条备用渠道独有的模型
  就会被永久搬过去。
- 本亲和键近期在归属渠道上**超出过输入上限**（见 2.2 容量类错误）时，记忆有效期内同样绕行
  （`detour_reason=capacity_exceeded`，判定顺序在额度已满之后、水位线之前）；请求内在归属渠道上
  命中容量错误换渠道时，日志同样标这个原因。
- 请求内重试切到别的渠道成功也不改写归属；无归属时首次落位记实际成功的渠道。
- 日志 `admin_info.channel_affinity` 增加 `home_channel_id` / `detour` / `detour_reason`
  供事后核对。

两种模式通用：级联开启时，亲和的 `SkipRetryOnFailure` 不再阻断故障类错误的切换。
归属渠道**今日额度已满**（见 2.11）按临时不可用处理（`detour_reason=over_daily_quota`，
判定顺序在熔断之后、水位线之前）：旧模式让位即清除，归属模式保留归属本次绕行——
次日额度重置后归属用户自动回到它，便宜渠道仍能优先吃满当天额度。
归属渠道只在该用户**被封的分组**下可用时同样按临时不可用处理（`detour_reason=user_group_blocked`），
解封后回归；见 [user-group-block.md](user-group-block.md)。

### 2.5 探活恢复（`controller/channel_health_probe.go`）

> 探活走渠道测试通道会真实消耗 token。本 fork 给探活请求打了标记：消耗日志的令牌名/内容为
> `熔断探活`（常量 `controller.CascadeProbeTokenName`），`other.cascade_probe = true`，
> 便于在用量统计里把探活流量与手动「模型测试」、真实业务流量区分开。

- 后台循环，间隔 `probe_interval_seconds`（默认 60）。
- 只探 `cooling/probing` 状态且未被手动禁用的渠道，复用 `testChannel` 基建
 （渠道配置的测试模型）。
- 结果回写健康注册表；连续 N 次成功恢复，期间任一失败清零重数。

### 2.6 配置块（`setting/operation_setting/cascade_setting.go`）

注册键 `cascade_setting`，走现有 `config.GlobalConfig` 持久化与 option API：

| 字段 | 默认 | 说明 |
|---|---|---|
| `enabled` | false | 总开关（关闭时一切走原有逻辑） |
| `failure_threshold` | 2 | 连续故障 N 次触发熔断（上游 429 偶发，1 次即熔断过激） |
| `cooldown_seconds` | 120 | 冷却时长（探活关闭时的半开兜底窗口） |
| `probe_enabled` | true | 是否对熔断渠道主动探活 |
| `probe_interval_seconds` | 60 | 探活间隔 |
| `recovery_success_count` | 3 | 连续成功恢复门槛 |
| `max_attempts_per_request` | 0 | 单请求最大尝试渠道数，0=组内全部 |
| `watermark_enabled` | false | RPM 水位线总开关（关闭时照常统计 RPM，但不参与选路） |
| `extra_fault_status_codes` | [] | 白名单：额外视为渠道故障的状态码 |
| `extra_fault_keywords` | [] | 白名单：命中即视为渠道故障的关键词 |
| `ignore_fault_keywords` | [] | 排除：命中即不视为渠道故障的关键词 |
| `capacity_detour_keywords` | ["native input budget"] | 容量绕行：命中即本次换渠道，不计故障、不熔断、不改归属，该亲和键 10 分钟内跳过该渠道 |

### 2.7 编排 API（`controller/cascade.go` + `router/api-router.go`）

- `GET /api/cascade/overview`（AdminAuth）：分组 → 有序渠道列表
  （id/名称/类型/优先级/权重/启用状态/健康快照/近 60s RPM/水位线/
  每日额度上限 `daily_quota_cap`/今日累计消耗 `today_used_quota`）+ 当前配置。
- `POST /api/cascade/order`（AdminAuth）：`{orders:[{group, channel_ids}], group_sequence:[组名]}`，
  两块都可单独提交。`orders` 按分组保存组内编排顺序（写 `cascade_order.group_orders`，
  不改渠道优先级，跨组互不影响）；`group_sequence` 保存分组泳道在编排页的展示顺序
  （写 `cascade_order.group_sequence`，**纯展示**，不参与路由，孤儿分组不入列）。
- `POST /api/cascade/watermark`（AdminAuth）：`{watermarks:[{channel_id, rpm, daily_quota}]}`
  保存渠道限流方式——RPM 水位线（写 `cascade_watermark.channel_rpm`）或每日额度上限
  （写 `cascade_watermark.channel_daily_quota`，quota 单位），**二选一**：两者同时 > 0 直接拒绝；
  提交即整体覆盖该渠道的限流方式（只传 rpm 会清掉额度上限，反之亦然），两者都 <= 0 = 不限
  （清除即删键，不留 0 值残渣）。未提交的渠道保持原值。
- `POST /api/cascade/purge_group`（AdminAuth）：清理孤儿分组，把组名从所有渠道的
  `group` 字段摘除并重建 abilities，同时删掉该组的 `cascade_order` 组内顺序与展示顺序占位。
  安全阀：只接受**不在**分组倍率配置里的分组；只挂着这一个分组的渠道会被跳过
  （摘完将失去全部 ability 成为不可路由的孤岛），在响应 `skipped` 里返回。
- 配置读写复用现有 option API（键 `cascade_setting`、`cascade_order`）。

### 2.8 前端页面（`web/default/src/features/cascade/`）

- 侧边栏「管理员」组新增「渠道编排」（url `/cascade`）。
- 级联画布：按分组分区，组内渠道卡片从左到右按编排顺序排列，卡片间箭头示意流向；
  卡片显示序号、名称、健康徽标（正常/熔断中+冷却剩余/恢复中 探活 x/N）、
  限流行（点开就地切换「RPM 水位线 / 每日额度上限」并改数值，0/留空 = 不限）：水位线模式显示
  `RPM 当前/水位线`，额度模式显示 `今天 已用/上限` + 右侧小字 RPM，均带负载条
  （<70% 绿、≥70% 黄、打满红；额度金额随敏感信息开关打码）；今日额度已满时健康徽标旁加
  「今日额度已满」徽标（悬停提示 0 点自动恢复）且卡片按禁用样式淡化；近 1h 错误率/延迟、近 1h 缓存命中率/写入率（按 token 加权，
  命中 ≥85% 绿 / 60–85% 黄 / <60% 红，悬停出样本数与未缓存占比，24h 口径在健康时间线弹窗）、
  已使用量（hover 出精确总量 + 近 3 天分日消耗，
  与渠道列表共用 `/api/channel/:id/daily_usage` 查询缓存，仅悬停时发起请求）、
  连续失败次数、最近错误摘要；HTML5 拖拽换序 + 保存落库。
- 用户消耗排名：卡片与渠道列表的「…」菜单新增「用户消耗排名」弹窗（`GET /api/channel/:id/user_usage`，
  `start_timestamp / end_timestamp` unix 秒，跨度 ≤ 90 天，最多 200 行），数据源为数据看板小时桶 `quota_data`
  按 `channel_id` 聚合并合并未落库的内存增量；预设今天 / 近 3 天 / 近 7 天 / 近 30 天，列为排名、用户、消耗、
  倍率、占比、请求数、Tokens，点用户名跳 `/users/:id?dim=model&channel=<id>` 看该用户在本渠道的模型明细。
  响应按 用户 × 分组 带 `groups` 明细，每个分组附当前定价配置下的生效倍率（`service.FillChannelUserUsageRatios`，
  取值顺序与 `relay/helper/price.go` 一致：千人千面 > 用户组特殊倍率 > 分组默认倍率，再乘站长折扣；
  `ratio_source` 标明来源），千人千面来源在弹窗里标黄——注意是当前配置值，不是请求当时的实际倍率。
- 缓存口径（`service/text_quota.go` → `model.RecordChannelCacheUsage`）：消费结算时按渠道记入
  同一套分钟桶。Claude 语义 prompt 不含缓存，总输入 = 未缓存 + 命中 + 写入；OpenAI / Gemini 语义
  prompt 已含命中，总输入 = prompt、无写入。只收聊天类入站格式（chat / responses / messages /
  gemini），embedding / 图片 / 音频 / rerank 不进分母。与错误率一样进程重启清零。
- 配置集中区：cascade_setting 全部字段的表单。
- 分组显示顺序表（页面最底部）：一行一个在役分组（组名 + 渠道数），拖左侧手柄改上下顺序，
  与组内拖拽共用顶部那条「未保存的顺序改动」悬浮栏，一次保存同时提交两种顺序；
  未入列分组按服务端顺序垫底，孤儿分组恒沉底、不进此表。
- 孤儿分组（overview 返回 `orphan: true`）：泳道标题打「已失效」红标 + Tooltip 说明，
  并提供「清理分组」按钮（确认弹窗点名将被跳过的渠道，动作不可逆）；这类泳道排在最后。
- 数据 5s 轮询刷新。

### 2.9 流式「安静断流」的健康归因

流已向客户端吐字后失败无法透明切换，但仍会计入健康标记，让用户重发时绕开故障渠道。
对「未返回错误对象」的流式请求按结束原因归因（`service.ClassifyStreamEnd`）：

| 结束原因 | 归因 | 说明 |
|---|---|---|
| Done / HandlerStop | 成功 | 正常收尾 |
| Timeout | 故障 | 上游停止发送数据 |
| ScannerErr | 故障 | 连接读错误（RST 等） |
| EOF + 未收到完成标记 | 故障（可配 `incomplete_stream_as_fault`，默认开） | 上游安静断流 |
| EOF + 收到完成标记 | 成功 | Claude 正常流即 EOF 结束 |
| ClientGone / Panic / PingFail | 中性 | 无法归因于渠道，不计成功也不计故障 |

完成标记跟踪（`StreamStatus.EnableCompletionTracking/MarkCompletion`）当前由 Claude SSE
适配器实现（`claudeInfo.Done`，即 message_delta 携带 stop_reason）；未接入跟踪的适配器
（OpenAI/Gemini 等）EOF 维持成功语义不误伤，硬故障仍由 Timeout/ScannerErr 覆盖。

### 2.10 RPM 水位线溢出（`model/channel_rpm.go`）

给渠道配一条 RPM 水位线，压满即让路——把「报错才切换」的被动熔断，补上「压满就让路」
的主动限流（对标 Kiro-RS 号池的 `watermarkRpm`）。

- **计数**：每渠道 60 个 1 秒桶的滚动窗口，`RPM = 近 60 秒之和`；纯内存，重启清零，
  与健康注册表同语义。
- **记账点**：**选中即记账**——`service.CacheGetRandomSatisfiedChannel` 返回渠道时、
  以及亲和命中时各 +1。选中而非请求完成时记账，并发突发下第二条请求才能立刻看到新计数。
  口径 = 实际发往上游的 attempt 数（一次请求溢出 N 个渠道，则这 N 个渠道各 +1）；
  探活与渠道测试不走选择器，天然不计入。**与仪表盘 RPM（近 60s 落库消费日志数）
  不是同一口径**，高错误率时两者会有可见差异。
- **配置**：`cascade_watermark.channel_rpm`（渠道 ID → RPM 上限），与 `cascade_order`
  同构地存在 option 里，不落渠道表。`0` / 未配置 = 不限流，该渠道永不视为打满。
- **判定**：`IsChannelOverWatermark` = 当前 RPM ≥ 水位线；总开关 `watermark_enabled`
  关闭时恒为 false（RPM 照常统计，编排页可见，但不参与选路），便于先观察真实 RPM
  再决定水位线数值。
- **软限流**：全员打满时仍照常发请求（按负载率摊到最空的渠道），不向用户返 429，
  不替代真正的限速。

### 2.11 每日额度上限（`model/channel_daily_quota.go`）

给渠道配一个「每天最多消耗多少」的上限，与 RPM 水位线**二选一**（由编排 API 保证）。
典型用法：每天只能承受固定额度的便宜渠道排在编排前面优先吃满，满额后流量溢出到后面的渠道。

- **口径**：与卡片「已使用」一致 = 消费日志 `quota`（用户实付，含分组倍率 / 千人千面），
  不是上游成本。「天」= 服务器本地时间自然日（生产容器 `TZ=Asia/Shanghai`，即北京时间 0 点切日），
  与渠道每日用量统计（`channel_daily_usages`）同一个日期。
- **计数**：内存「今日累计」，在 `LogChannelDailyUsage` 同一把锁内累加（消费日志 + 任务计费日志），
  不随用量缓存刷库清零，跨日自动归零；选路热路径不碰数据库。进程启动时同步做种，
  此后每 60 秒与「库 + 未刷库缓存」取较大值对账——补上启动前已落库的消耗，以及蓝绿切换时
  另一实例（退出时才刷库）的消耗；对账只往大调。
- **配置**：`cascade_watermark.channel_daily_quota`（渠道 ID → 每日上限，quota 单位），
  与 `channel_rpm` 同一配置块。`0` / 未配置 = 不限。**不受 `watermark_enabled` 总开关控制**
  （那是 RPM 观察期的安全阀），配了即生效；与水位线一样只在级联模式下参与选路。
- **硬限流**：`IsChannelOverDailyQuota` = 今日累计 ≥ 上限。满额渠道三轮全跳过（含熔断兜底轮），
  效果等同手动禁用、次日 0 点自动恢复；但**不改渠道 status**——不触发禁用通知，
  不触发「渠道禁用即清归属」，也不需要零点任务重新启用。
- **会略超上限**：消耗在请求完成后才记账，跨线瞬间仍在途的并发请求会让当天实际消耗
  比上限多出几单的量；不做预扣。

### 2.12 渠道级超时（`relaykit/dto/channel_settings.go`）

外接号商渠道常见的挂死形态是：上游回了 200 + SSE 头，然后靠心跳行把连接吊着，
一条 `data:` 都不吐。这种请求全局配置抓不住——`RELAY_TIMEOUT` 是全局值，
必须放到最慢渠道的水平；`STREAMING_TIMEOUT` 是空闲计时，而心跳行会把它无限续命。
所以超时下放到渠道，配在渠道设置的高级区：

| 字段 | 语义 | 落点 |
|---|---|---|
| `total_timeout_seconds` | 单次上游请求墙钟硬上限（含流式读完 body），`0` = 沿用全局 `RELAY_TIMEOUT` | `doRequest` 里设在 `http.Client` 浅拷贝上，不污染共享缓存客户端 |
| `first_token_timeout_seconds` | 从**请求开始**算，没吐出任何有效 `data:` 就掐断，`0` = 不启用 | `StreamScannerHandler` 的独立定时器，收到首条 data 即解除 |

两者都受 `MaxChannelTimeoutSeconds`（7200）上限约束，且 `first_token ≤ total`（total > 0 时），
保存时由 `ChannelSettings.ValidateTimeouts` 校验。首字节超时按 `info.StartTime` 计时，
所以「等响应头」的时间也算在内。

首字节超时以 `StreamEndReasonTimeout` + `ErrFirstTokenTimeout` 收尾，因此直接复用 2.9 的
健康归因，被判为**故障**并计入熔断——反复挂死的渠道会被自动绕开，不需要额外接线。

配值靠日志里的 `other.frt`（首字节耗时，毫秒）分位数来定：取该渠道正常请求的 p99.9
再留一档余量，避免误杀「推理很久才开口」的正常长请求。

补充两个全局开关（`.env`）：

- `STREAMING_TIMEOUT_DATA_ONLY`（默认 `false`）：打开后 `STREAMING_TIMEOUT` 的空闲计时
  只由真正的 `data:` 行重置，心跳行不再续命，让它变成真正的「无内容超时」。默认关闭
  保持历史行为，确认没有误杀后再切开。
- `ZERO_RESPONSE_NO_CHARGE`（默认 `true`）：流式请求上游一条 data 都没吐时跳过计费
  （`service.PostTextConsumeQuota`），消费日志记 `other.zero_response_no_charge`。
  判定条件是「扫描器跑过（`StreamStatus != nil`）+ `ReceivedResponseCount == 0` +
  补全 token 为 0」，不走扫描器的适配器不受影响。

## 3. 边界与已知限制

1. **流式响应已开始向客户端吐字后 failure 无法透明切换**——级联的自动切换只覆盖首字节前
   的失败；吐字后的异常断流按 2.9 计入熔断标记，用户在同一会话重发即可绕开故障渠道。
   本 fork 在流已写出后不再回写 JSON 错误体（避免污染已发送响应），因此「该线路已临时隔离，
   直接重试即可换线路」的提示只随 relay 错误日志留痕，不会追加到客户端响应里。
2. 健康标记与 RPM 计数均为单实例内存态；多实例部署时各实例独立熔断、独立计数
   （实际水位线为配置值 × 实例数，当前生产为单实例，可接受；
   如未来多实例，可将注册表迁移到 Redis）。
3. 多 Key 渠道（multi-key）以整个渠道为熔断粒度，与「一个渠道=一个号池」的使用方式一致。
   RPM 水位线同样以整个渠道为粒度，不细分到 Key。
4. 指定渠道调试（specific_channel_id）与 SkipRetry 类内部错误不参与级联。
5. 每日额度上限按请求完成后的计费记账，跨线瞬间的在途请求会略超上限（见 2.11）；
   日期跟随服务器时区，改容器 `TZ` 会同时改变切日时刻与渠道每日用量的日期口径。
6. **编排页的分组来自渠道 `group` 字段，不是分组倍率配置**——在分组倍率里删掉一个分组
   只会让它从用户侧（`/api/group`、密钥创建下拉）消失，渠道身上的组名与 abilities 仍在，
   编排页因此仍会列出它。这类分组标记为 orphan，需用「清理分组」显式摘除。
7. 容量绕行记忆是单实例内存态，重启 / 蓝绿切换后清空（代价是多白跑一次）；只对命中亲和规则的
   请求记忆，无亲和键的请求每次仍会先试一次该渠道再换。上游若在开始回流后才报超限
   （吐字后），同第 1 条无法切换。

## 4. 阶段划分

- Phase 1a：健康状态层（model/channel_health.go）
- Phase 1b：错误分类 + cascade_setting 配置块
- Phase 1c：级联选择器接入重试循环 + 亲和耦合
- Phase 2：探活恢复循环
- Phase 3a：编排 API
- Phase 3b：前端「渠道编排」页面
- Phase 3c：孤儿分组「已失效」标记 + 一键清理
- Phase 4：RPM 水位线溢出（计数器 → 选路 → 编排 API → 卡片水位线编辑）
- Phase 5：分组泳道显示顺序 + 卡片已使用量（与渠道列表共用近 3 天用量查询缓存）
