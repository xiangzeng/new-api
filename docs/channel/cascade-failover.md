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

### 2.3 级联选择器（`model/channel_cache.go` + `service/channel_select.go`）

- 新增 `GetCascadeSatisfiedChannel(group, model, path, excludeIds)`（已试渠道集 =
  `use_channel`（选中即写）∪ `RetryParam.ExcludeChannelIDs`（失败即写））：
  候选渠道按分组「编排顺序」（`cascade_order.group_orders` 配置，与渠道优先级解耦）
  严格依次遍历，取第一个可用（健康且未试过）的渠道；未配置顺序的分组、
  未入列的渠道按优先级降序、id 升序兜底（同一渠道可在不同分组排不同位置）。
- 全部熔断时兜底：忽略健康标记，仍按顺序选一个未试过的渠道（服务可用性优先于标记）。
- 重试上限：级联开启时不再使用「最大重试次数」，上限 = 组内候选渠道数
  （可用 `max_attempts_per_request` 封顶）。
- auto 分组：组间顺序即级联外层，组内用级联选择器，当前组耗尽后切下一组。

### 2.4 渠道亲和耦合

- 粘住的渠道报故障类错误时：清除该会话亲和缓存（`ClearCurrentChannelAffinityCache`，
  同时重置 skip-retry 标记），级联接管选下一个；成功后由现有逻辑自动重绑到新渠道。
- 级联开启时，亲和的 `SkipRetryOnFailure` 不再阻断故障类错误的切换。

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
| `extra_fault_status_codes` | [] | 白名单：额外视为渠道故障的状态码 |
| `extra_fault_keywords` | [] | 白名单：命中即视为渠道故障的关键词 |
| `ignore_fault_keywords` | [] | 排除：命中即不视为渠道故障的关键词 |

### 2.7 编排 API（`controller/cascade.go` + `router/api-router.go`）

- `GET /api/cascade/overview`（AdminAuth）：分组 → 有序渠道列表
  （id/名称/类型/优先级/权重/启用状态/健康快照）+ 当前配置。
- `POST /api/cascade/order`（AdminAuth）：`[{group, channel_ids}]` 按分组保存编排顺序
  （写 `cascade_order.group_orders` 配置，不改渠道优先级，跨组互不影响）。
- `POST /api/cascade/purge_group`（AdminAuth）：清理孤儿分组，把组名从所有渠道的
  `group` 字段摘除并重建 abilities，同时删掉该组的 `cascade_order` 顺序。
  安全阀：只接受**不在**分组倍率配置里的分组；只挂着这一个分组的渠道会被跳过
  （摘完将失去全部 ability 成为不可路由的孤岛），在响应 `skipped` 里返回。
- 配置读写复用现有 option API（键 `cascade_setting`、`cascade_order`）。

### 2.8 前端页面（`web/default/src/features/cascade/`）

- 侧边栏「管理员」组新增「渠道编排」（url `/cascade`）。
- 级联画布：按分组分区，组内渠道卡片从左到右按编排顺序排列，卡片间箭头示意流向；
  卡片显示序号、名称、健康徽标（正常/熔断中+冷却剩余/恢复中 探活 x/N）、
  连续失败次数、最近错误摘要；HTML5 拖拽换序 + 保存落库。
- 配置集中区：cascade_setting 全部字段的表单。
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

## 3. 边界与已知限制

1. **流式响应已开始向客户端吐字后 failure 无法透明切换**——级联的自动切换只覆盖首字节前
   的失败；吐字后的异常断流按 2.9 计入熔断标记，用户在同一会话重发即可绕开故障渠道。
   本 fork 在流已写出后不再回写 JSON 错误体（避免污染已发送响应），因此「该线路已临时隔离，
   直接重试即可换线路」的提示只随 relay 错误日志留痕，不会追加到客户端响应里。
2. 健康标记为单实例内存态；多实例部署时各实例独立熔断（当前生产为单实例，可接受；
   如未来多实例，可将注册表迁移到 Redis）。
3. 多 Key 渠道（multi-key）以整个渠道为熔断粒度，与「一个渠道=一个号池」的使用方式一致。
4. 指定渠道调试（specific_channel_id）与 SkipRetry 类内部错误不参与级联。
5. **编排页的分组来自渠道 `group` 字段，不是分组倍率配置**——在分组倍率里删掉一个分组
   只会让它从用户侧（`/api/group`、密钥创建下拉）消失，渠道身上的组名与 abilities 仍在，
   编排页因此仍会列出它。这类分组标记为 orphan，需用「清理分组」显式摘除。

## 4. 阶段划分

- Phase 1a：健康状态层（model/channel_health.go）
- Phase 1b：错误分类 + cascade_setting 配置块
- Phase 1c：级联选择器接入重试循环 + 亲和耦合
- Phase 2：探活恢复循环
- Phase 3a：编排 API
- Phase 3b：前端「渠道编排」页面
- Phase 3c：孤儿分组「已失效」标记 + 一键清理
