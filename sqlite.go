package sqlite

import (
	"github.com/gradii/framework/contracts/config"
	"github.com/gradii/framework/contracts/database"
	"github.com/gradii/framework/contracts/database/driver"
	"github.com/gradii/framework/contracts/log"
	"github.com/gradii/framework/contracts/testing/docker"
	"github.com/gradii/sqlite/contracts"
	sqliteerrors "github.com/gradii/sqlite/errors"
)

var _ driver.Driver = (*Sqlite)(nil)
var _ driver.DriverWithDialect = (*Sqlite)(nil)

type Sqlite struct {
	config contracts.ConfigBuilder
	log    log.Log
}

func NewSqlite(config config.Config, log log.Log, connection string) *Sqlite {
	return &Sqlite{config: NewConfig(config, connection), log: log}
}
func (r *Sqlite) Docker() (docker.DatabaseDriver, error) { return nil, sqliteerrors.DockerNotSupported }
func (r *Sqlite) Dialect() driver.Dialect                { return NewDialect() }
func (r *Sqlite) Grammar() driver.Grammar {
	prefix := ""
	if writers := r.config.Writers(); len(writers) > 0 {
		prefix = writers[0].Prefix
	}
	return NewSchemaGrammar(r.log, prefix)
}
func (r *Sqlite) Pool() database.Pool {
	return database.Pool{Readers: r.fullConfigsToConfigs(r.config.Readers()), Writers: r.fullConfigsToConfigs(r.config.Writers())}
}
func (r *Sqlite) Processor() driver.Processor { return NewProcessor() }
func (r *Sqlite) fullConfigsToConfigs(fullConfigs []contracts.FullConfig) []database.Config {
	configs := make([]database.Config, len(fullConfigs))
	for i, value := range fullConfigs {
		configs[i] = database.Config{Connector: value.Connector, Connection: value.Connection, Dsn: value.Dsn, Database: value.Database, Driver: Name, Prefix: value.Prefix}
	}
	return configs
}
