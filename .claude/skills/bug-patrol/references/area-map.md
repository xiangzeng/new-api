# 功能区域地图（New API）

> 本文件是 bug-patrol 的排查坐标系：把项目拆成若干功能区，每区记录核心文件、风险等级、检查点。排查时对照本表定位区域，排查结果写入 `00_debug-notes/bug-patrol/`。风险等级：🔴 高 / 🟡 中 / 🟢 低。

## R1 中继与渠道选择 🔴

- **核心文件**：
  - `controller/relay.go`（重试循环、请求级排除、Written 断流保护）
  - `service/channel_select.go` / `model/channel_cache.go` / `model/ability.go`（渠道选择 + 排除过滤）
  - `service/channel_affinity.go`（亲和缓存）、`middleware/distributor.go`（分发与分组）
- **检查点**：
  - [ ] 单渠道场景重试是否耗尽后正确 503（排除逻辑不误伤 LockedChannel）
  - [ ] 流已开始写出后不再重试 / 不再写 JSON 错误体
  - [ ] auto 分组跨组重试与排除的组合行为
  - [ ] type 58（AdvancedCustom）requestPath 过滤导致的"无可用渠道"

## R2 计费与配额 🔴

- **核心文件**：
  - `relay/helper/price.go`（HandleGroupRatio，千人千面用户级倍率优先）
  - `service/quota.go` / `service/billing_session.go` / `common/quota_math.go`（饱和防护）
  - `pkg/billingexpr/`（上游分层计费）
- **检查点**：
  - [ ] 千人千面倍率覆盖顺序：用户级 > 分组特殊倍率 > 分组倍率
  - [ ] quota 溢出饱和路径（Strict 变体在 pre-consume 的 fail-fast）
  - [ ] 订阅额度与普通余额的扣费分流

## R3 千人千面（用户级定价/可见性）🟡

- **核心文件**：
  - `controller/custom_pricing.go`、`dto/user_custom_pricing.go`
  - `model/user.go` / `model/user_cache.go`（custom_pricing 列 + UserBase 缓存 schema v3）
  - `service/group.go`（ExtraGroups/HideGroups 覆盖）
- **检查点**：
  - [ ] 缓存回源后 CustomPricing 不丢（ToBaseUser 单点映射；schema 版本围栏）
  - [ ] 可见性覆盖在 TokenAuth / playground / GetUserGroups / GetPricing / GetUserModels 五个入口一致
  - [ ] 上游新增绕行路径（GetRequestAutoGroups / controller/token.go）是否需要补覆盖

## R4 认证与用户会话 🟡

- **核心文件**：
  - `middleware/auth.go`、`model/user_session.go` / `user_auth_cache.go`（上游新会话体系）
- **检查点**：
  - [ ] auth_version 围栏与降权回流
  - [ ] API token 鉴权不受 Web 会话迁移影响

## R5 错误日志与审计 🟡

- **核心文件**：
  - `controller/relay.go` processChannelError、`logger/error_audit.go`、`model/log.go`
- **检查点**：
  - [ ] 审计条目 SkipReason / DbLogged 与实际落库一致
  - [ ] ClickHouse 与 SQL 双路径（RecordErrorLog / DeleteErrorLogs / DeleteOldLogBatch）

## R6 前端（TSX 新架构）🟡

- **核心文件**：
  - `web/src/features/`、`web/src/routes/_authenticated/`（TanStack 文件路由）
  - 定制 UI 重做落点：features/custom-pricing、features/invitations、features/users、features/pricing
- **检查点**：
  - [ ] 路由注册与 routeTree.gen.ts 再生成
  - [ ] 定价页数值由后端直出（千人千面覆盖值显示正确）
  - [ ] i18n key 登记（web/src/i18n/locales/*.json）

---

## 排查状态总览

- R1 中继与渠道选择：⬜ 未排查
- R2 计费与配额：⬜ 未排查
- R3 千人千面：⬜ 未排查
- R4 认证与用户会话：⬜ 未排查
- R5 错误日志与审计：⬜ 未排查
- R6 前端：⬜ 未排查
