package sqlite

import (
	"database/sql/driver"
	"github.com/gradii/framework/contracts/database"
	contractsdriver "github.com/gradii/framework/contracts/database/driver"
	contractsquery "github.com/gradii/framework/contracts/database/query"
	"github.com/gradii/framework/errors"
	sqlitedriver "github.com/ncruces/go-sqlite3/driver"
)

type Dialect struct{}

func NewDialect() *Dialect                { return &Dialect{} }
func (*Dialect) Name() string             { return Name }
func (*Dialect) ServerVersionSQL() string { return "select sqlite_version()" }
func (*Dialect) NewConnector(config database.Config) (driver.Connector, error) {
	return NewConnector(config)
}
func NewConnector(config database.Config) (driver.Connector, error) {
	if config.Connector != nil {
		return config.Connector, nil
	}
	dsn := config.Dsn
	if dsn == "" {
		dsn = config.Database
	}
	if dsn == "" {
		return nil, errors.DatabaseConfigNotFound
	}
	return (&sqlitedriver.SQLite{}).OpenConnector(dsn)
}
func (*Dialect) NewQueryGrammar(options contractsdriver.GrammarOptions) contractsquery.Grammar {
	return NewQueryGrammar(options.Prefix)
}
func (*Dialect) NewProcessor() contractsquery.Processor { return NewQueryProcessor() }
