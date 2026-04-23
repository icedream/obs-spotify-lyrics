package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// githubReleasesURL is the GitHub API endpoint for the latest release.
// Set at build time via -X main.githubReleasesURL=... for custom forks/mirrors.
var githubReleasesURL = "https://api.github.com/repos/icedream/obs-spotify-lyrics/releases/latest"

type releaseInfo struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// checkForUpdates fetches the latest release from GitHub and returns info if a
// newer version is available. Returns nil, nil if already on the latest version.
func checkForUpdates() (*releaseInfo, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", githubReleasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "obs-spotify-lyrics/"+pluginVersion)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var info releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	// Never prompt dev/dirty builds to update.
	current := pluginVersion
	if !strings.HasPrefix(current, "v") {
		current = "v" + current
	}
	latest := info.TagName
	if !strings.HasPrefix(latest, "v") {
		latest = "v" + latest
	}
	// Only suggest update if latest is strictly newer than current.
	// This prevents downgrade prompts when a release is yanked and the
	// previous release becomes "latest" again.
	if !semver.IsValid(current) || semver.Compare(latest, current) <= 0 {
		return nil, nil
	}
	return &info, nil
}
