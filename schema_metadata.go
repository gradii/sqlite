package sqlite

import (
	"fmt"
	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/database/query/processors"
	"github.com/gradii/framework/errors"
)

func (r *SchemaGrammar) CurrentSchemas(string) []string { return []string{"main"} }
func (r *SchemaGrammar) ResolveCurrentSchemas(driver.SchemaConnectionInfo, driver.SchemaCatalog) ([]string, error) {
	return []string{"main"}, nil
}
func (r *SchemaGrammar) CompileSchemaMetadata(query driver.SchemaMetadataQuery) (driver.SchemaStatement, error) {
	namespace := sqliteSchemaName(query.Schema)
	table := query.Table
	var sql string
	var bindings []any
	switch query.Kind {
	case "schemas":
		sql = `select name, file as path, name = 'main' as "default" from pragma_database_list order by name`
	case "columns":
		sql = `select name, type, not "notnull" as "nullable", dflt_value as "default", pk as "primary", hidden as "extra" from pragma_table_xinfo(?, ?) order by cid asc`
		bindings = []any{table, namespace}
	case "indexes":
		sql = `select 'primary' as name, group_concat(col) as columns, 1 as "unique", 1 as "primary" from (select name as col from pragma_table_xinfo(?, ?) where pk > 0 order by pk, cid) group by name union select name, group_concat(col) as columns, "unique", origin = 'pk' as "primary" from (select il.*, ii.name as col from pragma_index_list(?, ?) il, pragma_index_info(il.name, ?) ii order by il.seq, ii.seqno) group by name, "unique", "primary"`
		bindings = []any{table, namespace, table, namespace, namespace}
	case "foreignKeys":
		sql = `select group_concat("from") as columns, ? as foreign_schema, "table" as foreign_table, group_concat("to") as foreign_columns, on_update, on_delete from (select * from pragma_foreign_key_list(?, ?) order by id desc, seq) group by id, "table", on_update, on_delete`
		bindings = []any{namespace, table, namespace}
	case "hasTable":
		sql = `select exists (select 1 from ` + quoteSchemaIdentifier(namespace) + `.sqlite_master where name = ? and type = 'table') as "exists"`
		bindings = []any{table}
	case "types":
		return driver.SchemaStatement{}, nil
	default:
		return driver.SchemaStatement{}, errors.SchemaMetadataNotSupported.Args(query.Kind, Name)
	}
	return driver.SchemaStatement{SQL: sql, Bindings: bindings}, nil
}
func (r *SchemaGrammar) ResolveSchemaMetadata(query driver.SchemaMetadataQuery, catalog driver.SchemaCatalog) (driver.SchemaMetadataResult, error) {
	if query.Kind == "tables" || query.Kind == "views" {
		return r.resolveRelations(query, catalog)
	}
	statement, err := r.CompileSchemaMetadata(query)
	if err != nil {
		return driver.SchemaMetadataResult{}, err
	}
	if statement.SQL == "" {
		return driver.SchemaMetadataResult{}, nil
	}
	rows, err := catalog.Select(statement)
	if err != nil {
		return driver.SchemaMetadataResult{}, err
	}
	result := driver.SchemaMetadataResult{Rows: rows}
	if query.Kind == "columns" {
		definitions, err := catalog.Select(driver.SchemaStatement{SQL: "select sql from " + quoteSchemaIdentifier(sqliteSchemaName(query.Schema)) + ".sqlite_master where type = 'table' and name = ?", Bindings: []any{query.Table}})
		if err != nil {
			return driver.SchemaMetadataResult{}, err
		}
		if len(definitions) > 0 {
			result.ColumnSQL = []string{processors.MetadataString(definitions[0]["sql"])}
		}
	}
	return result, nil
}
func (r *SchemaGrammar) resolveRelations(query driver.SchemaMetadataQuery, catalog driver.SchemaCatalog) (driver.SchemaMetadataResult, error) {
	names := query.Schemas
	if len(names) == 0 {
		statement, _ := r.CompileSchemaMetadata(driver.SchemaMetadataQuery{Kind: "schemas"})
		schemas, err := catalog.Select(statement)
		if err != nil {
			return driver.SchemaMetadataResult{}, err
		}
		for _, row := range schemas {
			names = append(names, processors.MetadataString(row["name"]))
		}
	}
	modernTables := false
	stats := false
	if query.Kind == "tables" {
		versions, err := catalog.Select(driver.SchemaStatement{SQL: "select sqlite_version() as version"})
		if err != nil {
			return driver.SchemaMetadataResult{}, err
		}
		if len(versions) > 0 {
			var major, minor int
			_, _ = fmt.Sscanf(processors.MetadataString(versions[0]["version"]), "%d.%d", &major, &minor)
			modernTables = major > 3 || major == 3 && minor >= 37
		}

		rows, err := catalog.Select(driver.SchemaStatement{SQL: "select exists (select 1 from pragma_compile_options where compile_options = 'ENABLE_DBSTAT_VTAB') as enabled"})
		if err != nil {
			return driver.SchemaMetadataResult{}, err
		}
		stats = len(rows) > 0 && processors.MetadataBool(rows[0]["enabled"])
	}
	var rows []map[string]any
	for _, name := range names {
		name = sqliteSchemaName(name)
		var statement driver.SchemaStatement
		if query.Kind == "views" {
			statement = driver.SchemaStatement{SQL: "select name, ? as schema, sql as definition from " + quoteSchemaIdentifier(name) + ".sqlite_master where type = 'view' order by name", Bindings: []any{name}}
		} else {
			sql := "select name, ? as schema"
			if stats {
				sql += ", (select sum(pgsize) from dbstat(?) where name = m.name or name in (select name from pragma_index_list(m.name, ?))) as size"
			}
			if modernTables {
				sql += " from pragma_table_list as m where schema = ? and type in ('table', 'virtual') and name not like 'sqlite\\_%' escape '\\' order by name"
			} else {
				sql += " from " + quoteSchemaIdentifier(name) + ".sqlite_master as m where type = 'table' and name not like 'sqlite\\_%' escape '\\' order by name"
			}
			statement = driver.SchemaStatement{SQL: sql, Bindings: []any{name}}
			if stats {
				statement.Bindings = append(statement.Bindings, name, name)
			}
			if modernTables {
				statement.Bindings = append(statement.Bindings, name)
			}
		}
		batch, err := catalog.Select(statement)
		if err != nil {
			return driver.SchemaMetadataResult{}, err
		}
		rows = append(rows, batch...)
	}
	return driver.SchemaMetadataResult{Rows: rows}, nil
}
