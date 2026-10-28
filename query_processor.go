package sqlite

import (
	contractsquery "github.com/gradii/framework/contracts/database/query"
	"github.com/gradii/framework/database/query/processors"
	"github.com/spf13/cast"
	"regexp"
	"strings"
)

// QueryProcessor inherits the base insert-ID processing, as in Laravel.
type QueryProcessor struct{ processors.BaseProcessor }

// ProcessColumns ports SQLiteProcessor::processColumns. The optional CREATE TABLE statement
// supplies collations and generated expressions that SQLite's column pragma does not expose.
func (p *QueryProcessor) ProcessColumns(results []contractsquery.Row, sql ...string) []contractsquery.Row {
	primaryCount := 0
	for _, result := range results {
		primaryCount += cast.ToInt(result["primary"])
	}
	statement := ""
	if len(sql) > 0 {
		statement = sql[0]
	}
	columns := make([]contractsquery.Row, 0, len(results))
	for _, result := range results {
		typ := strings.ToLower(processors.MetadataString(result["type"]))
		typeName, _, _ := strings.Cut(typ, "(")
		extra := cast.ToInt(result["extra"])
		generated := extra == 2 || extra == 3
		var collation, expression, generation any
		if statement != "" {
			name := regexp.QuoteMeta(processors.MetadataString(result["name"]))
			collate := regexp.MustCompile(`(?i)\b` + name + `\b[^,(]+(?:\([^()]+\)[^,]*)?(?:(?:default|check|as)\s*(?:\(.*?\))?[^,]*)*collate\s+["'` + "`" + `]?(\w+)`)
			if match := collate.FindStringSubmatch(statement); match != nil {
				collation = strings.ToLower(match[1])
			}
			if generated {
				pattern := regexp.MustCompile(`(?i)\b` + name + `\b[^,]+\s+as\s+\(((?:[^()]+|\((?:[^()]+|\([^()]*\))*\))*)\)`)
				if match := pattern.FindStringSubmatch(statement); match != nil {
					expression = match[1]
				}
			}
		}
		if generated {
			kind := "virtual"
			if extra == 3 {
				kind = "stored"
			}
			generation = contractsquery.Row{"type": kind, "expression": expression}
		}
		columns = append(columns, contractsquery.Row{
			"name": result["name"], "type_name": typeName, "type": typ,
			"collation": collation, "nullable": processors.MetadataBool(result["nullable"]), "default": result["default"],
			"auto_increment": primaryCount == 1 && processors.MetadataBool(result["primary"]) && typ == "integer",
			"comment":        nil, "generation": generation,
		})
	}
	return columns
}

func NewQueryProcessor() *QueryProcessor { return &QueryProcessor{} }

func (p *QueryProcessor) ProcessIndexes(rows []contractsquery.Row) []contractsquery.Row {
	result := make([]contractsquery.Row, 0, len(rows))
	primaryCount := 0
	for _, row := range rows {
		primary := processors.MetadataBool(row["primary"])
		if primary {
			primaryCount++
		}
		result = append(result, contractsquery.Row{"name": strings.ToLower(processors.MetadataString(row["name"])), "columns": processors.MetadataColumns(row["columns"]), "type": nil, "unique": processors.MetadataBool(row["unique"]), "primary": primary})
	}
	if primaryCount > 1 {
		filtered := result[:0]
		for _, row := range result {
			if row["name"] != "primary" {
				filtered = append(filtered, row)
			}
		}
		result = filtered
	}
	return result
}
func (p *QueryProcessor) ProcessForeignKeys(rows []contractsquery.Row) []contractsquery.Row {
	result := make([]contractsquery.Row, 0, len(rows))
	for _, row := range rows {
		result = append(result, contractsquery.Row{"name": nil, "columns": processors.MetadataColumns(row["columns"]), "foreign_schema": row["foreign_schema"], "foreign_table": row["foreign_table"], "foreign_columns": processors.MetadataColumns(row["foreign_columns"]), "on_update": strings.ToLower(processors.MetadataString(row["on_update"])), "on_delete": strings.ToLower(processors.MetadataString(row["on_delete"]))})
	}
	return result
}
