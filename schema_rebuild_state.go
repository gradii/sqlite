package sqlite

import (
	"sort"
	"strings"

	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/database/schema"
	sqliteerrors "github.com/gradii/sqlite/errors"
)

// Foreign target identifiers belong to their own table. Only a self-reference
// follows this blueprint's column and table renames.
func sqliteRenameForeign(sql, table, finalTable string, renames map[string]string) string {
	tokens := sqliteTokens(sql)
	for i, token := range tokens {
		if token.quoted || !strings.EqualFold(token.value, "references") || i+1 >= len(tokens) {
			continue
		}
		prefix := sqliteRename(sql[:token.start], renames)
		target := tokens[i+1]
		if !strings.EqualFold(target.value, table) {
			return prefix + sql[token.start:]
		}
		name := sql[target.start:target.end]
		if finalTable != table {
			name = quoteSchemaIdentifier(finalTable)
		}
		tail := sql[target.end:]
		if i+2 < len(tokens) && tokens[i+2].value == "(" {
			for j := i + 3; j < len(tokens); j++ {
				if !tokens[j].quoted && tokens[j].value == ")" {
					tail = sqliteRename(sql[target.end:tokens[j].end], renames) + sql[tokens[j].end:]
					break
				}
			}
		}
		return prefix + sql[token.start:target.start] + name + tail
	}
	return sqliteRename(sql, renames)
}

type sqliteTextChange struct {
	start, end int
	value      string
}

// Column names and local CHECK/generated expressions are writable identifiers;
// types, constraint names, collations and external REFERENCES are not.
func sqliteRenameColumnDefinition(sql, table, finalTable string, renames map[string]string) string {
	tokens := sqliteTokens(sql)
	if len(tokens) == 0 {
		return sql
	}
	var changes []sqliteTextChange
	if name, ok := renames[strings.ToLower(tokens[0].value)]; ok {
		changes = append(changes, sqliteTextChange{tokens[0].start, tokens[0].end, quoteSchemaIdentifier(name)})
	}
	for _, constraint := range sqliteInlineConstraints(sql) {
		if constraint.kind == "references" {
			value := sqliteRenameForeign(sql[constraint.start:constraint.end], table, finalTable, renames)
			changes = append(changes, sqliteTextChange{constraint.start, constraint.end, value})
		}
	}
	depth := 0
	for i := 1; i+1 < len(tokens); i++ {
		token := tokens[i]
		if token.quoted {
			continue
		}
		if token.value == "(" {
			depth++
		} else if token.value == ")" {
			depth--
		}
		if depth != 0 || (!strings.EqualFold(token.value, "check") && !strings.EqualFold(token.value, "as")) || tokens[i+1].value != "(" {
			continue
		}
		level := 1
		for j := i + 2; j < len(tokens); j++ {
			if !tokens[j].quoted {
				if tokens[j].value == "(" {
					level++
				} else if tokens[j].value == ")" {
					level--
				}
			}
			if level == 0 {
				start, end := tokens[i+1].end, tokens[j].start
				changes = append(changes, sqliteTextChange{start, end, sqliteRename(sql[start:end], renames)})
				i = j
				break
			}
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].start < changes[j].start })
	var out strings.Builder
	start := 0
	for _, change := range changes {
		out.WriteString(sql[start:change.start])
		out.WriteString(change.value)
		start = change.end
	}
	out.WriteString(sql[start:])
	return out.String()
}

type sqliteRebuildIndex struct{ name, sql string }

func (r *SchemaGrammar) sqliteRebuildIndexes(blueprint driver.Blueprint, objects []map[string]any, namespace, table string, renames map[string]string, dropped map[string]bool) ([]string, error) {
	var indexes []sqliteRebuildIndex
	for _, object := range objects {
		if object["type"] == "index" {
			name, _ := object["name"].(string)
			sql, _ := object["sql"].(string)
			indexes = append(indexes, sqliteRebuildIndex{name, sql})
		}
	}
	find := func(name string) int {
		for i, index := range indexes {
			if strings.EqualFold(index.name, name) {
				return i
			}
		}
		return -1
	}
	for _, command := range blueprint.GetCommands() {
		switch command.Name {
		case schema.CommandRenameIndex:
			i := find(command.From)
			if i < 0 {
				return nil, sqliteerrors.SchemaIndexNotFound.Args(command.From, blueprint.GetTableName())
			}
			if existing := find(command.To); existing >= 0 && existing != i {
				return nil, sqliteerrors.SchemaIndexAlreadyExists.Args(command.To, blueprint.GetTableName())
			}
			indexes[i].name = command.To
		case schema.CommandDropIndex, schema.CommandDropUnique, schema.CommandDropFullText, "dropSpatialIndex":
			if i := find(command.Index); i >= 0 {
				indexes = append(indexes[:i], indexes[i+1:]...)
			}
		case schema.CommandIndex, schema.CommandUnique, "rawIndex":
			if find(command.Index) >= 0 {
				return nil, sqliteerrors.SchemaIndexAlreadyExists.Args(command.Index, blueprint.GetTableName())
			}
			sql := r.compileSchemaIndex(blueprint, command, command.Name == schema.CommandUnique)
			indexes = append(indexes, sqliteRebuildIndex{command.Index, sql})
		}
	}
	var statements []string
	for _, index := range indexes {
		tokens := sqliteTokens(index.sql)
		for _, token := range tokens {
			if token.quoted || token.value != "(" {
				continue
			}
			if sqliteReferences(index.sql[token.start:], dropped) {
				break
			}
			unique := ""
			if len(tokens) > 1 && strings.EqualFold(tokens[1].value, "unique") {
				unique = "unique "
			}
			statements = append(statements, "create "+unique+"index "+quoteSchemaIdentifier(namespace)+"."+quoteSchemaIdentifier(index.name)+" on "+quoteSchemaIdentifier(table)+" "+sqliteRename(index.sql[token.start:], renames))
			break
		}
	}
	return statements, nil
}

func sqliteHasAutoincrement(sql string) bool {
	for _, token := range sqliteTokens(sql) {
		if !token.quoted && strings.EqualFold(token.value, "autoincrement") {
			return true
		}
	}
	return false
}

func sqliteRebuildSequence(catalog driver.SchemaCatalog, namespace, table, finalTable, original string, definitions []string) ([]driver.SchemaStatement, error) {
	if !sqliteHasAutoincrement(original) || !sqliteHasAutoincrement(strings.Join(definitions, ", ")) {
		return nil, nil
	}
	sequence := quoteSchemaIdentifier(namespace) + ".sqlite_sequence"
	rows, err := catalog.Select(driver.SchemaStatement{SQL: "select seq from " + sequence + " where name = ?", Bindings: []any{table}})
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	value := rows[0]["seq"]
	return []driver.SchemaStatement{
		{SQL: "update " + sequence + " set seq = max(coalesce(seq, 0), ?) where name = ?", Bindings: []any{value, finalTable}},
		{SQL: "insert into " + sequence + " (name, seq) select ?, ? where not exists (select 1 from " + sequence + " where name = ?)", Bindings: []any{finalTable, value, finalTable}},
	}, nil
}

// Keep view definitions and their INSTEAD OF triggers intact. Validate direct
// and transitive dependants after the name handoff, before committing the rebuild.
func sqliteRebuildViews(catalog driver.SchemaCatalog, namespace, table string) ([]driver.SchemaStatement, error) {
	type view struct{ namespace, name, sql string }
	var views []view
	namespaces := []string{namespace}
	if namespace != "temp" {
		namespaces = append(namespaces, "temp")
	}
	for _, name := range namespaces {
		rows, err := catalog.Select(driver.SchemaStatement{SQL: "select name, sql from " + quoteSchemaIdentifier(name) + ".sqlite_master where type = 'view' order by name"})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			viewName, _ := row["name"].(string)
			definition, _ := row["sql"].(string)
			views = append(views, view{name, viewName, definition})
		}
	}
	affected := map[string]bool{strings.ToLower(table): true}
	selected := make([]bool, len(views))
	for changed := true; changed; {
		changed = false
		for i, view := range views {
			if !selected[i] && sqliteReferences(view.sql, affected) {
				selected[i], changed = true, true
				affected[strings.ToLower(view.name)] = true
			}
		}
	}
	var statements []driver.SchemaStatement
	for i, view := range views {
		if selected[i] {
			statements = append(statements, driver.SchemaStatement{SQL: "select * from " + quoteSchemaIdentifier(view.namespace) + "." + quoteSchemaIdentifier(view.name) + " limit 0", ReadOnly: true})
		}
	}
	return statements, nil
}
