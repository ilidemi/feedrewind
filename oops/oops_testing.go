//go:build testing

package oops

import (
	"fmt"
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

		t.Helper()
		message := fmt.Sprintf("Received unexpected error:\n%s", sterr.Error())
		require.Fail(t, message, msgAndArgs...)
	}
}
