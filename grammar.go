package sqlite

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/contracts/log"
	"github.com/gradii/framework/database/query/core"
	"github.com/gradii/framework/database/query/grammars"
	"github.com/gradii/framework/database/schema"
	"github.com/gradii/framework/support/collect"
	"github.com/spf13/cast"
)

var _ driver.Grammar = &SchemaGrammar{}

type SchemaGrammar struct {
	attributeCommands []string
	log               log.Log
	modifiers         []func(driver.Blueprint, driver.ColumnDefinition) string
	prefix            string
	serials           []string
	wrap              *schema.Wrap
}

func NewSchemaGrammar(log log.Log, prefix string) *SchemaGrammar {
	grammar := &SchemaGrammar{
		attributeCommands: []string{},
		log:               log,
		prefix:            prefix,
		serials:           []string{"bigInteger", "integer", "mediumInteger", "smallInteger", "tinyInteger"},
		wrap:              schema.NewWrap(prefix),
	}
	grammar.modifiers = []func(driver.Blueprint, driver.ColumnDefinition) string{
		grammar.ModifyDefault,
		grammar.ModifyIncrement,
		grammar.ModifyNullable,
	}

	return grammar
}
func (r *SchemaGrammar) CompileAdd(blueprint driver.Blueprint, command *driver.Command) string {
	return fmt.Sprintf("alter table %s add column %s", r.wrap.Table(blueprint.GetTableName()), r.getColumn(blueprint, command.Column))
}

func (r *SchemaGrammar) CompileChange(blueprint driver.Blueprint, command *driver.Command) []string {
	return nil
}

func (r *SchemaGrammar) CompileColumns(_, table string) (string, error) {
	table = r.prefix + table

	return fmt.Sprintf(
		`select name, type, not "notnull" as "nullable", dflt_value as "default", pk as "primary", hidden as "extra" `+
			"from pragma_table_xinfo(%s) order by cid asc", r.wrap.Quote(strings.ReplaceAll(table, ".", "__"))), nil
}

func (r *SchemaGrammar) CompileDefault(_ driver.Blueprint, _ *driver.Command) string {
	return ""
}

func (r *SchemaGrammar) CompileComment(blueprint driver.Blueprint, command *driver.Command) string {
	return ""
}

func (r *SchemaGrammar) CompileCreate(blueprint driver.Blueprint) string {
	return fmt.Sprintf("create table %s (%s%s%s)",
		r.wrap.Table(blueprint.GetTableName()),
		strings.Join(r.getColumns(blueprint), ", "),
		r.addForeignKeys(getCommandsByName(blueprint.GetCommands(), "foreign")),
		r.addPrimaryKeys(getCommandByName(blueprint.GetCommands(), "primary")))
}

func (r *SchemaGrammar) CompileDisableWriteableSchema() string {
	return r.pragma("writable_schema", "0")
}

func (r *SchemaGrammar) CompileDrop(blueprint driver.Blueprint) string {
	return fmt.Sprintf("drop table %s", r.wrap.Table(blueprint.GetTableName()))
}

func (r *SchemaGrammar) CompileDropAllDomains(domains []string) string {
	return ""
}

func (r *SchemaGrammar) CompileDropAllTables(_ string, _ []driver.Table) []string {
	return []string{
		r.CompileEnableWriteableSchema(),
		"delete from sqlite_master where type in ('table', 'index', 'trigger')",
		r.CompileDisableWriteableSchema(),
	}
}

func (r *SchemaGrammar) CompileDropAllTypes(_ string, _ []driver.Type) []string {
	return nil
}

func (r *SchemaGrammar) CompileDropAllViews(_ string, _ []driver.View) []string {
	return []string{
		r.CompileEnableWriteableSchema(),
		"delete from sqlite_master where type in ('view')",
		r.CompileDisableWriteableSchema(),
	}
}

func (r *SchemaGrammar) CompileDropColumn(blueprint driver.Blueprint, command *driver.Command) []string {
	// TODO check Sqlite 3.35
	table := r.wrap.Table(blueprint.GetTableName())
	columns := r.wrap.PrefixArray("drop column", r.wrap.Columns(command.Columns))

	return collect.Map(columns, func(column string, _ int) string {
		return fmt.Sprintf("alter table %s %s", table, column)
	})
}

func (r *SchemaGrammar) CompileDropForeign(_ driver.Blueprint, _ *driver.Command) string {
	return ""
}

func (r *SchemaGrammar) CompileDropFullText(_ driver.Blueprint, _ *driver.Command) string {
	return ""
}

func (r *SchemaGrammar) CompileDropIfExists(blueprint driver.Blueprint) string {
	return fmt.Sprintf("drop table if exists %s", r.wrap.Table(blueprint.GetTableName()))
}

func (r *SchemaGrammar) CompileDropIndex(blueprint driver.Blueprint, command *driver.Command) string {
	index := r.wrap.Value(command.Index)
	if namespace, _, qualified := strings.Cut(blueprint.GetTableName(), "."); qualified {
		index = r.wrap.Value(namespace) + "." + index
	}
	return "drop index " + index
}

func (r *SchemaGrammar) CompileDropPrimary(_ driver.Blueprint, _ *driver.Command) string {
	return ""
}

func (r *SchemaGrammar) CompileDropUnique(blueprint driver.Blueprint, command *driver.Command) string {
	return r.CompileDropIndex(blueprint, command)
}

func (r *SchemaGrammar) CompileEnableWriteableSchema() string {
	return r.pragma("writable_schema", "1")
}

func (r *SchemaGrammar) CompileForeign(_ driver.Blueprint, _ *driver.Command) string {
	return ""
}

func (r *SchemaGrammar) CompileForeignKeys(_, table string) string {
	return fmt.Sprintf(
		`SELECT 
			GROUP_CONCAT("from") AS columns, 
			"table" AS foreign_table, 
			GROUP_CONCAT("to") AS foreign_columns, 
			on_update, 
			on_delete 
		FROM (
			SELECT * FROM pragma_foreign_key_list(%s) 
			ORDER BY id DESC, seq
		) 
		GROUP BY id, "table", on_update, on_delete`,
		r.wrap.Quote(strings.ReplaceAll(table, ".", "__")),
	)
}

func (r *SchemaGrammar) CompileFullText(_ driver.Blueprint, _ *driver.Command) string {
	return ""
}

func (r *SchemaGrammar) CompileIndex(blueprint driver.Blueprint, command *driver.Command) string {
	return r.compileSchemaIndex(blueprint, command, false)
}

func (r *SchemaGrammar) compileSchemaIndex(blueprint driver.Blueprint, command *driver.Command, unique bool) string {
	namespace, table, qualified := strings.Cut(blueprint.GetTableName(), ".")
	index := r.wrap.Value(command.Index)
	if qualified {
		index = r.wrap.Value(namespace) + "." + index
	} else {
		table = namespace
	}
	kind := ""
	if unique {
		kind = "unique "
	}
	columns := r.wrap.Columnize(command.Columns)
	if command.Name == "rawIndex" {
		columns = command.Value
	}
	return "create " + kind + "index " + index + " on " + r.wrap.Table(table) + " (" + columns + ")"
}

func (r *SchemaGrammar) CompileIndexes(_, table string) (string, error) {
	table = r.prefix + table
	quotedTable := r.wrap.Quote(strings.ReplaceAll(table, ".", "__"))

	return fmt.Sprintf(
		`select 'primary' as name, group_concat(col) as columns, 1 as "unique", 1 as "primary" `+
			`from (select name as col from pragma_table_info(%s) where pk > 0 order by pk, cid) group by name `+
			`union select name, group_concat(col) as columns, "unique", origin = 'pk' as "primary" `+
			`from (select il.*, ii.name as col from pragma_index_list(%s) il, pragma_index_info(il.name) ii order by il.seq, ii.seqno) `+
			`group by name, "unique", "primary"`,
		quotedTable,
		r.wrap.Quote(table),
	), nil
}

func (r *SchemaGrammar) CompileJsonColumnsUpdate(values map[string]any) (map[string]any, error) {
	var (
		compiled = make(map[string]any)
		json     = App.GetJson()
	)
	for key, value := range values {
		if strings.Contains(key, "->") {
			segments := strings.SplitN(key, "->", 2)
			column, path := segments[0], strings.Trim(r.wrap.JsonPath(segments[1]), "'")

			val := reflect.ValueOf(value)
			if val.Kind() == reflect.Pointer {
				val = val.Elem()
			}

			if kind := val.Kind(); kind == reflect.Slice || kind == reflect.Array || kind == reflect.Map || kind == reflect.Struct || kind == reflect.Bool {
				binding, err := json.Marshal(value)
				if err != nil {
					return nil, err
				}
				value = legacyRaw("json(?)", string(binding))
			}

			expr, ok := compiled[column]
			if !ok {
				expr = legacyRaw(r.wrap.Column(column))
			}

			compiled[column] = legacyRaw("json_set(?,?,?)", expr, path, value)

			continue
		}

		compiled[key] = value
	}

	return compiled, nil
}

func (r *SchemaGrammar) CompileJsonContains(column string, value any, isNot bool) (string, []any, error) {
	field, path := r.wrap.JsonFieldAndPath(column)
	query := r.wrap.Not(fmt.Sprintf("exists (select 1 from json_each(json_extract(%s%s)) where value = ? )", field, path), isNot)

	if val := reflect.ValueOf(value); val.Kind() == reflect.Slice || val.Kind() == reflect.Array {
		values := make([]any, val.Len())
		queries := make([]string, val.Len())
		for i := 0; i < val.Len(); i++ {
			values[i] = val.Index(i).Interface()
			queries[i] = query
		}

		return strings.Join(queries, " AND "), values, nil
	}

	return query, []any{value}, nil
}

func (r *SchemaGrammar) CompileJsonContainsKey(column string, isNot bool) string {
	field, path := r.wrap.JsonFieldAndPath(column)

	return r.wrap.Not(fmt.Sprintf("json_type(%s%s) is not null", field, path), isNot)
}

func (r *SchemaGrammar) CompileJsonLength(column string) string {
	field, path := r.wrap.JsonFieldAndPath(column)

	return fmt.Sprintf("json_array_length(%s%s)", field, path)
}

func (r *SchemaGrammar) CompileJsonSelector(column string) string {
	field, path := r.wrap.JsonFieldAndPath(column)

	return fmt.Sprintf("json_extract(%s%s)", field, path)
}

func (r *SchemaGrammar) CompileJsonValues(args ...any) []any {

	return args
}

func (r *SchemaGrammar) CompileLockForUpdate(builder sq.SelectBuilder, conditions *driver.Conditions) sq.SelectBuilder {
	return builder
}

func (r *SchemaGrammar) CompilePlaceholderFormat() driver.PlaceholderFormat {
	return nil
}

func (r *SchemaGrammar) CompilePrimary(_ driver.Blueprint, _ *driver.Command) string {
	return ""
}

func (r *SchemaGrammar) CompileInRandomOrder(builder sq.SelectBuilder, conditions *driver.Conditions) sq.SelectBuilder {
	if conditions.InRandomOrder != nil && *conditions.InRandomOrder {
		conditions.OrderBy = []string{"RANDOM()"}
	}

	return builder
}

func (r *SchemaGrammar) CompilePrune(_ string) string {
	return "vacuum"
}

func (r *SchemaGrammar) CompileRename(blueprint driver.Blueprint, command *driver.Command) string {
	return fmt.Sprintf("alter table %s rename to %s", r.wrap.Table(blueprint.GetTableName()), r.wrap.Table(command.To))
}

func (r *SchemaGrammar) CompileRenameColumn(blueprint driver.Blueprint, command *driver.Command, _ []driver.Column) (string, error) {
	return fmt.Sprintf("alter table %s rename column %s to %s",
		r.wrap.Table(blueprint.GetTableName()),
		r.wrap.Column(command.From),
		r.wrap.Column(command.To),
	), nil
}

func (r *SchemaGrammar) CompileRenameIndex(blueprint driver.Blueprint, command *driver.Command, indexes []driver.Index) []string {
	indexes = collect.Filter(indexes, func(index driver.Index, _ int) bool {
		return index.Name == command.From
	})

	if len(indexes) == 0 {
		r.log.Warningf("index %s does not exist", command.From)
		return nil
	}
	if indexes[0].Primary {
		r.log.Warning("SQLite does not support altering primary keys")
		return nil
	}
	if indexes[0].Unique {
		return []string{
			r.CompileDropUnique(blueprint, &driver.Command{
				Index: indexes[0].Name,
			}),
			r.CompileUnique(blueprint, &driver.Command{
				Index:   command.To,
				Columns: indexes[0].Columns,
			}),
		}
	}

	return []string{
		r.CompileDropIndex(blueprint, &driver.Command{
			Index: indexes[0].Name,
		}),
		r.CompileIndex(blueprint, &driver.Command{
			Index:   command.To,
			Columns: indexes[0].Columns,
		}),
	}
}

func (r *SchemaGrammar) CompileSharedLock(builder sq.SelectBuilder, conditions *driver.Conditions) sq.SelectBuilder {
	return builder
}

func (r *SchemaGrammar) CompileTables(database string) string {
	return "select name from sqlite_master where type = 'table' and name not like 'sqlite_%' order by name"
}

func (r *SchemaGrammar) CompileTableComment(_ driver.Blueprint, _ *driver.Command) string {
	return ""
}

func (r *SchemaGrammar) CompileTypes() string {
	return ""
}

func (r *SchemaGrammar) CompileUnique(blueprint driver.Blueprint, command *driver.Command) string {
	return r.compileSchemaIndex(blueprint, command, true)
}

func (r *SchemaGrammar) CompileVersion() string {
	return "SELECT sqlite_version() AS value;"
}

func (r *SchemaGrammar) CompileViews(database string) string {
	return "select name, sql as definition from sqlite_master where type = 'view' order by name"
}

func (r *SchemaGrammar) GetAttributeCommands() []string {
	return r.attributeCommands
}

func (r *SchemaGrammar) GetModifiers() []func(blueprint driver.Blueprint, column driver.ColumnDefinition) string {
	return r.modifiers
}

func (r *SchemaGrammar) ModifyDefault(blueprint driver.Blueprint, column driver.ColumnDefinition) string {
	if column.GetDefault() != nil {
		return fmt.Sprintf(" default %s", schema.ColumnDefaultValue(column.GetDefault()))
	}

	return ""
}

func (r *SchemaGrammar) ModifyNullable(blueprint driver.Blueprint, column driver.ColumnDefinition) string {
	if column.GetNullable() {
		return " null"
	} else {
		return " not null"
	}
}

func (r *SchemaGrammar) ModifyIncrement(blueprint driver.Blueprint, column driver.ColumnDefinition) string {
	if slices.Contains(r.serials, column.GetType()) && column.GetAutoIncrement() {
		return " primary key autoincrement"
	}

	return ""
}

func (r *SchemaGrammar) TypeBigInteger(column driver.ColumnDefinition) string {
	return "integer"
}

func (r *SchemaGrammar) TypeBoolean(_ driver.ColumnDefinition) string {
	return "tinyint(1)"
}

func (r *SchemaGrammar) TypeChar(column driver.ColumnDefinition) string {
	return "varchar"
}

func (r *SchemaGrammar) TypeDate(column driver.ColumnDefinition) string {
	return "date"
}

func (r *SchemaGrammar) TypeDateTime(column driver.ColumnDefinition) string {
	return r.TypeTimestamp(column)
}

func (r *SchemaGrammar) TypeDateTimeTz(column driver.ColumnDefinition) string {
	return r.TypeDateTime(column)
}

func (r *SchemaGrammar) TypeDecimal(column driver.ColumnDefinition) string {
	return "numeric"
}

func (r *SchemaGrammar) TypeDouble(column driver.ColumnDefinition) string {
	return "double"
}

func (r *SchemaGrammar) TypeEnum(column driver.ColumnDefinition) string {
	return fmt.Sprintf(`varchar check ("%s" in (%s))`, column.GetName(), strings.Join(r.wrap.Quotes(cast.ToStringSlice(column.GetAllowed())), ", "))
}

func (r *SchemaGrammar) TypeFloat(column driver.ColumnDefinition) string {
	return "float"
}

func (r *SchemaGrammar) TypeInteger(column driver.ColumnDefinition) string {
	return "integer"
}

func (r *SchemaGrammar) TypeJson(column driver.ColumnDefinition) string {
	return "text"
}

func (r *SchemaGrammar) TypeJsonb(column driver.ColumnDefinition) string {
	return "text"
}

func (r *SchemaGrammar) TypeLongText(column driver.ColumnDefinition) string {
	return "text"
}

func (r *SchemaGrammar) TypeMediumInteger(column driver.ColumnDefinition) string {
	return "integer"
}

func (r *SchemaGrammar) TypeMediumText(column driver.ColumnDefinition) string {
	return "text"
}

func (r *SchemaGrammar) TypeSmallInteger(column driver.ColumnDefinition) string {
	return "integer"
}

func (r *SchemaGrammar) TypeString(column driver.ColumnDefinition) string {
	return "varchar"
}

func (r *SchemaGrammar) TypeText(column driver.ColumnDefinition) string {
	return "text"
}

func (r *SchemaGrammar) TypeTime(column driver.ColumnDefinition) string {
	return "time"
}

func (r *SchemaGrammar) TypeTimeTz(column driver.ColumnDefinition) string {
	return r.TypeTime(column)
}

func (r *SchemaGrammar) TypeTimestamp(column driver.ColumnDefinition) string {
	if column.GetUseCurrent() {
		column.Default(schema.Expression("CURRENT_TIMESTAMP"))
	}

	return "datetime"
}

func (r *SchemaGrammar) TypeTimestampTz(column driver.ColumnDefinition) string {
	return r.TypeTimestamp(column)
}

func (r *SchemaGrammar) TypeTinyInteger(column driver.ColumnDefinition) string {
	return "integer"
}

func (r *SchemaGrammar) TypeTinyText(column driver.ColumnDefinition) string {
	return "text"
}

func (r *SchemaGrammar) TypeUuid(column driver.ColumnDefinition) string {
	return "varchar"
}

func (r *SchemaGrammar) addForeignKeys(commands []*driver.Command) string {
	var sql string

	for _, command := range commands {
		sql += r.getForeignKey(command)
	}

	return sql
}

func (r *SchemaGrammar) addPrimaryKeys(command *driver.Command) string {
	if command == nil {
		return ""
	}

	return fmt.Sprintf(", primary key (%s)", r.wrap.Columnize(command.Columns))
}

func (r *SchemaGrammar) getColumns(blueprint driver.Blueprint) []string {
	var columns []string
	for _, column := range blueprint.GetAddedColumns() {
		columns = append(columns, r.getColumn(blueprint, column))
	}

	return columns
}

func (r *SchemaGrammar) getColumn(blueprint driver.Blueprint, column driver.ColumnDefinition) string {
	sql := fmt.Sprintf("%s %s", r.wrap.Column(column.GetName()), schema.ColumnType(r, column))

	for _, modifier := range r.modifiers {
		sql += modifier(blueprint, column)
	}

	return sql
}

func (r *SchemaGrammar) getForeignKey(command *driver.Command) string {
	sql := fmt.Sprintf(", foreign key(%s) references %s(%s)",
		r.wrap.Columnize(command.Columns),
		r.wrap.Table(command.On),
		r.wrap.Columnize(command.References))

	if command.OnDelete != "" {
		sql += " on delete " + command.OnDelete
	}
	if command.OnUpdate != "" {
		sql += " on update " + command.OnUpdate
	}

	return sql
}

func (r *SchemaGrammar) pragma(name, value string) string {
	return fmt.Sprintf("pragma %s = %s", name, value)
}

func getCommandByName(commands []*driver.Command, name string) *driver.Command {
	commands = getCommandsByName(commands, name)
	if len(commands) == 0 {
		return nil
	}

	return commands[0]
}

func getCommandsByName(commands []*driver.Command, name string) []*driver.Command {
	var filteredCommands []*driver.Command
	for _, command := range commands {
		if command.Name == name {
			filteredCommands = append(filteredCommands, command)
		}
	}

	return filteredCommands
}

type Grammar = SchemaGrammar

func NewGrammar(log log.Log, prefix string) *SchemaGrammar { return NewSchemaGrammar(log, prefix) }
func legacyRaw(sql string, args ...any) any {
	return core.Raw(grammars.NewBaseGrammar("").SubstituteBindingsIntoRawSQL(sql, args))
}
