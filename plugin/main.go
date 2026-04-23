package main

/*
#cgo linux CFLAGS: -I/usr/include/obs
#cgo linux LDFLAGS: -lobs

#include <obs-module.h>
#include <obs-frontend-api.h>
#include <util/platform.h>

// Callback typedefs (using uintptr_t for the data/type_data arg)

typedef char*              (*get_name_t)            (uintptr_t type_data);
typedef uintptr_t          (*source_create_t)       (obs_data_t *settings, obs_source_t *source);
typedef void               (*destroy_t)             (uintptr_t data);
typedef obs_properties_t * (*get_properties_t)      (uintptr_t data);
typedef void               (*get_defaults_t)        (obs_data_t *settings);
typedef void               (*update_t)              (uintptr_t data, obs_data_t *settings);
typedef void               (*video_render_t)        (uintptr_t data, gs_effect_t *effect);
typedef uint32_t           (*get_size_t)            (uintptr_t data);
typedef void               (*source_enum_sources_t) (uintptr_t data, obs_source_enum_proc_t enum_cb, void *param);

// Function pointer wrapper for cgo calling, see helpers.c
void call_enum_proc(obs_source_enum_proc_t proc, obs_source_t *parent, obs_source_t *child, void *param);

// Lyrics source

extern char *              source_get_name            (uintptr_t type_data);
extern uintptr_t           source_create              (obs_data_t *settings, obs_source_t *source);
extern void                source_destroy             (uintptr_t data);
extern obs_properties_t *  source_get_props           (uintptr_t data);
extern void                source_get_defaults        (obs_data_t *settings);
extern void                source_update              (uintptr_t data, obs_data_t *settings);
extern void                source_video_render        (uintptr_t data, gs_effect_t *effect);
extern uint32_t            source_get_width           (uintptr_t data);
extern uint32_t            source_get_height          (uintptr_t data);
extern void                source_activate            (uintptr_t data);
extern void                source_deactivate          (uintptr_t data);
extern void                source_show                (uintptr_t data);
extern void                source_hide                (uintptr_t data);
extern void                source_enum_active_sources (uintptr_t data, obs_source_enum_proc_t enum_cb, void *param);

// Config dummy source

extern char *              dummy_get_name      (uintptr_t type_data);
extern uintptr_t           dummy_create        (obs_data_t *settings, obs_source_t *source);
extern void                dummy_destroy       (uintptr_t data);
extern obs_properties_t *  dummy_get_props     (uintptr_t data);
extern void                dummy_get_defaults  (obs_data_t *settings);
extern void                dummy_update        (uintptr_t data, obs_data_t *settings);

// Frontend

extern void frontend_event_cb        (enum obs_frontend_event event, uintptr_t data);
extern void frontend_cb              (uintptr_t data);

// Property callbacks

extern bool mode_changed_cb            (obs_properties_t *props, obs_property_t *prop, obs_data_t *settings);
extern bool source_css_mode_changed_cb (obs_properties_t *props, obs_property_t *prop, obs_data_t *settings);
extern bool source_reload_cb           (obs_properties_t *props, obs_property_t *prop, void *data);
extern bool check_update_cb            (obs_properties_t *props, obs_property_t *prop, void *data);

// C helper declared in helpers.c

void blog_string(int log_level, const char *string);
*/
import "C"

import (
	"runtime"
	"unsafe"

	"github.com/icedream/obs-spotify-lyrics/internal/logger"
)

var obsModulePointer *C.obs_module_t

//export obs_module_set_pointer
func obs_module_set_pointer(module *C.obs_module_t) {
	obsModulePointer = module
}

//export obs_current_module
func obs_current_module() *C.obs_module_t {
	return obsModulePointer
}

//export obs_module_ver
func obs_module_ver() C.uint32_t {
	return C.LIBOBS_API_VER
}

// Package-level C string constants.
var (
	sourceIDStr   = C.CString("spotify-lyrics")
	sourceNameStr = C.CString("Spotify Lyrics")
	dummyIDStr    = C.CString("spotify-lyrics-config")
	frontMenuStr  = C.CString("Spotify Lyrics")
)

// pluginVersion is set at build time via -ldflags.
var pluginVersion = "dev"

// OBS log levels as Go ints, initialised from C constants to avoid hardcoding.
var (
	logLevelDebug = int(C.LOG_DEBUG)
	logLevelInfo  = int(C.LOG_INFO)
	logLevelWarn  = int(C.LOG_WARNING)
	logLevelError = int(C.LOG_ERROR)
)

//export obs_module_load
func obs_module_load() C.bool {
	logger.Set(&obsLogger{})
	logger.Infof("version: %s, go: %s", pluginVersion, runtime.Version())

	C.obs_register_source_s(&C.struct_obs_source_info{
		id:                  sourceIDStr,
		_type:               C.OBS_SOURCE_TYPE_INPUT,
		icon_type:           C.OBS_ICON_TYPE_BROWSER,
		output_flags:        C.OBS_SOURCE_VIDEO | C.OBS_SOURCE_CUSTOM_DRAW,
		get_name:            C.get_name_t(unsafe.Pointer(C.source_get_name)),
		create:              C.source_create_t(unsafe.Pointer(C.source_create)),
		destroy:             C.destroy_t(unsafe.Pointer(C.source_destroy)),
		get_properties:      C.get_properties_t(unsafe.Pointer(C.source_get_props)),
		get_defaults:        C.get_defaults_t(unsafe.Pointer(C.source_get_defaults)),
		update:              C.update_t(unsafe.Pointer(C.source_update)),
		video_render:        C.video_render_t(unsafe.Pointer(C.source_video_render)),
		get_width:           C.get_size_t(unsafe.Pointer(C.source_get_width)),
		get_height:          C.get_size_t(unsafe.Pointer(C.source_get_height)),
		activate:            C.destroy_t(unsafe.Pointer(C.source_activate)),
		deactivate:          C.destroy_t(unsafe.Pointer(C.source_deactivate)),
		show:                C.destroy_t(unsafe.Pointer(C.source_show)),
		hide:                C.destroy_t(unsafe.Pointer(C.source_hide)),
		enum_active_sources: C.source_enum_sources_t(unsafe.Pointer(C.source_enum_active_sources)),
	}, C.sizeof_struct_obs_source_info) //nolint:gocritic // CGo sizeof false positive

	C.obs_register_source_s(&C.struct_obs_source_info{
		id:             dummyIDStr,
		_type:          C.OBS_SOURCE_TYPE_FILTER,
		output_flags:   C.OBS_SOURCE_CAP_DISABLED,
		get_name:       C.get_name_t(unsafe.Pointer(C.dummy_get_name)),
		create:         C.source_create_t(unsafe.Pointer(C.dummy_create)),
		destroy:        C.destroy_t(unsafe.Pointer(C.dummy_destroy)),
		get_properties: C.get_properties_t(unsafe.Pointer(C.dummy_get_props)),
		get_defaults:   C.get_defaults_t(unsafe.Pointer(C.dummy_get_defaults)),
		update:         C.update_t(unsafe.Pointer(C.dummy_update)),
	}, C.sizeof_struct_obs_source_info) //nolint:gocritic // CGo sizeof false positive

	C.obs_frontend_add_event_callback(
		C.obs_frontend_event_cb(unsafe.Pointer(C.frontend_event_cb)), nil)

	return true
}

func blog(level int, msg string) {
	cs := C.CString(msg)
	defer C.free(unsafe.Pointer(cs))
	C.blog_string(C.int(level), cs)
}

func main() {}
