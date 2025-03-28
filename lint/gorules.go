// Run `golangci-lint cache clean` after modifying this file.

package gorules

import (
	"github.com/quasilyte/go-ruleguard/dsl"
)

//nolint:unused
func callToTimeNow(m dsl.Matcher) {
	m.Match(`require.NoError`).
		Report(`calls to require.NoError() are disallowed, use oops.RequireNoError() for nice stacktraces`)
	m.Match(`time.LoadLocation`).
		Report(`calls to time.LoadLocation() are disallowed, use tzdata.LocationByName instead`)
	m.Match(`db.Conn`).
		Where(!m.File().PkgPath.Matches(`feedrewind.com/routes`) &&
			!m.File().PkgPath.Matches(`feedrewind.com/db`) &&
			!m.File().PkgPath.Matches(`feedrewind.com/middleware`) &&
			!m.File().PkgPath.Matches(`^feedrewind.com$`)).
		Report(`references to db.Conn are only allowed in main, db, routes, and middleware, use pgw.Queryable instead`)
	m.Match(`context.Background`).
		Where(!m.File().PkgPath.Matches(`feedrewind.com/db`) &&
			!m.File().PkgPath.Matches(`feedrewind.com/cmd/crawl`) &&
			!(m.File().PkgPath.Matches(`feedrewind.com/jobs`) && m.File().Name.Matches(`worker.go`)) &&
			!m.File().PkgPath.Matches(`^feedrewind.com$`)).
		Report(`Background context is probably a mistake. Pass through request context instead`)
	m.Match(`time.Sleep`).
		Where(m.File().PkgPath.Matches(`feedrewind.com/jobs`) && !m.File().Name.Matches(`worker.go`)).
		Report(`Don't use time.Sleep in jobs, use util.Sleep(ctx, delay) instead`)
}
