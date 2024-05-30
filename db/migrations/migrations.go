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

func (tx *Tx) MustAddTimestamps(tableName string) {
	tx.MustExec(`alter table ` + tableName + ` add column created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL`)
	tx.MustExec(`alter table ` + tableName + ` add column updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL`)
	tx.MustExec(`create trigger bump_updated_at before update on ` + tableName + ` FOR EACH ROW EXECUTE FUNCTION bump_updated_at_utc();`)
}

func (tx *Tx) MustUpdateDiscardedViews(tableName string, checkTableUsage *bool) {
	*checkTableUsage = false
	tx.MustExec(`
		create or replace view ` + tableName + `_with_discarded as
			select * from ` + tableName + `
		with cascaded check option
	`)
	tx.MustExec(`
		create or replace view ` + tableName + `_without_discarded as
			select * from ` + tableName + `
			where discarded_at is null
		with cascaded check option
	`)
	*checkTableUsage = true
}

var DeleteJobByName func(pgw.Queryable, string) error

func (tx *Tx) MustDeleteJobByName(name string) {
	err := DeleteJobByName(tx.impl, name)
	if err != nil {
		panic(err)
	}
}
