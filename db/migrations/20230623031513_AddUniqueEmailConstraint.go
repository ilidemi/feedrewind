package migrations

import (
	"context"
	"feedrewind/db/pgw"
)

type AddUniqueEmailConstraint struct{}

func init() {
	registerMigration(&AddUniqueEmailConstraint{})
}

func (m *AddUniqueEmailConstraint) Version() string {
	return "20230623031513"
}

func (m *AddUniqueEmailConstraint) Up(ctx context.Context, tx *pgw.Tx) {
	tx.MustExec(ctx, "alter table users add constraint users_email_unique unique (email)")
}

func (m *AddUniqueEmailConstraint) Down(ctx context.Context, tx *pgw.Tx) {
	tx.MustExec(ctx, "alter table users drop constraint users_email_unique")
}
