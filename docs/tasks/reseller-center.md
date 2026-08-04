# 任务档案：站长中心与代理返佣系统

> 状态：进行中｜分支：`feature/reseller-center`｜创建：2026-08-04｜更新：2026-08-04

## 0. 新会话启动指令（AI 必读）

1. 读 `@AGENTS.md`、`@web/AGENTS.md` 和 `@.claude/skills/protocol-dev/SKILL.md`。
2. 读本档案全文及 `@docs/reseller/behavior-contract.md`。
3. 复述当前阶段、上次停点和下一步；本任务已获用户整体验证与阶段提交授权。
4. 不得在原始 `new-api` worktree 或龙津仓库实现本功能。
5. 不得把目标站推断项描述成已确认的后端事实。

## 1. 目标与验收标准

- 目标：在 New API 中建立独立的站长中心业务域，让代理通过不透明邀请绑定直属客户、设置相对平台价的客户倍率，并按成功请求的真实差价获得内部收益。
- 功能验收：目标站已确认的邀请、定价、收益、额度安全、转账、用户码和 API 行为均有对应实现。
- 计费验收：Token、固定价、阶梯表达式、缓存、音频/WSS、图片、视频、Midjourney 和异步任务均使用同一权威报价逻辑生成 base/retail quote。
- 会计验收：佣金、释放、转换、转账和用户码均通过不可变账本对账；重试、并发和任务回调不会重复入账。
- 兼容验收：SQLite、MySQL 5.7.8+、PostgreSQL 9.6+ 语义兼容。
- 前端验收：`/reseller` 在桌面和移动视口可用，覆盖加载、空、错误、冲突、冻结和未知交易结果状态。
- 对标验收：`docs/reseller/behavior-contract.md` 中每项契约均记录本地自动化测试和目标站对比结果。

## 2. 分阶段计划

- [x] Phase 0 基线与证据契约：建立 worktree、任务档案和对标契约。完成（commit `bf41dc76a`）
- [x] Phase 1 核心模型与定价解析器：三库兼容模型、四级优先级、延迟涨价、乐观锁。完成（commit `ef7e31e68`）
- [x] Phase 2 邀请与归属：不透明邀请、密码/OAuth 注册原子直属绑定、来源分类。完成（commit `a6c2402dc`）
- [x] Phase 3 双报价与佣金：覆盖全部计费路径并按唯一请求引用幂等入账。完成（commit `50f0f513`）
- [x] Phase 4 收益释放：pending/available 账本及北京时间 04:10 可恢复批次。完成（commit `e147a8bb`）
- [x] Phase 5 额度操作：额度密码、冻结、preview/commit 转账、收益转换和用户码 escrow。完成（commit `38a6e78e`）
- [x] Phase 6 API：完成 `/api/reseller/*` 契约、鉴权、限流、审计与错误语义。完成（本阶段提交）
- [ ] Phase 7 前端：完成站长中心路由、视图、交互、响应式和 i18n。
- [ ] Phase 8 预览与验证：本地服务、三库测试、构建、Playwright 和跨视口检查。
- [ ] Phase 9 差异收敛：目标站逐项对比、记录限制并修正可复现差异。

## 3. 架构与上下文

- 开发 worktree：`../new-api--feature-reseller-center/`。
- 逆向报告：`../../../analysis/zzone-reseller/2026-08-04_web-reverse-zzone-reseller-report.md`。
- 原始业务分块：`../../../analysis/zzone-reseller/artifacts/static/js/async/3984.3771b86648.js`。
- 格式化业务分块：`../../../analysis/zzone-reseller/artifacts/static/js/async/3984.3771b86648.pretty.js`。
- 现有用户归属：`model/user.go` 中的 `InviterId` 和注册/OAuth 邀请链。
- 现有绝对定价：`dto/user_custom_pricing.go`、`relay/helper/price.go`、`service/quota.go`。
- 现有计费生命周期：`service/billing.go`、`service/billing_session.go`、`service/task_billing.go`。
- 现有多实例任务：`service/system_task.go`、`controller/system_task_handlers.go`。
- 技术决策：代理倍率独立存储为整数 `multiplier_bps`，不长期写入 `users.custom_pricing`。
- 技术决策：新绑定表是代理归属的权威状态；`users.inviter_id` 仅作兼容投影和历史回填来源。
- 技术决策：旧 `aff_quota`/`aff_history` 不自动并入新账本，避免双重记账。

## 4. 进度台账（每次会话末追加，倒序）

### 2026-08-04（会话 5）

- API：实现 status/profile、邀请、客户、定价、收益、账本、额度安全、转账、用户码签发/reveal/兑换的完整 `/api/reseller/*` envelope；列表统一分页。
- 安全：所有资源读取按当前 reseller owner 限定；敏感 mutation 使用已有 2FA/Passkey security proof；资金 mutation 强制 `Idempotency-Key`；写入挂载关键限流和禁用缓存。
- 错误：业务错误映射为稳定 HTTP 状态与 `data.code`，包括版本冲突、冻结、限额、余额不足、preview/idempotency/voucher 错误；security proof 保留旧顶层 `code` 并增加统一 `data.code`。
- 审计：记录资源公开引用、金额、倍率和 owner，不记录密码、proof、nonce、完整邀请 token、用户码或密文；读取 DTO 显式排除 hash/digest/ciphertext。
- 兼容：旧 `/api/user/aff_transfer` 返回 `410 AFFILIATE_TRANSFER_RETIRED`，旧余额不迁移；数据库资金事务成功后强制读取权威余额以刷新 Redis quota cache。
- 验证：owner 越权、陈旧版本、缺 proof/idempotency、敏感字段不泄漏、旧接口退役测试及完整 `go test ./...` 通过。
- 下一步：实现 `/reseller` 工作台、`/j/{token}` 注册入口、完整交互状态和 i18n，并移除旧邀请返利 UI。

### 2026-08-04（会话 4）

- 做了：独立六位 bcrypt 额度密码；修改与安全重置；重置后发送/签发冻结 24 小时；preview nonce 只存 HMAC digest，绑定双方、金额、过期时间并一次消费。
- 资金：收益转换、额度转账、用户码批量签发/再次 reveal/一次兑换全部走平衡 ledger；用户码明文只在响应内出现，数据库保存 HMAC digest 和 AEAD ciphertext；签发立即从钱包进入 escrow。
- 防重与限额：转账、转换、签发使用 `(user, operation, Idempotency-Key)` 唯一记录；相同 payload 返回原结果，不同 payload 冲突；转账与用户码在锁定安全行后共享滚动 24 小时 4000 限额。
- 边界：单次 1..2000，批量最多 50，备注最多 255 字符；不可取消或退款；密码重置冻结不妨碍接收与收益转换。
- 验证：密码生命周期、转账重放、转换重放、共享限额、escrow/reveal/redeem、完整 `go test ./...` 与 race detector 通过。
- 下一步：定义 reseller security-proof scopes，把全部模型能力接入 `/api/reseller/*`，增加鉴权、限流、审计、envelope 与错误码。

### 2026-08-04（会话 3）

- 做了：新增不可变 `ResellerLedgerTransaction` / `ResellerLedgerLine`；commission accrual 原子写平台成本与代理 pending；到期释放原子写 pending/available、commission 状态和余额投影。
- 调度：新增 `reseller_commission_release` scheduled system-task；仅存在到期项时每分钟调度，复用多实例数据库 lease 与心跳；每条 commission 独立事务，崩溃后按权威 pending 状态继续。
- 不变量：journal 至少两行且总和为零；引用唯一；pending 不得透支；投影或状态 CAS 不一致时整笔回滚。
- 验证：accrual 并发重放、journal 平衡、单次释放、重复批次、投影失败回滚、全仓 `go test ./...` 与 race detector 通过。
- 下一步：实现独立额度密码、安全复核与重置冻结，并在同一账本上完成转账、收益转换和用户码 escrow。

### 2026-08-04（会话 2）

- 做了：请求级保存平台/零售倍率与预扣/实际双报价；同步文本、固定价、缓存、工具附加费、阶梯表达式、Audio/WSS、Midjourney 和任务计费均接入；异步任务将代理快照持久化到 `TaskBillingContext`，只在成功终态按稳定 task reference 入账。
- 会计：新增 `ResellerCommissionEntry`，`request_reference` 数据库唯一；同引用同报价安全重放，同引用不同报价显式冲突；佣金为 `max(retail-base, 0)`，释放时间固定到下一个北京时间 04:10。
- 修正：WSS 增量用量通过 `BillingSession.Reserve` 累计预留，避免中途直接扣费后最终重复扣费。
- 验证：新增解析、并发幂等、引用冲突、04:10 边界和双报价舍入测试；完整 `go test ./...` 通过。
- 下一步：建立不可变复式账本、pending/available 投影和可恢复的多实例 04:10 释放批次。
- 遗留/坑：adaptor 自定义完成态报价接口当前没有生产实现；将来若出现返回非预扣值的实现，必须扩展接口同时返回 base/retail，现逻辑会拒绝猜算并跳过佣金。

### 2026-08-04（会话 1）

- 做了：完成核心模型与定价状态机；增加 `i1` HMAC 不透明邀请；密码与 OAuth 用户创建事务原子写入直属归属；OAuth flow 只保存 invitation ID/version，不保存 bearer token。
- 基线：`353b702c2`；Phase 0：`bf41dc76a`；Phase 1：`ef7e31e68`。
- 验证：代理模型、完整 `model`/`controller` 测试、前端生产构建和 `go build ./...` 通过。
- 下一步：把相对倍率接入所有计费路径，保留 base/retail quote，并在成功 settlement 后幂等创建佣金。
- 遗留/坑：目标后端源码不可见；邀请 tenant 的反向代理实现和部分 mutation 响应体仍属于推断或待动态验证项。

## 5. 决策与坑记录

- “1:1”以可观察业务行为、API 契约、状态机和账本结果为准，不伪造目标后端内部实现。
- 客户最终倍率为平台分组倍率乘代理倍率；平台倍率变化必须无需重写代理规则即可影响下一请求。
- 佣金必须由相同计费函数分别计算 base/retail 后求差，禁止对最终整数额度做简单乘法推导。
- 一切资金类写操作必须可重放、可对账，并在数据库层防止重复执行。
- 历史邀请关系只保留一级；不会沿 `inviter_id` 链递归计算收益。
