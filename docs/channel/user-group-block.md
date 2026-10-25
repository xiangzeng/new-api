# 用户分组限制（防蒸馏临时封禁）

管理员对**单个用户**临时封禁**某些分组**，到期自动恢复。用于发现蒸馏时最快止血：
中转站用户的其他分组 / 模型照常使用，只有被封分组不可用。

粒度是**分组**而不是渠道：号池里的渠道本质无差别，运营按分组划分类别；
按分组封也能自动覆盖之后加进该分组的渠道。

## 1. 数据

- 存储：`users.group_blocks`（TEXT，`dto.UserGroupBlocks` JSON），只由
  `model.UpdateUserGroupBlocks` 维护，写后清用户缓存。
- 条目：`group` / `expires_at`（unix 秒，0 = 永久）/ `note`（管理员内部备注）/
  `created_by` / `created_at`。
- `User.GroupBlocks` 为 `json:"-"`：含内部备注，不随任何 User 序列化返回给用户；
  `UserBase` 缓存带该字段，鉴权时 `WriteContext` 写入 `ContextKeyUserGroupBlocks`。
- 到期在请求时按当前时间判断（`UserGroupBlock.IsActive`），无定时任务；
  过期条目在列表里不再出现，下次保存时被丢弃。

## 2. 拦截（`service/user_group_block.go`、`middleware/distributor.go`）

- **指定分组**（令牌分组、令牌未设分组时的用户本组、游乐场所选分组）命中封禁：
  `Distribute` 在选渠道前返回 **403**，明示
  「分组 X 暂时对您的账号停止服务，预计 YYYY-MM-DD HH:MM (UTC+08:00) 恢复」；
  永久封禁不带恢复时间。所有 relay 路由都经 `Distribute`，一处覆盖。
- **auto 分组**：选路（`CacheGetRandomSatisfiedChannel` 两种模式）与级联重试上限
  （`GetCascadeMaxRetryTimes`）改用 `GetRequestUserAutoGroup`，跳过被封分组顺延；
  用户全部 auto 分组都被封时同样 403，恢复时间取最早解封的那个。
- **亲和归属**：归属渠道只在被封分组下可用 → `detour_reason=user_group_blocked`，
  按临时不可用处理（归属模式保留归属本次绕行，解封后回归；旧模式让位即清除）。
  渠道同时在未封分组下可用时照常命中（封的是分组不是渠道）。
- 游乐场走会话鉴权、不写用户缓存到 context：`requestUserGroupBlocks` 按用户 id 读缓存补齐。
- 不改：用户分组列表、令牌分组下拉、模型广场、定价页仍显示被封分组（临时封禁只在请求时拦）。

## 3. 管理接口（AdminAuth，写操作由审计中间件兜底留痕）

- `GET /api/user/group-blocks/list`：有生效中限制的用户
- `GET /api/user/:id/group-blocks`：生效中限制 + 可选分组（用户可用分组，不含 auto）
- `PUT /api/user/:id/group-blocks`：整体覆盖 `{blocks:[{group, expires_at, note}]}`，
  请求里没有的分组即解除；到期时间未变的分组保留原创建时间 / 操作人。
  校验：不能是 auto、分组须存在于分组倍率、不能重复、到期须晚于当前、备注 ≤ 255 字
- `DELETE /api/user/:id/group-blocks`：解除全部

## 4. 前端（web/default）

- 页面 `/group-blocks`「分组限制」（侧边栏千人千面下方，挂 `user` 模块开关）
- 配置弹窗 `UserGroupBlocksDialog`：逐分组开关 + 时长（1h / 6h / 24h / 3d / 7d / 永久 / 自定义；
  已有限制默认「保持不变」）+ 内部备注（只写入本次新限制或改过时长的分组）
- 用户列表行操作「分组限制」入口
