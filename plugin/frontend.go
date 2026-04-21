package main

/*
#include <obs-module.h>
#include <obs-frontend-api.h>
#include <util/platform.h>
#include <stdlib.h>

void blog_string(int log_level, const char *string);

extern void frontend_cb     (uintptr_t data);
extern bool mode_changed_cb (obs_properties_t *props, obs_property_t *prop, obs_data_t *settings);
*/
import "C"

import (
	"fmt"
	"runtime/cgo"
	"sync"
	"unsafe"
)

/* Config */

const (
	modeInternal = "internal"
	modeExternal = "external"
)

type pluginConfig struct {
	Mode        string
	Port        int
	SpDC        string
	DeviceID    string
	ExternalURL string
}

func defaultConfig() pluginConfig {
	return pluginConfig{Mode: modeInternal, Port: 0}
}

var (
	cfgMu sync.Mutex
	cfg   = defaultConfig()
)

/* Dummy source (config page) */

var dummySource *C.obs_source_t // accessed only from OBS main thread

//export dummy_get_name
func dummy_get_name(_ C.uintptr_t) *C.char {
	return C.CString("Spotify Lyrics Config")
}

// dummy_create returns a non-zero handle so OBS calls get_properties.
//
//export dummy_create
func dummy_create(settings *C.obs_data_t, source *C.obs_source_t) C.uintptr_t {
	_ = settings
	_ = source
	return C.uintptr_t(cgo.NewHandle(&struct{}{}))
}

//export dummy_destroy
func dummy_destroy(data C.uintptr_t) {
	cgo.Handle(data).Delete()
	dummySource = nil
}

//export dummy_get_defaults
func dummy_get_defaults(settings *C.obs_data_t) {
	C.obs_data_set_default_string(settings, C.CString("mode"), C.CString(modeInternal))
	C.obs_data_set_default_int(settings, C.CString("port"), 0)
	C.obs_data_set_default_string(settings, C.CString("sp_dc"), C.CString(""))
	C.obs_data_set_default_string(settings, C.CString("device_id"), C.CString(""))
	C.obs_data_set_default_string(settings, C.CString("external_url"), C.CString(""))
}

//export dummy_get_props
func dummy_get_props(_ C.uintptr_t) *C.obs_properties_t {
	props := C.obs_properties_create()
	C.obs_properties_set_flags(props, C.OBS_PROPERTIES_DEFER_UPDATE)

	modeList := C.obs_properties_add_list(
		props,
		C.CString("mode"),
		C.CString("Mode"),
		C.OBS_COMBO_TYPE_LIST,
		C.OBS_COMBO_FORMAT_STRING,
	)
	C.obs_property_list_add_string(modeList, C.CString("Internal server"), C.CString(modeInternal))
	C.obs_property_list_add_string(modeList, C.CString("External server"), C.CString(modeExternal))
	C.obs_property_set_modified_callback(modeList, C.obs_property_modified_t(unsafe.Pointer(C.mode_changed_cb)))

	C.obs_properties_add_int(props, C.CString("port"), C.CString("Port (0 = automatic)"), 0, 65535, 1)
	C.obs_properties_add_text(props, C.CString("sp_dc"), C.CString("sp_dc cookie (auto-discover if empty)"), C.OBS_TEXT_PASSWORD)
	C.obs_properties_add_text(props, C.CString("device_id"), C.CString("Device ID (random if empty)"), C.OBS_TEXT_DEFAULT)

	srvMu.Lock()
	url := widgetBaseURL
	srvMu.Unlock()
	statusMsg := "Server not running."
	if url != "" {
		statusMsg = fmt.Sprintf("Server running at %s", url)
	}
	p := C.obs_properties_add_text(props, C.CString("status_info"), C.CString(statusMsg), C.OBS_TEXT_INFO)
	C.obs_property_text_set_info_type(p, C.OBS_TEXT_INFO_NORMAL)

	C.obs_properties_add_text(props, C.CString("external_url"), C.CString("External server URL"), C.OBS_TEXT_DEFAULT)

	cfgMu.Lock()
	mode := cfg.Mode
	cfgMu.Unlock()
	updateModeVisibility(props, mode)

	return props
}

//export dummy_update
func dummy_update(_ C.uintptr_t, settings *C.obs_data_t) {
	newMode := C.GoString(C.obs_data_get_string(settings, C.CString("mode")))
	newPort := int(C.obs_data_get_int(settings, C.CString("port")))
	newSpDC := C.GoString(C.obs_data_get_string(settings, C.CString("sp_dc")))
	newDevID := C.GoString(C.obs_data_get_string(settings, C.CString("device_id")))
	newExtURL := C.GoString(C.obs_data_get_string(settings, C.CString("external_url")))

	if newPort < 0 {
		newPort = 0
	}
	if newMode == "" {
		newMode = modeInternal
	}

	cfgMu.Lock()
	cfg = pluginConfig{
		Mode:        newMode,
		Port:        newPort,
		SpDC:        newSpDC,
		DeviceID:    newDevID,
		ExternalURL: newExtURL,
	}
	cfgMu.Unlock()

	// Run off the OBS main thread: server start may block on the system keyring.
	go applyConfig()
}

func applyConfig() {
	cfgMu.Lock()
	c := cfg
	cfgMu.Unlock()

	serverStop()

	switch c.Mode {
	case modeInternal:
		serverStart(c.Port, c.SpDC, c.DeviceID)
	case modeExternal:
		srvMu.Lock()
		widgetBaseURL = c.ExternalURL
		srvMu.Unlock()
	}

	notifySourcesURLChanged()
}

/* Frontend callback */

var (
	frontMenuCS      *C.char
	configFilenameCS = C.CString("config.json")
)

//export frontend_event_cb
func frontend_event_cb(event C.enum_obs_frontend_event, _ C.uintptr_t) {
	switch event {
	case C.OBS_FRONTEND_EVENT_FINISHED_LOADING:
		// Create config directory.
		configDir := C.obs_module_get_config_path(obs_current_module(), nil)
		C.os_mkdirs(configDir)
		C.bfree(unsafe.Pointer(configDir))

		frontMenuCS = C.CString("Spotify Lyrics")
		C.obs_frontend_add_tools_menu_item(
			frontMenuCS,
			C.obs_frontend_cb(unsafe.Pointer(C.frontend_cb)),
			nil,
		)

		// Load any previously saved settings from disk. obs_source_create does
		// NOT call the update callback automatically, so we must call
		// obs_source_update explicitly (matching obs-teleport's pattern).
		configFile := C.obs_module_get_config_path(obs_current_module(), configFilenameCS)
		savedSettings := C.obs_data_create_from_json_file(configFile)
		C.bfree(unsafe.Pointer(configFile))
		if savedSettings == nil {
			// First run, create empty with defaults
			savedSettings = C.obs_data_create()
		}

		dummySource = C.obs_source_create(dummyIDStr, frontMenuCS, nil, nil)
		// Explicitly trigger dummy_update -> go applyConfig() -> server start.
		C.obs_source_update(dummySource, savedSettings)
		C.obs_data_release(savedSettings)

		blog(C.LOG_INFO, "plugin loaded")

	case C.OBS_FRONTEND_EVENT_EXIT:
		if dummySource != nil {
			configFile := C.obs_module_get_config_path(obs_current_module(), configFilenameCS)
			settings := C.obs_source_get_settings(dummySource)
			C.obs_data_save_json(settings, configFile)
			C.obs_data_release(settings)
			C.bfree(unsafe.Pointer(configFile))

			C.obs_source_release(dummySource)
			dummySource = nil
		}
		serverStop()
	}
}

//export frontend_cb
func frontend_cb(_ C.uintptr_t) {
	if dummySource != nil {
		C.obs_frontend_open_source_properties(dummySource)
	}
}

/* Property modified callback */

//export mode_changed_cb
func mode_changed_cb(props *C.obs_properties_t, _ *C.obs_property_t, settings *C.obs_data_t) C.bool {
	mode := C.GoString(C.obs_data_get_string(settings, C.CString("mode")))
	updateModeVisibility(props, mode)
	return true
}

func updateModeVisibility(props *C.obs_properties_t, mode string) {
	internal := mode != modeExternal
	for _, name := range []string{"port", "sp_dc", "device_id", "status_info"} {
		cs := C.CString(name)
		p := C.obs_properties_get(props, cs)
		C.free(unsafe.Pointer(cs))
		if p != nil {
			C.obs_property_set_visible(p, C.bool(internal))
		}
	}
	extCS := C.CString("external_url")
	p := C.obs_properties_get(props, extCS)
	C.free(unsafe.Pointer(extCS))
	if p != nil {
		C.obs_property_set_visible(p, C.bool(!internal))
	}
}
