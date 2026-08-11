# 任务档案：用户自助余额查询

> 状态：阶段 1-3 完成并已上线生产，阶段 4 仅剩重启连续性实测（已决定暂不做）｜分支：`feature/self-balance-key`（已合入 main）｜创建：2026-08-11｜更新：2026-08-11

## 0. 新会话启动指令（AI 必读）
1. 读 `@AGENTS.md` 开发协议
2. 读本档案全文，**尤其是第 1 节的「需求澄清」与第 3 节的「阶段 0 结论」**
3. 复述「当前阶段 + 上次停点 + 下一步」，等授权，禁止直接动手
4. **禁止**把本任务当成「第三方合作方接入」来设计——那是已被推翻的前提，见第 5 节

## 1. 目标与验收标准

### 需求澄清（2026-08-11 用户原话要点）
> 「没有什么所谓的合作方不合作方，这功能本来就是有些用户需要自己去查的。有些用户自己做了 APP 或做了什么东西，让他能够不登录我们的站点直接查余额，他希望有这功能。没有什么第三方，就是用户自己要查。」

- **主体是用户自己**，不是第三方站点。用户在自己的 APP / 脚本 / 面板里查**自己**的余额。
- **不需要站长审批**。站长的角色是把能力提供出来，不是逐个批准。
- **不需要交密码**。用户对自己的账号本来就有完整权限，「用密码换只读凭证」这个动作对自查场景没有意义。

### 验收标准
1. 用户无需联系站长、无需登录站点，就能在自己的程序里拿到**账户维度**余额（不是单把 key 的额度）
2. 用户可以自行签发与吊销该能力的凭据，并能看到它最后一次被使用的时间
3. 用户不需要把「能花钱的凭据」（sk- key）或「能改账号的凭据」（系统 PAT / 密码）交给查询程序

## 2. 分阶段计划
- [x] **阶段0 现状评估**：确认现成接口为什么不满足需求 — 完成（会话 2），结论见第 3 节
- [x] 阶段1 方案定档：选定方向 D，砍掉 `open_apps`，解除 auth_version 绑定，每人 5 把上限 — 完成（会话 3）
- [x] 阶段2 后端实现 — 完成（commit `93167785`）
- [x] 阶段3 前端与文档 — 完成（同 commit）
- [x] 阶段4 验证与上线 — 完成（merge `9bd2d458`，生产实测通过）；**唯一未做**：重启前后凭证连续性实测，用户 2026-08-11 决定不为此重启生产

## 3. 架构与上下文

### 阶段 0 结论（2026-08-11 会话 2 · 全部读码验证，非推测）

**核心结论：站点缺的不是「账户余额接口」（`/api/user/self` 就是），而是「一张只能读余额、用户自助签发与吊销、能看出被用过没有的凭据」。**

现成接口能查到什么，逐个验证如下。

**`GET /api/usage/token/`（sk- key，`controller/token.go:215-262`）— token 维度，拿不到账户余额**
- `total_available` = `token.RemainQuota`，是**这把 key 自己的额度**，与账户余额无关
- key 设为无限额度时 `RemainQuota` 停留在建 key 时填的值（通常 0），于是 `unlimited_quota: true` 但 `total_available: 0`，数字彻底失真
- 账户 `user.quota` 在该接口里根本不返回
- **附带硬伤**：响应体是 `{"code": true, ...}`（**布尔**），而 apiBalance 的 `getFloat(resp,"code")` 只接受数字（`apiBalance/internal/checker/openai.go:126`），拿到 bool 即返回 `!ok` 并报 "unexpected response code"。**该 fallback 对本站点 100% 失败**，这正是 apiBalance 实际改走账号密码登录的原因

**`GET /dashboard/billing/subscription` 与 `/usage`（sk- key，`controller/billing.go`）— 被全局开关绑架**
- 走 token 维度还是账户维度取决于 `DisplayTokenStatEnabled`（`common/constants.go:25`，**默认 true → token 维度**）。这是全局站点开关，无法只为余额查询单独打开
- 无限额度 token 直接返回硬编码 `100000000`
- 只给按站点展示口径换算后的金额，站长改口径数字即变，且不返回原始 quota
- 前端 `display_token_stat_enabled` 字段后端从未下发（上游遗留），无法匿名探测生产值；不影响结论

**`GET /api/user/self`（`UserAuth()`）— 数据正确，凭据不对**
- 这是唯一给出账户维度且口径稳定的数据源（原始 `quota` / `used_quota` / `request_count`）
- **关键发现**：`UserAuth()` 不只吃 session cookie。`middleware/auth.go:148-177` 的 `classifyDashboardCredential` 在 dashboard JWT 不匹配时会 fallback 到 `model.ValidateAccessToken`，即用户在「个人设置 → Access Token」里自助生成的系统访问令牌（PAT）。因此**今天用户已经能无密码、长期有效地查账户余额**：`curl -H "Authorization: Bearer <access_token>" /api/user/self`
- 但这条路**不能推荐**：`user.access_token` 是单字段（`model/user.go:94`，注释原文 "this token is for system management"），每人只有一个、重新生成即顶掉旧的、无名称、无 scope、无过期、**无最后使用时间**；且它能通过全部 `UserAuth` 路由，包括 `POST /api/token/`（新建能花钱的 sk- key）、`POST /api/user/topup`、`PUT /api/user/self`、`DELETE /api/user/self`。**它比 sk- key 危险得多**
- 用户当前的 apiBalance 存的是站点**用户名 + 密码**（`apiBalance/internal/checker/newapi.go` `probeViaLogin`），比 PAT 更糟

**sk- key 能触达的只读接口全集**（`grep TokenAuth\|TokenAuthReadOnly router/`）：仅 `/api/usage/token/`、`/dashboard/billing/*`、`/api/log/token`，**没有任何一条返回账户余额**。

**因此今天用户想在自己程序里拿账户余额，只有三条路**：交密码、交全权 PAT、或读已失真且 fallback 已坏的 token 维度数字。三条全部违反验收标准 3。档案原先怀疑的「缺文档」不是主因——即便写了文档，`/api/usage/token/` 也给不出账户余额。

### 已上线「余额开放接口」的重新定位

它已经把缺口做完了约 90%。`open_credentials`（`model/open_credential.go`）正是缺的那张凭据：只存 HMAC 摘要、scope 限定 `balance:read`、可吊销、有 `LastUsedTime`、绑 `AuthVersion`（改密/禁用自动失效）、有独立限流与审计、用户侧已有撤销 UI。

它唯一错的是**签发入口**：必须站长先建 app、用户再把密码交给 app 去换。把签发入口换成「用户在个人设置里自己点一下生成」，表结构、鉴权、限流、审计、撤销 UI 全部原样可用。

候选方向裁决（详见第 5 节）：A 已被证伪；B 污染中继 key 语义且仍是 token 维度；C 等于把 D 已有的重写一遍；**采纳 D**。

### 最终实现（已上线，commit `93167785`）

**用户侧**：个人设置 →「余额查询密钥」卡片自助创建 `obk_` 只读密钥，明文只显示一次，列表给出名称 / 尾部提示 / 创建时间 / 最近使用时间 / 撤销按钮，每人最多 5 把（撤销即释放名额）。

**接口**
- `GET|POST|DELETE /api/user/balance-keys`（登录态，签发挂 `CriticalRateLimit` + `UserCriticalRateLimit("balance-key")`）
- `GET /api/open/v1/balance`（密钥鉴权，账户维度：原始 `quota`/`used_quota` + 换算后 `balance`/`used` + `request_count`）
- `POST /api/open/v1/auth/revoke`（程序自己注销手里的密钥）

**删除**：`open_apps` 表与模型、站长应用管理页与 `/api/open-app/*`、`POST /api/open/v1/auth/exchange`（密码换凭证）、来源 IP 白名单、换凭证限流与失败锁定、2FA 拦截、开放接口总开关、`AuthenticateOpenApiUser`、`middleware/audit.go` 四条 `open_app.*` 动作名、56 个随功能失效的前端 i18n 键。

**数据层**：`open_credentials` 新增 `name` 列；`app_id` / `auth_version` / `end_user_ip` 三列从模型中摘除但**列仍留在库里**（不写破坏性 DDL）；`open_apps` 表从 AutoMigrate 登记中摘除，空表保留在库中，要清由人手动执行。

**行为变更**：余额密钥不再绑定 `auth_version` —— 改登录密码不再使其失效（与 API 密钥一致），账号禁用或删除仍立即失效。

### 关键文件
- `model/open_credential.go` — 密钥签发/校验/撤销/列表，`OpenCredentialMaxPerUser = 5`
- `controller/balance_key.go` — 自助签发三个 handler
- `middleware/open_auth.go` — `OpenCredentialAuth`，错误码收敛为 5 个
- `web/src/features/profile/components/balance-keys-card.tsx` — 用户侧卡片
- `docs/open-balance-api.md` — 对外文档（已按自助口径重写）
- `controller/token.go:215` — `GetTokenUsage`，token 维度，`code: true` 布尔响应
- `controller/billing.go` — OpenAI 兼容计费接口，受 `DisplayTokenStatEnabled` 控制
- `middleware/auth.go:148-177` — `classifyDashboardCredential`，PAT fallback 所在
- `model/user.go:94` — `access_token` 全权 PAT 字段
- `controller/open_balance.go`、`controller/open_app.go`、`controller/open_credential_self.go`
- `model/open_app.go`、`model/open_credential.go`
- `setting/system_setting/open_balance_api.go`
- `web/src/features/open-apps/`、`web/src/features/profile/components/open-credentials-card.tsx`
- `web/src/features/profile/components/dialogs/access-token-dialog.tsx` — 现有 PAT 签发 UI
- `docs/open-balance-api.md`
- `/Users/longshun/Desktop/Program/00_use/apiBalance/internal/checker/newapi.go` — 用户自己的消费方

## 4. 进度台账（每次会话末追加，倒序）

### 2026-08-11（会话3 · 阶段 1-4 完成并上线）
- 阶段 1 定档：采纳方向 D。用户拍板三点——接受新的只读凭据形态、砍掉 `open_apps`、apiBalance 本次不动
- 阶段 2-3 实现：见上「最终实现」。worktree `../new-api--feature-self-balance-key/`
- 验证：`go build ./...`、`go vet ./...` 干净；`go test ./model/ ./router/ ./controller/ ./middleware/` 全绿；`bun run typecheck`、`bun run build` 通过
- commit：`93167785` feat(balance-api) → `9bd2d458` merge 到 main（`--no-commit --no-ff`，无冲突）
- 部署：build `31453825747` ✅ → deploy `31454017333` ✅；生产容器拉 `ghcr.io/xiangzeng/new-api:beta` 重启并 healthy
- 生产实测（root 账号一把密钥）：`GET /api/open/v1/balance` 200/8.5ms，返回 `quota=48,984,581,075` / `balance=$97,969.16215`，换算自洽且为**账户维度**；改一个字符 → 401 `CREDENTIAL_INVALID`；不带头 → 401；容器日志无任何 open credential 相关错误，异步写 `last_used_time` 在生产 PG 正常
- 下一步：本任务已可收尾。若日后要补重启连续性实证，用同一把密钥在 `docker restart new-api` 前后各查一次即可
- 遗留/坑：
  - **误部署事故**：用户说「合并到 main 推送部署」后 AI 未再确认即推送，`docker-image-main.yml` 对 push main **无 paths 过滤**、`deploy-hk.yml` 又挂在其成功之后，生产随即重启。用户随后说「先不要部署」时已经完成。教训见第 5 节
  - apiBalance 仍存站点用户名+密码，且其 `/api/usage/token/` fallback 因 `code: true` 是死的。本次按用户决定未动，若日后要改，切到余额密钥即可
  - 三份任务档案（本档案、`credential-signing-stability.md`、`reseller-center.md`）本地已提交但未推送，等下次有真代码要发时一起推，避免又触发一次无谓的生产重启

### 2026-08-11（会话2 · 阶段 0 完成，仍未写代码）
- 完成阶段 0 现状评估，全部结论基于读码验证，写入第 3 节
- 三项关键发现：① `/api/usage/token/` 是 token 维度且对无限额度 key 失真；② `UserAuth()` 已支持 PAT，但那是全权令牌，不可用于自查场景；③ apiBalance 的 token fallback 因 `code: true` 已是死路
- 用户拍板：接受新的只读余额凭据形态；**砍掉 `open_apps`**；授权把阶段 0 结论写回档案
- 下一步：阶段 1 出完整方案——数据库迁移影响、路由变更清单、前端改动面、生产已上线功能的下线步骤
- 遗留/坑：
  - **apiBalance 是否切到新凭据（并顺手修 `code: true` 死 fallback）尚未拍板**，待用户决定
  - `docs/open-balance-api.md` 整篇是围绕第三方 app 写的，方案落地后需重写而非增补

### 2026-08-11（会话1 · 仅澄清需求，未写代码）
- 用户指出前期理解错误：本功能的主体是**用户自己查自己**，不存在第三方合作方
- 建立本档案，记录被推翻的前提与阶段 0 的前置评估要求
- 下一步：阶段 0，确认 `/api/usage/token/` 等现成接口为什么不满足需求
- 遗留/坑：
  - **上一轮关于「每用户开关，默认关闭（opt-in）」的选型作废**，见第 5 节
  - 已上线的余额开放接口尚未决定保留 / 改造 / 下线

## 5. 决策与坑记录

- **2026-08-11：推 main 等于部署生产，必须单独确认。** `docker-image-main.yml` 触发条件是 push 到 main 且**没有 paths 过滤**，`deploy-hk.yml` 又挂在它构建成功之后自动接力。因此**任何**推到 main 的提交（哪怕只有 markdown）都会重建镜像并重启生产容器。本次 AI 已提前识别出这条链路，却在用户说「合并到 main 推送部署」后没有再确认一次时间点就一路推到底，用户随后叫停时部署已完成。此后凡涉及推 main，先说明「这会重启生产」并单独取得确认。
- **2026-08-11：不为一次实测重启生产（用户拍板）。** 重启前后凭证连续性由单测 `TestOpenCredentialSurvivesRestart` 锁死（模拟 `SessionSecret` 重掷后凭证仍可验证），生产实证的边际价值不值一次在途请求中断。
- **2026-08-11：apiBalance 本次不动（用户拍板）。** 它仍用站点用户名+密码登录换 cookie，其 `/api/usage/token/` fallback 因本站返回 `"code": true`（布尔）而恒定失败。改与不改都不影响本功能上线。
- **2026-08-11：选定方向 D，砍掉 `open_apps`（用户拍板）。** 复用 `open_credentials` 表与其鉴权/限流/审计/撤销链路，把签发入口从「站长建 app + 用户交密码换取」改成「用户在个人设置自助生成」。随之下线：`open_apps` 表与其管理页、`POST /api/open/v1/auth/exchange`、站长侧总开关。两张表在生产仍为空，此刻改造成本最低。
- **2026-08-11：不要用系统 PAT（`user.access_token`）当只读余额凭据。** 它能通过全部 `UserAuth` 路由（建 sk- key、充值、改账号、销号），单字段无 scope 无过期无最后使用时间。把它塞进自己的 APP 等于交出账号。若日后有人提议「让用户拿 access_token 查余额就行了」，此条即为否决依据。
- **2026-08-11：作废的结论 —— 不要照着实现。** 同一天早些时候曾讨论「给每个用户加一个『是否允许第三方查我的余额』开关，默认关闭（opt-in）」，并已选定该方案。该决策建立在「第三方站点代用户查余额」的前提上，**前提已被用户推翻**。自查场景下用户就是凭据持有者，不存在「允不允许别人查我」这个语义，因此该开关失去意义。新会话若看到相关讨论残留，一律忽略。
- **2026-08-11：需求归属判据。** 判断一个能力该由 root 管还是用户自管，看**凭据落在谁手里、风险由谁承担**。第三方站点收集本站用户密码 → 站点级信任决策，必须 root 批；用户在自己程序里查自己的余额 → 用户自己的事，root 不该介入。前期实现把后者按前者建模，是这次返工的根因。
- **2026-08-11：先查现成能力再开工。** 本功能第二次出现「已有接口能满足但没人知道」的情况（第一次是 2026-08-11 澄清 New API 本就支持余额查询）。阶段 0 的存在就是为了堵住这个反复。**但阶段 0 同时证明「文档化现成接口」这条零开发路线是走不通的**——现成接口给不出账户维度余额，别把「先查现成」误读成「多半不用开发」。

## 6. 关联档案
- `docs/tasks/credential-signing-stability.md` — 签名密钥去 `SessionSecret` 的修复。**该修复本身依然有效且必要**（`openCredentialSigningKey()` 就是本功能在用的那把）。其阶段 4 实测方案原依赖 `open_apps`，本任务砍掉 app 层后已在该档案中改写为「用余额密钥在容器重启前后各查一次」。
- `docs/tasks/reseller-center.md` — 站长中心，与本任务无直接依赖。
