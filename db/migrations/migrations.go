package migrations

import (
	"feedrewind/db/pgw"
	"sort"

	"github.com/jackc/pgx/v5/pgconn"
)

type Migration interface {
	Version() string
	Up(tx *Tx)
	Down(tx *Tx)
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

type Tx struct {
	impl *pgw.Tx
}

func WrapTx(tx *pgw.Tx) *Tx {
	return &Tx{
		impl: tx,
	}
}

func (tx *Tx) MustExec(sql string, arguments ...any) pgconn.CommandTag {
	tag, err := tx.impl.Exec(sql, arguments...)
	if err != nil {
		panic(err)
	}
	return tag
}
