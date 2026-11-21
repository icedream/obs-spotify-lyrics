PLUGIN_SO := spotify-lyrics.so
INSTALL_DIR := $(HOME)/.config/obs-studio/plugins/spotify-lyrics/bin/64bit

VERSION ?= $(shell git -C .. describe --always --tags 2>/dev/null || echo dev)

# Standalone builds get no OBS floor; the root Makefile passes both when it
# drives this one.
OBS_VERSION ?= unknown
OBS_MIN_VERSION ?= 0.0.0

GITHUB_REPO ?= icedream/obs-spotify-lyrics

CGO_CFLAGS  := -I/usr/include/obs
CGO_LDFLAGS := -lobs

.PHONY: all FORCE
all: $(PLUGIN_SO)

# FORCE: the OBS SDK and the version strings arrive as build flags rather than
# as files, so an artifact already on disk may well be built against an older
# SDK or an older version. Relinking is cheap; a stale plugin is not.
$(PLUGIN_SO): FORCE
	CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" \
		go build -buildmode=c-shared \
		-trimpath \
		-ldflags="-s -w -X main.pluginVersion=$(VERSION) -X main.obsSDKVersion=$(OBS_VERSION) -X main.obsMinVersion=$(OBS_MIN_VERSION) -X main.githubReleasesURL=https://api.github.com/repos/$(GITHUB_REPO)/releases" \
		-o $@ .

.PHONY: install
install: $(PLUGIN_SO)
	mkdir -p "$(INSTALL_DIR)"
	cp $(PLUGIN_SO) "$(INSTALL_DIR)/"

.PHONY: clean
clean:
	rm -f $(PLUGIN_SO) $(PLUGIN_SO:.so=.h)
