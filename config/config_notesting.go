//go:build !testing

package config

const isTesting = false

func testingConfig() Config {
	panic("Not implemented")
}
