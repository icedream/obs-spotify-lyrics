//go:build windows

package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	"github.com/icedream/obs-spotify-lyrics/internal/progressdlg"
)

// errDownloadCancelled is returned by progressReader when the user cancels.
var errDownloadCancelled = errors.New("download cancelled by user")

// progressReader wraps an io.Reader, reports bytes read via onProgress, and
// returns errDownloadCancelled when onCancel returns true.
type progressReader struct {
	r          io.Reader
	read       int64
	total      int64
	onProgress func(read, total int64)
	onCancel   func() bool
}

func (pr *progressReader) Read(p []byte) (int, error) {
	if pr.onCancel != nil && pr.onCancel() {
		return 0, errDownloadCancelled
	}
	n, err := pr.r.Read(p)
	if n > 0 {
		pr.read += int64(n)
		if pr.onProgress != nil {
			pr.onProgress(pr.read, pr.total)
		}
	}
	return n, err
}

var updateHTTPClient = &http.Client{Timeout: 15 * time.Second}

// openUpdate on Windows: prompt user, then download and launch the installer.
// If the installer is verified and launched, OBS will quit automatically once
// the installer confirms it is running (after UAC elevation).
func openUpdate(info *releaseInfo) {
	msg := fmt.Sprintf(
		"Version %s is available (you have %s).\n\n"+
			"The installer will download automatically. OBS will close and relaunch "+
			"once the update is complete.\n\nProceed?",
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
	// No .exe suffix here; it is added by the rename below so Windows executes it.
	tmp, err := os.CreateTemp("", "obs-spotify-lyrics-*-setup")
	if err != nil {
		openBrowser(info.HTMLURL)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) //nolint:errcheck

	resp, err := updateHTTPClient.Get(installerURL) //nolint:noctx
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

	// Show a native progress dialog during the download. New() returns nil if
	// the dialog cannot be created; all Dialog methods tolerate a nil receiver,
	// so the download proceeds regardless.
	dlg, _ := progressdlg.New()
	defer dlg.Release()
	_ = dlg.SetTitle("Spotify Lyrics for OBS - Update")
	_ = dlg.SetLine(1, "Downloading update...", false)
	_ = dlg.SetLine(2, installerName, false)
	_ = dlg.SetCancelMsg("Cancelling download...")
	var dlgFlags uint32
	totalBytes := resp.ContentLength
	if totalBytes > 0 {
		dlgFlags = progressdlg.FlagAutoTime | progressdlg.FlagNoMinimize
	} else {
		dlgFlags = progressdlg.FlagMarquee | progressdlg.FlagNoTime | progressdlg.FlagNoMinimize
	}
	_ = dlg.Start(0, dlgFlags)

	pr := &progressReader{
		r:     resp.Body,
		total: totalBytes,
		onProgress: func(read, total int64) {
			if total > 0 {
				_ = dlg.SetProgress(uint64(read), uint64(total))
			}
		},
		onCancel: dlg.HasUserCancelled,
	}

	h := sha256.New()
	if _, err := io.Copy(tmp, io.TeeReader(pr, h)); err != nil {
		tmp.Close()
		dlg.Stop()
		if !errors.Is(err, errDownloadCancelled) {
			openBrowser(info.HTMLURL)
		}
		return
	}
	tmp.Close()
	dlg.Stop()

	if hex.EncodeToString(h.Sum(nil)) != expectedHash {
		openBrowser(info.HTMLURL)
		return
	}

	// Rename to .exe so Windows can execute it.
	exePath := tmpPath + ".exe"
	if err := os.Rename(tmpPath, exePath); err != nil {
		openBrowser(info.HTMLURL)
		return
	}
	// exePath is intentionally not deferred for removal: the file is locked by
	// the running installer; the OS will clean it from the temp directory later.

	// Create a named event that the installer will signal once it has successfully
	// elevated (UAC approved).  We wait for this signal before quitting OBS so
	// that cancelling UAC does not leave OBS closed without the update applied.
	pid := os.Getpid()
	eventName, _ := windows.UTF16PtrFromString(fmt.Sprintf("ObsSpotifyLyricsUpdate_%d", pid))
	hEvent, err := windows.CreateEvent(nil, 1 /* manual-reset */, 0 /* initially unset */, eventName)
	if err != nil {
		openBrowser(info.HTMLURL)
		return
	}

	// ShellExecute triggers UAC elevation for admin installers and handles
	// paths with spaces safely, unlike cmd /c start.
	exePtr, _ := windows.UTF16PtrFromString(exePath)
	argsPtr, _ := windows.UTF16PtrFromString(fmt.Sprintf("/AUTOUPDATE=%d", pid))
	const swShowNormal = 1
	if err := windows.ShellExecute(0, nil, exePtr, argsPtr, nil, swShowNormal); err != nil {
		windows.CloseHandle(hEvent) //nolint:errcheck
		openBrowser(info.HTMLURL)
		return
	}

	// Wait (in a background goroutine) for the installer to signal the event,
	// then quit OBS on the UI thread.  Timeout after 2 minutes in case the user
	// cancels or dismisses the UAC prompt.
	go func() {
		defer func() { _ = windows.CloseHandle(hEvent) }()
		const twoMinutesMS = 2 * 60 * 1000
		ret, _ := windows.WaitForSingleObject(hEvent, twoMinutesMS)
		if ret == windows.WAIT_OBJECT_0 {
			scheduleQuitOBS()
		}
	}()
}

// fetchVerifiedChecksums downloads SHA256SUMS and its detached GPG signature,
// verifies the signature against the embedded distribution key, and returns
// the raw SHA256SUMS content if verification succeeds.
func fetchVerifiedChecksums(checksumURL, checksumSigURL string) ([]byte, error) {
	checksumData, err := fetchBytes(checksumURL)
	if err != nil {
		return nil, err
	}
	sigData, err := fetchBytes(checksumSigURL)
	if err != nil {
		return nil, err
	}

	keyRing, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(distribution.SigningKeyASC))
	if err != nil {
		return nil, fmt.Errorf("reading distribution signing key: %w", err)
	}
	if _, err := openpgp.CheckArmoredDetachedSignature(keyRing, bytes.NewReader(checksumData), bytes.NewReader(sigData), nil); err != nil {
		return nil, fmt.Errorf("SHA256SUMS signature verification failed: %w", err)
	}
	return checksumData, nil
}

// fetchBytes downloads a URL and returns its body, requiring HTTP 200.
func fetchBytes(url string) ([]byte, error) {
	resp, err := updateHTTPClient.Get(url) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// parseExpectedHash parses sha256sum output and returns the lowercase hex hash
// for filename, or ("", nil) if the file is not listed.
func parseExpectedHash(data []byte, filename string) (string, error) {
	// sha256sum format: "<64 hex chars>  <filename>" (text) or "<64 hex chars> *<filename>" (binary)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 66 {
			continue
		}
		hash := strings.ToLower(line[:64])
		if !isHex(hash) {
			continue
		}
		// Skip the separator ("  " or " *") to get the filename.
		rest := strings.TrimLeft(line[64:], " *")
		if rest == filename {
			return hash, nil
		}
	}
	return "", scanner.Err()
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func openBrowser(url string) {
	_ = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url).Start()
}
