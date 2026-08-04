# 任务档案：站长中心与代理返佣系统

> 状态：已完成｜分支：`feature/reseller-center`｜创建：2026-08-04｜更新：2026-08-04

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
- [x] Phase 6 API：完成 `/api/reseller/*` 契约、鉴权、限流、审计与错误语义。完成（commit `515a8a325`）
- [x] Phase 7 前端：完成站长中心路由、视图、交互、响应式和 i18n。完成（commit `c44eb6774`）
- [x] Phase 8 预览与验证：本地服务、三库测试、构建、浏览器逐视图和跨视口检查。完成（本阶段提交）
- [x] Phase 9 差异收敛：目标站逐项对比、记录限制并修正可复现差异。完成（commit `b2c90910`）
- [x] Phase 10 直属客户消费与佣金可见性：对齐目标站客户表的定价、站长口径使用量、请求数和收益，并保证代理数据隔离。完成（本阶段提交）
- [x] Phase 11 对标界面收敛：逐项移除目标站未观察到的站长中心展示，仅保留运行态和已服务资源确认的交互。完成（本阶段提交）

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

### 2026-08-04（会话 10）

- 需求：以目标站作为唯一产品范围，目标没有的功能或概览展示不得保留。
- 已确认差异：本地概览错误展示“24 小时已发送 0 / 4000”；目标站概览四项统计为钱包余额、可用收益、待释放收益和直属客户。滚动 24 小时额度上限保留为目标已确认的转账/用户码校验，不再作为概览统计。
- 收敛：概览改为目标已确认的钱包余额、可用收益、待释放收益和直属客户四项统计；移除“24 小时已发送”、概览快捷操作、概览编辑定价按钮和“仅一级”徽章。
- 补齐：增加目标已有的额度安全与收款标题、收款链接二维码、默认客户价格和结算规则展示；额度滚动上限仍保留在目标同样存在的转账/用户码校验中。
- 验证：定向、race、全仓 Go 测试和构建通过；TypeScript、前端单测、lint、format 与生产构建通过；`33101` 登录后的状态 API 不再返回 `outbound_used_24h`，生产资源检索不到被移除文案并包含目标概览与收款二维码代码。

### 2026-08-04（会话 9）

- 需求：站长需要在直属客户表中查看邀请客户、当前定价、站长口径使用量、成功请求数和为本站长产生的累计收益。
- 对标：目标站已服务前端确认 `GET /api/reseller/customers?p={page}&page_size={pageSize}` 返回 `current_multiplier_bps`、pending 字段、`customer_retail_quota_text`、`reseller_request_count` 和 `reseller_commission_quota_text`；表格为客户、当前价格、使用量、你的收益、操作五列，分页大小为 20。
- 语义：本地使用量与请求数从 `ResellerCommissionEntry` 聚合，只包括已成功并进入该站长佣金账本的请求；倍率为 1.0000x、没有价差的请求当前不会生成该账本记录，不能误称为客户全局历史用量。
- 实现：列表查询分页内批量读取客户、默认整体规则、客户整体规则和佣金账本聚合；限定 `(reseller_id, customer_id)` 与直属绑定集合，避免跨站长或未绑定客户数据泄漏。禁用客户保留可见但禁止编辑定价。
- 验证：定向和全仓 Go 测试、race、`go build ./...`、TypeScript、前端单测、lint、format、生产构建均通过；独立 `33101` 预览连接本地 SQLite，登录后的客户接口返回 3 名直属客户、`page_size=20` 和全部新增字段。本地样本尚无佣金账本记录，累计字段为 0。

### 2026-08-04（会话 8）

- 对标：目标站已登录桌面工作台再次确认顶部“额度安全与收款”、32 位收款码及链接、五标签、未设额度密码的禁用状态、全额收益转换和设置密码弹窗。弹窗要求“新额度密码 + 登录密码确认”；未提交任何目标站写操作。
- 收敛：首次设置/重置支持登录密码或既有安全 proof；日常额度授权收敛为独立额度密码。转账 preview 支持用户名、32 位收款码和带 `receive` 参数的收款链接，commit 校验标准化接收人、金额、nonce 和额度密码。
- 收敛：定价完整文档在一个 owner-wide CAS 与事务中保存；收益只允许全额转换；用户码加入单张/批量与状态筛选；流水变为逐账户显示 delta 与余额快照。
- 兼容：SQLite 本地预览、MySQL `5.7.44`、PostgreSQL `9.6.24` 均真实启动迁移；`reseller_ledger_lines.balance_after` 存在，三库均为 14 张 `reseller_*` 表。
- 验证：Phase 9 Go/Bun 定向测试、race、构建、格式和 lint 门禁将在提交前重跑；本地持续预览保留在 `http://127.0.0.1:33100/reseller`。
- 下一步：运行最终门禁，展示确认过的提交信息并提交 Phase 9；不得停止本地预览服务。

### 2026-08-04（会话 7）

- 三库：SQLite 本地预览、MySQL `5.7.44` 和 PostgreSQL `9.6.24` 均完成 `InitDB/AutoMigrate` 与 `/api/status` 启动检查；三库创建 14 张 `reseller_*` 表，佣金引用、客户归属、幂等 scope 和账本 reference 的唯一索引均存在。
- 前置条件：MySQL 官方镜像默认 `latin1` 会被项目字符集门禁拒绝；将测试库改为 `utf8mb4_unicode_ci` 后迁移成功，故部署契约明确要求 `utf8mb4`。
- 运行时修复：桌面逐标签检查发现无账本时后端返回 `items: null`，前端点击“账本”触发 `.length` 500；后端所有 reseller 空列表改为 `[]`，前端分页解析兼容旧节点的 `null`，并新增 Go/TypeScript 回归测试。
- 退役收敛：管理员侧栏删除旧“邀请返利”入口，系统设置删除已经失效的邀请人与被邀请人固定奖励输入；历史 option、字段和旧路由继续保留兼容审计。
- 视图：桌面 `1440x900` 与移动 `390x844` 完成概览、客户、账本、转账、用户码、安全及全部弹窗检查；全页无横向溢出，移动 Tabs 与表格使用各自滚动容器，弹窗保持视口内可操作。
- 证据：无敏感值截图位于 `analysis/zzone-reseller/local-preview/reseller-customers-desktop.png` 和 `reseller-customers-mobile.png`；临时 reveal 占位记录已在验证后删除。
- 下一步：使用同一浏览器会话对目标站与本地逐项比较可见行为、只读 API、错误 envelope 和移动布局，修正有运行时证据支持的差异。

### 2026-08-04（会话 6）

- 工作台：新增 `/reseller` 站长中心，覆盖开通、汇总、邀请、默认/客户分组定价与继承、收益转换、preview/commit 转账、用户码签发/reveal、六位额度密码、收款地址轮换及冻结/冲突/未知结果状态。
- 邀请：新增 `/j/{opaque_token}` 单标签页邀请入口；密码注册和 OAuth state 只传 `reseller_invitation`，成功后清理；不透明 token 不写 localStorage。
- 退役：密码注册和 OAuth 明确拒绝旧 `aff`，`GET /api/user/aff` 与 `POST /api/user/aff_transfer` 返回 `410`；固定邀请奖励不再新增；旧钱包组件、hooks 和 API 包装已下线，历史字段不迁移、不清零。
- 定价：新增默认/客户规则 `DELETE` API，分组关闭覆盖后真正恢复继承；删除仍使用 owner scope 和 owner-wide `expected_version` 乐观锁。
- 可用性：客户、账本、转账、用户码和批次支持分页；新增简中/繁中/英文站长词典；安全复核显示服务端业务错误；钱包现有兑换框按 `RV-` 前缀选择用户码兑换。
- 验证：定向 Go 回归、TypeScript、邀请存储/兑换路由单测和本阶段文件 lint 通过；全仓 lint 存在本分支修改前已有的无关错误，使用变更文件 lint 作为本阶段门禁。
- 下一步：执行全量测试/构建后提交 Phase 7，随后启动本地三库与桌面/移动预览验证。

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
