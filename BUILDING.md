# Building from source

## Prerequisites

- **Go** ≥ 1.26
- **make** (standard GNU Make)

Additional dependencies depend on what you are building:

- native build on Linux: Development headers for libobs
- cross-compile build on Linux for Windows: mingw-w64 + mingw-w64 tools (for `gendef`)

On Ubuntu/Debian, dependencies can be installed like this:

```sh
# Linux plugin
sudo apt-get install libobs-dev

# Windows plugin (cross-compile from Linux)
sudo apt-get install mingw-w64 mingw-w64-tools
```

## Lyrics server binary

Pure Go, no CGO, cross-compiles freely:

```sh
make build-binary-linux-amd64    # build for 64-bit x86
make build-binary-linux-arm64    # build for 64-bit ARM
make build-binary-windows-amd64  # cross-compile for Windows 64-bit x86
make build-binary-windows-arm64  # cross-compile for Windows 64-bit ARM
```

## OBS plugin

### Linux (native, amd64 or arm64)

Run on the target machine (or a matching runner) with `libobs-dev` installed:

```sh
make build-plugin-linux-amd64  # builds 64-bit x86 spotify-lyrics-linux-amd64.so
make build-plugin-linux-arm64  # builds 64-bit ARM spotify-lyrics-linux-arm64.so
```

For rapid iteration you can also build and install directly into your local OBS
plugin directory in one step:

```sh
cd plugin
make install   # builds + copies to ~/.config/obs-studio/plugins/spotify-lyrics/bin/64bit/
```

### Windows (cross-compiled from Linux)

Requires `mingw-w64` and `mingw-w64-tools`. The Makefile will automatically
download the OBS Windows SDK and sparse-checkout the required OBS headers on
first run:

```sh
make build-plugin-windows-amd64  # outputs Windows 64-bit spotify-lyrics-windows-amd64.dll
```

### The OBS SDK pin

`OBS_VERSION` picks the OBS release the plugin is compiled and linked against,
and `OBS_MIN_VERSION` the oldest OBS that build supports. From OBS 32.0.2 the
loader rejects any plugin reporting a libobs API version above the host's, so
the SDK decides the floor: builds pin the oldest OBS you support, not the newest.

`OBS_MIN_VERSION` is also passed to the Windows installer, which compares it
against the OBS Studio it finds and refuses to install into an older one. That
turns a mismatched plugin release into a dialog instead of a plugin that sits
there unloaded.

```sh
make verify-obs-version   # Makefile and both workflows must agree
```

CI builds the shipped plugin against `OBS_VERSION`, and against `OBS_NEWEST` in
`ci.yml` as well when that is a higher release, to show the same artifact still
loads on a newer OBS.

The SDK is fetched per platform. Windows uses the release's `Windows-x64.zip`
plus the headers from the OBS source tag. Linux amd64 uses the release's Ubuntu
`.deb`: its `usr/local/include/obs` and `libobs.so` are exactly what that OBS was
built with, and only about 2MB of the package is extracted. Both legs link with
`-Wl,--no-undefined`, so an OBS symbol the pinned release does not export fails
the link instead of turning into a plugin that loads and then crashes.

OBS publishes no Linux package for arm64, so `build-plugin-linux-arm64` links
against the runner's `libobs-dev`; the `build-plugin` job does the same on
purpose, to keep the distro-package compile path covered. Locally, build against
your own libobs headers with `make -C plugin`.

## Windows version information

The server binaries, the plugin DLL and the installer embed a PE version
resource, so their properties in Explorer show the product name, the company
and the file version. `goversioninfo` generates the resources for the Go builds
from `cmd/lyrics/versioninfo.json` and `plugin/versioninfo.json`, and the
installer sets the same values with `VIProductVersion`.

All of them take the four-part version from `PE_VERSION`, derived from
`git describe`: tag `v0.3.3` plus 9 commits gives `0.3.3.9`. A checkout without
a usable semver tag, for example a shallow clone, falls back to `0.0.0.0`. To
set it explicitly, pass `make PE_VERSION=1.2.3.0 ...`.

## Build everything

```sh
make   # equivalent to make all
```

Builds all binary and plugin targets listed above.

## Clean

```sh
make clean
```

Removes all build outputs and the downloaded Windows SDK files.

> **Gotcha:** do not combine `clean` and a build target in a single `make`
> invocation (e.g. `make clean build-installer-windows-amd64`). Make evaluates
> prerequisite freshness before running any recipes, so it will see the OBS SDK
> stamp files as up-to-date, skip re-downloading them, then `clean` will delete
> the SDK mid-run and the linker will fail with `cannot find -lobs`. Run them as
> two separate commands instead:
>
> ```sh
> make clean
> make build-installer-windows-amd64
> ```

## Tests and linting

```sh
# Unit tests (pure-Go packages only; plugin/ requires OBS CGO headers)
go test ./cmd/... ./internal/...

# Linter (requires libobs-dev for the plugin package to type-check)
golangci-lint run ./...
```
