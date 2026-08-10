# 余额开放接口（Balance Open API）

供第三方站点集成：用户在合作方网站输入 **本站用户名 + 密码**，合作方后端换取一个**长期有效、只读余额**的凭证，之后凭该凭证查询余额。用户无需登录本站。

- 基础路径：`https://<你的站点>/api/open/v1`
- 认证：换凭证用 `X-App-Id` / `X-App-Secret`；查余额用 `Authorization: Bearer <credential>`
- 默认关闭。站长需在后台「余额开放接口」页面开启后才可用。

---

## 1. 安全须知（对接前必读）

**这套接口要求用户把本站密码输入在你的网站上。** 请务必：

1. **`X-App-Secret` 只能放在你的服务端**，绝不能出现在浏览器代码、App 包体或任何客户端可读的位置。
2. **不要长期存储用户密码**。密码只用于换取一次凭证，用完即丢；后续所有查询都用返回的 `credential`。
3. **凭证按用户隔离存储**，并按你自己站点的会话边界保护它。凭证虽然只读，但能持续读到该用户的余额。
4. 用户在你的站点登出时，建议调用 `POST /auth/revoke` 主动吊销凭证。

`app_id` / `app_secret` / `credential` **均无有效期**，本站升级、重启或重新部署都不会使其失效。以下是失效的**完整清单**，出现时接口返回 `CREDENTIAL_REVOKED`，需要引导用户重新授权：

- 用户修改了本站密码
- 用户账号被禁用或删除
- 用户在本站「个人设置 → 第三方余额访问」中手动撤销
- 站长重置了你的 `app_secret`，或禁用/删除了你的应用
- 同一用户在你的站点再次换凭证（旧凭证作废，保证一个用户在一个站点只有一份有效授权）

因此不必为凭证设计定期重新授权的逻辑；只需在收到 `CREDENTIAL_REVOKED` 时引导用户重新走一次 `auth/exchange`。

---

## 2. 获取应用凭证

由本站站长在后台 **管理 → 余额开放接口** 中创建应用，创建后一次性展示：

| 字段 | 示例 | 说明 |
|------|------|------|
| `app_id` | `oapp_a1b2c3d4e5f6g7h8` | 应用标识，可长期保存 |
| `app_secret` | `oas_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx` | **仅展示一次**，服务端不存明文，丢失只能重置 |

站长还可为你的应用配置：

- **来源 IP 白名单**：填你后端服务器的出口 IP/CIDR，留空表示不限
- **换凭证限流**：默认走全局限额，可单独调高

---

## 3. 接口

### 3.1 换取凭证

```http
POST /api/open/v1/auth/exchange
Content-Type: application/json
X-App-Id: oapp_a1b2c3d4e5f6g7h8
X-App-Secret: oas_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

{
  "username": "alice",
  "password": "用户在你站点输入的密码",
  "end_user_ip": "198.51.100.4"
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `username` | 是 | 本站用户名**或**注册邮箱 |
| `password` | 是 | 本站登录密码 |
| `end_user_ip` | 否 | 终端用户真实 IP，仅写入审计日志，不参与鉴权 |

成功响应：

```json
{
  "success": true,
  "message": "",
  "data": {
    "credential": "obk_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
    "scope": "balance:read",
    "user": { "id": 42, "username": "alice", "display_name": "Alice" }
  }
}
```

> `credential` **仅此一次返回**，服务端只保存其 HMAC 摘要，无法找回。请立即持久化。

### 3.2 查询余额

```http
GET /api/open/v1/balance
Authorization: Bearer obk_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

```json
{
  "success": true,
  "message": "",
  "data": {
    "user_id": 42,
    "username": "alice",
    "display_name": "Alice",
    "quota": 500000,
    "used_quota": 123456,
    "balance": 1.0,
    "used": 0.246912,
    "display_type": "USD",
    "currency_symbol": "$",
    "request_count": 831
  }
}
```

| 字段 | 含义 |
|------|------|
| `quota` / `used_quota` | **原始额度单位**，不随站点展示设置变化。需要稳定数值时用这两个 |
| `balance` / `used` | 按站点当前展示口径换算后的金额，与本站面板显示一致 |
| `display_type` | `USD` / `CNY` / `TOKENS` / `CUSTOM`，站长可在后台修改 |
| `currency_symbol` | 对应符号；`TOKENS` 时为空字符串 |
| `request_count` | 该用户历史请求总次数 |

> **注意**：`balance` 会随站长调整展示口径而变化。如果你的页面需要口径稳定，请基于 `quota` 自行换算，或与站长约定固定口径。

### 3.3 吊销凭证

```http
POST /api/open/v1/auth/revoke
Authorization: Bearer obk_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

```json
{ "success": true, "message": "", "data": { "revoked": true } }
```

---

## 4. 错误码

失败响应统一为：

```json
{ "success": false, "code": "INVALID_CREDENTIALS", "message": "Incorrect username or password." }
```

`code` 是稳定契约，请基于它做分支判断；`message` 为固定英文，可能调整措辞，**不要用于逻辑判断**。

| `code` | HTTP | 出现在 | 含义与处理建议 |
|--------|------|--------|----------------|
| `OPEN_API_DISABLED` | 503 | 全部 | 站点尚未开启该接口，联系站长 |
| `APP_UNAUTHORIZED` | 401 | exchange | `app_id`/`app_secret` 缺失或不匹配 |
| `APP_DISABLED` | 403 | exchange | 你的应用被站长禁用 |
| `APP_IP_NOT_ALLOWED` | 403 | exchange | 请求来源 IP 不在白名单，联系站长补录服务器 IP |
| `INVALID_PARAMS` | 400 | exchange | 请求体格式错误，或缺少 `username`/`password` |
| `INVALID_CREDENTIALS` | 401 | exchange | 用户名或密码错误，提示用户重试 |
| `USER_DISABLED` | 403 | exchange / balance | 账号已被禁用 |
| `REQUIRE_2FA_UNSUPPORTED` | 403 | exchange | 该用户开启了两步验证，**不支持**通过本接口授权，请引导其到本站面板查看余额 |
| `CREDENTIAL_INVALID` | 401 | balance / revoke | 凭证缺失或无效，需重新换凭证 |
| `CREDENTIAL_REVOKED` | 401 | balance | 凭证已失效（原因见第 1 节），需引导用户重新授权 |
| `RATE_LIMITED` | 429 | exchange / balance | 触发限流或失败锁定，按响应头 `Retry-After`（秒）退避后重试 |
| `INTERNAL_ERROR` | 500 | 全部 | 服务端异常，可重试；持续出现请联系站长 |

---

## 5. 限流与锁定

限流**不按客户端 IP 计算**——你的所有用户都从你的后端发起请求，按 IP 限流会把整站用户当成一个客户端。实际维度：

| 维度 | 默认值 | 说明 |
|------|--------|------|
| 每个应用换凭证 | 300 次/分钟 | 站长可为单个应用单独调整 |
| 每个来源 IP 换凭证 | 600 次/分钟 | 鉴权前的兜底，防匿名洪水 |
| 每个凭证查余额 | 120 次/分钟 | 建议在你侧对余额做短时缓存 |
| 失败锁定 | 连续 5 次 → 锁 15 分钟 | 按「应用 + 用户名」计数；期间即使密码正确也返回 `RATE_LIMITED`；一次成功登录即清零 |

以上默认值站长可在后台调整。

---

## 6. 完整示例

```bash
APP_ID="oapp_a1b2c3d4e5f6g7h8"
APP_SECRET="oas_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
BASE="https://your-site.example.com"

# ① 换凭证（服务端调用）
CREDENTIAL=$(curl -sS -X POST "$BASE/api/open/v1/auth/exchange" \
  -H "Content-Type: application/json" \
  -H "X-App-Id: $APP_ID" \
  -H "X-App-Secret: $APP_SECRET" \
  -d '{"username":"alice","password":"******"}' \
  | jq -r '.data.credential')

# ② 查余额（之后每次）
curl -sS "$BASE/api/open/v1/balance" \
  -H "Authorization: Bearer $CREDENTIAL" | jq

# ③ 用户登出时吊销
curl -sS -X POST "$BASE/api/open/v1/auth/revoke" \
  -H "Authorization: Bearer $CREDENTIAL" | jq
```

---

## 7. 站长侧配置速查

| 位置 | 可配置项 |
|------|----------|
| 管理 → 余额开放接口 → 开放接口设置 | 总开关、四项限流参数、失败锁定阈值与时长 |
| 管理 → 余额开放接口 → 应用列表 | 新建/编辑/禁用/删除应用、重置密钥、来源 IP 白名单、单应用换凭证限额 |
| 个人设置 → 第三方余额访问 | **用户自己**查看已授权的站点并随时撤销 |

审计：每次成功换凭证都会写入登录日志（`type=login`，`op.action=open_api_exchange`），可在日志页按用户查询，记录中包含应用名、应用 ID 与终端用户 IP。
