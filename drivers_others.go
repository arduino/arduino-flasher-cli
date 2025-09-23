//go:build !windows

package main

// installDrivers is a no-op on non-Windows platforms
func installDrivers() error {
	return nil
}
