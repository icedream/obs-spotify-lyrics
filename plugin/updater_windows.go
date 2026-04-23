//go:build windows

package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"golang.org/x/sys/windows"

	"github.com/icedream/obs-spotify-lyrics/distribution"
)

var updateHTTPClient = &http.Client{Timeout: 15 * time.Second}

// openUpdate on Windows: prompt user, then download and launch the installer.
func openUpdate(info *releaseInfo) {
	msg := fmt.Sprintf("Version %s is available (you have %s).\n\nDownload and run the installer now?",
		info.TagName, pluginVersion)
	msgPtr, _ := windows.UTF16PtrFromString(msg)
	titlePtr, _ := windows.UTF16PtrFromString("Spotify Lyrics for OBS - Update Available")
	const idYes = 6 // IDYES
	ret, _ := windows.MessageBox(0, msgPtr, titlePtr, windows.MB_YESNO|windows.MB_ICONINFORMATION)
	if ret != idYes {
		return
	}

	// Find installer, SHA256SUMS, and SHA256SUMS.asc assets.
	var installerURL, installerName, checksumURL, checksumSigURL string
	for _, asset := range info.Assets {
		switch asset.Name {
		case "SHA256SUMS":
			checksumURL = asset.BrowserDownloadURL
		case "SHA256SUMS.asc":
			checksumSigURL = asset.BrowserDownloadURL
		default:
			if strings.HasSuffix(asset.Name, "-setup.exe") {
				installerURL = asset.BrowserDownloadURL
				installerName = asset.Name
			}
		}
	}
	if installerURL == "" || checksumURL == "" || checksumSigURL == "" {
		openBrowser(info.HTMLURL)
		return
	}

	// Fetch and verify SHA256SUMS before downloading the installer (fail fast).
	checksumData, err := fetchVerifiedChecksums(checksumURL, checksumSigURL)
	if err != nil {
		openBrowser(info.HTMLURL)
		return
	}

	expectedHash, err := parseExpectedHash(checksumData, installerName)
	if err != nil || expectedHash == "" {
		openBrowser(info.HTMLURL)
		return
	}

	// Download installer to a temp file, hashing while writing.
	tmp, err := os.CreateTemp("", "obs-spotify-lyrics-*-setup.exe")
	if err != nil {
		openBrowser(info.HTMLURL)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	resp, err := http.Get(installerURL) //nolint:noctx
	if err != nil {
		tmp.Close()
		openBrowser(info.HTMLURL)
		return
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		openBrowser(info.HTMLURL)
		return
	}

	h := sha256.New()
	if _, err := io.Copy(tmp, io.TeeReader(resp.Body, h)); err != nil {
		tmp.Close()
		openBrowser(info.HTMLURL)
		return
	}
	tmp.Close()

	if hex.EncodeToString(h.Sum(nil)) != expectedHash {
		openBrowser(info.HTMLURL)
		return
	}

	// Rename to .exe so cmd can run it as an executable.
	exePath := tmpPath + ".exe"
	if err := os.Rename(tmpPath, exePath); err != nil {
		openBrowser(info.HTMLURL)
		return
	}
	defer os.Remove(exePath)

	exec.Command("cmd", "/c", "start", "", exePath).Start() //nolint:errcheck
}
