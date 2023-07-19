package migrations

import "feedrewind/db/pgw"

type UtcNow struct{}

func init() {
	registerMigration(&UtcNow{})
}

func (m *UtcNow) Version() string {
	return "20230712025233"
}

func (m *UtcNow) Up(tx *pgw.Tx) {
	tx.MustExec(`
create function utc_now()
returns timestamp(6) without time zone as $$
begin
	return (now() at time zone 'utc');
end;
$$ language 'plpgsql'
`)
}

func (m *UtcNow) Down(tx *pgw.Tx) {
	tx.MustExec(`drop function utc_now`)
}
