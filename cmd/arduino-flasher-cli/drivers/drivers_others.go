// This file is part of arduino-flasher-cli.
//
// SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package drivers

// InstallDrivers is a no-op on non-Windows platforms
func InstallDrivers(logFile string) error {
	return nil
}
