package model

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// PostgreSQL 连接的断线检测：空闲 15 秒开始探测、每 15 秒一次、连续 4 次无回应即断开；
// 已发出的数据 60 秒内没被确认也直接断开（TCP_USER_TIMEOUT，仅 Linux）。
// pgx 默认保活是 5 分钟起探、5 分钟一次，网络单向丢包时一条连接能挂几十分钟，
// 2026-11-14 美国从节点的批量扣费回写就因此卡死 10 分钟。
const (
	postgresKeepAliveIdle     = 15 * time.Second
	postgresKeepAliveInterval = 15 * time.Second
	postgresKeepAliveCount    = 4
	postgresTCPUserTimeout    = 60 * time.Second
)

// 启动迁移时的等锁上限：拿不到表锁就放弃并重试，避免 ALTER 排在长事务后面、
// 把后续所有业务查询堵在锁队列里（2026-11-14 部署事故）。
const (
	postgresMigrationLockTimeout = "10s"
	postgresMigrationAttempts    = 3
	postgresMigrationRetryDelay  = 5 * time.Second
)

var postgresTimeZoneMatcher = regexp.MustCompile("(time_zone|TimeZone)=(.*?)($|&| )")

// openPostgres 按 DSN 建 PostgreSQL 连接池：带断线检测的拨号器、简单查询协议，
// runtimeParams 会作为会话参数在每条连接建立时下发（如迁移专用的 lock_timeout）。
func openPostgres(dsn string, runtimeParams map[string]string, gormConfig *gorm.Config) (*gorm.DB, error) {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	if result := postgresTimeZoneMatcher.FindStringSubmatch(dsn); len(result) > 2 {
		config.RuntimeParams["timezone"] = result[2]
	}
	for key, value := range runtimeParams {
		config.RuntimeParams[key] = value
	}
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		KeepAliveConfig: net.KeepAliveConfig{
			Enable:   true,
			Idle:     postgresKeepAliveIdle,
			Interval: postgresKeepAliveInterval,
			Count:    postgresKeepAliveCount,
		},
		Control: setTCPUserTimeout(postgresTCPUserTimeout),
	}
	config.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, addr)
	}

	dialector := postgres.New(postgres.Config{Conn: stdlib.OpenDB(*config)}).(*postgres.Dialector)
	return gorm.Open(stablePostgresDialector{Dialector: dialector}, gormConfig)
}

// migrateWithLockTimeout 用带 lock_timeout 的独立连接池跑一次迁移，等锁超时则稍后重试。
// 迁移代码都通过全局 DB 访问数据库，这里在启动阶段（尚无业务流量与后台任务）临时替换全局 DB。
func migrateWithLockTimeout(dsn string, migrate func() error) error {
	migrationDB, err := openPostgres(dsn, map[string]string{"lock_timeout": postgresMigrationLockTimeout}, newGormConfig(true))
	if err != nil {
		return err
	}
	sqlDB, err := migrationDB.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if common.DebugEnabled {
		migrationDB = migrationDB.Debug()
	}

	previousDB := DB
	DB = migrationDB
	defer func() { DB = previousDB }()

	for attempt := 1; ; attempt++ {
		err = migrate()
		if err == nil || !isPostgresLockTimeout(err) || attempt >= postgresMigrationAttempts {
			return err
		}
		common.SysLog(fmt.Sprintf("database migration waited too long for a table lock (attempt %d/%d), retrying in %s: %v", attempt, postgresMigrationAttempts, postgresMigrationRetryDelay, err))
		time.Sleep(postgresMigrationRetryDelay)
	}
}

func isPostgresLockTimeout(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "55P03"
}

// stablePostgresDialector 只替换迁移器，其余行为与官方 postgres 驱动一致。
type stablePostgresDialector struct {
	*postgres.Dialector
}

func (d stablePostgresDialector) Migrator(db *gorm.DB) gorm.Migrator {
	return stablePostgresMigrator{Migrator: d.Dialector.Migrator(db).(postgres.Migrator)}
}

// stablePostgresMigrator 修正 GORM 1.25.2 / postgres 驱动 1.5.2 的误判：带唯一索引的列、
// char/varchar 列、空串默认值的列每次启动都会被判定为需要 ALTER（约 85 条语句、每条都要表级排它锁）。
// 库里的列已经与字段定义一致时跳过 ALTER；不确定时照旧交给驱动处理。
type stablePostgresMigrator struct {
	postgres.Migrator
}

func (m stablePostgresMigrator) AlterColumn(value interface{}, fieldName string) error {
	alreadyMatches := false
	_ = m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if stmt.Schema == nil {
			return nil
		}
		field := stmt.Schema.LookUpField(fieldName)
		if field == nil {
			return nil
		}
		columnTypes, err := m.ColumnTypes(value)
		if err != nil {
			return nil
		}
		// 驱动读列信息时不认唯一索引（Unique() 恒为 false），与驱动自身一样改看同名唯一索引是否存在
		uniqueSatisfied := !field.Unique || m.HasIndex(stmt.Table, m.DB.Config.NamingStrategy.IndexName(stmt.Table, field.DBName))
		for _, columnType := range columnTypes {
			if columnType.Name() == field.DBName {
				alreadyMatches = postgresColumnMatchesField(m.DataTypeOf(field), field, columnType, uniqueSatisfied)
				break
			}
		}
		return nil
	})
	if alreadyMatches {
		return nil
	}
	return m.Migrator.AlterColumn(value, fieldName)
}

var postgresSizedTypePattern = regexp.MustCompile(`^(char|varchar)\((\d+)\)$`)

// postgresColumnMatchesField 判断库里的列是否已与字段定义一致。只认得 char(n)/varchar(n)/text 与
// 同名类型，其余一律返回 false 交给驱动，保证真正的结构变更不会被跳过。
func postgresColumnMatchesField(desiredType string, field *schema.Field, column gorm.ColumnType, uniqueIndexExists bool) bool {
	if field.PrimaryKey || field.AutoIncrement {
		return false
	}
	if !postgresTypeMatches(desiredType, column) {
		return false
	}
	if nullable, ok := column.Nullable(); !ok || nullable == field.NotNull {
		return false
	}
	if field.Unique {
		if unique, _ := column.Unique(); !unique && !uniqueIndexExists {
			return false
		}
	}
	return postgresDefaultMatches(field, column)
}

func postgresTypeMatches(desiredType string, column gorm.ColumnType) bool {
	desired := strings.ToLower(strings.TrimSpace(desiredType))
	actual := strings.ToLower(column.DatabaseTypeName())
	switch actual {
	case "bpchar", "character":
		actual = "char"
	case "character varying":
		actual = "varchar"
	}
	if matches := postgresSizedTypePattern.FindStringSubmatch(desired); matches != nil {
		length, ok := column.Length()
		return ok && actual == matches[1] && fmt.Sprint(length) == matches[2]
	}
	return desired == actual
}

var postgresDefaultCastPattern = regexp.MustCompile(`^(.*?)(::[a-z ]+(\(\d+\))?)+$`)

// postgresDefaultMatches 比较默认值：库里读回来的形如 ”::character varying、'abc'::text、0.000000，
// 字段定义里是去掉引号的 ""、abc、0。
func postgresDefaultMatches(field *schema.Field, column gorm.ColumnType) bool {
	actual, hasDefault := column.DefaultValue()
	if !field.HasDefaultValue || !hasDefault {
		return field.HasDefaultValue == hasDefault
	}
	actual = normalizePostgresDefault(actual)
	desired := normalizePostgresDefault(field.DefaultValue)
	if actual == desired {
		return true
	}
	actualNumber, actualErr := strconv.ParseFloat(actual, 64)
	desiredNumber, desiredErr := strconv.ParseFloat(desired, 64)
	return actualErr == nil && desiredErr == nil && actualNumber == desiredNumber
}

func normalizePostgresDefault(value string) string {
	value = strings.TrimSpace(value)
	if matches := postgresDefaultCastPattern.FindStringSubmatch(value); matches != nil {
		value = matches[1]
	}
	if len(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		value = value[1 : len(value)-1]
	}
	return value
}
