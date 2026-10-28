package sqlite

import (
	"github.com/gradii/framework/contracts/binding"
	"github.com/gradii/framework/contracts/foundation"
	"github.com/gradii/framework/errors"
)

const (
	Binding = "goravel.sqlite"
	Name    = "SQLite"
)

var App foundation.Application

type ServiceProvider struct {
}

func (r *ServiceProvider) Relationship() binding.Relationship {
	return binding.Relationship{
		Bindings: []string{
			Binding,
		},
		Dependencies: []string{
			binding.Config,
			binding.Log,
		},
		ProvideFor: []string{
			binding.DB,
			binding.Orm,
		},
	}
}

func (r *ServiceProvider) Register(app foundation.Application) {
	App = app

	app.BindWith(Binding, func(app foundation.Application, parameters map[string]any) (any, error) {
		config := app.MakeConfig()
		if config == nil {
			return nil, errors.ConfigFacadeNotSet.SetModule(Name)
		}

		log := app.MakeLog()
		if log == nil {
			return nil, errors.LogFacadeNotSet.SetModule(Name)
		}

		return NewSqlite(config, log, parameters["connection"].(string)), nil
	})
}

func (r *ServiceProvider) Boot(app foundation.Application) {

}
