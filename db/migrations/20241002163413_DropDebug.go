package migrations

type DropDebug struct{}

func init() {
	registerMigration(&DropDebug{})
}

func (m *DropDebug) Version() string {
	return "20241002163413"
}

func (m *DropDebug) Up(tx *Tx) {
	tx.MustExec(`drop table debug`)
}

func (m *DropDebug) Down(tx *Tx) {
	panic("Not implemented")
}
