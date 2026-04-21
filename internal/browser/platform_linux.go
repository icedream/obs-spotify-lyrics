package browser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/godbus/dbus/v5"
)

// firefoxProfileDirs returns all Firefox profile directories on Linux,
// including snap and flatpak installations.
func firefoxProfileDirs() []string {
	home := os.Getenv("HOME")
	if home == "" {
		return nil
	}

	iniPaths := []string{
		// Standard package install
		filepath.Join(home, ".mozilla", "firefox", "profiles.ini"),
		// Firefox snap
		filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox", "profiles.ini"),
		// Firefox flatpak
		filepath.Join(home, ".var", "app", "org.mozilla.firefox", ".mozilla", "firefox", "profiles.ini"),
	}

	seen := map[string]bool{}
	var dirs []string
	for _, ini := range iniPaths {
		for _, d := range parseFirefoxProfilesINI(ini) {
			if !seen[d] {
				seen[d] = true
				dirs = append(dirs, d)
			}
		}
	}
	return dirs
}

// getSecretServicePassword queries the GNOME Secret Service (via D-Bus) for
// the Chromium-family Safe Storage password. application is e.g. "chrome",
// "chromium", "brave", "msedge".
func getSecretServicePassword(application string) (string, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return "", fmt.Errorf("D-Bus session: %w", err)
	}

	service := conn.Object("org.freedesktop.secrets", "/org/freedesktop/secrets")

	// Open a plain (unencrypted transport) session.
	var sessionPath dbus.ObjectPath
	var ignored dbus.Variant
	if err := service.Call(
		"org.freedesktop.Secret.Service.OpenSession", 0,
		"plain", dbus.MakeVariant(""),
	).Store(&ignored, &sessionPath); err != nil {
		return "", fmt.Errorf("OpenSession: %w", err)
	}

	// Search for items matching this application.
	// ref https://chromium.googlesource.com/chromium/src/+/640d483f00a40ea99f9b5949ae395ac93566e050/components/os_crypt/async/browser/freedesktop_secret_key_provider.h#147
	// kApplicationAttributeKey="application"
	attrs := map[string]string{"application": application}
	var unlocked, locked []dbus.ObjectPath
	if err := service.Call(
		"org.freedesktop.Secret.Service.SearchItems", 0, attrs,
	).Store(&unlocked, &locked); err != nil {
		return "", fmt.Errorf("SearchItems: %w", err)
	}

	paths := append(unlocked, locked...)
	if len(paths) == 0 {
		return "", fmt.Errorf("no secret found for application=%q", application)
	}

	// GetSecret from the first result.
	item := conn.Object("org.freedesktop.secrets", paths[0])
	var secret struct {
		Session     dbus.ObjectPath
		Parameters  []byte
		Value       []byte
		ContentType string
	}
	if err := item.Call(
		"org.freedesktop.Secret.Item.GetSecret", 0, sessionPath,
	).Store(&secret); err != nil {
		return "", fmt.Errorf("GetSecret: %w", err)
	}
	return string(secret.Value), nil
}

// getLinuxChromiumDecrypt returns a decrypt function for a Chrome/Chromium
// profile on Linux. It tries the GNOME Secret Service first, falling back to
// the default password "peanuts" used when no keyring is configured.
func getLinuxChromiumDecrypt(application string) func([]byte) (string, error) {
	return func(encrypted []byte) (string, error) {
		if len(encrypted) < 3 {
			return "", errors.New("encrypted value too short")
		}
		prefix := string(encrypted[:3])
		// ref https://chromium.googlesource.com/chromium/src/+/c3dcd10b9e276234f4bbeafd2c71e282234d54a0/components/os_crypt/sync/os_crypt_linux.cc#36
		// kObfuscationPrefixV10, kObfuscationPrefixV11
		if prefix != "v10" && prefix != "v11" {
			return "", fmt.Errorf("unrecognised encryption prefix %q", prefix)
		}
		data := encrypted[3:]

		// Try Secret Service key first, then fall back to "peanuts".
		password, secretErr := getSecretServicePassword(application)
		if secretErr == nil {
			if val, err := decryptChromiumV10CBC(data, chromiumKeyFromPassword(password)); err == nil {
				return val, nil
			}
		}

		// ref https://chromium.googlesource.com/chromium/src/+/c3dcd10b9e276234f4bbeafd2c71e282234d54a0/components/os_crypt/sync/os_crypt_linux.cc#46
		// kObsoleteEncryptionKey: hardcoded fallback when no keyring is present
		val, err := decryptChromiumV10CBC(data, chromiumKeyFromPassword("peanuts"))
		if err != nil {
			if secretErr != nil {
				return "", fmt.Errorf("decryption failed (keyring: %v)", secretErr)
			}
			return "", err
		}
		return val, nil
	}
}

// chromiumSources returns all Chromium-family cookie databases to search on Linux.
func chromiumSources() []chromiumSource {
	home := os.Getenv("HOME")
	if home == "" {
		return nil
	}

	type browserDef struct {
		label       string
		userDataDir string
		application string // Secret Service application name
	}

	// Application names used as Secret Service "application" attribute.
	// ref https://chromium.googlesource.com/chromium/src/+/640d483f00a40ea99f9b5949ae395ac93566e050/components/os_crypt/async/browser/freedesktop_secret_key_provider.h#178
	browsers := []browserDef{
		{"Google Chrome", filepath.Join(home, ".config", "google-chrome"), "chrome"},
		{"Chromium", filepath.Join(home, ".config", "chromium"), "chromium"},
		{"Chromium (snap)", filepath.Join(home, "snap", "chromium", "common", ".config", "chromium"), "chromium"},
		{"Microsoft Edge", filepath.Join(home, ".config", "microsoft-edge"), "msedge"},
		{"Brave", filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser"), "brave"},
		{"Opera", filepath.Join(home, ".config", "opera"), "opera"},
		{"Opera GX", filepath.Join(home, ".config", "opera-gx"), "opera-gx"},
	}

	var sources []chromiumSource
	for _, b := range browsers {
		decrypt := getLinuxChromiumDecrypt(b.application)
		for _, profileDir := range chromiumProfileDirs(b.userDataDir) {
			base := filepath.Base(profileDir)
			// Modern Chrome stores the DB under Network/.
			sources = append(sources,
				chromiumSource{
					label:       fmt.Sprintf("%s/%s/Network", b.label, base),
					cookiesFile: filepath.Join(profileDir, "Network", "Cookies"),
					decrypt:     decrypt,
				},
				chromiumSource{
					label:       fmt.Sprintf("%s/%s", b.label, base),
					cookiesFile: filepath.Join(profileDir, "Cookies"),
					decrypt:     decrypt,
				},
			)
		}
	}
	return sources
}
