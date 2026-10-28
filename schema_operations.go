package sqlite

import (
	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/errors"
	"os"
)

func (r *SchemaGrammar) ForeignKeyStateQuery() string { return "pragma foreign_keys" }
func (r *SchemaGrammar) CompileSchemaOperation(operation driver.SchemaOperation) ([]driver.SchemaStatement, error) {
	var statements []driver.SchemaStatement
	switch operation.Kind {
	case "foreignKeys":
		sql := "PRAGMA foreign_keys = OFF"
		if operation.Enabled {
			sql = "PRAGMA foreign_keys = ON"
		}
		statements = append(statements, driver.SchemaStatement{SQL: sql})
	case "dropAllTables":
		for _, table := range operation.Tables {
			statements = append(statements, driver.SchemaStatement{SQL: "drop table " + quoteSchemaIdentifier(sqliteSchemaName(table.Schema)) + "." + quoteSchemaIdentifier(table.Name)})
		}
	case "dropAllViews":
		for _, view := range operation.Views {
			statements = append(statements, driver.SchemaStatement{SQL: "drop view " + quoteSchemaIdentifier(sqliteSchemaName(view.Schema)) + "." + quoteSchemaIdentifier(view.Name)})
		}
	case "dropAllTypes":
		return nil, nil
	default:
		return nil, errors.SchemaFeatureNotSupported.Args(Name, operation.Kind)
	}
	return statements, nil
}
func (r *SchemaGrammar) SchemaOperationPolicy(operation driver.SchemaOperation) driver.SchemaExecutionPolicy {
	return driver.SchemaExecutionPolicy{Transactional: true, DisableForeignKeys: operation.Kind == "dropAllTables", ForeignKeysOutsideTransaction: true}
}
func (r *SchemaGrammar) CreateDatabase(name string) error {
	if name == ":memory:" {
		return nil
	}
	file, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o666)
	if err != nil {
		return err
	}
	return file.Close()
}
func (r *SchemaGrammar) DropDatabaseIfExists(name string) error {
	if name == ":memory:" {
		return nil
	}
	if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
