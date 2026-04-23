PLUGIN_SO := spotify-lyrics.so
INSTALL_DIR := $(HOME)/.config/obs-studio/plugins/spotify-lyrics/bin/64bit

VERSION ?= $(shell git -C .. describe --always --tags 2>/dev/null || echo dev)

GITHUB_REPO ?= icedream/obs-spotify-lyrics

CGO_CFLAGS  := -I/usr/include/obs
CGO_LDFLAGS := -lobs

.PHONY: all
all: $(PLUGIN_SO)

$(PLUGIN_SO):
	CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" \
		go build -buildmode=c-shared \
		-trimpath \
		-ldflags="-s -w -X main.pluginVersion=$(VERSION) -X main.githubReleasesURL=https://api.github.com/repos/$(GITHUB_REPO)/releases/latest" \
		-o $@ .

.PHONY: install
install: $(PLUGIN_SO)
	mkdir -p "$(INSTALL_DIR)"
	cp $(PLUGIN_SO) "$(INSTALL_DIR)/"

.PHONY: clean
clean:
	rm -f $(PLUGIN_SO) $(PLUGIN_SO:.so=.h)
