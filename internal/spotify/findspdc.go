package spotify

import "github.com/icedream/spotify-lyrics-widget/internal/browser"

// FindSpDC searches for the Spotify sp_dc session cookie in Firefox and
// Chrome/Chromium browser profiles.
func FindSpDC() (string, error) {
	return browser.FindCookie("sp_dc", ".spotify.com")
}
