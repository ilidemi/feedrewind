package migrations

type BannedIps struct{}

func init() {
	registerMigration(&BannedIps{})
}

func (m *BannedIps) Version() string {
	return "20241018153403"
}

func (m *BannedIps) Up(tx *Tx) {
	tx.MustExec(`
		create table banned_ips (
			ip text primary key
		)
	`)
	tx.MustAddTimestamps("banned_ips")
}

func (m *BannedIps) Down(tx *Tx) {
	tx.MustExec(`drop table banned_ips`)
}
