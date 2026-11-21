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

1. Tag the commit with an annotated, signed tag whose message is the release
   notes: `git tag -s v<VERSION>`. The release body is generated from that
   message, so write it for users: plain points under `What's Changed`, without
   the `by <author> in <PR>` entries or `fix(scope):` prefixes that the changelog
   link already covers.
2. Push the tag: `git push origin v<VERSION>`. Tag refs are protected once the
   tag exists, so the message cannot be edited afterwards. Correct the release
   body on the release page instead.
3. The release CI workflow runs automatically and publishes the release with
   signed assets. It appends the OBS support window, a `Full Changelog` compare
   link and the `obs-compat` marker to the body.

---

## Release lines and OBS SDK bumps

A plugin build is refused by any OBS older than the SDK it was built against, so
bumping `OBS_VERSION` in the Makefile ends support for the older OBS line.

- Cut the maintenance branch from the **parent** of the bump commit the same day
  the bump lands: `git branch maintenance/obs-32 <bump>^`. Only fixes get
  cherry-picked onto it. CI also runs on pushes to `maintenance/**` and on pull
  requests targeting it, so those branches build exactly like main.
- Keep the maintenance line's versions below the current line's so semver order
  stays meaningful: `v0.3.x` there, `v0.4.x` and up on the current branch.
- Publish the maintenance release that carries the version-aware updater
  **before** the first release built against the new SDK. An older plugin asks
  GitHub for `releases/latest`, which cannot tell OBS versions apart, and would
  install a plugin the host then refuses to load.
- While that transition runs `release.yml` publishes the current line with
  `PRE_RELEASE: "true"`: a pre-release is downloadable but never auto-offered.
  Flip it to `"false"` once enough installs are version-aware.
- Every release advertises its OBS window twice, both generated from the
  Makefile: an `obs-compat` comment in the release body, which the updater
  reads, and a `plugin-compat.json` asset covered by `SHA256SUMS` and cosign.
- The Windows installer compares the OBS Studio it found against
  `OBS_MIN_VERSION` and refuses an older one, so installing a release meant for
  another line fails with a message instead of leaving a plugin that never
  loads.
- `make verify-obs-version` fails when the Makefile and the two workflows
  disagree about `OBS_VERSION`, or when `OBS_MIN_VERSION` sits above it.

---

## Yanking a Release

If a release must be pulled due to a critical bug, **do not simply delete it**. The in-app update checker only suggests upgrades (newer semver), so if a user is on the yanked version and it becomes unavailable without a replacement, they will never be prompted to update.

The correct procedure:

1. Tag a new release at a higher version (e.g. `v0.2.1` to replace a bad `v0.2.0`) containing the fix or revert.
2. Push the new tag and let CI publish it.
3. Only then remove or mark the bad release as a pre-release/draft on GitHub to suppress it from `/releases/latest`.

This ensures users on the broken version are always offered a path forward.
