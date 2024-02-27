package migrations

import (
	"feedrewind/db/pgw"
	"feedrewind/util/schedule"
)

type RefreshSuggestionsJob struct{}

func init() {
	registerMigration(&RefreshSuggestionsJob{})
}

func (m *RefreshSuggestionsJob) Version() string {
	return "20240226094124"
}

var RefreshSuggestionsJob_PerformAtFunc func(pgw.Queryable, schedule.Time) error

func (m *RefreshSuggestionsJob) Up(tx *Tx) {
	tx.MustExec(`
		alter table blogs
		add column start_feed_id int8
	`)
	tx.MustExec(`
		alter table blogs
		add constraint fk_blogs_start_feeds foreign key (start_feed_id) references start_feeds(id)
	`)
	err := RefreshSuggestionsJob_PerformAtFunc(tx.impl, schedule.UTCNow())
	if err != nil {
		panic(err)
	}
}

func (m *RefreshSuggestionsJob) Down(tx *Tx) {
	tx.MustExec(`
		alter table blogs
		drop constraint fk_blogs_start_feeds
	`)
	tx.MustExec(`
		alter table blogs
		drop column start_feed_id
	`)
	tx.MustExec(`delete from delayed_jobs where handler like '%class: RefreshSuggestionsJob'`)
}
