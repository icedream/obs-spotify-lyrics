package main

/*
#include <obs-module.h>
#include <obs-frontend-api.h>
#include <stdlib.h>
#include <string.h>

void blog_string(int log_level, const char *string);
void call_enum_proc(obs_source_enum_proc_t proc, obs_source_t *parent, obs_source_t *child, void *param);

extern bool source_css_mode_changed_cb(obs_properties_t *props, obs_property_t *prop, obs_data_t *settings);
*/
import "C"

import (
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"github.com/icedream/spotify-lyrics-widget/internal/obscolor"
	"github.com/icedream/spotify-lyrics-widget/internal/widget"
)

/* lyricsSource represents one "Spotify Lyrics" source instance. */

type lyricsSource struct {
	self      *C.obs_source_t
	nested    *C.obs_source_t
	mu        sync.Mutex
	width     uint32
	height    uint32
	url       string
	isActive  bool
	isShowing bool
	cssMode   string // "simple" | "advanced"
	customCSS string // rendered CSS injected into the nested browser_source
}

var (
	sourcesMu sync.Mutex
	sources   []*lyricsSource
)

/* CSS variable definitions are parsed from the :root block in widget.html
   at init time via widget.CSSVars. See internal/widget/widget.go. */

// cssGroupKey converts a group display name to a stable OBS settings key.
// e.g. "Active line" -> "css_group_active_line"
func cssGroupKey(group string) string {
	return "css_group_" + strings.ReplaceAll(strings.ToLower(group), " ", "_")
}

// OBS font flags (from obs-properties.h).
const (
	obsFontBold      = uint32(1 << 0)
	obsFontItalic    = uint32(1 << 1)
	obsFontUnderline = uint32(1 << 2)
	obsFontStrikeout = uint32(1 << 3)
)

// fontPickerDefault holds hardcoded defaults for each font picker property.
type fontPickerDefault struct {
	face  string
	size  int
	flags uint32
}

var fontDefaults = map[string]fontPickerDefault{
	"css_current_font":  {"Segoe UI", 24, obsFontBold},
	"css_adjacent_font": {"Segoe UI", 16, 0},
}

// buildCSSFromSettings builds the CSS string to inject into the nested browser_source.
func buildCSSFromSettings(settings *C.obs_data_t) string {
	modeCS := C.CString("css_mode")
	mode := C.GoString(C.obs_data_get_string(settings, modeCS))
	C.free(unsafe.Pointer(modeCS))

	if mode == "advanced" {
		advCS := C.CString("css_advanced")
		css := C.GoString(C.obs_data_get_string(settings, advCS))
		C.free(unsafe.Pointer(advCS))
		return css
	}

	var sb strings.Builder
	sb.WriteString(":root {\n")
	for _, v := range widget.CSSVars {
		keyCS := C.CString(v.Key)
		if v.Type == "font" {
			prefix := strings.TrimSuffix(v.Prop, "-font")
			fontObj := C.obs_data_get_obj(settings, keyCS)
			C.free(unsafe.Pointer(keyCS))

			var face string
			var size int
			var flags uint32
			if fontObj != nil {
				faceKeyCS := C.CString("face")
				sizeKeyCS := C.CString("size")
				flagsKeyCS := C.CString("flags")
				face = C.GoString(C.obs_data_get_string(fontObj, faceKeyCS))
				size = int(C.obs_data_get_int(fontObj, sizeKeyCS))
				flags = uint32(C.obs_data_get_int(fontObj, flagsKeyCS))
				C.free(unsafe.Pointer(faceKeyCS))
				C.free(unsafe.Pointer(sizeKeyCS))
				C.free(unsafe.Pointer(flagsKeyCS))
				C.obs_data_release(fontObj)
			}
			if def, ok := fontDefaults[v.Key]; ok {
				if face == "" {
					face = def.face
				}
				if size <= 0 {
					size = def.size
				}
				if fontObj == nil {
					flags = def.flags
				}
			}

			weight := 400
			if flags&obsFontBold != 0 {
				weight = 700
			}
			style := "normal"
			if flags&obsFontItalic != 0 {
				style = "italic"
			}
			var decorParts []string
			if flags&obsFontUnderline != 0 {
				decorParts = append(decorParts, "underline")
			}
			if flags&obsFontStrikeout != 0 {
				decorParts = append(decorParts, "line-through")
			}
			decoration := "none"
			if len(decorParts) > 0 {
				decoration = strings.Join(decorParts, " ")
			}
			escapedFace := strings.ReplaceAll(face, `"`, `\"`)
			fmt.Fprintf(&sb, "  %s-family: \"%s\", system-ui, sans-serif;\n", prefix, escapedFace)
			fmt.Fprintf(&sb, "  %s-size: %dpt;\n", prefix, size)
			fmt.Fprintf(&sb, "  %s-weight: %d;\n", prefix, weight)
			fmt.Fprintf(&sb, "  %s-style: %s;\n", prefix, style)
			fmt.Fprintf(&sb, "  %s-decoration: %s;\n", prefix, decoration)
			continue
		}

		var val string
		if v.Type == "color-alpha" {
			raw := uint32(C.obs_data_get_int(settings, keyCS))
			if raw == 0 {
				// Default was never applied; fall back to parsed CSS default.
				if parsed, ok := obscolor.FromCSS(v.DefVal); ok {
					raw = parsed
				}
			}
			val = obscolor.ToCSS(raw)
		} else {
			val = C.GoString(C.obs_data_get_string(settings, keyCS))
			if val == "" {
				val = v.DefVal
			}
		}
		C.free(unsafe.Pointer(keyCS))
		fmt.Fprintf(&sb, "  %s: %s;\n", v.Prop, val)
	}
	sb.WriteString("}\n")

	extraCS := C.CString("css_extra")
	extra := C.GoString(C.obs_data_get_string(settings, extraCS))
	C.free(unsafe.Pointer(extraCS))
	if extra != "" {
		sb.WriteString("\n")
		sb.WriteString(extra)
		sb.WriteString("\n")
	}
	return sb.String()
}

// updateCSSModeVisibility shows or hides property groups based on the selected CSS mode.
func updateCSSModeVisibility(props *C.obs_properties_t, mode string) {
	isSimple := mode != "advanced"
	seenGroups := map[string]bool{}
	for _, v := range widget.CSSVars {
		if v.Group == "" {
			keyCS := C.CString(v.Key)
			if p := C.obs_properties_get(props, keyCS); p != nil {
				C.obs_property_set_visible(p, C.bool(isSimple))
			}
			C.free(unsafe.Pointer(keyCS))
		} else if !seenGroups[v.Group] {
			seenGroups[v.Group] = true
			keyCS := C.CString(cssGroupKey(v.Group))
			if p := C.obs_properties_get(props, keyCS); p != nil {
				C.obs_property_set_visible(p, C.bool(isSimple))
			}
			C.free(unsafe.Pointer(keyCS))
		}
	}
	extraCS := C.CString("css_extra")
	if p := C.obs_properties_get(props, extraCS); p != nil {
		C.obs_property_set_visible(p, C.bool(isSimple))
	}
	C.free(unsafe.Pointer(extraCS))
	advCS := C.CString("css_advanced")
	if p := C.obs_properties_get(props, advCS); p != nil {
		C.obs_property_set_visible(p, C.bool(!isSimple))
	}
	C.free(unsafe.Pointer(advCS))
}

func trackSource(s *lyricsSource) {
	sourcesMu.Lock()
	defer sourcesMu.Unlock()
	sources = append(sources, s)
}

func untrackSource(s *lyricsSource) {
	sourcesMu.Lock()
	defer sourcesMu.Unlock()
	for i, v := range sources {
		if v == s {
			sources = append(sources[:i], sources[i+1:]...)
			break
		}
	}
}

// notifySourcesURLChanged is called from frontend/server code when the widget
// URL changes. It triggers update on all tracked source instances.
func notifySourcesURLChanged() {
	sourcesMu.Lock()
	list := make([]*lyricsSource, len(sources))
	copy(list, sources)
	sourcesMu.Unlock()

	for _, s := range list {
		s.mu.Lock()
		s.applyURL()
		s.mu.Unlock()
	}
}

/* OBS callbacks */

//export source_get_name
func source_get_name(_ C.uintptr_t) *C.char {
	return C.CString("Spotify Lyrics")
}

//export source_create
func source_create(settings *C.obs_data_t, self *C.obs_source_t) C.uintptr_t {
	s := &lyricsSource{
		self:    self,
		width:   1920,
		height:  1080,
		cssMode: "simple",
	}

	if settings != nil {
		if w := uint32(C.obs_data_get_int(settings, C.CString("width"))); w > 0 {
			s.width = w
		}
		if h := uint32(C.obs_data_get_int(settings, C.CString("height"))); h > 0 {
			s.height = h
		}
		modeCS := C.CString("css_mode")
		if m := C.GoString(C.obs_data_get_string(settings, modeCS)); m != "" {
			s.cssMode = m
		}
		C.free(unsafe.Pointer(modeCS))
		s.customCSS = buildCSSFromSettings(settings)
	}

	trackSource(s)
	s.applyURL()

	handle := cgoNewHandle(s)
	return C.uintptr_t(handle)
}

//export source_destroy
func source_destroy(data C.uintptr_t) {
	s := cgoHandleValue(data).(*lyricsSource)
	cgoDeleteHandle(data)
	untrackSource(s)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.destroyNested()
}

//export source_video_render
func source_video_render(data C.uintptr_t, _ *C.gs_effect_t) {
	s := cgoHandleValue(data).(*lyricsSource)
	s.mu.Lock()
	nested := s.nested
	s.mu.Unlock()
	if nested != nil {
		C.obs_source_video_render(nested)
	}
}

//export source_get_width
func source_get_width(data C.uintptr_t) C.uint32_t {
	s := cgoHandleValue(data).(*lyricsSource)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nested != nil {
		if w := C.obs_source_get_width(s.nested); w > 0 {
			return w
		}
	}
	return C.uint32_t(s.width)
}

//export source_get_height
func source_get_height(data C.uintptr_t) C.uint32_t {
	s := cgoHandleValue(data).(*lyricsSource)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nested != nil {
		if h := C.obs_source_get_height(s.nested); h > 0 {
			return h
		}
	}
	return C.uint32_t(s.height)
}

//export source_get_defaults
func source_get_defaults(settings *C.obs_data_t) {
	C.obs_data_set_default_int(settings, C.CString("width"), 1920)
	C.obs_data_set_default_int(settings, C.CString("height"), 1080)
	C.obs_data_set_default_string(settings, C.CString("css_mode"), C.CString("simple"))
	for _, v := range widget.CSSVars {
		keyCS := C.CString(v.Key)
		if v.Type == "color-alpha" {
			if parsed, ok := obscolor.FromCSS(v.DefVal); ok {
				C.obs_data_set_default_int(settings, keyCS, C.longlong(parsed))
			}
		} else if v.Type == "font" {
			def := fontDefaults[v.Key]
			faceKeyCS := C.CString("face")
			sizeKeyCS := C.CString("size")
			flagsKeyCS := C.CString("flags")
			faceValCS := C.CString(def.face)
			fontObj := C.obs_data_create()
			C.obs_data_set_string(fontObj, faceKeyCS, faceValCS)
			C.obs_data_set_int(fontObj, sizeKeyCS, C.longlong(def.size))
			C.obs_data_set_int(fontObj, flagsKeyCS, C.longlong(def.flags))
			C.obs_data_set_default_obj(settings, keyCS, fontObj)
			C.obs_data_release(fontObj)
			C.free(unsafe.Pointer(faceValCS))
			C.free(unsafe.Pointer(faceKeyCS))
			C.free(unsafe.Pointer(sizeKeyCS))
			C.free(unsafe.Pointer(flagsKeyCS))
		} else {
			valCS := C.CString(v.DefVal)
			C.obs_data_set_default_string(settings, keyCS, valCS)
			C.free(unsafe.Pointer(valCS))
		}
		C.free(unsafe.Pointer(keyCS))
	}
}

//export source_get_props
func source_get_props(data C.uintptr_t) *C.obs_properties_t {
	s := cgoHandleValue(data).(*lyricsSource)
	props := C.obs_properties_create()
	// No DEFER_UPDATE here, we want live preview as fields change

	C.obs_properties_add_int(props, C.CString("width"), C.CString("Width"), 1, 7680, 1)
	C.obs_properties_add_int(props, C.CString("height"), C.CString("Height"), 1, 4320, 1)

	s.mu.Lock()
	url := s.url
	cssMode := s.cssMode
	s.mu.Unlock()

	infoLabel := "Server not running."
	if url != "" {
		infoLabel = fmt.Sprintf("Widget URL: %s/", url)
	}
	p := C.obs_properties_add_text(props, C.CString("url_info"), C.CString(infoLabel), C.OBS_TEXT_INFO)
	C.obs_property_text_set_info_type(p, C.OBS_TEXT_INFO_NORMAL)

	/* Style / CSS customization */
	modeList := C.obs_properties_add_list(
		props,
		C.CString("css_mode"),
		C.CString("Style"),
		C.OBS_COMBO_TYPE_LIST,
		C.OBS_COMBO_FORMAT_STRING,
	)
	C.obs_property_list_add_string(modeList, C.CString("Simple (CSS variables)"), C.CString("simple"))
	C.obs_property_list_add_string(modeList, C.CString("Custom CSS"), C.CString("advanced"))
	C.obs_property_set_modified_callback(modeList, C.obs_property_modified_t(unsafe.Pointer(C.source_css_mode_changed_cb)))

	// Add CSS variable fields, creating OBS property groups on first encounter.
	groups := map[string]*C.obs_properties_t{}
	for _, v := range widget.CSSVars {
		var container *C.obs_properties_t
		if v.Group == "" {
			container = props
		} else {
			if g, ok := groups[v.Group]; ok {
				container = g
			} else {
				// First var in this group: create the sub-properties and register it.
				g = C.obs_properties_create()
				groups[v.Group] = g
				groupKeyCS := C.CString(cssGroupKey(v.Group))
				groupLabelCS := C.CString(v.Group)
				C.obs_properties_add_group(props, groupKeyCS, groupLabelCS, C.OBS_GROUP_NORMAL, g)
				C.free(unsafe.Pointer(groupKeyCS))
				C.free(unsafe.Pointer(groupLabelCS))
				container = g
			}
		}
		keyCS := C.CString(v.Key)
		labelCS := C.CString(v.Label)
		if v.Type == "color-alpha" {
			C.obs_properties_add_color_alpha(container, keyCS, labelCS)
		} else if v.Type == "font" {
			C.obs_properties_add_font(container, keyCS, labelCS)
		} else {
			C.obs_properties_add_text(container, keyCS, labelCS, C.OBS_TEXT_DEFAULT)
		}
		C.free(unsafe.Pointer(keyCS))
		C.free(unsafe.Pointer(labelCS))
	}
	C.obs_properties_add_text(props, C.CString("css_extra"), C.CString("Additional CSS"), C.OBS_TEXT_MULTILINE)
	C.obs_properties_add_text(props, C.CString("css_advanced"), C.CString("Custom CSS"), C.OBS_TEXT_MULTILINE)

	if cssMode == "" {
		cssMode = "simple"
	}
	updateCSSModeVisibility(props, cssMode)

	return props
}

//export source_css_mode_changed_cb
func source_css_mode_changed_cb(props *C.obs_properties_t, _ *C.obs_property_t, settings *C.obs_data_t) C.bool {
	modeCS := C.CString("css_mode")
	mode := C.GoString(C.obs_data_get_string(settings, modeCS))
	C.free(unsafe.Pointer(modeCS))
	updateCSSModeVisibility(props, mode)
	return true
}

//export source_activate
func source_activate(data C.uintptr_t) {
	s := cgoHandleValue(data).(*lyricsSource)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isActive = true
	if s.nested != nil {
		C.obs_source_add_active_child(s.self, s.nested)
	}
}

//export source_deactivate
func source_deactivate(data C.uintptr_t) {
	s := cgoHandleValue(data).(*lyricsSource)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isActive = false
	if s.nested != nil {
		C.obs_source_remove_active_child(s.self, s.nested)
	}
}

//export source_show
func source_show(data C.uintptr_t) {
	s := cgoHandleValue(data).(*lyricsSource)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isShowing = true
	if s.nested != nil {
		C.obs_source_inc_showing(s.nested)
	}
}

//export source_hide
func source_hide(data C.uintptr_t) {
	s := cgoHandleValue(data).(*lyricsSource)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isShowing = false
	if s.nested != nil {
		C.obs_source_dec_showing(s.nested)
	}
}

//export source_enum_active_sources
func source_enum_active_sources(data C.uintptr_t, enumCB C.obs_source_enum_proc_t, param unsafe.Pointer) {
	s := cgoHandleValue(data).(*lyricsSource)
	s.mu.Lock()
	nested := s.nested
	self := s.self
	s.mu.Unlock()
	if nested != nil {
		C.call_enum_proc(enumCB, self, nested, param)
	}
}

/* source_update and nested CSS management (called with s.mu held for applyURL) */

//export source_update
func source_update(data C.uintptr_t, settings *C.obs_data_t) {
	s := cgoHandleValue(data).(*lyricsSource)
	s.mu.Lock()

	if w := uint32(C.obs_data_get_int(settings, C.CString("width"))); w > 0 {
		s.width = w
	}
	if h := uint32(C.obs_data_get_int(settings, C.CString("height"))); h > 0 {
		s.height = h
	}

	modeCS := C.CString("css_mode")
	s.cssMode = C.GoString(C.obs_data_get_string(settings, modeCS))
	C.free(unsafe.Pointer(modeCS))
	if s.cssMode == "" {
		s.cssMode = "simple"
	}

	s.customCSS = buildCSSFromSettings(settings)

	prevNested := s.nested
	s.applyURL()

	// Capture what we need for the nested update, then release the mutex
	// before calling obs_source_update to avoid a re-entrancy deadlock
	// (OBS may call source_enum_active_sources synchronously, which locks s.mu).
	nested := s.nested
	doUpdate := nested != nil && nested == prevNested
	css := s.customCSS
	w := s.width
	h := s.height
	s.mu.Unlock()

	if doUpdate {
		applyNestedSettings(nested, css, w, h)
	}
}

// applyNestedSettings pushes css, width, and height to an existing nested
// browser_source. Must be called WITHOUT s.mu held.
func applyNestedSettings(nested *C.obs_source_t, css string, w, h uint32) {
	nsettings := C.obs_data_create()
	defer C.obs_data_release(nsettings)

	cssKeyCS := C.CString("css")
	cssCS := C.CString(css)
	widthCS := C.CString("width")
	heightCS := C.CString("height")
	defer func() {
		C.free(unsafe.Pointer(cssKeyCS))
		C.free(unsafe.Pointer(cssCS))
		C.free(unsafe.Pointer(widthCS))
		C.free(unsafe.Pointer(heightCS))
	}()

	C.obs_data_set_string(nsettings, cssKeyCS, cssCS)
	C.obs_data_set_int(nsettings, widthCS, C.longlong(w))
	C.obs_data_set_int(nsettings, heightCS, C.longlong(h))
	C.obs_source_update(nested, nsettings)
}

/* Nested browser_source management (called with s.mu held) */

func (s *lyricsSource) applyURL() {
	url := currentWidgetURL()
	if url == s.url {
		return
	}
	s.url = url
	s.destroyNested()
	if url == "" {
		return
	}
	if !browserSourceAvailable() {
		blog(C.LOG_WARNING, "browser_source not available, add Browser Source manually with URL: "+url)
		return
	}
	blog(C.LOG_INFO, fmt.Sprintf("creating nested browser_source for %s/", url))
	s.createNested(url + "/")
}

func (s *lyricsSource) createNested(url string) {
	settings := C.obs_data_create()
	defer C.obs_data_release(settings)

	urlCS := C.CString(url)
	shutdownCS := C.CString("shutdown")
	widthCS := C.CString("width")
	heightCS := C.CString("height")
	urlKey := C.CString("url")
	defer func() {
		C.free(unsafe.Pointer(urlCS))
		C.free(unsafe.Pointer(shutdownCS))
		C.free(unsafe.Pointer(widthCS))
		C.free(unsafe.Pointer(heightCS))
		C.free(unsafe.Pointer(urlKey))
	}()

	C.obs_data_set_string(settings, urlKey, urlCS)
	C.obs_data_set_int(settings, widthCS, C.longlong(s.width))
	C.obs_data_set_int(settings, heightCS, C.longlong(s.height))
	C.obs_data_set_bool(settings, shutdownCS, false)

	cssKeyCS := C.CString("css")
	cssCS := C.CString(s.customCSS)
	defer func() {
		C.free(unsafe.Pointer(cssKeyCS))
		C.free(unsafe.Pointer(cssCS))
	}()
	C.obs_data_set_string(settings, cssKeyCS, cssCS)

	idCS := C.CString("browser_source")
	nameCS := C.CString("spotify-lyrics-browser")
	defer func() {
		C.free(unsafe.Pointer(idCS))
		C.free(unsafe.Pointer(nameCS))
	}()
	s.nested = C.obs_source_create_private(idCS, nameCS, settings)
	if s.nested != nil {
		blog(C.LOG_INFO, fmt.Sprintf("nested browser_source created (active=%v showing=%v)", s.isActive, s.isShowing))
		if s.isActive {
			C.obs_source_add_active_child(s.self, s.nested)
		}
		if s.isShowing {
			C.obs_source_inc_showing(s.nested)
		}
	} else {
		blog(C.LOG_WARNING, "obs_source_create_private returned nil for browser_source")
	}
}

func (s *lyricsSource) destroyNested() {
	if s.nested != nil {
		if s.isShowing {
			C.obs_source_dec_showing(s.nested)
		}
		if s.isActive {
			C.obs_source_remove_active_child(s.self, s.nested)
		}
		C.obs_source_release(s.nested)
		s.nested = nil
	}
}

func browserSourceAvailable() bool {
	idCS := C.CString("browser_source")
	defer C.free(unsafe.Pointer(idCS))
	var idx C.size_t
	for {
		var id *C.char
		if !bool(C.obs_enum_source_types(idx, &id)) {
			break
		}
		if C.strcmp(id, idCS) == 0 {
			return true
		}
		idx++
	}
	return false
}

/* cgo handle helpers (simple map-based, avoids runtime/cgo import issues) */

var (
	handleMu  sync.Mutex
	handleMap = map[C.uintptr_t]interface{}{}
	handleSeq C.uintptr_t
)

func cgoNewHandle(v interface{}) C.uintptr_t {
	handleMu.Lock()
	defer handleMu.Unlock()
	handleSeq++
	handleMap[handleSeq] = v
	return handleSeq
}

func cgoHandleValue(h C.uintptr_t) interface{} {
	handleMu.Lock()
	defer handleMu.Unlock()
	return handleMap[h]
}

func cgoDeleteHandle(h C.uintptr_t) {
	handleMu.Lock()
	defer handleMu.Unlock()
	delete(handleMap, h)
}
