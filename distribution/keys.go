// Package distribution provides the embedded release signing public key.
package distribution

import _ "embed"

// SigningKeyASC is the armored GPG public key used to sign release assets.
// Update this file by running: gpg --armor --export <FINGERPRINT> > distribution/signing-key.asc
//
//go:embed signing-key.asc
var SigningKeyASC []byte
