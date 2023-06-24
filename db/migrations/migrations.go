package migrations

import (
	"context"
	"feedrewind/db/pgw"
	"sort"
)

type Migration interface {
	Version() string
	Up(ctx context.Context, tx *pgw.Tx)
	Down(ctx context.Context, tx *pgw.Tx)
}

var All []Migration

func init() {
	sort.Slice(All, func(i, j int) bool {
		return All[i].Version() < All[j].Version()
	})
}

func registerMigration(migration Migration) {
	All = append(All, migration)
}
