//go:build linux

package main

import "os/exec"

// openUpdate on Linux: open the releases page in the default browser.
func openUpdate(info *releaseInfo) {
	exec.Command("xdg-open", info.HTMLURL).Start() //nolint:errcheck
}
