# SQLite

SQLite runtime driver for `github.com/gradii/framework`, forked from
[goravel/sqlite](https://github.com/goravel/sqlite). Connections use the native
`github.com/ncruces/go-sqlite3` connector and embedded SQLite engine without GORM
or CGO.

Register `&sqlite.ServiceProvider{}` and select the connection through `via`:

```go
import (
    "github.com/gradii/framework/contracts/database/driver"
    sqlitefacades "github.com/gradii/sqlite/facades"
)

"sqlite": map[string]any{
    "driver": "sqlite",
    "database": "database/app.sqlite",
    "prefix": "",
    "via": func() (driver.Driver, error) {
        return sqlitefacades.Sqlite("sqlite")
    },
},
```

An explicit `Connector` takes priority over `dsn`, which takes priority over
`database`. DSNs accept the native driver's file URI and pragma options. Reader
and writer entries use `[]contracts.Config`; empty fields inherit the named
connection's values. The framework owns pools, transactions, and connection
lifetimes. `Pool()` only normalizes configuration and performs no network I/O.

The driver provides query compilation, schema types and modifiers, metadata,
database file operations, and transactional table rebuilding. Rebuilds retain
indexes, triggers, generated columns and constraints, including attached schemas.
Foreign keys must be disabled before starting a transaction that rebuilds a table;
the framework automatically leases and restores the session for normal schema
operations. SQLite database-file operations accept paths or `:memory:`.

Use explicit table and column mappings; `prefix` remains supported while
`singular`, `name_replacer` and `no_lower_case` are removed. Docker helpers and
installation tooling are outside this runtime module. `Driver.Docker()` returns
an unsupported error.

## Development

This driver requires the matching framework changes. The dependency must be
updated to a published revision containing the new contracts before release.
For sibling checkouts, create an ignored local workspace:

```sh
go work init . ../goravel-framework
go work edit -replace=github.com/gradii/framework@v1.18.0=../goravel-framework
go test -race ./...
```

The runtime suite includes native in-memory and file database integration tests.
Upstream test cases have been ported to the new query/schema/connector APIs.
Database-fixture lifecycle coverage lives in the framework's integration module.
The old implementation and its GORM dependencies are not retained.
