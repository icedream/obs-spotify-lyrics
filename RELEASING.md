# Release Guide

This document describes the steps for setting up release signing and publishing a new release of Spotify Lyrics for OBS.

## GPG Signing Key Setup

Releases are signed with a dedicated GPG signing key, certified by the maintainer's personal key to establish a chain of trust. Follow these steps once to set up signing.

### 1. Generate the project signing key

Use ed25519 for the key type. The user ID should clearly identify the key's purpose:

    gpg --quick-gen-key "Your Name (obs-spotify-lyrics release signing) <your@email.com>" ed25519 sign 0

- Replace `Your Name` and `<your@email.com>` with your real name and email.
- The comment `obs-spotify-lyrics release signing` identifies the key's scope.
- `sign 0` creates a signing-only key with no expiry.

Note the fingerprint printed after generation, referred to below as `<FINGERPRINT>`.

### 2. Certify the key with your personal key

Sign the new project key with your personal key to publish that you vouch for it:

    gpg --default-key <YOUR_PERSONAL_KEY_FINGERPRINT> --sign-key <FINGERPRINT>

### 3. Export and commit the public key

Export the certified public key and commit it to the repository:

    gpg --armor --export <FINGERPRINT> > distribution/signing-key.asc
    git add distribution/signing-key.asc
    git commit -m "chore: update release signing public key"

### 4. Upload secrets to GitHub Actions

Export the private key and store it as a GitHub Actions secret named `GPG_SIGNING_KEY`:

    gpg --armor --export-secret-keys <FINGERPRINT>

Copy the full output (including `-----BEGIN PGP PRIVATE KEY BLOCK-----` and the ending line) and paste it as the value of the `GPG_SIGNING_KEY` secret in the repository settings under Settings > Secrets and variables > Actions.

Also store the key passphrase as `GPG_SIGNING_PASSPHRASE`.

---

## Verifying a Release Signature

Download `SHA256SUMS` and `SHA256SUMS.asc` from the release, then import the signing key and verify:

    gpg --import distribution/signing-key.asc
    gpg --verify SHA256SUMS.asc SHA256SUMS

You can also verify individual file checksums:

    sha256sum --check --ignore-missing SHA256SUMS

---

## Publishing a Release

1. Tag the commit: `git tag -s v<VERSION>` (use a signed tag if possible)
2. Push the tag: `git push origin v<VERSION>`
3. The release CI workflow runs automatically and publishes the release with signed assets.
