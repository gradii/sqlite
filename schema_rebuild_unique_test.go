package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	frameworkconfig "github.com/gradii/framework/config"
	contractsdriver "github.com/gradii/framework/contracts/database/driver"
	contractsschema "github.com/gradii/framework/contracts/database/schema"
	frameworkdb "github.com/gradii/framework/database/db"
	frameworkschema "github.com/gradii/framework/database/schema"
	frameworkerrors "github.com/gradii/framework/errors"
	"github.com/gradii/framework/testing/utils"
	"github.com/stretchr/testify/require"
)

// A migration that drops a column and re-adds it under the same name keeps the indexes it declares:
// the rebuild used to discard them through the "references a dropped column" filter and still report
// success, leaving the new column without its unique constraint.
func TestRebuildKeepsUniqueIndexOnRecreatedColumn(t *testing.T) {
	config := frameworkconfig.NewApplication("")
	name := "sqlite_rebuild_recreated_column"
	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "recreated.db"))
	config.Add("database", map[string]any{
		"default": name,
		"pool":    map[string]any{"max_open_conns": 1, "max_idle_conns": 1},
		"connections": map[string]any{name: map[string]any{
			"driver":   "sqlite",
			"database": dsn,
			"via":      func() (contractsdriver.Driver, error) { return NewSqlite(config, nil, name), nil },
		}},
	})
	log := utils.NewTestLog()
	db, err := frameworkdb.BuildDB(context.Background(), config, log, name, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Purge()) })
	schema, err := frameworkschema.NewSchema(config, log, nil, NewSqlite(config, nil, name), nil, db)
	require.NoError(t, err)

	require.NoError(t, schema.Create("items", func(table contractsschema.Blueprint) {
		table.BigIncrements("id")
		table.String("label").Nullable()
		table.String("keep").Nullable()
	}))
	require.NoError(t, schema.Table("items", func(table contractsschema.Blueprint) {
		table.DropColumn("label")
		table.String("label").Nullable()
		table.Unique("label").Name("requested_index")
	}))

	var indexes []struct {
		Name string `db:"name"`
	}
	require.NoError(t, db.SelectFromWriteConnection(&indexes, "select name from sqlite_master where type = 'index' and tbl_name = 'items'"))
	var names []string
	for _, index := range indexes {
		names = append(names, index.Name)
	}
	require.Contains(t, names, "requested_index")

	_, err = db.Table("items").Insert(map[string]any{"label": "dup", "keep": "a"})
	require.NoError(t, err)
	_, err = db.Table("items").Insert(map[string]any{"label": "dup", "keep": "b"})
	require.ErrorContains(t, err, "UNIQUE constraint failed")
}

// A trigger attached to a view is not part of the rebuilt table's own object set, so the rebuild has to
// refuse the migration rather than commit with a view whose write path can no longer resolve.
func TestRebuildRejectsViewTriggerOnDroppedColumn(t *testing.T) {
	config := frameworkconfig.NewApplication("")
	name := "sqlite_rebuild_view_trigger"
	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "view_trigger.db"))
	config.Add("database", map[string]any{
		"default": name,
		"pool":    map[string]any{"max_open_conns": 1, "max_idle_conns": 1},
		"connections": map[string]any{name: map[string]any{
			"driver":   "sqlite",
			"database": dsn,
			"via":      func() (contractsdriver.Driver, error) { return NewSqlite(config, nil, name), nil },
		}},
	})
	log := utils.NewTestLog()
	db, err := frameworkdb.BuildDB(context.Background(), config, log, name, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Purge()) })
	schema, err := frameworkschema.NewSchema(config, log, nil, NewSqlite(config, nil, name), nil, db)
	require.NoError(t, err)

	require.NoError(t, schema.Create("items", func(table contractsschema.Blueprint) {
		table.BigIncrements("id")
		table.String("label").Nullable()
	}))
	require.NoError(t, schema.Sql("create view items_view as select * from items"))
	require.NoError(t, schema.Sql("create trigger items_view_insert instead of insert on items_view begin insert into items (id, label) values (new.id, new.label); end"))

	err = schema.Table("items", func(table contractsschema.Blueprint) {
		table.DropColumn("label")
	})
	require.ErrorIs(t, err, frameworkerrors.SchemaSQLiteTriggerDependencyInvalid)

	// The refused migration executed nothing, so the view and its trigger still work.
	require.NoError(t, schema.Sql("insert into items_view (id, label) values (7, 'kept')"))
}

// A table-level constraint that names a recreated column is still valid after the rebuild, and the
// state-seeded Laravel rebuild keeps it; the port used to discard it together with the old column.
func TestRebuildKeepsConstraintOnRecreatedColumn(t *testing.T) {
	config := frameworkconfig.NewApplication("")
	name := "sqlite_rebuild_recreated_constraint"
	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "recreated_constraint.db"))
	config.Add("database", map[string]any{
		"default": name,
		"pool":    map[string]any{"max_open_conns": 1, "max_idle_conns": 1},
		"connections": map[string]any{name: map[string]any{
			"driver":   "sqlite",
			"database": dsn,
			"via":      func() (contractsdriver.Driver, error) { return NewSqlite(config, nil, name), nil },
		}},
	})
	log := utils.NewTestLog()
	db, err := frameworkdb.BuildDB(context.Background(), config, log, name, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Purge()) })
	schema, err := frameworkschema.NewSchema(config, log, nil, NewSqlite(config, nil, name), nil, db)
	require.NoError(t, err)

	require.NoError(t, schema.Sql("create table items (id integer primary key, label text, keep text, unique(label))"))
	require.NoError(t, schema.Table("items", func(table contractsschema.Blueprint) {
		table.DropColumn("label")
		table.String("label").Nullable()
	}))

	_, err = db.Table("items").Insert(map[string]any{"label": "dup", "keep": "a"})
	require.NoError(t, err)
	_, err = db.Table("items").Insert(map[string]any{"label": "dup", "keep": "b"})
	require.ErrorContains(t, err, "UNIQUE constraint failed")
}
