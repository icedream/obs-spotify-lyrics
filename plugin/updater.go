package main

import (
	"github.com/icedream/obs-spotify-lyrics/internal/logger"
	"github.com/icedream/obs-spotify-lyrics/internal/update"
)

// githubReleasesURL is the GitHub API endpoint for releases, set at build time
// via -X main.githubReleasesURL=... for custom forks/mirrors.
//
// It is the list endpoint rather than releases/latest because only the list
// carries every release's body, and the body is where a release advertises the
// OBS versions it can load.
var githubReleasesURL = "https://api.github.com/repos/icedream/obs-spotify-lyrics/releases"

// checkForUpdates lists the releases and picks the newest one that is both newer
// than this build and loadable on the running OBS. A decision with no Update and
// no TooNew means nothing to offer.
func checkForUpdates() (*update.Decision, error) {
	releases, err := update.List(githubReleasesURL, "obs-spotify-lyrics/"+pluginVersion)
	if err != nil {
		return nil, err
	}

	decision := update.Choose(releases, pluginVersion, hostOBSVersion())
	for _, tag := range decision.Malformed {
		logger.Warnf("release %s has an unreadable obs-compat marker, skipping it", tag)
	}
	return &decision, nil
}
