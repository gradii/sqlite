package sqlite

import (
	"database/sql"
	"testing"

	"github.com/gradii/framework/contracts/database"
	query "github.com/gradii/framework/database/query"
	"github.com/gradii/framework/database/query/core"
	"github.com/gradii/framework/database/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONValuePathsAndMembershipExecuteWithTheirJSONTypes(t *testing.T) {
	connector, err := NewConnector(database.Config{Dsn: ":memory:"})
	require.NoError(t, err)
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec(`create table docs (id integer primary key, data text)`)
	require.NoError(t, err)
	_, err = db.Exec(`insert into docs values (1, '{"items":[0]}'), (2, '[1,"雪"]'), (3, '["1","雪"]')`)
	require.NoError(t, err)
	type labels map[string]string
	active := true
	values := core.UpdateValues{
		{Column: "data->items[0]", Value: types.JSON(`9007199254740993`)},
		{Column: "data->active", Value: &active},
		{Column: "data->label", Value: types.JSON(`"雪"`)},
		{Column: "data->nested", Value: labels{"name": "kept"}},
	}
	builder := query.NewBuilder(NewQueryGrammar("")).From("docs", "").Where("id", 1)
	statement, bindings := builder.ToUpdateSQL(values)
	require.NoError(t, builder.Error())
	_, err = db.Exec(statement, bindings...)
	require.NoError(t, err)
	var document string
	require.NoError(t, db.QueryRow(`select data from docs where id = 1`).Scan(&document))
	assert.JSONEq(t, `{"items":[9007199254740993],"active":true,"label":"雪","nested":{"name":"kept"}}`, document)
	var number int64
	require.NoError(t, db.QueryRow(`select json_extract(data, '$.items[0]') from docs where id = 1`).Scan(&number))
	assert.EqualValues(t, 9007199254740993, number)

	for _, negate := range []bool{false, true} {
		builder := query.NewBuilder(NewQueryGrammar("")).From("docs", "").Select("id").Where("id", ">", 1).
			WhereJsonContains("data", types.JSON(`[1,"雪"]`), core.BooleanAnd, negate)
		statement, bindings := builder.ToSelectSQL()
		require.NoError(t, builder.Error())
		var id int
		require.NoError(t, db.QueryRow(statement, bindings...).Scan(&id))
		if negate {
			assert.Equal(t, 3, id)
		} else {
			assert.Equal(t, 2, id)
		}
	}
}
