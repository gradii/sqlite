package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/gradii/framework/contracts/database"
	query "github.com/gradii/framework/database/query"
	"github.com/gradii/framework/database/query/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryGrammarPreservesExplicitIdentifiers(t *testing.T) {
	builder := query.NewBuilder(NewQueryGrammar("tenant_")).From("InventoryEntry", "").Select("SKUCode", "DisplayLabel").Where("SKUCode", "sku-17")
	statement, bindings := builder.ToSelectSQL()
	require.NoError(t, builder.Error())
	assert.Equal(t, `select "SKUCode", "DisplayLabel" from "tenant_InventoryEntry" where "SKUCode" = ?`, statement)
	assert.Equal(t, []any{"sku-17"}, bindings)
}

// These cases also exercise the query runtime for the JSON cases in GrammarSuite.
func TestQueryGrammarCompileJsonColumnsUpdate(t *testing.T) {
	tests := []struct {
		name     string
		values   map[string]any
		bindings []any
		hasError bool
	}{
		{
			name:     "invalid values",
			values:   map[string]any{"data->invalid": map[string]any{"value": func() {}}},
			hasError: true,
		},
		{
			name:     "update single json column",
			values:   map[string]any{"data->details": "details value"},
			bindings: []any{`{"details":"details value"}`, 17},
		},
		{
			name:     "update multiple json columns",
			values:   map[string]any{"data->details": "details value", "data->info": "info value"},
			bindings: []any{`{"details":"details value","info":"info value"}`, 17},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := query.NewBuilder(NewQueryGrammar("goravel_")).From("users", "").Where("id", 17)
			statement, bindings := builder.ToUpdateSQL(tt.values)
			if tt.hasError {
				require.Error(t, builder.Error())
				return
			}
			require.NoError(t, builder.Error())
			assert.Equal(t, `update "goravel_users" set "data" = json_patch(ifnull("data", json('{}')), json(?)) where "id" = ?`, statement)
			assert.Equal(t, tt.bindings, bindings)
		})
	}
}

func TestQueryGrammarCompileJsonColumnsUpdateNestedArray(t *testing.T) {
	connector, err := NewConnector(database.Config{Dsn: ":memory:"})
	require.NoError(t, err)
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	ctx := context.Background()
	_, err = db.ExecContext(ctx, `create table users (id integer primary key, data text)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `insert into users (id, data) values (?, ?)`, 17, `{"details":{"subdetails":["before","preserved"],"other":true}}`)
	require.NoError(t, err)

	builder := query.NewBuilder(NewQueryGrammar("")).From("users", "").Where("id", 17)
	statement, bindings := builder.ToUpdateSQL(map[string]any{"data->details->subdetails[0]": "subdetails value"})
	require.NoError(t, builder.Error())
	_, err = db.ExecContext(ctx, statement, bindings...)
	require.NoError(t, err)
	var document string
	require.NoError(t, db.QueryRowContext(ctx, `select data from users where id = ?`, 17).Scan(&document))
	assert.JSONEq(t, `{"details":{"subdetails":["subdetails value","preserved"],"other":true}}`, document)
}

func TestQueryGrammarCompileJsonContains(t *testing.T) {
	tests := []struct {
		name     string
		column   string
		value    any
		not      bool
		sql      string
		bindings []any
	}{
		{"single path with single value", "data->details", "value1", false,
			`exists (select 1 from json_each("data", '$."details"') where "json_each"."value" is ?)`, []any{"value1"}},
		{"single path with multiple values", "data->details", []string{"value1", "value2"}, false,
			`exists (select 1 from json_each("data", '$."details"') where "json_each"."value" is ?) and exists (select 1 from json_each("data", '$."details"') where "json_each"."value" is ?)`, []any{"value1", "value2"}},
		{"nested path with single value", "data->details->subdetails[0]", "value1", false,
			`exists (select 1 from json_each("data", '$."details"."subdetails"[0]') where "json_each"."value" is ?)`, []any{"value1"}},
		{"nested path with multiple values", "data->details[0]->subdetails", []string{"value1", "value2"}, false,
			`exists (select 1 from json_each("data", '$."details"[0]."subdetails"') where "json_each"."value" is ?) and exists (select 1 from json_each("data", '$."details"[0]."subdetails"') where "json_each"."value" is ?)`, []any{"value1", "value2"}},
		{"with is not condition", "data->details", "value1", true,
			`not exists (select 1 from json_each("data", '$."details"') where "json_each"."value" is ?)`, []any{"value1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := query.NewBuilder(NewQueryGrammar("goravel_")).From("users", "").WhereJsonContains(tt.column, tt.value, core.BooleanAnd, tt.not)
			statement, bindings := builder.ToSelectSQL()
			require.NoError(t, builder.Error())
			assert.Equal(t, `select * from "goravel_users" where `+tt.sql, statement)
			assert.Equal(t, tt.bindings, bindings)
		})
	}
}

func TestQueryGrammarCompileJsonContainsKey(t *testing.T) {
	tests := []struct {
		name   string
		column string
		not    bool
		sql    string
	}{
		{"single path", "data->details", false, `json_type("data", '$."details"') is not null`},
		{"single path with is not", "data->details", true, `not json_type("data", '$."details"') is not null`},
		{"nested path", "data->details->subdetails", false, `json_type("data", '$."details"."subdetails"') is not null`},
		{"nested path with array index", "data->details[0]->subdetails", false, `json_type("data", '$."details"[0]."subdetails"') is not null`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := query.NewBuilder(NewQueryGrammar("goravel_")).From("users", "").WhereJsonContainsKey(tt.column, core.BooleanAnd, tt.not)
			statement, bindings := builder.ToSelectSQL()
			require.NoError(t, builder.Error())
			assert.Equal(t, `select * from "goravel_users" where `+tt.sql, statement)
			assert.Empty(t, bindings)
		})
	}
}

func TestQueryGrammarCompileJsonLength(t *testing.T) {
	tests := []struct {
		name   string
		column string
		sql    string
	}{
		{"single path", "data->details", `json_array_length("data", '$."details"') > ?`},
		{"nested path", "data->details->subdetails", `json_array_length("data", '$."details"."subdetails"') > ?`},
		{"nested path with array index", "data->details[0]->subdetails", `json_array_length("data", '$."details"[0]."subdetails"') > ?`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := query.NewBuilder(NewQueryGrammar("goravel_")).From("users", "").WhereJsonLength(tt.column, ">", 2, core.BooleanAnd)
			statement, bindings := builder.ToSelectSQL()
			require.NoError(t, builder.Error())
			assert.Equal(t, `select * from "goravel_users" where `+tt.sql, statement)
			assert.Equal(t, []any{2}, bindings)
		})
	}
}
