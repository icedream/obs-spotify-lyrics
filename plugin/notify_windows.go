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
