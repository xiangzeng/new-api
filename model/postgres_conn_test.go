package model

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

func pgColumn(typeName string, length int64, nullable bool, defaultValue *string) migrator.ColumnType {
	column := migrator.ColumnType{
		DataTypeValue:   sql.NullString{String: typeName, Valid: true},
		LengthValue:     sql.NullInt64{Int64: length, Valid: length > 0},
		NullableValue:   sql.NullBool{Bool: nullable, Valid: true},
		UniqueValue:     sql.NullBool{Valid: true},
		PrimaryKeyValue: sql.NullBool{Valid: true},
	}
	if defaultValue != nil {
		column.DefaultValueValue = sql.NullString{String: *defaultValue, Valid: true}
	}
	return column
}

func strPtr(value string) *string { return &value }

// 库里的列已经符合定义时不能再 ALTER（否则每次启动都对 users/logs/tokens 等表加排它锁），
// 真正不一致时必须交给驱动去改。
func TestPostgresColumnMatchesField(t *testing.T) {
	tests := []struct {
		name              string
		desiredType       string
		field             schema.Field
		column            migrator.ColumnType
		uniqueIndexExists bool
		want              bool
	}{
		{
			name:              "varchar with existing unique index",
			desiredType:       "varchar(32)",
			field:             schema.Field{Unique: true},
			column:            pgColumn("varchar", 32, true, nil),
			uniqueIndexExists: true,
			want:              true,
		},
		{
			name:              "char stored as bpchar",
			desiredType:       "char(32)",
			field:             schema.Field{Unique: true},
			column:            pgColumn("bpchar", 32, true, nil),
			uniqueIndexExists: true,
			want:              true,
		},
		{
			name:        "empty string default read back with cast",
			desiredType: "varchar(64)",
			field:       schema.Field{HasDefaultValue: true, DefaultValue: "", DefaultValueInterface: ""},
			column:      pgColumn("varchar", 64, true, strPtr("''::character varying")),
			want:        true,
		},
		{
			name:        "text default read back with cast",
			desiredType: "text",
			field:       schema.Field{HasDefaultValue: true, DefaultValue: "", DefaultValueInterface: ""},
			column:      pgColumn("text", 0, true, strPtr("''::text")),
			want:        true,
		},
		{
			name:        "length differs",
			desiredType: "varchar(32)",
			column:      pgColumn("varchar", 16, true, nil),
			want:        false,
		},
		{
			name:        "type differs",
			desiredType: "text",
			column:      pgColumn("varchar", 64, true, nil),
			want:        false,
		},
		{
			name:        "default value differs",
			desiredType: "text",
			field:       schema.Field{HasDefaultValue: true, DefaultValue: "b", DefaultValueInterface: "b"},
			column:      pgColumn("text", 0, true, strPtr("'a'::text")),
			want:        false,
		},
		{
			name:        "default missing in database",
			desiredType: "text",
			field:       schema.Field{HasDefaultValue: true, DefaultValue: "", DefaultValueInterface: ""},
			column:      pgColumn("text", 0, true, nil),
			want:        false,
		},
		{
			name:        "nullability differs",
			desiredType: "varchar(32)",
			field:       schema.Field{NotNull: true},
			column:      pgColumn("varchar", 32, true, nil),
			want:        false,
		},
		{
			name:        "unique required but no unique index",
			desiredType: "varchar(32)",
			field:       schema.Field{Unique: true},
			column:      pgColumn("varchar", 32, true, nil),
			want:        false,
		},
		{
			name:        "primary key always left to driver",
			desiredType: "bigint",
			field:       schema.Field{PrimaryKey: true},
			column:      pgColumn("bigint", 0, false, nil),
			want:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := tt.field
			assert.Equal(t, tt.want, postgresColumnMatchesField(tt.desiredType, &field, tt.column, tt.uniqueIndexExists))
		})
	}
}

func TestIsPostgresLockTimeout(t *testing.T) {
	assert.True(t, isPostgresLockTimeout(fmt.Errorf("migrate users: %w", &pgconn.PgError{Code: "55P03"})))
	assert.False(t, isPostgresLockTimeout(&pgconn.PgError{Code: "42P01"}))
	assert.False(t, isPostgresLockTimeout(errors.New("connection refused")))
}
