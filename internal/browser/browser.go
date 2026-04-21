// Package browser provides helpers for reading cookies from installed web
// browsers. It supports Firefox and Chromium-family browsers (Chrome,
// Chromium, Edge, Brave) on Linux and Windows.
package browser

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// FindCookie searches for a cookie by name across Firefox and Chromium-family
// browser profiles and returns the first value found.
//
// domain should include a leading dot for subdomain matching (e.g.
// ".example.com"), which is consistent with the cookie spec.
//
// If no value is found, the returned error lists every source that was tried
// and why it failed.
func FindCookie(name, domain string) (string, error) {
	var errs error

	for _, dir := range firefoxProfileDirs() {
		dbPath := filepath.Join(dir, "cookies.sqlite")
		val, err := readFirefoxCookie(dbPath, name, domain)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("firefox %s: %w", dir, err))
			continue
		}
		if val != "" {
			return val, nil
		}
	}

	for _, src := range chromiumSources() {
		val, err := src.readCookie(name, domain)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("%s: %w", src.label, err))
			continue
		}
		if val != "" {
			return val, nil
		}
	}

	if errs != nil {
		return "", fmt.Errorf("cookie %q not found: %w", name, errs)
	}
	return "", fmt.Errorf("cookie %q not found in any known browser profile", name)
}

// readFirefoxCookie reads a named cookie from a Firefox cookies.sqlite.
// Prefers cookies not in any container; falls back to any container.
func readFirefoxCookie(dbPath, name, domain string) (string, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return "", err
	}
	tmp, cleanup, err := copyDBToTemp(dbPath)
	if err != nil {
		return "", err
	}
	defer cleanup()

	db, err := sql.Open("sqlite", "file:"+tmp+"?mode=ro&immutable=1")
	if err != nil {
		return "", err
	}
	defer func() { _ = db.Close() }()

	// Try non-container cookie first, then fall back to any container.
	for _, extraWhere := range []string{
		"AND (originAttributes = '' OR originAttributes IS NULL)",
		"",
	} {
		var value string
		q := `SELECT value FROM moz_cookies
		      WHERE name = ? AND host LIKE '%' || ? ` + extraWhere + `
		      ORDER BY lastAccessed DESC LIMIT 1`
		err := db.QueryRow(q, name, domain).Scan(&value)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
	}
	return "", nil
}

// chromiumSource represents a single Chromium-family cookie database with
// associated decryption logic.
type chromiumSource struct {
	label       string
	cookiesFile string
	// decrypt decrypts a raw encrypted_value blob. If nil, only cookies
	// stored as plain text can be read.
	decrypt func([]byte) (string, error)
}

// readCookie reads a named cookie from this Chromium cookie database.
func (s *chromiumSource) readCookie(name, domain string) (string, error) {
	if _, err := os.Stat(s.cookiesFile); err != nil {
		return "", err
	}
	tmp, cleanup, err := copyDBToTemp(s.cookiesFile)
	if err != nil {
		return "", err
	}
	defer cleanup()

	db, err := sql.Open("sqlite", "file:"+tmp+"?mode=ro&immutable=1")
	if err != nil {
		return "", err
	}
	defer func() { _ = db.Close() }()

	rows, err := db.Query(
		`SELECT COALESCE(value, ''), COALESCE(encrypted_value, x'')
		 FROM cookies
		 WHERE name = ? AND host_key LIKE '%' || ?
		 ORDER BY last_access_utc DESC LIMIT 5`,
		name, domain,
	)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var plainValue string
		var encryptedValue []byte
		if err := rows.Scan(&plainValue, &encryptedValue); err != nil {
			continue
		}
		if plainValue != "" {
			return plainValue, nil
		}
		if len(encryptedValue) == 0 || s.decrypt == nil {
			continue
		}
		if val, err := s.decrypt(encryptedValue); err == nil && val != "" {
			return val, nil
		}
	}
	return "", rows.Err()
}

// copyDBToTemp copies src (and its -wal/-shm siblings if present) to a temp
// location. The returned cleanup function removes the temp files.
func copyDBToTemp(src string) (string, func(), error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return "", func() {}, err
	}
	f, err := os.CreateTemp("", "browser-cookie-*.sqlite")
	if err != nil {
		return "", func() {}, err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return "", func() {}, err
	}
	_ = f.Close()

	// Copy WAL and SHM so SQLite sees a consistent snapshot.
	for _, suffix := range []string{"-wal", "-shm"} {
		if sidecar, err := os.ReadFile(src + suffix); err == nil {
			_ = os.WriteFile(tmp+suffix, sidecar, 0o600)
		}
	}

	cleanup := func() {
		_ = os.Remove(tmp)
		_ = os.Remove(tmp + "-wal")
		_ = os.Remove(tmp + "-shm")
	}
	return tmp, cleanup, nil
}

// parseFirefoxProfilesINI returns profile directories listed in a Firefox
// profiles.ini file.
func parseFirefoxProfilesINI(iniPath string) []string {
	data, err := os.ReadFile(iniPath)
	if err != nil {
		return nil
	}
	base := filepath.Dir(iniPath)

	var dirs []string
	var currentPath string
	var isRelative bool

	flush := func() {
		if currentPath == "" {
			return
		}
		if isRelative {
			dirs = append(dirs, filepath.Join(base, currentPath))
		} else {
			dirs = append(dirs, currentPath)
		}
		currentPath = ""
		isRelative = false
	}

	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") {
			flush()
		} else if after, ok := strings.CutPrefix(line, "Path="); ok {
			currentPath = filepath.FromSlash(after)
		} else if line == "IsRelative=1" {
			isRelative = true
		}
	}
	flush()
	return dirs
}

// chromiumProfileDirs returns all profile subdirectories within a Chromium
// User Data directory.
func chromiumProfileDirs(userDataDir string) []string {
	entries, err := os.ReadDir(userDataDir)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "Default" || strings.HasPrefix(name, "Profile ") {
			dirs = append(dirs, filepath.Join(userDataDir, name))
		}
	}
	return dirs
}
