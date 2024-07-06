package migrations

import "feedrewind/config"

type RemoveLogPostgresRowCountJob struct{}

func init() {
	registerMigration(&RemoveLogPostgresRowCountJob{})
}

func (m *RemoveLogPostgresRowCountJob) Version() string {
	return "20240705185918"
}

func (m *RemoveLogPostgresRowCountJob) Up(tx *Tx) {
	if config.Cfg.Env != config.EnvTesting {
		tx.MustDeleteJobByName("LogPostgresRowCountJob")
	}
}

func (m *RemoveLogPostgresRowCountJob) Down(tx *Tx) {
	panic("Not implemented")
}
