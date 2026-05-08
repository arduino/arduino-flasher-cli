//go:build !windows

package drivers

// installDrivers is a no-op on non-Windows platforms
func installDrivers() error {
	return nil
}
