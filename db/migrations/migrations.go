package migrations

import (
	"feedrewind/db/pgw"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type Migration interface {
	Version() string
	Up(tx *Tx)
	Down(tx *Tx)
}

var All []Migration

func init() {
	slices.SortFunc(All, func(a, b Migration) int {
		return strings.Compare(a.Version(), b.Version())
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
