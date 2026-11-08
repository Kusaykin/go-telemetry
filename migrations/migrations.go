// Package migrations встраивает SQL-миграции схемы БД в бинарь.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
