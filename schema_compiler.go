package sqlite

import (
	"fmt"
	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/database/schema"
	"github.com/gradii/framework/errors"
	"strings"
)

func describeColumn(column driver.ColumnDefinition) driver.ColumnDescription {
	if readable, ok := column.(driver.ColumnDescriber); ok {
		return readable.DescribeColumn()
	}
	return driver.ColumnDescription{}
}
func describeBlueprint(blueprint driver.Blueprint) driver.BlueprintDescription {
	if readable, ok := blueprint.(driver.BlueprintDescriber); ok {
		return readable.DescribeBlueprint()
	}
	return driver.BlueprintDescription{Table: blueprint.GetTableName(), Columns: blueprint.GetAddedColumns(), Commands: blueprint.GetCommands()}
}
func schemaExpression(value any) (string, error) {
	switch value := value.(type) {
	case nil:
		return "", nil
	case string:
		return value, nil
	case schema.Expression:
		return string(value), nil
	case interface{ SQL() string }:
		return value.SQL(), nil
	case interface{ GetValue() string }:
		return value.GetValue(), nil
	default:
		return "", errors.DatabaseUnsupportedType.Args(fmt.Sprintf("%T", value), "SQL string or expression")
	}
}
func (r *SchemaGrammar) schemaColumn(blueprint driver.Blueprint, column driver.ColumnDefinition) (string, error) {
	options := describeColumn(column)
	var kind string
	switch column.GetType() {
	case "raw":
		kind = options.RawDefinition
	case "binary":
		kind = "blob"
	case "year":
		kind = "integer"
	case "ipAddress":
		kind = "varchar(45)"
	case "macAddress":
		kind = "varchar(17)"
	case "geometry", "geography":
		kind = "geometry"
	case "vector", "set":
		return "", errors.SchemaFeatureNotSupported.Args(Name, column.GetType()+" columns")
	case "computed":
		return "", errors.SchemaComputedRequiresType
	default:
		kind = schema.ColumnType(r, column)
	}
	sql := r.wrap.Value(column.GetName()) + " " + kind
	if options.StoredAs != nil || options.VirtualAs != nil {
		if options.Nullable != nil && !*options.Nullable {
			sql += " not null"
		}
	} else if !column.GetNullable() {
		sql += " not null"
	}
	sql += r.ModifyDefault(blueprint, column)
	for _, generated := range []struct {
		value  any
		stored bool
	}{{options.VirtualAs, false}, {options.StoredAs, true}} {
		if generated.value == nil {
			continue
		}
		expression, err := schemaExpression(generated.value)
		if err != nil {
			return "", err
		}
		sql += " as (" + expression + ")"
		if generated.stored {
			sql += " stored"
		}
	}
	sql += r.ModifyIncrement(blueprint, column)
	if options.Collation != "" {
		sql += " collate " + r.wrap.Value(options.Collation)
	}
	return sql, nil
}
func (r *SchemaGrammar) CompileSchemaCommand(blueprint driver.Blueprint, command *driver.Command) ([]driver.SchemaStatement, bool, error) {
	var sql string
	switch command.Name {
	case schema.CommandCreate:
		options := describeBlueprint(blueprint)
		var columns []string
		for _, column := range options.Columns {
			definition, err := r.schemaColumn(blueprint, column)
			if err != nil {
				return nil, true, err
			}
			columns = append(columns, definition)
		}
		for _, constraint := range blueprint.GetCommands() {
			switch constraint.Name {
			case schema.CommandPrimary:
				columns = append(columns, "primary key ("+r.wrap.Columnize(constraint.Columns)+")")
				constraint.ShouldBeSkipped = true
			case schema.CommandForeign:
				columns = append(columns, r.sqliteForeignDefinition(constraint))
				constraint.ShouldBeSkipped = true
			}
		}
		sql = "create "
		if options.Temporary {
			sql += "temporary "
		}
		sql += "table " + r.wrap.Table(blueprint.GetTableName()) + " (" + strings.Join(columns, ", ") + ")"
	case schema.CommandAdd:
		if command.Column.IsChange() {
			return nil, true, errors.SchemaSQLiteRebuildRequired
		}
		definition, err := r.schemaColumn(blueprint, command.Column)
		if err != nil {
			return nil, true, err
		}
		sql = "alter table " + r.wrap.Table(blueprint.GetTableName()) + " add column " + definition
	case "rawIndex":
		sql = r.compileSchemaIndex(blueprint, command, false)
	case schema.CommandFullText, schema.CommandDropFullText:
		return nil, true, errors.SchemaFeatureNotSupported.Args(Name, "fulltext indexes")
	case "spatialIndex", "dropSpatialIndex":
		return nil, true, errors.SchemaFeatureNotSupported.Args(Name, "spatial indexes")
	default:
		return nil, false, nil
	}
	return []driver.SchemaStatement{{SQL: sql}}, true, nil
}
func (r *SchemaGrammar) CompileSchemaStartingValues(driver.Blueprint) ([]driver.SchemaStatement, error) {
	return nil, nil
}
func (r *SchemaGrammar) sqliteForeignDefinition(command *driver.Command) string {
	sql := "foreign key(" + r.wrap.Columnize(command.Columns) + ") references " + r.wrap.Table(command.On) + "(" + r.wrap.Columnize(command.References) + ")"
	if command.OnDelete != "" {
		sql += " on delete " + command.OnDelete
	}
	if command.OnUpdate != "" {
		sql += " on update " + command.OnUpdate
	}
	return sql
}
func (r *SchemaGrammar) SchemaExecutionPolicy(blueprint driver.Blueprint) driver.SchemaExecutionPolicy {
	return driver.SchemaExecutionPolicy{Transactional: true, DisableForeignKeys: blueprint != nil && sqliteRebuild(blueprint), ForeignKeysOutsideTransaction: true}
}
func (r *SchemaGrammar) CompileSchemaBlueprint(blueprint driver.Blueprint, catalog driver.SchemaCatalog) ([]driver.SchemaStatement, bool, error) {
	if !sqliteRebuild(blueprint) {
		return nil, false, nil
	}
	if catalog == nil {
		return nil, true, errors.DatabaseConnectionIsNil
	}
	sqls, err := r.compileSQLiteRebuild(blueprint, catalog)
	var statements []driver.SchemaStatement
	for _, sql := range sqls {
		statements = append(statements, driver.SchemaStatement{SQL: sql})
	}
	return statements, true, err
}
