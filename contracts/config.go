package contracts

import (
	"database/sql/driver"
	contractsconfig "github.com/gradii/framework/contracts/config"
)

type ConfigBuilder interface {
	Config() contractsconfig.Config
	Connection() string
	Readers() []FullConfig
	Writers() []FullConfig
}

// Replacer replacer interface like strings.Replacer
type Replacer interface {
	Replace(name string) string
}

// Config Used in config/database.go
type Config struct {
	Connector driver.Connector
	Dsn       string
	Database  string
}

// FullConfig Fill the default value for Config
type FullConfig struct {
	Config
	Connection string
	Driver     string
	Prefix     string
}
