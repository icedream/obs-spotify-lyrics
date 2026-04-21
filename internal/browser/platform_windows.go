package browser

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// firefoxProfileDirs returns all Firefox profile directories on Windows.
func firefoxProfileDirs() []string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return nil
	}
	iniPaths := []string{
		filepath.Join(appData, "Mozilla", "Firefox", "profiles.ini"),
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

// decryptChromiumAESGCM decrypts a Chrome v10 encrypted value (Windows scheme)
// using AES-256-GCM. The caller strips the "v10" prefix before passing data.
// Layout: nonce(12) || ciphertext+tag.
func decryptChromiumAESGCM(data, key []byte) (string, error) {
	// ref https://chromium.googlesource.com/chromium/src/+/c3dcd10b9e276234f4bbeafd2c71e282234d54a0/components/os_crypt/sync/os_crypt_win.cc#44
	// kNonceLength = 96/8
	const nonceLen = 12
	if len(data) < nonceLen+16 {
		return "", errors.New("v10 GCM data too short")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plain, err := gcm.Open(nil, data[:nonceLen], data[nonceLen:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// dpapiDecrypt decrypts a DPAPI-encrypted blob using CryptUnprotectData.
func dpapiDecrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, errors.New("empty ciphertext")
	}
	input := windows.DataBlob{
		Size: uint32(len(ciphertext)),
		Data: &ciphertext[0],
	}
	var output windows.DataBlob
	if err := windows.CryptUnprotectData(&input, nil, nil, 0, nil, 0, &output); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data)))
	result := make([]byte, output.Size)
	copy(result, unsafe.Slice(output.Data, output.Size))
	return result, nil
}

// readLocalStateKey reads and DPAPI-decrypts the AES master key stored in a
// Chromium Local State file. The result is the 32-byte AES-256 key used to
// decrypt v10 GCM cookies.
func readLocalStateKey(localStatePath string) ([]byte, error) {
	data, err := os.ReadFile(localStatePath)
	if err != nil {
		return nil, err
	}
	var state struct {
		OSCrypt struct {
			// ref https://chromium.googlesource.com/chromium/src/+/c3dcd10b9e276234f4bbeafd2c71e282234d54a0/components/os_crypt/sync/os_crypt_win.cc#34
			// kOsCryptEncryptedKeyPrefName = "os_crypt.encrypted_key"
			EncryptedKey string `json:"encrypted_key"`
		} `json:"os_crypt"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.OSCrypt.EncryptedKey == "" {
		return nil, errors.New("no encrypted_key in Local State")
	}
	keyBlob, err := base64.StdEncoding.DecodeString(state.OSCrypt.EncryptedKey)
	if err != nil {
		return nil, err
	}
	// ref https://chromium.googlesource.com/chromium/src/+/c3dcd10b9e276234f4bbeafd2c71e282234d54a0/components/os_crypt/sync/os_crypt_win.cc#50
	// kDPAPIKeyPrefix
	const dpapiPrefix = "DPAPI"
	if len(keyBlob) < len(dpapiPrefix) || string(keyBlob[:len(dpapiPrefix)]) != dpapiPrefix {
		return nil, errors.New("encrypted_key missing DPAPI prefix")
	}
	return dpapiDecrypt(keyBlob[len(dpapiPrefix):])
}

// getWindowsChromiumDecrypt returns a decrypt function for a Chrome/Chromium
// profile on Windows. Cookies are encrypted with AES-256-GCM using a master
// key stored in Local State (itself DPAPI-encrypted). Older cookies (pre-
// Chrome 80) are raw DPAPI blobs with no version prefix.
func getWindowsChromiumDecrypt(localStatePath string) func([]byte) (string, error) {
	var cachedKey []byte
	var keyErr error
	keyLoaded := false

	return func(encrypted []byte) (string, error) {
		// ref https://chromium.googlesource.com/chromium/src/+/c3dcd10b9e276234f4bbeafd2c71e282234d54a0/components/os_crypt/sync/os_crypt_win.cc#47
		// kEncryptionVersionPrefix
		if len(encrypted) >= 3 && string(encrypted[:3]) == "v10" {
			if !keyLoaded {
				cachedKey, keyErr = readLocalStateKey(localStatePath)
				keyLoaded = true
			}
			if keyErr != nil {
				return "", fmt.Errorf("Local State key: %w", keyErr)
			}
			return decryptChromiumAESGCM(encrypted[3:], cachedKey)
		}

		// Pre-Chrome 80: raw DPAPI blob.
		plain, err := dpapiDecrypt(encrypted)
		if err != nil {
			return "", err
		}
		return string(plain), nil
	}
}

// chromiumSources returns all Chromium-family cookie databases to search on Windows.
func chromiumSources() []chromiumSource {
	localAppData := os.Getenv("LOCALAPPDATA")
	appData := os.Getenv("APPDATA")

	type browserDef struct {
		label       string
		userDataDir string
	}
	browsers := []browserDef{
		{"Google Chrome", filepath.Join(localAppData, "Google", "Chrome", "User Data")},
		{"Chromium", filepath.Join(localAppData, "Chromium", "User Data")},
		{"Microsoft Edge", filepath.Join(localAppData, "Microsoft", "Edge", "User Data")},
		{"Brave", filepath.Join(localAppData, "BraveSoftware", "Brave-Browser", "User Data")},
		{"Opera", filepath.Join(appData, "Opera Software", "Opera Stable")},
		{"Opera GX", filepath.Join(appData, "Opera Software", "Opera GX Stable")},
	}

	var sources []chromiumSource
	for _, b := range browsers {
		localState := filepath.Join(b.userDataDir, "Local State")
		decrypt := getWindowsChromiumDecrypt(localState)
		for _, profileDir := range chromiumProfileDirs(b.userDataDir) {
			base := filepath.Base(profileDir)
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
