// Run `golangci-lint cache clean` after modifying this file.

package gorules

import (
	"github.com/quasilyte/go-ruleguard/dsl"
)

func callToTimeNow(m dsl.Matcher) {
	m.Match(`time.Now`).Report(`call to time.Now`)
}
