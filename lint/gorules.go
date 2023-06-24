// Run `golangci-lint cache clean` after modifying this file.

package gorules

import (
	"github.com/quasilyte/go-ruleguard/dsl"
)

func callToTimeNow(m dsl.Matcher) {
	// m.Match(`time.Now`).Report(`call to time.Now`)
	m.Match(`time.LoadLocation`).
		Report(`calls to time.LoadLocation() are disallowed, use tzdata.LocationByName instead`)
	m.Match(`db.Conn`).
		Where(
			!m.File().PkgPath.Matches(`feedrewind/routes`) &&
				!m.File().PkgPath.Matches(`feedrewind/db`) &&
				!m.File().PkgPath.Matches(`feedrewind/middleware`)).
		Report(`references to db.Conn are only allowed in db, middleware and routes, use pgw.Queryable instead`)
	m.Match(`pgw.Tx`).
		Where(
			!m.File().PkgPath.Matches(`feedrewind/routes`) &&
				!m.File().PkgPath.Matches(`feedrewind/db`) &&
				!m.File().PkgPath.Matches(`feedrewind/middleware`)).
		Report(`references to pgw.Tx are only allowed in db, middleware and routes, use pgw.Queryable instead`)
}
