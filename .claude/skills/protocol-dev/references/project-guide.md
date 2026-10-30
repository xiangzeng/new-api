# 项目专属指南（RsTroubleDebug/new-api）

> 本文件是项目件：只写本项目特有的做法，模板同步时不会覆盖。protocol-dev 的通用规则与本文件冲突时，以本文件为准。

## 协议入口

- 本项目是上游 QuantumNous/new-api 的 RelayTeam 二开副本。`AGENTS.md`、`CLAUDE.md` 是上游自带的文件，不按模板改写：`CLAUDE.md` 是上游的 `@AGENTS.md` 导入桩，原样不动；我们的协议只写在 `AGENTS.md` 头部「Agent 协作协议（RelayTeam fork 扩展）」一节，该节之外是上游项目约定原文，合并上游时尽量保持原样。
- 前端细则见 `web/AGENTS.md`。

## 运行环境与版本底线

- **Go**：以 `go.mod` 的 go 指令为准（当前 1.25.x）；新语法 / 标准库特性使用前对照该版本。`relaykit/` 是独立 go module，改动后需在其目录内单独 build / test。
- **数据库（最硬的兼容底线）**：SQLite / MySQL >= 5.7.8 / PostgreSQL >= 9.6 **三库必须同时兼容**；日志库另有 ClickHouse 分支（`common.UsingLogDatabase`）。
- **前端**：Node 18+ / Bun，构建走 rsbuild；浏览器目标以 `web/` 构建配置为准。
- **配置位置**：`go.mod`、`relaykit/go.mod`、`web/package.json`、`model/main.go`（三库分支模式）。

三库兼容检查流程：

1. **方案设计阶段**：涉及 SQL / 迁移 / 新依赖时，必须确认三库兼容性并在方案中说明。
2. **编码阶段**：优先 GORM 抽象（Create / Find / Where / Updates）；裸 SQL 必须逐库核对方言。
3. **如需库特有能力**：用 `common.UsingPostgreSQL / UsingSQLite / UsingMySQL`（或 `common.UsingMainDatabase` / `UsingLogDatabase`）显式分支，并为其余库提供等价路径。
4. 迁移变更在三库语义下都推演一遍（AutoMigrate 只增不删是底线）。

禁止：不核对方言直接写裸 SQL；只在一种数据库上验证就提交迁移；使用无跨库降级的库特有函数 / 列类型。

## 构建与验证

- 编译 / 检查：
  - 后端 `go build ./...`；相关包跑 `go test ./<包>/`。
  - 改动 `relaykit/` 时，在其目录内再跑 `GOWORK=off go build ./...` + `go test ./...`（`cd relaykit && GOWORK=off go build ./...`）。
  - 前端 `cd web && bun run build`（首次编译需 `web/dist` 占位或真实产物满足 go:embed）。
- 运行策略：允许本机直接运行验证（服务型项目）；涉及数据库的验证优先用本地 Docker 临时库，禁止直连生产库。

## 新文件登记

方案里的新增文件，如需登记到构建 / 路由 / i18n 体系，一并注明：

- Go 源文件：无需登记（包内自动编译）；新 HTTP 接口需在 `router/` 注册路由。
- 前端页面 / 组件：TanStack 文件路由放 `web/src/routes/_authenticated/`（构建自动生成 routeTree.gen.ts）；业务代码放 `web/src/features/<域>/`。
- 用户可见文案：需登记 `web/src/i18n/locales/*.json`（后端为 `i18n/locales/*.yaml`）。

创建新文件后，提醒用户完成登记（如需要）：

1. 新 API 路由：确认 `router/api-router.go` 已注册且鉴权中间件正确（AdminAuth / UserAuth / TokenAuth）。
2. 管理写接口：确认 `middleware/audit.go` 的路由 → 审计动作 map 是否需要加条目。
3. 前端新页面：跑一次 `bun run build` 让 routeTree 再生成；新文案跑 `bun run i18n:*` 工具链。

## 项目专属流程

- 版本发布与部署（GHCR + 香港自动部署）→ `references/release-guide.md`

## 踩过的坑

三库方言差异 → 兼容写法：

- 保留字列名 `group` / `key` → 用 `model/main.go` 的 `commonGroupCol` / `commonKeyCol`，禁止硬编码引号风格。
- 布尔字面量（PG `true/false` vs MySQL/SQLite `1/0`）→ `commonTrueVal` / `commonFalseVal`。
- `GROUP_CONCAT`（MySQL）→ PG 需 `STRING_AGG` 等价分支，否则禁用。
- PG 专属操作符（`@>`、`?`、JSONB）→ 禁用；JSON 存储统一 `TEXT` 列。
- `DELETE ... LIMIT`（MySQL 可用，PG 不支持）→ 先 `Pluck("id")` 取批次再 `WHERE id IN` 删除（参见 `model.DeleteErrorLogs`）。
- SQLite 不支持 `ALTER COLUMN` → 只用 `ADD COLUMN`（模式见 `model/main.go` 迁移段）。
- ClickHouse 日志库删除 → `ALTER TABLE logs DELETE ... SETTINGS mutations_sync=1` 单次 mutation，不走分批（参见 `DeleteOldLogBatch`）。
- 主键自增 → 交给 GORM，禁止手写 `AUTO_INCREMENT` / `SERIAL`。
