package migrations

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"
)

type Migration interface {
	Version() string
	Up(ctx context.Context, tx pgx.Tx)
	Down(ctx context.Context, tx pgx.Tx)
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
