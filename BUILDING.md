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
