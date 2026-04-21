package browser

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"errors"

	"golang.org/x/crypto/pbkdf2"
)

// chromiumKeyFromPassword derives a 16-byte AES key from a Chrome/Chromium
// Safe Storage password using PBKDF2-SHA1 with the fixed salt "saltysalt" and
// a single iteration, matching Chrome's non-standard scheme on Linux/macOS.
func chromiumKeyFromPassword(password string) []byte {
	// ref https://chromium.googlesource.com/chromium/src/+/c3dcd10b9e276234f4bbeafd2c71e282234d54a0/components/os_crypt/sync/os_crypt_linux.cc#43
	// kSalt="saltysalt", iterations=1, kDerivedKeyBytes=16
	return pbkdf2.Key([]byte(password), []byte("saltysalt"), 1, 16, sha1.New)
}

// decryptChromiumV10CBC decrypts a Chrome v10/v11 encrypted value using
// AES-128-CBC. The caller strips the "v10"/"v11" prefix before passing data.
// Chrome uses a fixed IV of 16 space bytes and PKCS7 padding.
func decryptChromiumV10CBC(data, key []byte) (string, error) {
	if len(key) != 16 {
		return "", errors.New("v10 CBC requires a 16-byte key")
	}
	if len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return "", errors.New("invalid ciphertext length for AES-CBC")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	// ref https://chromium.googlesource.com/chromium/src/+/c3dcd10b9e276234f4bbeafd2c71e282234d54a0/components/os_crypt/sync/os_crypt_linux.cc#58
	// kIv: 16 space (0x20) bytes
	iv := bytes.Repeat([]byte{' '}, aes.BlockSize)
	mode := cipher.NewCBCDecrypter(block, iv)
	plain := make([]byte, len(data))
	mode.CryptBlocks(plain, data)

	// PKCS7 unpad.
	pad := int(plain[len(plain)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(plain) {
		return "", errors.New("invalid PKCS7 padding")
	}
	return string(plain[:len(plain)-pad]), nil
}
