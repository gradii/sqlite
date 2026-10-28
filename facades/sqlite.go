package facades

import (
	"github.com/gradii/framework/contracts/database/driver"

	"github.com/gradii/sqlite"
	sqliteerrors "github.com/gradii/sqlite/errors"
)

func Sqlite(connection string) (driver.Driver, error) {
	if sqlite.App == nil {

		return nil, sqliteerrors.ServiceProviderNotRegistered
	}

	instance, err := sqlite.App.MakeWith(sqlite.Binding, map[string]any{
		"connection": connection,
	})
	if err != nil {
		return nil, err
	}

	return instance.(*sqlite.Sqlite), nil
}
