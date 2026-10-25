# 渠道级联失败转移（Cascade Failover）开发指南

> 面向想复刻/维护这套机制的开发者。
> 设计规格书见 `docs/channel/cascade-failover.md`；本文讲 **为什么这么设计** 和 **怎么一步步做出来**。

---

## 0. 一句话说清这个功能

> 一个模型分组下挂 N 个上游渠道，排成一条流水线。
> 请求打到第 1 个，报故障就把**原样的请求**转给第 2 个，直到成功。
> 全程用户只看到一次成功响应，中间的失败、切换、熔断、恢复都对用户不可见。

它解决的不是"重试"，而是"**故障对用户透明**"。这两件事的差别决定了下面所有设计。

---

## 1. 先搞清楚：为什么现有的"重试"不够用

网关一般都有重试。但原生重试有三个致命伤，级联要一个个拆掉：

| 原生行为 | 问题 | 级联怎么改 |
|---|---|---|
| `重试次数` 是个全局数字，默认 0 | 最高优先级渠道一挂，请求直接失败，根本轮不到低优先级渠道 | 重试上限 = **组内候选渠道数**，不是拍脑袋的常数 |
| 重试 = "同一优先级档位里再随机抽一个" | 抽到的可能还是那个坏渠道；顺序不可预测 | 改成**按编排顺序严格遍历**，且排除本次已试过的渠道 |
| 渠道挂了就"自动禁用"，改数据库 `status` 字段 | 与人工启停状态互相污染，恢复要人工介入，重启后遗留脏状态 | 熔断只打**内存标记**，不碰数据库；进程重启即自愈 |

**记住第一条设计原则：运行时健康状态（易变、可自愈）和持久配置状态（人工意图）必须分开存。**
把"这个渠道现在不太行"写进 `channels.status`，是所有网关最常犯的架构错误。

---

## 2. 五条设计铁律

整套代码是从这五条推导出来的。理解它们，实现细节就都是必然结果。

### 铁律 1 —— 运行时态放内存，持久态放数据库

熔断标记、RPM 计数、错误率指标，全是**纯内存 map + 锁**，重启清零。
好处：零 DB 压力、零脏状态遗留、故障自愈。
代价：多实例部署时各实例独立熔断（见 §8）。

唯一落库的是**状态变迁事件**（trip / restore），因为那是要给人回溯的历史，不是运行时判断依据。

### 铁律 2 —— 只有"渠道的错"才切换，"用户的错"原样返回

上游返回 400（参数写错了）时切下一个渠道，是纯粹的浪费——下一个也会返 400，
而且用户拿到的错误还慢了 N 倍。

所以需要一个**故障分类器**，把错误分成两类：

- **渠道故障**：网络错误、429、401、403、5xx → 切换 + 计入熔断
- **其它**：400 等 → 不切换、不标记，原样吐给用户

这条线画在哪，决定了级联是"救命"还是"添乱"。

### 铁律 3 —— 首字节是不可回退的分界线

一旦已经往客户端写了流式数据，就**再也不能切换渠道**了——
换渠道意味着把第二条流的内容接在第一条后面，输出直接错乱。

所以：
- 首字节前失败 → 透明切换，用户无感
- 首字节后失败 → **不能切**，但仍然要把这个渠道标记成故障，让用户**重发时**自动绕开

这也是为什么错误消息里要附一句"直接重试即可自动切换其他线路"。

### 铁律 4 —— 顺序是独立配置，不是渠道优先级

第一版直接复用渠道的 `priority` 字段做顺序。踩坑了：
`priority` 是**渠道全局唯一**的字段，而一个渠道可以属于多个分组。
用户想让 #51 在 A 组排第 2、在 B 组排末尾——在"优先级即顺序"的模型下无解。

改成独立配置 `cascade_order.group_orders`（`map[分组名][]渠道ID`）后问题消失。

### 铁律 5 —— 默认全关，未配置 == 旧行为

- 总开关 `enabled` 默认 `false`
- 水位线未配置 = 0 = 不限流
- 分组未配置编排顺序 = 按优先级降序兜底（与旧版逐字节一致）

上线一个改路由的功能，必须能做到"不开开关时行为零变化"，否则没人敢部署。

---

## 3. 架构总览

```
                  ┌─────────────────────────────────────────────┐
  客户端请求 ───▶ │ middleware/distributor.go                    │
                  │  亲和命中？(会话粘性)                          │
                  │   └─ 级联开启时额外校验：熔断中 or 压满 → 让位  │
                  └───────────────┬─────────────────────────────┘
                                  │ 未命中/让位
                                  ▼
                  ┌─────────────────────────────────────────────┐
                  │ controller/relay.go  重试循环                 │
                  │  for retry := 0; retry <= maxRetryTimes      │
                  │      maxRetryTimes = 组内候选渠道数 - 1        │
                  └───────────────┬─────────────────────────────┘
                                  │ 每轮迭代
                    ┌─────────────▼──────────────┐
                    │ 级联选择器                   │  model/channel_cascade.go
                    │  按编排顺序遍历，三轮兜底：     │
                    │   ① 健康 + 未试过 + 未压满    │
                    │   ② 全压满 → 负载率最低者     │
                    │   ③ 全熔断 → 忽略标记按顺序取  │
                    └─────────────┬──────────────┘
                                  │ 发往上游 → 报错
                                  ▼
                    ┌────────────────────────────┐
                    │ 故障分类器                   │  service/channel_health.go
                    │  IsChannelFaultError()      │
                    └──┬──────────────────────┬──┘
                  故障 │                      │ 非故障(400)
                       ▼                      ▼
        ┌──────────────────────────┐   原样返回客户端，循环结束
        │ ① 健康注册表 +1 失败       │
        │   达阈值 → 熔断 + 冷却     │  model/channel_health.go
        │ ② 清除会话亲和            │
        │ ③ 记 attempt/fault 指标   │  model/channel_health_metrics.go
        │ ④ 落 trip 事件            │  model/channel_health_event.go
        └──────────┬───────────────┘
                   │ 回到循环顶部，选下一个渠道
                   ▼

  ┌──────────────────────────────────────────────────────────┐
  │ 后台探活循环（常驻 goroutine，每 probe_interval 秒）        │
  │  只探"熔断中且未被人工禁用"的渠道                            │
  │  连续 N 次成功 → 移出注册表，恢复健康                        │
  └──────────────────────────────────────────────────────────┘
                                            controller/channel_health_probe.go
```

**文件地图**（照着这个顺序读代码，是从底往上的）：

| 层 | 文件 | 职责 |
|---|---|---|
| 状态 | `model/channel_health.go` | 熔断状态机（内存注册表） |
| 状态 | `model/channel_rpm.go` | RPM 滚动窗口 + 水位线判定 |
| 状态 | `model/channel_health_metrics.go` | 24h 分钟桶指标（错误率/延迟/首字） |
| 状态 | `model/channel_health_event.go` | 状态变迁事件落库 |
| 判定 | `service/channel_health.go` | 故障分类 + 流结束归因 |
| 选路 | `model/channel_cascade.go` | 级联选择器（三轮兜底） |
| 选路 | `service/channel_select.go` | 选择器入口 + auto 分组编排 |
| 选路 | `service/channel_select_cascade.go` | 重试上限计算、已试渠道集合 |
| 接入 | `controller/relay.go` | 重试循环、错误处理、成功归因 |
| 接入 | `middleware/distributor.go` | 亲和与级联的耦合 |
| 恢复 | `controller/channel_health_probe.go` | 探活循环 |
| 配置 | `setting/operation_setting/cascade_*.go` | 三个配置块 |
| API | `controller/cascade.go` + `router/api-router.go` | 编排页后端 |
| 前端 | `web/default/src/features/cascade/` | 编排画布 |

---

## 4. 逐层实现

### 4.1 健康状态层 —— 一个纯内存的熔断状态机

`model/channel_health.go`。整层只有一个 map 和一把读写锁：

```go
type channelHealthEntry struct {
    consecutiveFailures int       // 连续失败次数
    tripped             bool      // 是否已熔断
    trippedAt           time.Time
    cooldownUntil       time.Time // 冷却截止
    recoverySuccesses   int       // 恢复期连续成功次数
    lastError           string    // 截断到 512 字节，给面板看
    lastErrorAt         time.Time
}

var (
    channelHealthLock     sync.RWMutex
    channelHealthRegistry = make(map[int]*channelHealthEntry)
)
```

对外只有五个方法，全部幂等、全部自己加锁：

| 方法 | 语义 | 返回值的用途 |
|---|---|---|
| `MarkChannelHealthFailure(id, msg)` | 记一次故障 | 返回 **本次是否触发了熔断**（false→true 的那一刻），调用方据此记 trip 事件 |
| `MarkChannelHealthSuccess(id)` | 记一次成功 | 返回 **本次是否完成了恢复**，调用方据此记 restore 事件 |
| `IsChannelHealthAvailable(id)` | 能不能选它 | 选择器每次决策都调 |
| `ListTrippedChannelIds()` | 谁在熔断 | 探活循环的输入 |
| `ResetChannelHealth(id)` | 强制清标记 | 管理员"立即恢复"、渠道被删 |

**三个必须注意的实现点：**

**① 状态是三态，但只存两个字段。** `healthy / cooling / probing` 中的
`cooling` 和 `probing` 都是 `tripped=true`，靠 `cooldownUntil` 是否已过区分。
只在生成快照时才展开成三态：

```go
if entry.tripped {
    info.State = ChannelHealthStateProbing
    if now.Before(entry.cooldownUntil) {
        info.State = ChannelHealthStateCooling
        info.CooldownRemaining = int64(entry.cooldownUntil.Sub(now).Seconds())
    }
}
```

**② 健康 = 不在 map 里。** 恢复不是把 `tripped` 置回 false，而是 **`delete(registry, id)`**。
这让 map 天然只保存"有问题的渠道"，健康渠道零内存开销，也不需要任何清理逻辑。

**③ 探活关掉时必须有半开兜底。** 否则一旦熔断且没人探活，渠道永远起不来：

```go
if !setting.ProbeEnabled && time.Now().After(entry.cooldownUntil) {
    return true   // 冷却期满，半开放行一次；再失败会立刻刷新冷却窗口
}
```

注意配套逻辑：已熔断状态下再次失败时，**刷新冷却窗口而不是重复触发熔断**：

```go
if entry.tripped {
    entry.cooldownUntil = now.Add(cooldown)
    return false   // 不再返回 true，避免重复记 trip 事件
}
```

---

### 4.2 故障分类器 —— 这条线划错，整个功能就废了

`service/channel_health.go` 的 `IsChannelFaultError`。判定顺序是有讲究的：

```go
func IsChannelFaultError(err *types.NewAPIError) bool {
    setting := operation_setting.GetCascadeSetting()
    lowerMessage := strings.ToLower(err.Error())

    // ① 排除关键词优先级最高——用户明确说"这种不算渠道的错"
    if hit, _ := AcSearch(lowerMessage, setting.IgnoreFaultKeywords, true); hit {
        return false
    }
    // ② 网络/连接类错误（errorCode 以 "channel:" 开头）
    if types.IsChannelError(err) {
        return true
    }
    // ③ 默认故障状态码
    switch {
    case code == 429, code == 401, code == 403: return true
    case code >= 500 && code <= 599:            return true
    }
    // ④ 白名单：额外状态码
    for _, extraCode := range setting.ExtraFaultStatusCodes {
        if code == extraCode { return true }
    }
    // ⑤ 白名单：关键词
    if hit, _ := AcSearch(lowerMessage, setting.ExtraFaultKeywords, true); hit {
        return true
    }
    return false
}
```

**为什么 401/403 也算渠道故障？** 因为在网关场景里，用户的鉴权已经在入口完成了。
到了上游还报 401/403，只可能是**渠道自己的密钥失效/被封**——这是最典型的换线路场景。

**为什么留了关键词白名单/黑名单？** 现实里大量上游把渠道故障报成 200 或 400：
`{"error": "upstream capacity exceeded"}` 配 400 状态码。这种只能靠关键词捞。
用 AC 自动机（`AcSearch`）而不是循环 `strings.Contains`，是因为关键词表可能很长而这是请求热路径。

**三层优先级要背下来：`ignore` > `内置规则` > `extra`。**
`ignore` 是逃生舱——线上发现某类错误被误判成故障导致疯狂切换时，加一个关键词就能立刻止血，不用发版。

---

### 4.3 级联选择器 —— 三轮兜底

`model/channel_cascade.go`。核心只有二十行，但每一轮都是被真实故障逼出来的。

**先排序**：候选渠道按编排顺序排好。未入列的按优先级降序、id 升序垫底：

```go
func sortCascadeCandidates(group string, candidates []*Channel) {
    pos := CascadeOrderPositions(group)   // 配置里的 id → 1基位置
    sort.SliceStable(candidates, func(i, j int) bool {
        pi, pj := pos[candidates[i].Id], pos[candidates[j].Id]
        if pi != 0 || pj != 0 {
            if pi == 0 || pj == 0 { return pi != 0 }  // 入列的排未入列前面
            return pi < pj
        }
        // 都未入列 → 旧规则兜底
        if candidates[i].GetPriority() != candidates[j].GetPriority() {
            return candidates[i].GetPriority() > candidates[j].GetPriority()
        }
        return candidates[i].Id < candidates[j].Id
    })
}
```

这个"未入列走旧规则"的兜底是**无缝迁移的关键**：功能上线当天，没人配过顺序，
所有分组的行为跟旧版一模一样；用户拖一次保存，那条泳道才切到新概念。

**再选取**：

```go
func selectCascadeChannel(candidates []*Channel, excludeIds map[int]bool) *Channel {
    // 第一轮 · 正常：健康、未试过、RPM 未达水位线
    if ch := pickCascadeChannel(candidates, excludeIds, true); ch != nil { return ch }
    // 第二轮 · 打满：健康渠道全压满，交给负载率最低的接住溢出
    if ch := pickLeastLoadedCascadeChannel(candidates, excludeIds); ch != nil { return ch }
    // 第三轮 · 熔断兜底：全熔断时忽略所有标记，仍按顺序选一个未试过的
    return pickCascadeChannel(candidates, excludeIds, false)
}
```

**第二轮为什么不能"退回第一个"？** 全员压满时如果退回顺序第一个，等于把所有溢出流量
全砸在**最满的那个渠道**上。必须按 `当前RPM / 水位线` 的负载率挑最空的。

**第三轮为什么要忽略熔断标记？** 因为**服务可用性优先于健康标记**。
全部渠道都被熔断时，返回"无可用渠道"是最差选择——熔断标记可能只是刚刚一次抖动，
硬着头皮发一次请求远好过直接拒绝用户。

**`excludeIds` 从哪来？** 从 gin context 的 `use_channel` 切片，每选中一个渠道就 append：

```go
func GetTriedChannelIds(c *gin.Context) map[int]bool {
    tried := make(map[int]bool)
    for _, idStr := range c.GetStringSlice("use_channel") {
        if id, err := strconv.Atoi(idStr); err == nil { tried[id] = true }
    }
    return tried
}
```

复用已有的 `use_channel`（原本只用来打"重试：#39->#49"日志）而不是新建一个 key，
省了一套状态同步。这类"复用请求级已有状态"的机会要主动找。

---

### 4.4 接入重试循环

`controller/relay.go`。级联不新开一条路径，而是**改造已有的重试循环的三个决策点**。

**决策点 1 —— 循环上限：**

```go
maxRetryTimes := common.RetryTimes                       // 旧：全局常数
if service.CascadeEnabled() {
    maxRetryTimes = service.GetCascadeMaxRetryTimes(...)  // 新：候选渠道数 - 1
}
for ; retryParam.GetRetry() <= maxRetryTimes; retryParam.IncreaseRetry() {
```

`GetCascadeMaxRetryTimes` 会去数候选渠道数（auto 分组则把用户所有可用分组的候选数加总），
再用 `max_attempts_per_request` 封顶。**上限来自拓扑，不来自配置常数**——
组里 5 个渠道就该允许试 5 次，这是级联的定义。

**决策点 2 —— 要不要切下一个：**

```go
func cascadeShouldRetry(c, info, openaiErr, retryTimes) bool {
    if info != nil && info.HasSendResponse() { return false }  // ★ 铁律 3
    if types.IsSkipRetryError(openaiErr)     { return false }
    if retryTimes <= 0                       { return false }
    if _, ok := c.Get("specific_channel_id"); ok { return false }  // 指定渠道调试不参与
    if code := openaiErr.StatusCode; code >= 200 && code < 300 { return false }
    return service.IsChannelFaultError(openaiErr)               // ★ 铁律 2
}
```

`HasSendResponse()` 这道守卫是**上线后补的洞**。原先没有它，
"流式吐了一半才失败 + 还有重试次数"会把第二条流的内容拼进同一个响应，输出错乱。
**任何实现流式重试的地方，都必须有这道守卫。**

**决策点 3 —— 错误的副作用：**

```go
func processChannelError(c, channelError, err) {
    if service.CascadeEnabled() {
        if service.IsChannelFaultError(err) {
            model.RecordChannelAttempt(id, model.ChannelAttemptFault, 0, 0, false)
            if model.MarkChannelHealthFailure(id, err.ErrorWithStatusCode()) {
                model.RecordChannelTrip(id)
                model.AppendChannelHealthEvent(id, model.ChannelHealthEventTrip, ...)
            }
            if !model.IsChannelHealthAvailable(id) {
                common.SetContextKey(c, constant.ContextKeyCascadeRetryHint, true)
            }
            service.ClearCurrentChannelAffinityCache(c)   // ★ 见 4.5
        } else {
            // 非故障错误：计 attempt 不计 fault
            model.RecordChannelAttempt(id, model.ChannelAttemptOther, 0, 0, false)
        }
    }
    ...
}
```

**`ContextKeyCascadeRetryHint` 的用途**：吐字后失败没法透明切换，
但故障渠道已经被隔离了。在 `Relay()` 的 defer 里给错误消息追加一句提示：

```go
if relayInfo != nil && relayInfo.HasSendResponse() &&
    common.GetContextKeyBool(c, constant.ContextKeyCascadeRetryHint) {
    newAPIError.SetMessage(newAPIError.Error() +
        "（该线路已临时隔离，直接重试即可自动切换其他线路 | Faulty upstream isolated, just retry to be routed elsewhere）")
}
```

这是个小细节，但它把"无法透明处理的场景"转化成了"用户一次重发就解决"，
体验上等价于自动恢复。**做容错功能时，把无法自动化的那部分明确告诉用户，比沉默失败强得多。**

---

### 4.5 渠道亲和的耦合 —— 最容易漏的一层

如果网关有"会话粘性"（同一会话粘住同一渠道，用于命中上游 prompt cache），
它会**绕开选择器直接返回粘住的渠道**。不处理的话级联形同虚设：粘住的渠道熔断了照样往上撞。

`middleware/distributor.go` 里的改法：

```go
if preferred, err := model.CacheGetChannel(preferredChannelID); ... &&
    // 级联开启时额外两道校验
    (!service.CascadeEnabled() ||
        (model.IsChannelHealthAvailable(preferred.Id) && !model.IsChannelOverWatermark(preferred.Id))) {
    ...  // 亲和可用
}
if !affinityUsable && !service.ShouldKeepChannelAffinityOnChannelDisabled() {
    service.ClearCurrentChannelAffinityCache(c)   // 让位，走级联选择
}
if affinityUsable && channel != nil {
    model.RecordChannelRequest(channel.Id)        // ★ 亲和命中也要记 RPM
}
```

三处改动缺一不可：

1. **熔断让位** —— 粘住的渠道熔断中就放弃亲和
2. **压满让位** —— 否则长会话客户端会把水位线整个架空
3. **亲和命中单独记 RPM** —— 亲和绕开了选择器，不在这里记账的话粘住的渠道 RPM 恒为 0

另外 `shouldRetry` 里，级联开启时**不再检查** `ShouldSkipRetryAfterChannelAffinityFailure`——
亲和的 skip-retry 语义不应该阻断故障类错误的切换。

**归属模式（home_mode）—— 从「会话粘性」到「用户归属」**

上面的耦合上线后暴露了两个结构性问题：

1. 只有带 `metadata.user_id` 的 Claude Code 流量有亲和键，其余客户端（实测约 37% 流量）
   逐请求重选，水位线一抖就换渠道；
2. 粘住的渠道一熔断/压满，亲和就被**改写**到接盘渠道，此后留在那里——接盘的往往是
   顺位靠后、最不稳的渠道，它再熔断又把人踹回来，形成 `34↔53` 式乒乓。
   每次来回都是一轮上游 prompt cache 重建（1.25x 计费）+ 缓存读全部落空。

`channel_affinity_setting.home_mode` 开启后改成三条规则：

- **键改用户级**：默认规则键源 `context_int:id`（用户 ID）优先、`metadata.user_id` 兜底，
  `/v1/messages` 与 `/v1/chat/completions` 各一条规则（后者不带 CLI 头透传模板）。
  同一用户所有会话 / 客户端归属同一渠道，跨会话共享的 system prompt / tools 前缀也能命中。
- **绕行不搬家**：归属渠道熔断中 → `MarkChannelAffinityDetour` 标记绕行，本次走级联
  选择器，`RecordChannelAffinity` 写回的仍是归属渠道（刷新 TTL）；恢复后自动回归。
  relay 里故障清亲和的调用点换成 `ClearCurrentChannelAffinityCacheOnFault`
  （归属模式下只重置 skip-retry，不删缓存条目）。
- **落位与留驻同一把尺**：归属流量不受软水位线约束——否则会在阈值附近逐请求来回弹，两边都在
  建缓存；用 `home_hard_watermark_ratio` 硬顶兜底（`IsChannelOverWatermarkRatio`，
  RPM ≥ 水位线 × 倍数才绕行，防止归属流量把上游打到限流）。**首次落位也按同一硬顶**选路
  （`IsChannelAffinityFirstPlacement` → 选择器 `watermarkRatio` 参数）：第一版曾让首次落位走
  软水位线，结果主渠道稍忙就把 TTL 过期回来的用户落到顺位靠后的备用渠道，且此后每次请求都
  刷新 TTL、永远不回——落位阈值比留驻阈值严，落位就等于判无期。软水位线只留给绕行等临时流量。

配套地，`distributor.go` 的亲和判定拆成两类：渠道被禁用 / 删除 / 不在分组 = **永久失效**
→ 清除亲和重新落位；熔断中 / 达硬顶 = **临时不可用** → 绕行（旧模式下则维持让位即清除）。
这个 permanent vs temporary 的区分是归属模式的核心——只有「一定回不来」的状态才配得上
清除归属。

> **通用教训**：加一层新的选路逻辑时，先把代码里**所有绕开选路的旁路**找出来
> （亲和、指定渠道、playground、内部调用），逐个决定它们是让位、豁免、还是补记账。
> 漏一个，新逻辑就有一个静默失效的窗口。

---

### 4.6 探活恢复循环

`controller/channel_health_probe.go`，一个常驻 goroutine，`main.go` 里 `go controller.StartCascadeHealthProbeLoop()`。

```go
for {
    setting := operation_setting.GetCascadeSetting()
    time.Sleep(time.Duration(setting.GetProbeIntervalSeconds()) * time.Second)
    if !setting.Enabled || !setting.ProbeEnabled { continue }
    probeTrippedChannels()
}
```

四个设计要点：

1. **只探熔断中的渠道**。健康渠道由真实流量持续验证，探活成本为零。
   这是本方案和"全量健康检查"的核心区别——后者渠道一多就成了自我 DDoS。
2. **手动禁用的渠道跳过**。人工意图优先，别把管理员关掉的渠道自己探活起来。
3. **复用已有的渠道测试基建**（`testChannel`），不写第二套请求逻辑。
4. **探活流量不计入指标**（`RecordChannelAttempt` 不调），避免稀释真实流量的错误率口径；
   但探活促成的**恢复是状态变迁**，照记 `restore` 事件。

先 sleep 再干活（不是先干活再 sleep），是为了避免进程启动瞬间跟渠道缓存初始化抢资源。

---

### 4.7 流式"安静断流"归因 —— 表面成功的盲区

这是整个功能里最隐蔽的一块，也是最能体现"想清楚了"的一块。

**现象**：上游连接在流式输出中途被掐断，但没有吐任何 error 事件。
`relayHandler` 返回 `nil`，网关认为**这次请求成功了**——渠道健康计数还 +1。
结果是一个持续断流的坏渠道，在监控里显示 100% 成功率。

**解法**：对"没返回错误对象"的流式请求，按**流的结束原因**做二次归因。
`service/channel_health.go`：

```go
func ClassifyStreamEnd(status *relaycommon.StreamStatus) (StreamOutcome, string) {
    if status == nil { return StreamOutcomeSuccess, "" }  // 非流式，维持成功语义
    switch status.EndReason {
    case StreamEndReasonTimeout:
        return StreamOutcomeFault, "流式响应超时（上游停止发送数据）"
    case StreamEndReasonScannerErr:
        return StreamOutcomeFault, "流读取错误: " + status.EndError.Error()
    case StreamEndReasonEOF:
        // ★ EOF 有歧义：可能是正常收尾，也可能是被掐断
        if setting.IncompleteStreamAsFault && status.TracksCompletion() && !status.HasCompletion() {
            return StreamOutcomeFault, "上游中途断流（未收到协议完成标记）"
        }
        return StreamOutcomeSuccess, ""
    case StreamEndReasonClientGone, StreamEndReasonPanic, StreamEndReasonPingFail:
        return StreamOutcomeNeutral, ""   // 无法归因于渠道
    }
    return StreamOutcomeSuccess, ""
}
```

**三态而不是两态**是关键。客户端主动断开、网关内部 panic —— 这些**既不是成功也不是渠道的错**。
强行二分类会污染错误率指标。

**EOF 的歧义怎么破？** 引入"协议完成标记跟踪"（`relay/common/stream_status.go`）：

```go
// 适配器在流结束后声明"我支持完成标记跟踪"，并在收到协议完成标记时打点
info.StreamStatus.EnableCompletionTracking()
if claudeInfo.Done { info.StreamStatus.MarkCompletion() }
```

判定式是 **`EOF && 支持跟踪 && 没收到完成标记` → 故障**。
`TracksCompletion()` 这个"能力声明"位不能省——没接入跟踪的适配器（OpenAI/Gemini）
必须维持 EOF=成功的旧语义，否则会大面积误伤。

> **这是给渐进式改造的通用范式**：新判据只对"明确声明支持"的实现生效，
> 其余保持旧行为。这样一个适配器一个适配器地接，每步都安全。

**接入点**在 `relay.go` 循环里 `newAPIError == nil` 的分支——注意成功路径也要做健康归因：

```go
if newAPIError == nil {
    if service.CascadeEnabled() {
        outcome, faultReason := service.ClassifyStreamEnd(relayInfo.StreamStatus)
        switch outcome {
        case service.StreamOutcomeFault:
            model.RecordChannelAttempt(id, model.ChannelAttemptFault, 0, 0, false)
            if model.MarkChannelHealthFailure(id, faultReason) { /* 记 trip */ }
            service.ClearCurrentChannelAffinityCache(c)
        case service.StreamOutcomeSuccess:
            latencyMs := time.Since(attemptStart).Milliseconds()
            ttftMs := relayInfo.FirstResponseTime.Sub(attemptStart).Milliseconds()
            model.RecordChannelAttempt(id, model.ChannelAttemptSuccess, latencyMs, ttftMs, hasTtft)
            if model.MarkChannelHealthSuccess(id) { /* 记 restore */ }
        case service.StreamOutcomeNeutral:
            model.RecordChannelAttempt(id, model.ChannelAttemptOther, 0, 0, false)
        }
    }
    return
}
```

注意 `attemptStart := time.Now()` 是在**循环体内**取的，不是循环外：
重试后不能把前序失败渠道消耗的时间算到最终成功的渠道头上。

---

### 4.8 RPM 水位线 —— 从"报错才切"到"压满就让"

熔断是**被动**的：非得先失败一次。但很多时候我们**事先就知道**某个渠道扛不住 200 RPM。
水位线补上主动维度。

**计数器**（`model/channel_rpm.go`）：每渠道 60 个 1 秒桶的环形数组。

```go
type channelRpmBucket struct {
    ts    int64   // 桶对应的 unix 秒——槽位复用时校验用
    count int64
}
type channelRpmRing struct {
    buckets      [60]channelRpmBucket
    lastActiveTs int64
}
```

**环形数组 + ts 校验**这个模式值得记住。写入时：

```go
bucket := &ring.buckets[ts%60]
if bucket.ts != ts { *bucket = channelRpmBucket{ts: ts} }  // 槽位属于 60 秒前的同余秒，清零复用
bucket.count++
```

读取时靠 ts 天然排除过期槽位，**不需要任何就地清理**，所以读路径是纯读、只持读锁：

```go
func (r *channelRpmRing) sum(now int64) int64 {
    cutoff := now - 60
    var total int64
    for i := range r.buckets {
        if b := &r.buckets[i]; b.ts > cutoff && b.ts <= now { total += b.count }
    }
    return total
}
```

再配一个"超过 300 秒无流量整环剔除"的惰性回收（在快照函数里做，避免热路径开销），
内存就完全可控了。同样的结构复用到了 24h 指标环（1440 个分钟桶）。

**记账点的选择是这个模块最大的坑：**

✅ 正确：`service.CacheGetRandomSatisfiedChannel` 返回渠道时 + 亲和命中时
❌ 错误：`SetupContextForSelectedChannel`（探活的 `testChannel` 也会调它，会把探活流量算进 RPM）

口径定义为"**选中即记账**"而不是"请求完成时记账"，是因为并发突发时，
第二条请求必须能**立刻**看到第一条的计数，否则水位线判定永远滞后一个 RTT。

口径 = 实际发往上游的 attempt 数：一次请求溢出了 3 个渠道，则这 3 个渠道各 +1。
**这跟仪表盘的 RPM（近 60s 落库消费日志数）不是同一个东西**，高错误率时两者会有可见差异——
这种口径差异必须写进文档，否则一定会有人来报 bug。

**软限流，不是限速。** 全员打满时照常发请求（摊给最空的），**不给用户返 429**。
水位线的目的是"引导流量分布"，不是"拒绝服务"。这个定位要在设计阶段就说死。

---

### 4.9 观测层 —— 两套互补的数据

| | 内存分钟桶指标 | 落库事件 |
|---|---|---|
| 文件 | `model/channel_health_metrics.go` | `model/channel_health_event.go` |
| 内容 | attempts / faults / 错误率 / 延迟 / 首字 | trip / restore / manual_reset |
| 窗口 | 24h × 1min 桶，1h+24h 双窗口聚合 | 落库 90 天，查近 7d |
| 重启 | 清零 | 保留 |
| 写入量 | 每请求（内存） | 只在状态变迁时（极小） |

**为什么要两套？** 高频数据（每个请求）落库成本受不了；
低频但需要跨重启追溯的状态变迁，内存又存不住。**按写入频率 × 保留需求分而治之**。

事件落库的三个细节：

**① 异步写，失败只记日志**——观测绝不能拖慢或阻断请求路径：

```go
func AppendChannelHealthEvent(channelId int, event string, reason string) {
    gopool.Go(func() { appendChannelHealthEventSync(...) })
}
```

**② 截断时不能切坏 UTF-8**：

```go
if len(reason) > 500 { reason = reason[:500] }
return strings.ToValidUTF8(reason, "")   // 半个多字节字符会被 MySQL utf8mb4 拒写，整条事件丢失
```

**③ 过期清理搭车在写入里**，不开第二个定时器：

```go
day := nowTs / 86400
if day == last || !lastCleanupDay.CompareAndSwap(last, day) { return }  // 跨天首次写入才触发
DB.Where("created_at < ?", cutoff).Delete(&ChannelHealthEvent{})
```

用 `atomic.CompareAndSwap` 保证并发下每天只跑一次。

指标聚合里有个小技巧——`mergeInto` 期间 `AvgLatencyMs` 字段先存 **sum**，
`finalizeChannelMetricsWindow` 时再除以 count，省掉一个中间结构体。

---

### 4.10 配置持久化

三个配置块，全部走项目已有的 `config.GlobalConfig` 框架（注册后按 `模块名.json标签` 存 options 表）：

| 键 | 类型 | 存什么 |
|---|---|---|
| `cascade_setting` | 标量为主 | 总开关、阈值、冷却、探活、关键词白/黑名单 |
| `cascade_order` | `map[string][]int` + `[]string` | 分组内渠道顺序 + 分组泳道展示顺序 |
| `cascade_watermark` | `map[int]int` | 渠道 ID → RPM 上限 |

**为什么顺序和水位线不落渠道表？**
它们是**编排概念**，不是渠道属性。放进 option：改动不触发渠道缓存重建，
不与渠道的增删改产生耦合，一次 `UpdateOption` 就是全量替换（不留删除残渣）。

**配置读取要走 getter 做合法化，不要裸读字段：**

```go
func (s *CascadeSetting) GetFailureThreshold() int {
    if s.FailureThreshold < 1 { return 1 }
    return s.FailureThreshold
}
func (s *CascadeSetting) GetProbeIntervalSeconds() int {
    if s.ProbeIntervalSeconds < 10 { return 60 }   // 防止有人填 1 秒把上游打爆
    return s.ProbeIntervalSeconds
}
```

配置是从数据库读的字符串解析来的，**永远假设它可能是 0 或负数**。

水位线的清除语义也要想清楚：`rpm <= 0` 表示**删除这个 key**，而不是存一个 0：

```go
if item.Rpm <= 0 {
    delete(merged, item.ChannelId)   // 不留 0 值残渣
    continue
}
```

---

### 4.11 编排 API 与前端

**六个接口**（全部 `AdminAuth`，`router/api-router.go`）：

| 方法 | 路径 | 作用 |
|---|---|---|
| GET | `/api/cascade/overview` | 分组 → 有序渠道列表（健康/指标/RPM/水位线）+ 当前配置 |
| GET | `/api/cascade/health_events` | 单渠道近 7d 事件时间线 + 24h/7d 熔断次数 |
| POST | `/api/cascade/order` | 保存组内顺序 和/或 分组展示顺序（两块可单独提交） |
| POST | `/api/cascade/watermark` | 保存渠道 RPM 水位线 |
| POST | `/api/cascade/reset_health` | 管理员"立即恢复"（清熔断标记） |
| POST | `/api/cascade/purge_group` | 清理孤儿分组 |

`overview` 里排序规则必须**和选择器完全一致**（`controller/cascade.go` 直接复用
`model.CascadeOrderPositions`），否则页面显示的顺序和实际路由顺序不符，比没有页面还糟。

**前端要点**（`web/default/src/features/cascade/`）：

- **数据 5s 轮询**（`refetchInterval: 5000`），健康状态要接近实时
- **保存后必须做乐观更新**，否则会"闪回"：
  保存成功 → `cancelQueries` 取消在途轮询 → `setQueryData` 按新顺序重排缓存 → `invalidateQueries`。
  少了 `cancelQueries` 这步，在途的旧响应回来会把新顺序盖掉。
- **乐观更新里的排序必须镜像后端规则**（入列按位置、未入列按优先级降序/id 升序），
  否则乐观状态和真实响应不一致，会闪一下。
- **只提交脏泳道**：本地维护 `groupName -> 未保存顺序`，比对服务端顺序算出 dirty 集合。
- **拖拽与卡片上的按钮会冲突**：卡片 `draggable` 时，内部交互控件要包一层
  `onDragStart={e => { e.preventDefault(); e.stopPropagation() }}`。

---

## 5. 三个场景走一遍

**场景 A · 透明切换（最典型）**

```
请求 → 选中 #39（编排第1，健康）
     → 上游 503
     → IsChannelFaultError = true
     → 健康注册表 #39 连续失败 1 次 ≥ 阈值 → 熔断，冷却 60s，落 trip 事件
     → 清除会话亲和
     → cascadeShouldRetry = true（未吐字、还有次数、是故障）
     → 循环下一轮，excludeIds={39}，选中 #49
     → 200 → MarkChannelHealthSuccess(#49) → 返回客户端
客户端所见：一次 200。日志页可见一红一绿两行——那是同一请求的过程记录。
```

**场景 B · 用户参数错误**

```
请求 → 选中 #39 → 上游 400 "invalid model"
     → IsChannelFaultError = false
     → 不标记健康、不清亲和，只记 attempt(Other)
     → cascadeShouldRetry = false
     → 400 原样返回客户端
```

**场景 C · 吐字后断流**

```
请求 → 选中 #39 → 流开始，已吐 300 token → 上游连接被掐
     → 情况1：报错 → HasSendResponse()=true → 不切换
       processChannelError 已熔断 #39，并置 CascadeRetryHint
       → 错误消息追加"直接重试即可自动切换其他线路"
     → 情况2：不报错，EOF 结束但无完成标记（安静断流）
       → ClassifyStreamEnd = Fault → 熔断 #39
客户端所见：流被截断。用户重发 → #39 已在熔断中被跳过 → 直接走 #49。
```

---

## 6. 推荐开发顺序

按这个顺序做，每一步都能独立编译、独立测试、独立上线：

| 步 | 做什么 | 产出 | 能验证什么 |
|---|---|---|---|
| 1 | 健康状态层 + 单测 | `channel_health.go` | 状态机：阈值触发、恢复清零、半开兜底 |
| 2 | 故障分类器 + 配置块 | `channel_health.go`(service) + `cascade_setting.go` | 分类边界（400 不算、429 算、关键词优先级） |
| 3 | 级联选择器 + 单测 | `channel_cascade.go` | 排序规则、严格顺序、排除已试 |
| 4 | 接入重试循环 + 亲和耦合 | `relay.go` + `distributor.go` | **端到端跑通**：拿个假渠道故意报 503 |
| 5 | 探活恢复循环 | `channel_health_probe.go` | 熔断后能自愈 |
| 6 | 编排 API | `cascade.go` + 路由 | overview 顺序与选择器一致 |
| 7 | 前端编排页 | `features/cascade/` | 可视化，运维能自助 |
| 8 | 增强：安静断流归因 / RPM 水位线 / 观测指标 | — | 各自独立开关 |

**1–4 是最小可用集**，做完就是一个能救命的级联。5–8 都是增量，可以按需停在任何一步。

> 一个真实的经验：第 8 步的每一项都是**上线后被现实逼出来的**——
> 安静断流是发现"成功率 100% 但用户在骂"，水位线是发现"没报错但某个渠道明显慢了"。
> 别一开始就想做全，先把 1–4 上线，让真实故障告诉你下一步做什么。

---

## 7. 测试策略

单元测试集中在**纯逻辑层**，这三块必须有：

```
model/channel_health_test.go       # 状态机：阈值/恢复/清零/半开/快照
model/channel_cascade_test.go      # 排序规则 + 三轮兜底的每一轮
service/channel_health_test.go     # 故障分类边界 + 流结束归因
```

`model/channel_cascade_test.go` 的用例设计值得抄，它把三轮兜底拆成了独立断言：

- `TestSelectCascadeChannelOverflowsOnWatermark` —— 压满即溢出
- `TestSelectCascadeChannelDisabledWatermarkKeepsLegacyOrder` —— 总开关关时行为不变
- `TestSelectCascadeChannelAllFullPicksLeastLoaded` —— 全满选负载率最低
- `TestSelectCascadeChannelAllTrippedIgnoresWatermark` —— 全熔断忽略水位线
- `TestSelectCascadeChannelPrefersHealthyFullOverTripped` —— **健康但压满 优于 熔断**

最后一条是最容易写错的优先级关系，必须有专门用例钉住。

**能这么测的前提**：选择器接受 `[]*Channel` 而不是自己去查数据库。
把"取候选"和"选一个"拆成两个函数（`cascadeCandidateChannels` / `selectCascadeChannel`），
后者就是纯函数，测试不需要任何 DB。**这是设计可测性的关键动作，要在写第一行代码时就决定。**

端到端验证靠**真实故障**：上线后从日志里捞 5xx，确认每一条后面都跟着一次成功的切换。

---

## 8. 边界与已知限制（必须写进文档给运维看）

1. **吐字后无法透明切换**。级联的自动切换只覆盖首字节前的失败。
   吐字后的异常会计入熔断标记，用户重发即可绕开。

2. **单实例内存态**。健康标记和 RPM 计数都在进程内存里。
   多实例部署时各实例独立熔断、独立计数（**实际水位线 = 配置值 × 实例数**）。
   要上多实例，得把注册表和计数器一起迁到 Redis。

3. **熔断粒度是整个渠道**。多 Key 渠道不细分到 Key，与"一个渠道 = 一个号池"的用法一致。

4. **指定渠道调试（`specific_channel_id`）和 SkipRetry 类内部错误不参与级联**。

5. **编排页的分组来自渠道 `group` 字段，不是分组倍率配置**。
   在分组倍率里删掉一个分组，只会让它从用户侧消失，渠道身上的组名和 abilities 仍在——
   这就是"孤儿分组"。需要 `purge_group` 显式摘除。
   摘除时**只挂着这一个分组的渠道必须跳过**（摘完 group 变空串，该渠道失去全部 ability，
   成为不可路由的孤岛），并且 abilities 按 `group × model` 展开存储，
   改完 `group` 字段必须**整体重建 abilities**，不能只改渠道行。

---

## 9. 踩过的雷（每一条都花过时间）

| 雷 | 现象 | 根因 / 正确做法 |
|---|---|---|
| 流式重试没有 `HasSendResponse()` 守卫 | 输出里出现两段拼接的回答 | 已吐字就不能换渠道，必须提前 return false |
| 用渠道 `priority` 当编排顺序 | 一渠道多组时，A 组调序把 B 组顺序踩坏 | `priority` 全局唯一，顺序必须独立配置 |
| RPM 记账点选在 `SetupContextForSelectedChannel` | 探活流量被算进 RPM | 探活也会调它。记账点要选**只有真实选路才经过**的地方 |
| 亲和命中不记 RPM | 粘住的渠道 RPM 恒为 0，水位线形同虚设 | 所有绕开选择器的旁路都要补记账 |
| 全员压满时退回顺序第一个 | 溢出流量全砸在最满的渠道上 | 按负载率挑最空的 |
| 保存顺序后页面闪回旧顺序 | 在途轮询响应盖掉了新状态 | `cancelQueries` → 乐观写缓存 → `invalidate` |
| 事件 reason 按字节截断 | 整条事件写库失败 | 半个多字节字符会被 utf8mb4 拒写，截断后 `strings.ToValidUTF8` |
| 已熔断状态下再次失败又返回 true | trip 事件重复记录、指标虚高 | 已熔断时只刷新冷却窗口，返回 false |
| "修过的 bug 又出现了" | 用户看到已删除的 UI 文案 | **先怀疑是旧 bundle**：长期开着的标签页在跑部署前的 JS |

---

## 10. 移植到别的网关：最小集清单

如果你要在另一个项目里做同样的事，这是不能少的部分：

```
□ 内存健康注册表      —— map[渠道ID]状态 + 读写锁，恢复 = 从 map 删除
□ 故障分类器          —— 至少区分「渠道故障」和「客户端错误」两类，且可配关键词逃生舱
□ 有序候选 + 排除已试  —— 请求级的 triedIds 集合，从 context 取
□ 三轮兜底选择        —— 正常 / 降级 / 忽略标记，最后一轮保证服务可用性优先
□ 重试上限 = 候选数    —— 不是全局常数
□ 首字节守卫          —— 已响应就不切换，且给用户明确提示
□ 旁路盘点            —— 找出所有绕开选路的路径（亲和/指定渠道/内部调用），逐个处理
□ 探活只探坏的        —— 健康渠道由真实流量验证，零探活成本
□ 总开关 + 未配置=旧行为 —— 上线安全阀
```

可选但强烈建议：流结束归因（否则你的成功率指标是假的）、内存分钟桶指标、状态变迁事件落库。

---

## 附：配置项速查

| 字段 | 默认 | 说明 |
|---|---|---|
| `enabled` | false | 总开关，关闭时一切走原有逻辑 |
| `failure_threshold` | 1 | 连续故障 N 次触发熔断 |
| `cooldown_seconds` | 120 | 冷却时长（探活关闭时的半开兜底窗口） |
| `probe_enabled` | true | 是否对熔断渠道主动探活 |
| `probe_interval_seconds` | 60 | 探活间隔（下限 10，非法值回落 60） |
| `recovery_success_count` | 3 | 连续成功多少次恢复健康 |
| `max_attempts_per_request` | 0 | 单请求最大尝试渠道数，0 = 组内全部 |
| `watermark_enabled` | false | RPM 水位线总开关（关闭时照常统计，不参与选路） |
| `incomplete_stream_as_fault` | true | EOF 但无完成标记是否计入故障 |
| `extra_fault_status_codes` | `[]` | 额外视为渠道故障的状态码 |
| `extra_fault_keywords` | `[]` | 命中即视为渠道故障的关键词 |
| `ignore_fault_keywords` | `[]` | 命中即**不**视为渠道故障（优先级最高，线上止血用） |
