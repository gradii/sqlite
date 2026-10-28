package sqlite

import (
	"context"
	"testing"

	frameworkconfig "github.com/gradii/framework/config"
	contractsdriver "github.com/gradii/framework/contracts/database/driver"
	frameworkdb "github.com/gradii/framework/database/db"
	frameworkorm "github.com/gradii/framework/database/gorm"
	frameworkschema "github.com/gradii/framework/database/schema"
	"github.com/gradii/framework/testing/utils"
	nativedriver "github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"
	"github.com/ncruces/go-sqlite3/ext/rtree"
	"github.com/stretchr/testify/require"
)

func TestDropAllTablesExcludesVirtualTableShadows(t *testing.T) {
	config := frameworkconfig.NewApplication("")
	name := "sqlite_virtual_shadow_test"
	config.Add("database", map[string]any{
		"pool": map[string]any{"max_open_conns": 1},
		"connections": map[string]any{name: map[string]any{
			"database": ":memory:",
			"via":      func() (contractsdriver.Driver, error) { return NewSqlite(config, nil, name), nil },
		}},
	})
	log := utils.NewTestLog()
	db, err := frameworkdb.BuildDB(context.Background(), config, log, name, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Purge()) })
	schema, err := frameworkschema.NewSchema(config, log, nil, NewSqlite(config, nil, name), nil, db)
	require.NoError(t, err)
	query, _, err := frameworkorm.BuildQuery(context.Background(), config, name, log, nil, nil)
	require.NoError(t, err)
	pool, err := query.DB()
	require.NoError(t, err)
	connection, err := pool.Conn(context.Background())
	require.NoError(t, err)
	require.NoError(t, connection.Raw(func(value any) error {
		native := value.(nativedriver.Conn).Raw()
		if err := fts5.Register(native); err != nil {
			return err
		}
		return rtree.Register(native)
	}))
	require.NoError(t, connection.Close())
	require.NoError(t, schema.Sql("create virtual table docs using fts5(content)"))
	require.NoError(t, schema.Sql("create virtual table boxes using rtree(id, min_x, max_x)"))
	tables, err := schema.GetTables()
	require.NoError(t, err)
	var names []string
	for _, table := range tables {
		names = append(names, table.Name)
	}
	require.ElementsMatch(t, []string{"docs", "boxes"}, names)
	require.NoError(t, schema.DropAllTables())
	tables, err = schema.GetTables()
	require.NoError(t, err)
	require.Empty(t, tables)
}
