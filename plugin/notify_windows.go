//go:build windows

package main

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func notifyAlreadyUpToDate() {
	msg, _ := windows.UTF16PtrFromString("You are already running the latest version.")
	title, _ := windows.UTF16PtrFromString("Spotify Lyrics for OBS")
	windows.MessageBox(0, msg, title, windows.MB_OK|windows.MB_ICONINFORMATION) //nolint:errcheck
}

func notifyUpdateCheckError(err error) {
	msg, _ := windows.UTF16PtrFromString(fmt.Sprintf("Failed to check for updates:\n\n%v", err))
	title, _ := windows.UTF16PtrFromString("Spotify Lyrics for OBS")
	windows.MessageBox(0, msg, title, windows.MB_OK|windows.MB_ICONWARNING) //nolint:errcheck
}

// notifyNeedsNewerOBS tells the user a newer plugin exists that this OBS cannot
// load, instead of handing them an installer the loader would then refuse.
func notifyNeedsNewerOBS(tag, needOBS string) {
	msg, _ := windows.UTF16PtrFromString(fmt.Sprintf(
		"%s requires OBS Studio %s or newer.\n\n"+
			"This OBS Studio is older, so that update was not offered. "+
			"Update OBS Studio first to get it.",
		tag, needOBS))
	title, _ := windows.UTF16PtrFromString("Spotify Lyrics for OBS")
	windows.MessageBox(0, msg, title, windows.MB_OK|windows.MB_ICONINFORMATION) //nolint:errcheck
}
