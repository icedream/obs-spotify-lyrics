# Generates derived assets from source SVGs.
#
# Requires:
#   rsvg-convert  (librsvg)
#   magick        (ImageMagick 7+)

ICON_SIZES = 16 24 32 48 64 128 256
BUILDDIR   = .build

.PHONY: all clean

all: icon.ico icon-install.ico icon-uninstall.ico installer-sidebar.bmp installer-header.bmp

$(BUILDDIR):
	mkdir -p $@

# Pattern rules: more specific (shorter stem) takes precedence, so
# icon-install-%.png and icon-uninstall-%.png beat icon-%.png for their files.
$(BUILDDIR)/icon-%.png: icon-app.svg | $(BUILDDIR)
	rsvg-convert -w $* -h $* $< -o $@

$(BUILDDIR)/icon-install-%.png: icon-install.svg | $(BUILDDIR)
	rsvg-convert -w $* -h $* $< -o $@

$(BUILDDIR)/icon-uninstall-%.png: icon-uninstall.svg | $(BUILDDIR)
	rsvg-convert -w $* -h $* $< -o $@

icon.ico: $(ICON_SIZES:%=$(BUILDDIR)/icon-%.png)
	magick $^ $@

icon-install.ico: $(ICON_SIZES:%=$(BUILDDIR)/icon-install-%.png)
	magick $^ $@

icon-uninstall.ico: $(ICON_SIZES:%=$(BUILDDIR)/icon-uninstall-%.png)
	magick $^ $@

installer-sidebar.bmp: icon-app.svg | $(BUILDDIR)
	rsvg-convert -w 120 -h 120 $< -o $(BUILDDIR)/icon-120.png
	magick -size 164x314 gradient:'#0e1a0e-#0a1a14' \
	    $(BUILDDIR)/icon-120.png -gravity Center -geometry +0-20 -composite \
	    -alpha off BMP3:$@

installer-header.bmp: icon-app.svg | $(BUILDDIR)
	rsvg-convert -w 44 -h 44 $< -o $(BUILDDIR)/icon-44.png
	magick -size 150x57 gradient:'#0e1a0e-#0a1a14' \
	    $(BUILDDIR)/icon-44.png -gravity East -geometry +12+0 -composite \
	    -alpha off BMP3:$@

clean:
	rm -rf $(BUILDDIR) icon.ico icon-install.ico icon-uninstall.ico installer-sidebar.bmp installer-header.bmp
