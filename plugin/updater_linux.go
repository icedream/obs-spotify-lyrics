//go:build linux

package main

import (
	"os/exec"

	"github.com/icedream/obs-spotify-lyrics/internal/update"
)

// openUpdate on Linux: open the releases page in the default browser.
func openUpdate(info *update.Release) {
	exec.Command("xdg-open", info.HTMLURL).Start() //nolint:errcheck
}
