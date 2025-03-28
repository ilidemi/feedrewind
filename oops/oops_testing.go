//go:build testing || e2etesting || emailtesting || stripetesting

package oops

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func RequireNoError(t *testing.T, err error, msgAndArgs ...any) {
	if err != nil {
		sterr, ok := err.(*Error)
		if !ok {
			t.Helper()
			require.Fail(t, fmt.Sprintf("Received unexpected error:\n%+v", err), msgAndArgs...)
		}

		st := sterr.StackTrace()
		var b strings.Builder
		for i, frame := range st {
			if i > 0 {
				fmt.Fprint(&b, "\n")
			}
			frameText, _ := frame.MarshalText()
			fmt.Fprint(&b, string(frameText))
		}

		t.Helper()
		message := fmt.Sprintf("Received unexpected error:\n%+v\b%s", sterr.Inner.Error(), b.String())
		require.Fail(t, message, msgAndArgs...)
	}
}
