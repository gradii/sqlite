package sqlite

import (
	"database/sql/driver"
	"encoding/json"
	"reflect"

	"github.com/gradii/framework/database/query/core"
	"github.com/gradii/framework/errors"
)

// PrepareJsonContains keeps SQLite's scalar membership and the existing flat-list extension.
// JSON document Valuers are decoded before checking their shape; objects and nested collections
// cannot be compared structurally by json_each.value IS ? and must not reach binding flattening.
func (g *QueryGrammar) PrepareJsonContains(value any) (any, any, error) {
	normalized, err := core.NormalizeJSONMembershipValue(value)
	if err != nil {
		return nil, nil, err
	}
	if !core.IsArrayValue(normalized) {
		binding, err := sqliteJSONContainsScalar(normalized)
		return normalized, binding, err
	}
	if reflect.ValueOf(normalized).Kind() == reflect.Map {
		return nil, nil, errors.QueryJsonContainsValueNotSupported.Args(value)
	}
	items := core.ToAnySlice(normalized)
	values, bindings := make([]any, len(items)), make([]any, len(items))
	for i, item := range items {
		value, err := core.NormalizeJSONMembershipValue(item)
		if err != nil {
			return nil, nil, err
		}
		binding, err := sqliteJSONContainsScalar(value)
		if err != nil {
			return nil, nil, err
		}
		values[i], bindings[i] = value, binding
	}
	return values, bindings, nil
}

func sqliteJSONContainsScalar(value any) (any, error) {
	if core.IsExpression(value) || core.IsArrayValue(value) {
		return nil, errors.QueryJsonContainsValueNotSupported.Args(value)
	}
	if number, ok := value.(json.Number); ok {
		if integer, err := number.Int64(); err == nil {
			return integer, nil
		}
		return number.Float64()
	}
	if _, err := driver.DefaultParameterConverter.ConvertValue(value); err != nil {
		return nil, err
	}
	return value, nil
}
