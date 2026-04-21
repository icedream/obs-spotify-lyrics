package main

/*
#include <obs-module.h>
#include <obs-frontend-api.h>
#include <stdlib.h>
#include <string.h>

void blog_string(int log_level, const char *string);
*/
import "C"

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

/* lyricsSource represents one "Spotify Lyrics" source instance. */

type lyricsSource struct {
	self   *C.obs_source_t
	nested *C.obs_source_t
	mu     sync.Mutex
	width  uint32
	height uint32
	url    string
}

var (
	sourcesMu sync.Mutex
	sources   []*lyricsSource
)

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
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	s := &lyricsSource{
		self:   self,
		width:  1920,
		height: 1080,
	}

	if settings != nil {
		if w := uint32(C.obs_data_get_int(settings, C.CString("width"))); w > 0 {
			s.width = w
		}
		if h := uint32(C.obs_data_get_int(settings, C.CString("height"))); h > 0 {
			s.height = h
		}
	}

	trackSource(s)
	s.applyURL()

	handle := cgoNewHandle(s)
	return C.uintptr_t(handle)
}

//export source_destroy
func source_destroy(data C.uintptr_t) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

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
		return C.obs_source_get_width(s.nested)
	}
	return C.uint32_t(s.width)
}

//export source_get_height
func source_get_height(data C.uintptr_t) C.uint32_t {
	s := cgoHandleValue(data).(*lyricsSource)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nested != nil {
		return C.obs_source_get_height(s.nested)
	}
	return C.uint32_t(s.height)
}

//export source_get_defaults
func source_get_defaults(settings *C.obs_data_t) {
	C.obs_data_set_default_int(settings, C.CString("width"), 1920)
	C.obs_data_set_default_int(settings, C.CString("height"), 1080)
}

//export source_get_props
func source_get_props(data C.uintptr_t) *C.obs_properties_t {
	s := cgoHandleValue(data).(*lyricsSource)
	props := C.obs_properties_create()

	C.obs_properties_add_int(props, C.CString("width"), C.CString("Width"), 1, 7680, 1)
	C.obs_properties_add_int(props, C.CString("height"), C.CString("Height"), 1, 4320, 1)

	s.mu.Lock()
	url := s.url
	s.mu.Unlock()

	infoLabel := "Widget URL: (server not running)"
	if url != "" {
		infoLabel = fmt.Sprintf("Widget URL: %s/", url)
	}
	C.obs_properties_add_text(props, C.CString("url_info"), C.CString(infoLabel), C.OBS_TEXT_DEFAULT)

	return props
}

//export source_update
func source_update(data C.uintptr_t, settings *C.obs_data_t) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	s := cgoHandleValue(data).(*lyricsSource)
	s.mu.Lock()
	defer s.mu.Unlock()

	if w := uint32(C.obs_data_get_int(settings, C.CString("width"))); w > 0 {
		s.width = w
	}
	if h := uint32(C.obs_data_get_int(settings, C.CString("height"))); h > 0 {
		s.height = h
	}

	s.applyURL()
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

	idCS := C.CString("browser_source")
	nameCS := C.CString("spotify-lyrics-browser")
	defer func() {
		C.free(unsafe.Pointer(idCS))
		C.free(unsafe.Pointer(nameCS))
	}()
	s.nested = C.obs_source_create_private(idCS, nameCS, settings)
}

func (s *lyricsSource) destroyNested() {
	if s.nested != nil {
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
