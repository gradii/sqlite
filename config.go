package sqlite

import (
	sqldriver "database/sql/driver"
	"fmt"

	"github.com/gradii/framework/contracts/config"

	"github.com/gradii/sqlite/contracts"
)

type Config struct {
	config     config.Config
	connection string
}

func NewConfig(config config.Config, connection string) *Config {
	return &Config{
		config:     config,
		connection: connection,
	}
}

func (r *Config) Config() config.Config {
	return r.config
}

func (r *Config) Connection() string {
	return r.connection
}

func (r *Config) Readers() []contracts.FullConfig {
	configs := r.config.Get(fmt.Sprintf("database.connections.%s.read", r.connection))
	if readConfigs, ok := configs.([]contracts.Config); ok {
		return r.fillDefault(readConfigs)
	}

	return nil
}

func (r *Config) Writers() []contracts.FullConfig {
	configs := r.config.Get(fmt.Sprintf("database.connections.%s.write", r.connection))
	if writeConfigs, ok := configs.([]contracts.Config); ok {
		return r.fillDefault(writeConfigs)
	}

	// Use default db configuration when write is empty
	return r.fillDefault([]contracts.Config{{}})
}

func (r *Config) fillDefault(configs []contracts.Config) []contracts.FullConfig {
	if len(configs) == 0 {
		return nil
	}

	var fullConfigs []contracts.FullConfig
	for _, config := range configs {
		fullConfig := contracts.FullConfig{
			Config:     config,
			Connection: r.connection,
			Driver:     Name,
			Prefix:     r.config.GetString(fmt.Sprintf("database.connections.%s.prefix", r.connection)),
		}

		if fullConfig.Connector == nil {
			fullConfig.Connector, _ = r.config.Get(fmt.Sprintf("database.connections.%s.connector", r.connection)).(sqldriver.Connector)
		}
		// If read or write is empty, use the default config
		if fullConfig.Dsn == "" {
			fullConfig.Dsn = r.config.GetString(fmt.Sprintf("database.connections.%s.dsn", r.connection))
		}
		if fullConfig.Database == "" {
			fullConfig.Database = r.config.GetString(fmt.Sprintf("database.connections.%s.database", r.connection))
		}
		fullConfigs = append(fullConfigs, fullConfig)
	}

	return fullConfigs
}
