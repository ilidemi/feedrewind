//go:build !testing && !e2etesting && !emailtesting && !stripetesting

package oops

func RequireNoError(t any, err error, msgAndArgs ...any) {
	panic("not implemented")
}
