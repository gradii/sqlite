package errors

import "github.com/gradii/framework/errors"

var (
	ConfigNotFound               = errors.New("not found database configuration")
	FailedToGenerateDSN          = errors.New("failed to generate DSN, please check the database configuration")
	DockerNotSupported           = errors.New("SQLite Docker helpers are not part of the runtime driver")
	ServiceProviderNotRegistered = errors.New("please register sqlite service provider")
)
