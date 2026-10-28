package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	frameworkconfig "github.com/gradii/framework/config"
	"github.com/gradii/framework/contracts/database"
	contractsdriver "github.com/gradii/framework/contracts/database/driver"
	contractsschema "github.com/gradii/framework/contracts/database/schema"
	frameworkdb "github.com/gradii/framework/database/db"
	frameworkorm "github.com/gradii/framework/database/gorm"
	frameworkschema "github.com/gradii/framework/database/schema"
	"github.com/gradii/framework/testing/utils"
	"github.com/stretchr/testify/require"
)

type mappedRecord struct {
	RecordKey uint64 `gorm:"column:id;primaryKey;autoIncrement" db:"id"`
	Label     string `gorm:"column:display_name" db:"display_name"`
	Document  string `gorm:"column:document" db:"document"`
}

func (mappedRecord) TableName() string { return "driver_users" }

func TestNativeConnector(t *testing.T) {
	for _, dsn := range []string{":memory:", "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "connector.db")) + "?_pragma=foreign_keys(1)"} {
		t.Run(dsn, func(t *testing.T) {
			connector, err := NewConnector(database.Config{Dsn: dsn, Database: "ignored.db"})
			require.NoError(t, err)
			duplicate, err := NewConnector(database.Config{Connector: connector, Dsn: "ignored"})
			require.NoError(t, err)
			require.Same(t, connector, duplicate)
			db := sql.OpenDB(connector)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			require.NoError(t, db.PingContext(context.Background()))
			var version string
			require.NoError(t, db.QueryRow("select sqlite_version()").Scan(&version))
			require.NotEmpty(t, version)
		})
	}
	_, err := NewConnector(database.Config{})
	require.Error(t, err)
}

func TestFrameworkIntegration(t *testing.T) {
	for _, storage := range []string{"memory", "file"} {
		t.Run(storage, func(t *testing.T) {
			name := fmt.Sprintf("sqlite_driver_%d", time.Now().UnixNano())
			dsn := ":memory:"
			if storage == "file" {
				dsn = "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "runtime.db"))
			}
			config := frameworkconfig.NewApplication("")
			config.Add("database.default", name)
			config.Add("database.pool", map[string]any{"max_open_conns": 1, "max_idle_conns": 1})
			config.Add("database.connections."+name, map[string]any{
				"driver": Name, "dsn": dsn, "prefix": "pre_",
				"via": func() (contractsdriver.Driver, error) { return NewSqlite(config, nil, name), nil },
			})
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			log := utils.NewTestLog()
			db, err := frameworkdb.BuildDB(ctx, config, log, name, nil)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Purge()) })
			schema, err := frameworkschema.NewSchema(config, log, nil, NewSqlite(config, nil, name), nil, db)
			require.NoError(t, err)
			require.NoError(t, schema.EnableForeignKeyConstraints())
			require.NoError(t, schema.Create("driver_users", func(table contractsschema.Blueprint) {
				table.BigIncrements("id")
				table.String("display_name", 80).Collation("nocase")
				table.Json("document")
				table.Integer("label_length").StoredAs("length(display_name)")
				table.Unique("display_name").Name("pre_users_name_unique")
			}))
			require.NoError(t, schema.Create("children", func(table contractsschema.Blueprint) {
				table.BigIncrements("id")
				table.UnsignedBigInteger("user_id")
				table.Foreign("user_id").References("id").On("driver_users").CascadeOnDelete()
			}))
			require.NoError(t, schema.Sql(`create table pre_audit (label text)`))
			require.NoError(t, schema.Sql(`create trigger pre_users_update after update of display_name on pre_driver_users begin insert into pre_audit(label) values (NEW.display_name); end`))
			result, err := db.Table("driver_users").Insert(map[string]any{"display_name": "名字", "document": `{"roles":["reader","writer"]}`})
			require.NoError(t, err)
			require.EqualValues(t, 1, result.RowsAffected)
			count, err := db.Table("driver_users").WhereJsonContains("document->roles", []string{"reader", "writer"}).Count()
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			count, err = db.Table("driver_users").WhereJsonDoesntContain("document->roles", []string{"reader", "writer"}).Count()
			require.NoError(t, err)
			require.Zero(t, count)
			var records []mappedRecord
			require.NoError(t, db.Table("driver_users").Select("id", "display_name", "document").Get(&records))
			require.Len(t, records, 1)
			require.Greater(t, records[0].RecordKey, uint64(0))
			_, err = db.Table("children").Insert(map[string]any{"user_id": records[0].RecordKey})
			require.NoError(t, err)
			_, err = db.Table("children").Insert(map[string]any{"user_id": 99999})
			require.Error(t, err)

			require.NoError(t, schema.Table("driver_users", func(table contractsschema.Blueprint) {
				table.Text("display_name").Collation("nocase").Change()
				table.String("notes", 120).Nullable().Default("restored")
			}))
			columns, err := schema.GetColumnsMetadata("driver_users")
			require.NoError(t, err)
			byName := map[string]contractsdriver.ColumnMetadata{}
			for _, column := range columns {
				byName[column.Name] = column
			}
			require.Equal(t, "text", byName["display_name"].Type)
			require.Equal(t, "nocase", *byName["display_name"].Collation)
			require.NotNil(t, byName["label_length"].Generation)
			require.Equal(t, "stored", byName["label_length"].Generation.Type)
			require.True(t, schema.HasIndex("driver_users", "pre_users_name_unique"))
			_, err = db.Table("driver_users").Where("id", records[0].RecordKey).Update(map[string]any{"display_name": "changed"})
			require.NoError(t, err)
			audit, err := db.Table("audit").Count()
			require.NoError(t, err)
			require.EqualValues(t, 1, audit)
			require.NoError(t, schema.Table("driver_users", func(table contractsschema.Blueprint) {
				table.RenameColumn("display_name", "label")
				table.String("notes", 120).Nullable().Change()
			}))
			_, err = db.Table("driver_users").Where("id", records[0].RecordKey).Update(map[string]any{"label": "renamed"})
			require.NoError(t, err)
			audit, err = db.Table("audit").Count()
			require.NoError(t, err)
			require.EqualValues(t, 2, audit)
			require.True(t, schema.HasColumn("driver_users", "label"))
			require.NoError(t, schema.Table("children", func(table contractsschema.Blueprint) { table.DropForeign("user_id") }))
			keys, err := schema.GetForeignKeys("children")
			require.NoError(t, err)
			require.Empty(t, keys)
			require.NoError(t, schema.Table("children", func(table contractsschema.Blueprint) {
				table.Foreign("user_id").References("id").On("driver_users").CascadeOnDelete()
			}))
			require.NoError(t, schema.Table("driver_users", func(table contractsschema.Blueprint) { table.DropColumn("notes") }))
			require.False(t, schema.HasColumn("driver_users", "notes"))
			require.NoError(t, schema.Table("driver_users", func(table contractsschema.Blueprint) { table.RenameColumn("label", "display_name") }))

			err = schema.Table("driver_users", func(table contractsschema.Blueprint) {
				table.String("rollback_probe").Nullable()
				table.String("display_name")
			})
			require.Error(t, err)
			require.False(t, schema.HasColumn("driver_users", "rollback_probe"))
			var enabled int
			require.NoError(t, db.SelectOne(&enabled, "pragma foreign_keys"))
			require.Equal(t, 1, enabled)

			query, _, err := frameworkorm.BuildQuery(ctx, config, name, log, nil, nil)
			require.NoError(t, err)
			record := mappedRecord{Label: "ORM inserted", Document: `{"roles":["admin"]}`}
			require.NoError(t, query.Clone().Create(&record))
			require.Greater(t, record.RecordKey, uint64(0))
			var loaded mappedRecord
			require.NoError(t, query.Clone().Where("id", record.RecordKey).First(&loaded))
			require.Equal(t, record.Label, loaded.Label)
			tx, err := query.Clone().BeginTransaction()
			require.NoError(t, err)
			transient := mappedRecord{Label: "rollback", Document: `{}`}
			require.NoError(t, tx.Create(&transient))
			require.NoError(t, tx.Rollback())
			count, err = query.Clone().Model(&mappedRecord{}).Where("id", transient.RecordKey).Count()
			require.NoError(t, err)
			require.Zero(t, count)
			require.NoError(t, schema.Sql(`create view pre_names as select display_name from pre_driver_users`))
			views, err := schema.GetViews()
			require.NoError(t, err)
			require.Len(t, views, 1)
			tables, err := schema.GetTables()
			require.NoError(t, err)
			require.Len(t, tables, 3)
			require.NoError(t, schema.WithWriteConnection(func(scoped contractsschema.Schema) error {
				if err := scoped.Sql(`attach database ':memory:' as aux`); err != nil {
					return err
				}
				if err := scoped.Create("aux.extra", func(table contractsschema.Blueprint) {
					table.BigIncrements("id")
					table.String("name")
					table.Unique("name").Name("extra_name_unique")
				}); err != nil {
					return err
				}
				if err := scoped.Sql(`create table aux.audit (name text)`); err != nil {
					return err
				}
				if err := scoped.Sql(`create trigger aux.extra_update after update of name on pre_extra begin insert into audit(name) values (NEW.name); end`); err != nil {
					return err
				}
				if err := scoped.Sql(`insert into aux.pre_extra(name) values ('initial')`); err != nil {
					return err
				}
				if err := scoped.Table("aux.extra", func(table contractsschema.Blueprint) { table.Text("name").Change() }); err != nil {
					return err
				}
				if err := scoped.Sql(`update aux.pre_extra set name = 'updated'`); err != nil {
					return err
				}
				value, err := scoped.DB().Scalar(`select count(*) from aux.audit`)
				if err != nil {
					return err
				}
				require.EqualValues(t, 1, value)
				require.True(t, scoped.HasIndex("aux.extra", "extra_name_unique"))
				if err := scoped.Table("aux.extra", func(table contractsschema.Blueprint) { table.RenameIndex("extra_name_unique", "renamed_unique") }); err != nil {
					return err
				}
				require.True(t, scoped.HasIndex("aux.extra", "renamed_unique"))
				names, err := scoped.GetTables("aux")
				if err != nil {
					return err
				}
				require.Len(t, names, 2)
				if err := scoped.Drop("aux.extra"); err != nil {
					return err
				}
				return scoped.Sql(`detach database aux`)
			}))
			require.NoError(t, schema.DropAllViews())
			require.NoError(t, schema.DropAllTables())
			require.False(t, schema.HasTable("driver_users"))
		})
	}
}
