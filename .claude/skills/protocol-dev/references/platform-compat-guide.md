# 平台 / 版本兼容性规范 (The "VERSION-CHECK" Rule)

在编写任何有版本兼容约束的代码之前，**必须先确认项目的兼容底线**，避免使用不兼容的 API / SQL 方言。

## 当前项目配置

- **Go**：以 `go.mod` 的 go 指令为准（当前 1.25.x）；新语法/标准库特性使用前对照该版本。`relaykit/` 是独立 go module，改动后需在其目录内单独 build/test
- **数据库（最硬的兼容底线）**：SQLite / MySQL >= 5.7.8 / PostgreSQL >= 9.6 **三库必须同时兼容**；日志库另有 ClickHouse 分支（`common.UsingLogDatabase`）
- **前端**：Node 18+ / Bun，构建走 rsbuild；浏览器目标以 `web/` 构建配置为准
- **配置位置**：`go.mod`、`relaykit/go.mod`、`web/package.json`、`model/main.go`（三库分支模式）

## 强制检查流程

1. **方案设计阶段**：涉及 SQL / 迁移 / 新依赖时，必须确认三库兼容性并在方案中说明
2. **编码阶段**：优先 GORM 抽象；裸 SQL 必须逐库核对方言
3. **如需库特有能力**：用 `common.UsingPostgreSQL / UsingSQLite / UsingMySQL`（或 `common.UsingMainDatabase` / `UsingLogDatabase`）显式分支，并为其余库提供等价路径

## 常见"方言差异 → 兼容写法"（按项目维护）

- 保留字列名 `group` / `key` → 用 `model/main.go` 的 `commonGroupCol` / `commonKeyCol`，禁止硬编码引号风格
- 布尔字面量（PG `true/false` vs MySQL/SQLite `1/0`）→ `commonTrueVal` / `commonFalseVal`
- `GROUP_CONCAT`（MySQL）→ PG 需 `STRING_AGG` 等价分支，否则禁用
- PG 专属操作符（`@>`、`?`、JSONB）→ 禁用；JSON 存储统一 `TEXT` 列
- `DELETE ... LIMIT`（MySQL 可用，PG 不支持）→ 先 `Pluck("id")` 取批次再 `WHERE id IN` 删除（参见 `model.DeleteErrorLogs`）
- SQLite 不支持 `ALTER COLUMN` → 只用 `ADD COLUMN`（模式见 `model/main.go` 迁移段）
- ClickHouse 日志库删除 → `ALTER TABLE logs DELETE ... SETTINGS mutations_sync=1` 单次 mutation，不走分批（参见 `DeleteOldLogBatch`）
- 主键自增 → 交给 GORM，禁止手写 `AUTO_INCREMENT` / `SERIAL`

## 🚫 严禁行为

- ❌ 不核对方言直接写裸 SQL
- ❌ 只在一种数据库上验证就提交迁移
- ❌ 使用无跨库降级的库特有函数 / 列类型

## ✅ 正确做法

- ✅ 方案阶段主动标注 SQL 兼容性结论
- ✅ 优先 GORM 方法（Create/Find/Where/Updates）
- ✅ 迁移变更在三库语义下都推演一遍（AutoMigrate 只增不删是底线）
