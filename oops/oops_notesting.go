//go:build !testing

package oops

func RequireNoError(t any, err error, msgAndArgs ...any) {
	panic("not implemented")
}
