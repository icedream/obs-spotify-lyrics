# Build targets for the lyrics server binary and OBS plugin across platforms.
# Suitable for local use and CI. All outputs land in the project root.
#
# Common targets:
#   make build-binary-linux-amd64        lyrics server binary, Linux/amd64
#   make build-binary-windows-amd64      lyrics server binary, Windows/amd64
#   make build-plugin-linux-amd64        OBS plugin .so, Linux/amd64 (native CGO)
#   make build-plugin-linux-arm64        OBS plugin .so, Linux/arm64 (native CGO)
#   make build-plugin-windows-amd64      OBS plugin .dll, Windows/amd64 (MinGW cross-compile)
#   make clean                           clean all build outputs
#   make clean-plugin-windows-amd64      clean only Windows plugin outputs (incl. OBS SDK)

VERSION     ?= $(shell git describe --always --tags 2>/dev/null || echo dev)
# Four-part numeric version for PE resources: v0.3.3-9-g<sha> becomes 0.3.3.9.
# Anything that is not a plain semver tag (shallow clone, rc tag) yields
# 0.0.0.0 so that goversioninfo and makensis still get a valid version.
PE_VERSION  ?= $(shell (git describe --always --tags --long 2>/dev/null || echo 0.0.0-0) | \
	sed 's/^v//; s/-g.*$$//' | \
	awk -F'-' '{ if ($$1 ~ /^[0-9]+\.[0-9]+\.[0-9]+$$/) printf "%s.%d", $$1, $$2; else printf "0.0.0.0" }')
OBS_VERSION ?= 33.0.2
# Oldest OBS this build is supported on. OBS 32.0.2 and newer refuse to load a
# plugin built against a newer SDK, so this is the floor enforced by
# obs_module_load; keep it at or below OBS_VERSION.
OBS_MIN_VERSION ?= 33.0.0
GITHUB_REPO ?= icedream/obs-spotify-lyrics
MAKENSIS    ?= makensis

# Set UPX= to the upx binary (e.g. make UPX=upx ...) to enable optional
# compression of standalone binaries after stripping. Disabled by default.
# Plugin .so/.dll files are intentionally excluded: shared libraries loaded via
# dlopen/LoadLibrary are incompatible with UPX's entry-point wrapping.
UPX ?=

OBS_WIN_SRC := obs-src
OBS_WIN_SDK := obs-win-sdk
OBS_LINUX_SDK := obs-linux-sdk
# OBS names the Ubuntu release in its Linux .deb assets, and not every OBS
# release publishes the same set. They are tried in this order: the oldest one
# available is the safest to link against on a GitHub runner.
OBS_UBUNTU_RELEASES ?= 24.04 26.04 22.04
OBS_LINUX_ARCH ?= x86_64

.PHONY: all
all: build-binary-linux-amd64 build-binary-linux-arm64 build-binary-windows-amd64 build-binary-windows-arm64 build-plugin-linux-amd64 build-plugin-linux-arm64 build-plugin-windows-amd64 build-installer-windows-amd64

###############################################################################
# Binary

.PHONY: build-binary-linux-amd64
build-binary-linux-amd64:
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build \
		-trimpath \
		-ldflags "-s -w -X main.version=$(VERSION)" \
		-o lyrics-linux-amd64 ./cmd/lyrics/
	$(if $(UPX),$(UPX) --lzma lyrics-linux-amd64)

.PHONY: build-binary-linux-arm64
build-binary-linux-arm64:
	GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build \
		-trimpath \
		-ldflags "-s -w -X main.version=$(VERSION)" \
		-o lyrics-linux-arm64 ./cmd/lyrics/
	$(if $(UPX),$(UPX) --lzma lyrics-linux-arm64)

# FORCE: the version in these resources comes from git describe, which no file
# prerequisite can express.
.PHONY: FORCE
FORCE:

cmd/lyrics/gen_resource_windows_amd64.syso cmd/lyrics/gen_resource_windows_arm64.syso &: cmd/lyrics/versioninfo.json assets/icon.ico Makefile FORCE
	go tool goversioninfo -64      -file-version "$(PE_VERSION)" -product-version "$(PE_VERSION)" -icon assets/icon.ico -o cmd/lyrics/gen_resource_windows_amd64.syso cmd/lyrics/versioninfo.json
	go tool goversioninfo -64 -arm -file-version "$(PE_VERSION)" -product-version "$(PE_VERSION)" -icon assets/icon.ico -o cmd/lyrics/gen_resource_windows_arm64.syso cmd/lyrics/versioninfo.json

.PHONY: build-binary-windows-amd64
build-binary-windows-amd64: cmd/lyrics/gen_resource_windows_amd64.syso
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build \
		-trimpath \
		-ldflags "-s -w -X main.version=$(VERSION)" \
		-o lyrics-windows-amd64.exe ./cmd/lyrics/
	$(if $(UPX),$(UPX) --lzma lyrics-windows-amd64.exe)

.PHONY: build-binary-windows-arm64
build-binary-windows-arm64: cmd/lyrics/gen_resource_windows_arm64.syso
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build \
		-trimpath \
		-ldflags "-s -w -X main.version=$(VERSION)" \
		-o lyrics-windows-arm64.exe ./cmd/lyrics/

###############################################################################
# OBS plugin

.PHONY: build-plugin-linux-amd64
build-plugin-linux-amd64: $(OBS_LINUX_SDK)/.stamp-$(OBS_VERSION)
	$(MAKE) -C plugin \
		PLUGIN_SO="$(CURDIR)/spotify-lyrics-linux-amd64.so" \
		VERSION="$(VERSION)" \
		OBS_VERSION="$(OBS_VERSION)" \
		OBS_MIN_VERSION="$(OBS_MIN_VERSION)" \
		GITHUB_REPO="$(GITHUB_REPO)" \
		CGO_CFLAGS="-I$(CURDIR)/$(OBS_LINUX_SDK)/usr/local/include/obs" \
		CGO_LDFLAGS="-L$(CURDIR)/$(OBS_LINUX_SDK)/usr/local/lib/$(OBS_LINUX_ARCH)-linux-gnu -lobs -lobs-frontend-api -Wl,--no-undefined"

.PHONY: build-plugin-linux-arm64
build-plugin-linux-arm64:
	$(MAKE) -C plugin \
		PLUGIN_SO="$(CURDIR)/spotify-lyrics-linux-arm64.so" \
		VERSION="$(VERSION)" \
		OBS_VERSION="$(OBS_VERSION)" \
		OBS_MIN_VERSION="$(OBS_MIN_VERSION)" \
		GITHUB_REPO="$(GITHUB_REPO)"

# The stamp is named after OBS_VERSION: the checkout and the SDK on disk belong
# to one OBS release, so a version change has to re-fetch rather than reuse.
$(OBS_WIN_SRC)/.stamp-$(OBS_VERSION):
	rm -rf $(OBS_WIN_SRC)
	git clone --filter=blob:none --no-checkout --depth=1 \
		--branch "$(OBS_VERSION)" \
		https://github.com/obsproject/obs-studio.git $(OBS_WIN_SRC)
	cd $(OBS_WIN_SRC) && \
		git sparse-checkout init --cone && \
		git sparse-checkout set libobs frontend/api && \
		git checkout
	# obsconfig.h is generated by CMake at build time; produce a minimal
	# Windows-safe version from the in-tree template.
	sed \
		-e 's|^#cmakedefine \([^ ]*\).*|/* #undef \1 */|' \
		-e 's|@OBS_RELEASE_CANDIDATE@|0|g' \
		-e 's|@OBS_BETA@|0|g' \
		$(OBS_WIN_SRC)/libobs/obsconfig.h.in > $(OBS_WIN_SRC)/libobs/obsconfig.h
	touch $@

$(OBS_WIN_SDK)/.stamp-$(OBS_VERSION):
	rm -rf $(OBS_WIN_SDK) obs-win-tmp obs-win.zip
	wget -q \
		"https://github.com/obsproject/obs-studio/releases/download/$(OBS_VERSION)/OBS-Studio-$(OBS_VERSION)-Windows-x64.zip" \
		-O obs-win.zip
	unzip -q obs-win.zip -d obs-win-tmp
	mkdir -p $(OBS_WIN_SDK)
	find obs-win-tmp -name obs.dll             -exec cp {} $(OBS_WIN_SDK)/obs.dll \;
	find obs-win-tmp -name obs-frontend-api.dll -exec cp {} $(OBS_WIN_SDK)/obs-frontend-api.dll \;
	rm -rf obs-win-tmp obs-win.zip
	cd $(OBS_WIN_SDK) && \
		gendef obs.dll && \
		x86_64-w64-mingw32-dlltool -d obs.def             -l libobs.dll.a && \
		gendef obs-frontend-api.dll && \
		x86_64-w64-mingw32-dlltool -d obs-frontend-api.def -l libobs-frontend-api.dll.a
	touch $@

# OBS publishes an Ubuntu .deb per release that carries the libobs headers and
# the linkable libobs.so, so the Linux plugin can be linked against the very
# OBS release OBS_VERSION names instead of whatever libobs-dev the runner ships.
# --no-undefined is what makes that pin mean something: a shared object links
# fine with OBS symbols missing from its libobs unless the linker is told to
# refuse them.
$(OBS_LINUX_SDK)/.stamp-$(OBS_VERSION):
	rm -rf $(OBS_LINUX_SDK) obs-linux-tmp
	mkdir -p obs-linux-tmp $(OBS_LINUX_SDK)
	asset=""; \
	for ubuntu in $(OBS_UBUNTU_RELEASES); do \
		name="OBS-Studio-$(OBS_VERSION)-Ubuntu-$$ubuntu-$(OBS_LINUX_ARCH).deb"; \
		if curl -fsSL --retry 2 -o "obs-linux-tmp/$$name" \
			"https://github.com/obsproject/obs-studio/releases/download/$(OBS_VERSION)/$$name"; then \
			asset="$$name"; break; \
		fi; \
		rm -f "obs-linux-tmp/$$name"; \
	done; \
	test -n "$$asset" || { \
		echo "OBS $(OBS_VERSION) publishes no $(OBS_LINUX_ARCH) Ubuntu .deb (tried: $(OBS_UBUNTU_RELEASES))" >&2; \
		exit 1; \
	}; \
	echo "using $$asset"; \
	data=$$(ar t "obs-linux-tmp/$$asset" | grep '^data\.tar\.'); \
	ar x "obs-linux-tmp/$$asset" "$$data"; \
	tar xf "$$data" -C $(OBS_LINUX_SDK) --no-wildcards-match-slash --wildcards \
		'./usr/local/include/obs/*' \
		'./usr/local/lib/*/libobs.so*' \
		'./usr/local/lib/*/libobs-frontend-api.so*'; \
	rm -rf obs-linux-tmp "$$data"; \
	touch $@

plugin/gen_resource_windows_amd64.syso &: plugin/versioninfo.json assets/icon.ico Makefile FORCE
	go tool goversioninfo -64 -file-version "$(PE_VERSION)" -product-version "$(PE_VERSION)" -icon assets/icon.ico -o plugin/gen_resource_windows_amd64.syso plugin/versioninfo.json

.PHONY: build-plugin-windows-amd64
build-plugin-windows-amd64: $(OBS_WIN_SRC)/.stamp-$(OBS_VERSION) $(OBS_WIN_SDK)/.stamp-$(OBS_VERSION) plugin/gen_resource_windows_amd64.syso
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc \
		CGO_CFLAGS="-I$(CURDIR)/$(OBS_WIN_SRC)/libobs -I$(CURDIR)/$(OBS_WIN_SRC)/frontend/api" \
		CGO_LDFLAGS="-L$(CURDIR)/$(OBS_WIN_SDK) -lobs -lobs-frontend-api -Wl,--no-undefined" \
		go build -buildmode=c-shared \
		-trimpath \
		-ldflags "-s -w -X main.pluginVersion=$(VERSION) -X main.obsSDKVersion=$(OBS_VERSION) -X main.obsMinVersion=$(OBS_MIN_VERSION) -X main.githubReleasesURL=https://api.github.com/repos/$(GITHUB_REPO)/releases" \
		-o spotify-lyrics-windows-amd64.dll ./plugin/

.PHONY: build-installer-windows-amd64
build-installer-windows-amd64: build-binary-windows-amd64 build-plugin-windows-amd64
	$(MAKENSIS) -DVERSION=$(VERSION) -DPE_VERSION=$(PE_VERSION) installer/spotify-lyrics.nsi

.PHONY: package-installer-windows-amd64
package-installer-windows-amd64:
	$(MAKENSIS) -DVERSION=$(VERSION) -DPE_VERSION=$(PE_VERSION) installer/spotify-lyrics.nsi

###############################################################################
# Checks

# The OBS version is written down three times. A workflow env left behind would
# build the plugin against a different SDK than the release advertises.
.PHONY: verify-obs-version
verify-obs-version:
	@fail=0; \
	for f in .github/workflows/ci.yml .github/workflows/release.yml; do \
		got=$$(grep -m1 'OBS_VERSION:' $$f | tr -d '"' | sed 's/.*OBS_VERSION:[[:space:]]*//'); \
		if [ "$$got" != "$(OBS_VERSION)" ]; then \
			echo "$$f says OBS_VERSION $$got, Makefile says $(OBS_VERSION)" >&2; fail=1; \
		fi; \
	done; \
	if [ "$$(printf '%s\n%s\n' '$(OBS_MIN_VERSION)' '$(OBS_VERSION)' | sort -V | head -1)" != "$(OBS_MIN_VERSION)" ]; then \
		echo "OBS_MIN_VERSION $(OBS_MIN_VERSION) is above OBS_VERSION $(OBS_VERSION)" >&2; fail=1; \
	fi; \
	if [ $$fail -ne 0 ]; then exit 1; fi; \
	echo "OBS_VERSION $(OBS_VERSION) with floor $(OBS_MIN_VERSION): consistent"

###############################################################################
# Cleanup
.PHONY: clean
clean: clean-binary-linux-amd64 clean-binary-linux-arm64 \
       clean-binary-windows-amd64 clean-binary-windows-arm64 \
       clean-plugin-linux-amd64 clean-plugin-linux-arm64 \
       clean-plugin-windows-amd64 clean-installer-windows-amd64

.PHONY: clean-binary-linux-amd64
clean-binary-linux-amd64:
	rm -f lyrics-linux-amd64

.PHONY: clean-binary-linux-arm64
clean-binary-linux-arm64:
	rm -f lyrics-linux-arm64

.PHONY: clean-binary-windows-amd64
clean-binary-windows-amd64:
	rm -f lyrics-windows-amd64.exe cmd/lyrics/gen_resource_windows_amd64.syso

.PHONY: clean-binary-windows-arm64
clean-binary-windows-arm64:
	rm -f lyrics-windows-arm64.exe cmd/lyrics/gen_resource_windows_arm64.syso

.PHONY: clean-plugin-linux-amd64
clean-plugin-linux-amd64:
	rm -f spotify-lyrics-linux-amd64.so spotify-lyrics-linux-amd64.h

.PHONY: clean-plugin-linux-arm64
clean-plugin-linux-arm64:
	rm -f spotify-lyrics-linux-arm64.so spotify-lyrics-linux-arm64.h

.PHONY: clean-plugin-windows-amd64
clean-plugin-windows-amd64:
	rm -f spotify-lyrics-windows-amd64.dll spotify-lyrics-windows-amd64.h
	rm -f plugin/gen_resource_windows_amd64.syso
	rm -rf $(OBS_WIN_SRC) $(OBS_WIN_SDK) $(OBS_LINUX_SDK) obs-win.zip obs-win-tmp obs-linux-tmp

.PHONY: clean-installer-windows-amd64
clean-installer-windows-amd64:
	rm -f obs-spotify-lyrics-*-setup.exe
