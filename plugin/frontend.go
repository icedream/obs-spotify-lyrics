package main

/*
#include <obs-module.h>
#include <obs-frontend-api.h>
#include <util/platform.h>
#include <stdlib.h>
#include <string.h>

void blog_string(int log_level, const char *string);

extern void frontend_cb     (uintptr_t data);
extern bool mode_changed_cb (obs_properties_t *props, obs_property_t *prop, obs_data_t *settings);
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"
)

/* Config */

const (
	modeInternal = "internal"
	modeExternal = "external"
)

type pluginConfig struct {
	Mode        string `json:"mode"`
	Port        int    `json:"port"`
	SpDC        string `json:"sp_dc"`
	DeviceID    string `json:"device_id"`
	ExternalURL string `json:"external_url"`
}

func defaultConfig() pluginConfig {
	return pluginConfig{Mode: modeInternal, Port: 8080}
}

var (
	cfgMu  sync.Mutex
	cfg    = defaultConfig()
	cfgPath string
)

func configPath() string {
	cs := C.obs_module_get_config_path(obs_current_module(), C.CString("config.json"))
	if cs == nil {
		return ""
	}
	defer C.bfree(unsafe.Pointer(cs))
	return C.GoString(cs)
}

func loadConfig() {
	path := configPath()
	if path == "" {
		return
	}
	cfgPath = path
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	cfgMu.Lock()
	defer cfgMu.Unlock()
	parsed := defaultConfig()
	if err := json.Unmarshal(data, &parsed); err != nil {
		return
	}
	cfg = parsed
}

func saveConfig() {
	cfgMu.Lock()
	current := cfg
	cfgMu.Unlock()
	if cfgPath == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(cfgPath), 0o755)
	data, err := json.Marshal(current)
	if err != nil {
		return
	}
	_ = os.WriteFile(cfgPath, data, 0o600)
}

/* Dummy source (config page) */

var (
	dummySource *C.obs_source_t
	dummyMu     sync.Mutex
)

//export dummy_get_name
func dummy_get_name(_ C.uintptr_t) *C.char {
	return C.CString("Spotify Lyrics Config")
}

//export dummy_create
func dummy_create(_ *C.obs_data_t, source *C.obs_source_t) C.uintptr_t {
	dummyMu.Lock()
	if dummySource == nil {
		dummySource = source
	}
	dummyMu.Unlock()
	return 0
}

//export dummy_destroy
func dummy_destroy(_ C.uintptr_t) {
	dummyMu.Lock()
	dummySource = nil
	dummyMu.Unlock()
}

//export dummy_get_defaults
func dummy_get_defaults(settings *C.obs_data_t) {
	cfgMu.Lock()
	c := cfg
	cfgMu.Unlock()

	mode := C.CString(c.Mode)
	spdc := C.CString(c.SpDC)
	devid := C.CString(c.DeviceID)
	exturl := C.CString(c.ExternalURL)
	defer func() {
		C.free(unsafe.Pointer(mode))
		C.free(unsafe.Pointer(spdc))
		C.free(unsafe.Pointer(devid))
		C.free(unsafe.Pointer(exturl))
	}()

	C.obs_data_set_default_string(settings, C.CString("mode"), mode)
	C.obs_data_set_default_int(settings, C.CString("port"), C.longlong(c.Port))
	C.obs_data_set_default_string(settings, C.CString("sp_dc"), spdc)
	C.obs_data_set_default_string(settings, C.CString("device_id"), devid)
	C.obs_data_set_default_string(settings, C.CString("external_url"), exturl)
}

//export dummy_get_props
func dummy_get_props(_ C.uintptr_t) *C.obs_properties_t {
	props := C.obs_properties_create()

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

	C.obs_properties_add_int(props, C.CString("port"), C.CString("Port"), 1, 65535, 1)
	C.obs_properties_add_text(props, C.CString("sp_dc"), C.CString("sp_dc cookie (auto-discover if empty)"), C.OBS_TEXT_PASSWORD)
	C.obs_properties_add_text(props, C.CString("device_id"), C.CString("Device ID (random if empty)"), C.OBS_TEXT_DEFAULT)

	srvMu.Lock()
	url := widgetBaseURL
	srvMu.Unlock()
	statusLabel := "Server not running"
	if url != "" {
		statusLabel = "Server running on " + url + "/"
	}
	C.obs_properties_add_text(props, C.CString("status_info"), C.CString(statusLabel), C.OBS_TEXT_DEFAULT)

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

	if newPort <= 0 {
		newPort = 8080
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

	applyConfig()
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

var frontMenuCS *C.char

//export frontend_event_cb
func frontend_event_cb(event C.enum_obs_frontend_event, _ C.uintptr_t) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	switch event {
	case C.OBS_FRONTEND_EVENT_FINISHED_LOADING:
		loadConfig()
		applyConfig()

		frontMenuCS = C.CString("Spotify Lyrics")
		C.obs_frontend_add_tools_menu_item(
			frontMenuCS,
			C.obs_frontend_cb(unsafe.Pointer(C.frontend_cb)),
			nil,
		)

		dummyMu.Lock()
		if dummySource == nil {
			cs1 := C.CString("spotify-lyrics-config")
			cs2 := C.CString("spotify-lyrics-config-instance")
			dummySource = C.obs_source_create_private(cs1, cs2, nil)
			C.free(unsafe.Pointer(cs1))
			C.free(unsafe.Pointer(cs2))
		}
		dummyMu.Unlock()

		blog(C.LOG_INFO, fmt.Sprintf("plugin loaded (mode=%s)", cfg.Mode))

	case C.OBS_FRONTEND_EVENT_EXIT:
		serverStop()
		saveConfig()
	}
}

//export frontend_cb
func frontend_cb(_ C.uintptr_t) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	dummyMu.Lock()
	src := dummySource
	dummyMu.Unlock()
	if src != nil {
		C.obs_frontend_open_source_properties(src)
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
