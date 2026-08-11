# 余额查询接口（Balance API）

供你在**自己的程序**里查询**自己账号**的余额：手机 App、桌面小组件、监控脚本、命令行工具都可以。

- 基础路径：`https://<站点地址>/api/open/v1`
- 认证：`Authorization: Bearer <余额密钥>`
- 无需站长开通，无需联系任何人。登录站点，在「个人设置 → 余额查询密钥」自助生成即可。

---

## 1. 为什么不用 API 密钥或访问令牌

站点里有三种凭据，能力差别很大：

- **API 密钥（`sk-` 开头）**：能调用模型、能花钱。它查到的是**这把 key 自己的额度**，不是账户余额；无限额度的 key 查出来是 0，数字没有意义。
- **系统访问令牌（个人设置 → Access Token）**：能做你登录后能做的**一切**——新建 API 密钥、充值、改账号资料、注销账号。放进任何程序都等于把账号交出去。
- **余额密钥（`obk_` 开头，本文档）**：**只能读余额，别的什么都做不了**。泄露了，对方只能看到你还剩多少钱。

所以要在自己的程序里查余额，用余额密钥。

## 2. 生成与管理

登录站点 → **个人设置 → 余额查询密钥**：

- **创建**：给密钥起个名字（建议写用它的程序名，如「手机小组件」），点创建。**密钥明文只显示这一次**，服务端只保存摘要，关掉弹窗就再也看不到了；丢了就撤销重建。
- **查看**：列表显示名称、密钥尾部提示（如 `obk_…a1b2c3`）、创建时间、**最近使用时间**。想确认某个程序还在不在用，看这一列。
- **撤销**：点「撤销」立即失效，仍在使用它的程序会立刻查不到余额。
- **数量上限**：每个账号最多同时持有 **5 把**，撤销后名额即释放。

密钥**没有有效期**。站点升级、重启、重新部署都不会让它失效。失效的完整清单只有三条：

- 你自己在个人设置里撤销了它
- 你的程序调用了 `POST /auth/revoke`
- 账号被禁用或删除

> 修改站点登录密码**不会**让余额密钥失效——它是你自己的东西，行为与 API 密钥一致。若怀疑密钥泄露，请到个人设置里撤销。

## 3. 接口

### 3.1 查询余额

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
| `quota` / `used_quota` | **账户维度的原始额度单位**，不随站点展示设置变化。需要稳定数值时用这两个 |
| `balance` / `used` | 按站点当前展示口径换算后的金额，与站点面板显示一致 |
| `display_type` | `USD` / `CNY` / `TOKENS` / `CUSTOM`，由站长设置 |
| `currency_symbol` | 对应符号；`TOKENS` 时为空字符串 |
| `request_count` | 该账号历史请求总次数 |

> **注意**：`balance` 会随站长调整展示口径而变化。要口径稳定，请基于 `quota` 自行换算。

### 3.2 吊销密钥

程序自己注销手里的密钥，不需要知道它的 id：

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
{ "success": false, "code": "CREDENTIAL_INVALID", "message": "The credential is invalid." }
```

`code` 是稳定契约，请基于它做分支判断；`message` 为固定英文，措辞可能调整，**不要用于逻辑判断**。

| `code` | HTTP | 含义与处理建议 |
|--------|------|----------------|
| `CREDENTIAL_INVALID` | 401 | 密钥缺失、拼错或从未存在，检查 `Authorization` 头 |
| `CREDENTIAL_REVOKED` | 401 | 密钥已被撤销，去个人设置重新生成一把 |
| `USER_DISABLED` | 403 | 账号已被禁用 |
| `RATE_LIMITED` | 429 | 触发限流，按响应头 `Retry-After`（秒）退避后重试 |
| `INTERNAL_ERROR` | 500 | 服务端异常，可重试；持续出现请联系站长 |

## 5. 限流

按**密钥**计算，默认 **120 次/分钟**（站长可调）。余额不是高频变化的数据，建议在你的程序里缓存几十秒，不要空转轮询。

---

## 6. 完整示例

```bash
BASE="https://your-site.example.com"
BALANCE_KEY="obk_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"   # 个人设置里生成

# 查余额
curl -sS "$BASE/api/open/v1/balance" \
  -H "Authorization: Bearer $BALANCE_KEY" | jq

# 只取原始额度（口径不随站点设置漂移）
curl -sS "$BASE/api/open/v1/balance" \
  -H "Authorization: Bearer $BALANCE_KEY" | jq '.data.quota'

# 程序退役时自己注销
curl -sS -X POST "$BASE/api/open/v1/auth/revoke" \
  -H "Authorization: Bearer $BALANCE_KEY" | jq
```

Python：

```python
import requests

BASE = "https://your-site.example.com"
KEY = "obk_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

resp = requests.get(f"{BASE}/api/open/v1/balance",
                    headers={"Authorization": f"Bearer {KEY}"}, timeout=10)
body = resp.json()
if not body.get("success"):
    raise SystemExit(f"{body.get('code')}: {body.get('message')}")

data = body["data"]
print(f"余额 {data['balance']}{data['currency_symbol']}（原始额度 {data['quota']}）")
```

---

## 7. 站长侧

- **无总开关**：该能力对所有已登录用户默认开放，站长不需要也无法逐个批准。凭据由用户自己签发、自己撤销，风险由持有人承担。
- **可调项**：每密钥每分钟的余额读取上限（系统设置中的 `open_balance_api.balance_rate_limit_per_minute`，默认 120）。
