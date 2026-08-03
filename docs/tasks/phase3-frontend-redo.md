# 任务档案：Phase 3 — 定制 UI 在上游新前端重做

> 状态：进行中｜分支：`merge/upstream-20260803`（worktree `../new-api--merge-upstream-20260803/`）｜创建：2026-08-03｜更新：2026-08-03

## 0. 新会话启动指令（AI 必读）

1. 读 `@AGENTS.md` 开发协议（含 RelayTeam fork 扩展段 + 上游约定）与 `@web/AGENTS.md` 前端规范
2. 读本档案全文
3. 复述「当前阶段 + 上次停点 + 下一步」，等授权，禁止直接动手

## 1. 目标与验收标准

- 目标：把 fork 的定制 UI 在上游全新前端（React 19 + TS + TanStack 文件路由 + rsbuild + Base UI + Tailwind + Bun）上重做。后端 API 已全部就绪（Phase 2 完成），本任务纯前端。
- 验收：P0 三项功能可用且 `bun run build` 绿；P1/P2 按批次验收；文案全部走 i18n（英文 key）。

## 2. 分阶段计划

- [ ] **P0-1 千人千面管理页**：新建 `web/src/features/custom-pricing/` + 路由 `web/src/routes/_authenticated/custom-pricing/index.tsx`；用户列表（`GET /api/user/custom-pricing/list`→注意实际为 adminRoute 前缀，见下方 API 清单）、分组倍率编辑、ExtraGroups/HideGroups、启停；侧边栏注册（`web/src/components/layout/` 体系）
- [ ] **P0-2 用户表千人千面入口**：`web/src/features/users/` 列/行操作加入口按钮打开定价弹窗（仿 features/users/components/dialogs/ 模式）
- [ ] **P0-3 邀请返利管理页**：新建 `web/src/features/invitations/`（仿 features/users 结构：api + columns + table）+ 路由；汇总表 + 受邀人明细 + 时间段筛选 + 关键词搜索；侧边栏模块开关（后端 sidebar key `invitation` 已就绪）
- [ ] **P1-1 藏充值价切换器**：`web/src/features/pricing/components/pricing-toolbar.tsx:208` 附近的 'recharge'/'standard' 二态选择器藏掉，`use-filters.ts` 固定 `showRechargePrice=false`（**已拍板：只显示标准价**）；分组倍率数值后端直出无需改
- [ ] **P1-2 充值卡/折扣回归验证**：`web/src/features/wallet/` 上游整页重写，验证充值折扣配置是否仍有旧崩溃问题（4db82f4b 修的 bug，大概率 obsolete，验证后关闭）
- [ ] **P2-1 Logo 上传控件**：`web/src/features/system-settings/site/` 加图片上传（后端 `POST /api/option/upload/logo` 已就绪，≤2MB，png/jpg/jpeg/gif/svg/ico/webp）
- [ ] **P2-2 清理错误日志按钮**：usage-logs 功能区加管理员按钮调 `DELETE /api/log/errors`
- [ ] **P2-3 收尾**：i18n 六语言补全（`bun run i18n:sync`）、`bun run build` 全绿、整体走查

## 3. 架构与上下文

- **后端 API 已就绪（Phase 2 重放完成，全部在本分支）**：
  - 千人千面：`GET /api/user/custom-pricing/list`、`GET/PUT/DELETE /api/user/:id/custom-pricing`（AdminAuth；见 `router/api-router.go` adminRoute 段、`controller/custom_pricing.go`）
  - 邀请返利：`GET /api/invitation/summary`、`GET /api/invitation/invitees`（AdminAuth；`controller/invitation.go`）
  - Logo：`POST /api/option/upload/logo`；错误日志清理：`DELETE /api/log/errors`
  - 用户级定价对普通接口的影响：`/api/user/groups`、`/api/pricing` 已返回覆盖后的倍率，前端零改动即正确显示
- **旧前端参考实现（已删，从 git 历史取）**：`git show pre-upstream-merge-20260803:web/src/pages/CustomPricing/index.jsx`（503 行）、`...:web/src/pages/Invitation/index.jsx`、`...:web/src/components/table/invitations/`（5 组件）、`...:web/src/hooks/invitations/useInvitationsData.jsx`——交互逻辑照抄，UI 按新架构重写
- **新前端关键约定**：文件路由自动生成 routeTree（`bun run build` 再生成）；i18n 英文 key 扁平 JSON；组件风格看同类 feature（users/pricing/wallet）；详细规范 `web/AGENTS.md`
- **技术决策**：前端整体采用上游实现，定制只做增量；不复活任何旧 JSX/Semi 组件

## 4. 进度台账（每次会话末追加，倒序）

### 2026-08-03（会话1 = Phase 0-2/4 主会话）

- 做了：上游合并 + 后端全部重放（13 commit，HEAD `40e55f78`）；迁移安全评估通过（AutoMigrate 5s、零 DROP、custom_pricing 完好）；移植 agent 协作协议
- commit：`74107c66`(merge) … `27a2c06b`(gpt-5 倍率) + `40e55f78`(协议移植)
- 下一步：从 P0-1 千人千面管理页开始（先给方案再动手）
- 遗留/坑：① worktree 未推送、main 冻结，Phase 3 完成后再谈合回/部署（Phase 5 需用户授权 + 部署 compose 补 `MAX_REQUEST_BODY_MB=200`、`STREAMING_TIMEOUT=600`）；② 上游会话体系上线后用户需重新登录（低峰部署）；③ dd74cceb 可见性覆盖在上游新增绕行路径（GetRequestAutoGroups/controller/token.go:172 等）未补全——当前与旧版行为等价，做完整覆盖是后端可选增强，不阻塞前端

## 5. 决策与坑记录

- 已拍板：gpt-5 无点号家族补全倍率保持 6（让利，代码已改 `setting/ratio_setting/model_ratio.go`）；充值价切换器藏掉只显标准价
- 上游错误调试信息只入服务端日志不进客户端错误（上游测试契约）
- 回滚点：tag `pre-upstream-merge-20260803`；生产 DB 每日 04:00 备份在 database-newapi
