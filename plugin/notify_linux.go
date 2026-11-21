//go:build linux

package main

func notifyAlreadyUpToDate() {}

func notifyUpdateCheckError(_ error) {}

// notifyNeedsNewerOBS has no desktop dialog here; the OBS log carries the same
// message from the update check.
func notifyNeedsNewerOBS(_, _ string) {}
