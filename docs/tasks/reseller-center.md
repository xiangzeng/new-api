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
- [x] Phase 3 双报价与佣金：覆盖全部计费路径并按唯一请求引用幂等入账。完成（本阶段提交）
- [ ] Phase 4 收益释放：pending/available 账本及北京时间 04:10 可恢复批次。
- [ ] Phase 5 额度操作：额度密码、冻结、preview/commit 转账、收益转换和用户码 escrow。
- [ ] Phase 6 API：完成 `/api/reseller/*` 契约、鉴权、限流、审计与错误语义。
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
