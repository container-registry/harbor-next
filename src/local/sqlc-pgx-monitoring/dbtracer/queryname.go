package dbtracer

import (
	"regexp"
)

// sqlc queries are declared after a sql command in the form of -- name: TheQueryName :type
var queryNameRegex = regexp.MustCompile(`^(?:--|/\*)\s+name:\s+(?P<name>\w+) :(?P<command>\w+)`)

type queryMetadata struct {
	name    string
	command string
}

func queryMetadataFromSQL(sql string) *queryMetadata {
	m := queryNameRegex.FindStringSubmatch(sql)
	if m == nil {
		return nil
	}
	return &queryMetadata{name: m[1], command: m[2]}
}
