# 任务档案：凭证签名密钥的持久化与可控轮换

> 状态：阶段 1-3 已完成并上线生产；阶段 4 生产实测已解除阻塞但**暂缓**（不为它重启生产）｜分支：`fix/open-api-signing-key`（已合入 main）｜创建：2026-08-10｜更新：2026-08-11

## 0. 新会话启动指令（AI 必读）
1. 读 `@AGENTS.md` 开发协议
2. 读本档案全文
3. 复述「当前阶段 + 上次停点 + 下一步」，等授权，禁止直接动手

## 1. 目标与验收标准
- 目标：让已签发的 `app_secret` 与用户 `credential` **除非有人主动轮换，否则永久有效**，不再因进程重启而集体失效。
- 验收：
  1. 重启 New API 容器（或重新部署）后，重启前签发的 app_secret 与 credential 仍能通过校验 — **代码层已由单测锁死，生产实测待阶段 4**
  2. 存在一个**显式**的轮换入口，触发后旧凭证按预期失效，且该行为有明确提示与审计 — **已满足**（见阶段 3）
  3. 不依赖运维记得配置某个环境变量才成立 — **已满足**（签名密钥不再读任何环境变量）

## 2. 分阶段计划
- [x] 阶段1 方案定档：选定方案 **C′**，并决定不在本任务内处理 SessionSecret 的历史包袱 — 完成
- [x] 阶段2 后端改造：两处签名密钥去掉 `SessionSecret`，新增重启回归测试 — 完成（commit `872ca89c`）
- [x] 阶段3 轮换入口与 UI：核实既有入口已满足验收，补齐审计动作名与文档 — 完成（同 commit）
- [ ] 阶段4 验证：生产重启前后凭证连续性实测 — **暂缓（用户 2026-08-11 决定不为此重启生产）**｜已解除阻塞，实测方式见下

### 与 `docs/tasks/self-balance-query.md` 的关系（2026-08-11 更新：app 层已删除）
`self-balance-query` 已于 2026-08-11 定案并上线：余额查询改为**用户自助签发只读密钥**，`open_apps` 整层（应用管理、密码换凭证、来源 IP 白名单、总开关）已删除。

对本任务的影响：
- **阶段 1-3 的修复依然有效且必要**，且现在是唯一在用的那把：`model/open_credential.go` 的 `openCredentialSigningKey()` 常量支撑着所有余额密钥的摘要校验。`openAppSigningKey()` 随 `model/open_app.go` 一并删除。
- **阶段 4 的实测方式改写为**：用户在「个人设置 → 余额查询密钥」生成一把 → `GET /api/open/v1/balance` 应 200 → `docker restart new-api` → 用**同一把密钥**再查一次，仍 200 即证明摘要不随进程变化（修复前此处会变成 401 `CREDENTIAL_INVALID`）。不再需要总开关、测试应用或密码。
- **暂缓理由**：单测 `TestOpenCredentialSurvivesRestart` 已锁死同一条不变量（重掷 `SessionSecret` 后凭证仍可验证且仍能被撤销定位），生产实证的边际价值不值一次在途请求中断。

## 3. 架构与上下文

### 问题根因（已修复）
`common/constants.go:35` 中 `SessionSecret = uuid.New().String()`，`common/init.go:57` 仅在 `SESSION_SECRET` 环境变量存在时覆盖它。生产（DMIT 香港 `/root/new-api`）未配置该变量，因此**每个进程启动都是新值**。

本功能两处签名密钥原本都派生自它，导致每次重启 `open_apps.secret_hash` 与 `open_credentials.token_hash` 全部对不上，等于集体作废。

### 候选方案与定档结论
| | 做法 | 结论 |
|---|---|---|
| A | 配置 `SESSION_SECRET` 环境变量 | 否决：靠运维记得配，不满足验收 3；且把本功能绑死在全局密钥上，牵连会话/邀请/auth_flows |
| B | 独立签名密钥持久化到 `option` 表 | 否决：密钥与摘要同库，泄露面与无密钥一致，安全增益为零，却新增全局单点 |
| C | 明文 selector + 每行随机 salt（GitHub PAT 式） | 否决：方向对但手段过重，需改两张表结构与查询路径 |
| **C′** | **去掉密钥本身**：HMAC key 从 `"<域前缀>:" + SessionSecret` 改为固定域分隔常量 | **采用** |

选 C′ 的依据：两处被哈希的对象都是 `crypto/rand` 24 字节（192 bit）随机串，密钥化 HMAC 的价值在于防止**低熵**秘密被离线爆破，对 192 bit 随机串加不加密钥都爆不动。即密钥从一开始就没提供实质安全增益，只带来了重启失效这个副作用。等价于 GitHub PAT / Stripe API key 的做法：明文只回显一次，库里存摘要。

**唯一安全代价**：拿到只读 DB 的攻击者可离线校验「某个他已持有的 token 是否属于某用户」。方案 B 在同样场景下结果相同。

### 关键文件（2026-08-11 随 app 层删除后更新）
- `model/open_credential.go` — `openCredentialSigningKey()`，返回常量 `"open-credential-v1"`。**本任务现存的唯一落点**
- `web/src/features/profile/components/balance-keys-card.tsx` — 用户自助签发与撤销，轮换入口已由「重置应用密钥」变为「撤销后重新创建」
- `setting/system_setting/open_balance_api.go` — 本功能配置（已瘦身为仅剩余额读取限流）
- `docs/open-balance-api.md` — 对外文档（已按自助口径重写）
- ~~`model/open_app.go`（`openAppSigningKey` / `ResetOpenAppSecret`）~~、~~`web/src/features/open-apps/`~~、~~`middleware/audit.go` 四条 `open_app.*`~~ — 均已随 app 层删除

### 同源的历史包袱（本任务范围外，已决策不动）
以下也挂在随机 SessionSecret 上：
- `model/reseller_invitation.go` → 站长邀请链接。**与本任务同构，适用同一处方**，但有存量数据，建议单独立项
- `model/auth_flow.go` → OAuth / 2FA / Passkey 流程态。分钟级短周期，维持现状
- `model/user_session.go` → 会话摘要与 refresh 校验（重启即全员登出）。行为与上游一致，维持现状

## 4. 进度台账（每次会话末追加，倒序）

### 2026-08-11（会话3 · 受 self-balance-query 上线影响，未改本任务代码）
- `self-balance-query` 上线，`open_apps` 整层删除：`openAppSigningKey()` 随 `model/open_app.go` 一并消失，本任务的落点收敛为 `openCredentialSigningKey()` 一处
- 阶段 4 解除阻塞并改写实测方式（不再需要总开关与测试应用），同时按用户决定**暂缓**执行，理由记在上方
- 关键文件清单已按删除结果更新；`TestOpenCredentialSurvivesRestart` 随余额密钥改造一并保留并更新签名（`IssueOpenCredential(userId, name, ip)`）
- 下一步：若日后有一次计划内的生产重启（如下一次真代码部署），顺手用余额密钥在重启前后各查一次即可关闭阶段 4，无需专门重启

### 2026-08-11（会话2）
- 阶段1 定档：在 A/B/C 外补充 C′ 并选定，理由见上「候选方案与定档结论」
- 阶段2 完成：两处 `*SigningKey()` 去掉 `common.SessionSecret`；新增 `TestOpenAppSecretSurvivesRestart` 与 `TestOpenCredentialSurvivesRestart`（用 `uuid.New()` 重掷 SessionSecret 模拟重启，后者额外断言重启后 `RevokeOpenCredentialByToken` 仍能定位到同一行）；清理各测试夹具中失去意义的 SessionSecret 设置
- 阶段3 完成：核实轮换入口已存在且粒度更优（按应用轮换，非全局一刀切），无需新建；补齐 `middleware/audit.go` 四条 `open_app.*` 动作名；`docs/open-balance-api.md` 明确凭证无有效期、重启与重新部署不失效
- commit：`872ca89c` fix(open-api) → `25b60c18` merge 到 main
- 验证：`go build ./...`、`gofmt`、`go vet` 干净；`go test ./model/ ./router/ ./middleware/` 全绿
- 部署：Actions build `31397321087` ✅ → deploy `31397704776` ✅；生产容器 2026-08-10T14:21:54Z 重启并 healthy；`/api/open/v1/balance` 仍返回 `503 OPEN_API_DISABLED`（总开关未开），启动日志无迁移报错
- 下一步：阶段 4。开启总开关 → 建测试应用 → 用**正确的 app 凭据 + 故意写错的用户名密码**打 `auth/exchange`，期望 `INVALID_CREDENTIALS`（说明应用鉴权已过）→ 重启容器 → 同样请求仍返回 `INVALID_CREDENTIALS` 而非 `APP_UNAUTHORIZED`，即证明修复生效。测完删除测试应用
- 遗留/坑：
  - 无 schema 变更、无迁移。定档时两张表在生产为空，即使有存量行也只失效一次，与修复前每次重启的行为一致，无退化
  - 总开关现在**可以安全开启**了（凭证不再因重启失效），阶段 4 需要它开着

### 2026-08-10（会话1）
- 做了：完成余额开放接口全量开发并上线生产；部署后核验时发现本问题
- commit：`75259106` feat(open-api) / `6f7247df` merge 到 main
- 生产状态：镜像 `ghcr.io/xiangzeng/new-api:beta`（12:04Z），容器 healthy，迁移无报错，`/api/open/v1/*` 返回 `503 OPEN_API_DISABLED`（总开关关闭）
- 下一步：从「阶段1 方案定档」起，在 A/B/C 中选型
- 遗留/坑：
  - 生产 New API 端口是 **8000**（容器内 3000）；`127.0.0.1:3000` 上是 `relaypulse-monitor`，排障时别探错
  - `/api/status` 等接口要求 `Accept-Encoding: gzip`，curl 需带 `--compressed`
  - 功能总开关保持关闭，**在本任务收尾前不要开启**，否则会签发出注定失效的凭证 —— *已于会话 2 解除，见上*

## 5. 决策与坑记录
- 2026-08-10：开发期沿用了 `model/reseller_invitation.go` 既有的 `SessionSecret` 派生模式以保持一致；上线后确认该模式在本环境下不成立（生产未配置该变量），故立项重做。
- 2026-08-11：签名密钥的作用域判据 —— **被哈希的对象是否高熵**。对服务端 `crypto/rand` 生成的长随机串，密钥化 HMAC 不提供额外保护，只引入密钥生命周期问题；对用户口令一类低熵秘密则相反，必须保留密钥或用慢哈希。本仓库后续新增此类摘要时按这条判断，不要机械照抄既有写法。
- 2026-08-11：域分隔仍然必要且保留 —— 常量 `"open-app-secret-v1"` / `"open-credential-v1"` 保证两张表的摘要永远不会互相验证通过。改动去掉的是密钥的**秘密性**，不是**域分隔**。
- 2026-08-11：轮换粒度按应用（`ResetOpenAppSecret`）优于全局密钥轮换 —— 前者只影响一个合作方，后者一刀切作废所有凭证。这也是否决方案 B 的理由之一。
