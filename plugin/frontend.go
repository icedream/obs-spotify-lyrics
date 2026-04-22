package main

/*
#include <obs-module.h>
#include <obs-frontend-api.h>
#include <util/platform.h>
#include <stdlib.h>

extern void frontend_cb          (uintptr_t data);
extern void open_props_on_ui_thread(uintptr_t data);
extern bool mode_changed_cb      (obs_properties_t *props, obs_property_t *prop, obs_data_t *settings);

// Dispatches open_props_on_ui_thread to run on the OBS UI thread.
// No source pointer is passed; the callback reads dummySource on the main
// thread where it is safe to access.
static void open_props_task_wrapper(void *param) {
	open_props_on_ui_thread((uintptr_t)param);
}
static void schedule_open_source_properties(void) {
	obs_queue_task(OBS_TASK_UI, open_props_task_wrapper, (void*)0, false);
}
*/
import "C"

import (
	"fmt"
	"runtime/cgo"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/icedream/spotify-lyrics-widget/internal/logger"
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

var (
	dummySourceMu    sync.RWMutex
	dummySource      *C.obs_source_t
	pendingPropsOpen int32 // atomic; 1 = open-properties task already queued
)

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
	dummySourceMu.Lock()
	dummySource = nil
	dummySourceMu.Unlock()
}

// open_props_on_ui_thread is queued via obs_queue_task(OBS_TASK_UI) so it runs
// on the OBS main thread. It opens the plugin config dialog only if the server
// is still in an error state (avoids spurious opens if the user fixed config
// between the task being queued and it firing).
//
//export open_props_on_ui_thread
func open_props_on_ui_thread(_ C.uintptr_t) {
	atomic.StoreInt32(&pendingPropsOpen, 0)

	srvMu.Lock()
	hasError := serverLastError != ""
	srvMu.Unlock()

	if !hasError {
		return
	}

	dummySourceMu.RLock()
	s := dummySource
	dummySourceMu.RUnlock()
	if s != nil {
		C.obs_frontend_open_source_properties(s)
	}
}

//export dummy_get_defaults
func dummy_get_defaults(settings *C.obs_data_t) {
	modeKeyCS := C.CString("mode")
	defer C.free(unsafe.Pointer(modeKeyCS))
	modeValCS := C.CString(modeInternal)
	defer C.free(unsafe.Pointer(modeValCS))
	portKeyCS := C.CString("port")
	defer C.free(unsafe.Pointer(portKeyCS))
	spDCKeyCS := C.CString("sp_dc")
	defer C.free(unsafe.Pointer(spDCKeyCS))
	spDCValCS := C.CString("")
	defer C.free(unsafe.Pointer(spDCValCS))
	deviceIDKeyCS := C.CString("device_id")
	defer C.free(unsafe.Pointer(deviceIDKeyCS))
	deviceIDValCS := C.CString("")
	defer C.free(unsafe.Pointer(deviceIDValCS))
	extURLKeyCS := C.CString("external_url")
	defer C.free(unsafe.Pointer(extURLKeyCS))
	extURLValCS := C.CString("")
	defer C.free(unsafe.Pointer(extURLValCS))
	C.obs_data_set_default_string(settings, modeKeyCS, modeValCS)
	C.obs_data_set_default_int(settings, portKeyCS, 0)
	C.obs_data_set_default_string(settings, spDCKeyCS, spDCValCS)
	C.obs_data_set_default_string(settings, deviceIDKeyCS, deviceIDValCS)
	C.obs_data_set_default_string(settings, extURLKeyCS, extURLValCS)
}

//export dummy_get_props
func dummy_get_props(_ C.uintptr_t) *C.obs_properties_t {
	props := C.obs_properties_create()
	C.obs_properties_set_flags(props, C.OBS_PROPERTIES_DEFER_UPDATE)

	// Webserver group
	wsProps := C.obs_properties_create()
	modeKeyCS, modeLabelCS := C.CString("mode"), C.CString("Mode")
	modeList := C.obs_properties_add_list(
		wsProps,
		modeKeyCS,
		modeLabelCS,
		C.OBS_COMBO_TYPE_LIST,
		C.OBS_COMBO_FORMAT_STRING,
	)
	C.free(unsafe.Pointer(modeKeyCS))
	C.free(unsafe.Pointer(modeLabelCS))
	internalLabelCS, internalValCS := C.CString("Internal server"), C.CString(modeInternal)
	C.obs_property_list_add_string(modeList, internalLabelCS, internalValCS)
	C.free(unsafe.Pointer(internalLabelCS))
	C.free(unsafe.Pointer(internalValCS))
	externalLabelCS, externalValCS := C.CString("External server"), C.CString(modeExternal)
	C.obs_property_list_add_string(modeList, externalLabelCS, externalValCS)
	C.free(unsafe.Pointer(externalLabelCS))
	C.free(unsafe.Pointer(externalValCS))
	C.obs_property_set_modified_callback(modeList, C.obs_property_modified_t(unsafe.Pointer(C.mode_changed_cb)))

	portKeyCS, portLabelCS := C.CString("port"), C.CString("Port (0 = automatic)")
	C.obs_properties_add_int(wsProps, portKeyCS, portLabelCS, 0, 65535, 1)
	C.free(unsafe.Pointer(portKeyCS))
	C.free(unsafe.Pointer(portLabelCS))

	srvMu.Lock()
	url := widgetBaseURL
	lastErr := serverLastError
	srvMu.Unlock()
	var statusMsg string
	var infoType C.enum_obs_text_info_type
	switch {
	case lastErr != "":
		statusMsg = lastErr
		infoType = C.OBS_TEXT_INFO_WARNING
	case url != "":
		statusMsg = fmt.Sprintf("Server running at %s", url)
		infoType = C.OBS_TEXT_INFO_NORMAL
	default:
		statusMsg = "Server not running."
		infoType = C.OBS_TEXT_INFO_NORMAL
	}
	statusInfoKeyCS, statusInfoLabelCS := C.CString("status_info"), C.CString(statusMsg)
	p := C.obs_properties_add_text(wsProps, statusInfoKeyCS, statusInfoLabelCS, C.OBS_TEXT_INFO)
	C.free(unsafe.Pointer(statusInfoKeyCS))
	C.free(unsafe.Pointer(statusInfoLabelCS))
	C.obs_property_text_set_info_type(p, infoType)

	extURLKeyCS, extURLLabelCS := C.CString("external_url"), C.CString("External server URL")
	C.obs_properties_add_text(wsProps, extURLKeyCS, extURLLabelCS, C.OBS_TEXT_DEFAULT)
	C.free(unsafe.Pointer(extURLKeyCS))
	C.free(unsafe.Pointer(extURLLabelCS))

	wsGrpKeyCS, wsGrpLabelCS := C.CString("grp_webserver"), C.CString("Webserver")
	C.obs_properties_add_group(props, wsGrpKeyCS, wsGrpLabelCS, C.OBS_GROUP_NORMAL, wsProps)
	C.free(unsafe.Pointer(wsGrpKeyCS))
	C.free(unsafe.Pointer(wsGrpLabelCS))

	// Spotify group
	spotifyProps := C.obs_properties_create()
	spDCKeyCS, spDCLabelCS := C.CString("sp_dc"), C.CString("sp_dc cookie (auto-detect if empty)")
	C.obs_properties_add_text(spotifyProps, spDCKeyCS, spDCLabelCS, C.OBS_TEXT_PASSWORD)
	C.free(unsafe.Pointer(spDCKeyCS))
	C.free(unsafe.Pointer(spDCLabelCS))
	deviceIDKeyCS, deviceIDLabelCS := C.CString("device_id"), C.CString("Device ID (random if empty)")
	C.obs_properties_add_text(spotifyProps, deviceIDKeyCS, deviceIDLabelCS, C.OBS_TEXT_DEFAULT)
	C.free(unsafe.Pointer(deviceIDKeyCS))
	C.free(unsafe.Pointer(deviceIDLabelCS))
	spotifyGrpKeyCS, spotifyGrpLabelCS := C.CString("grp_spotify"), C.CString("Spotify")
	C.obs_properties_add_group(props, spotifyGrpKeyCS, spotifyGrpLabelCS, C.OBS_GROUP_NORMAL, spotifyProps)
	C.free(unsafe.Pointer(spotifyGrpKeyCS))
	C.free(unsafe.Pointer(spotifyGrpLabelCS))

	// About (flat, at the bottom)
	projURLKeyCS := C.CString("project_url")
	projURLLabelCS := C.CString(`<a href="https://github.com/icedream/obs-spotify-lyrics"><b>Spotify Lyrics for OBS</b></a> · ` + pluginVersion)
	p = C.obs_properties_add_text(props, projURLKeyCS, projURLLabelCS, C.OBS_TEXT_INFO)
	C.free(unsafe.Pointer(projURLKeyCS))
	C.free(unsafe.Pointer(projURLLabelCS))
	C.obs_property_text_set_info_type(p, C.OBS_TEXT_INFO_NORMAL)

	cfgMu.Lock()
	mode := cfg.Mode
	cfgMu.Unlock()
	updateModeVisibility(props, mode)

	return props
}

//export dummy_update
func dummy_update(_ C.uintptr_t, settings *C.obs_data_t) {
	modeKeyCS := C.CString("mode")
	defer C.free(unsafe.Pointer(modeKeyCS))
	portKeyCS := C.CString("port")
	defer C.free(unsafe.Pointer(portKeyCS))
	spDCKeyCS := C.CString("sp_dc")
	defer C.free(unsafe.Pointer(spDCKeyCS))
	deviceIDKeyCS := C.CString("device_id")
	defer C.free(unsafe.Pointer(deviceIDKeyCS))
	extURLKeyCS := C.CString("external_url")
	defer C.free(unsafe.Pointer(extURLKeyCS))
	newMode := C.GoString(C.obs_data_get_string(settings, modeKeyCS))
	newPort := int(C.obs_data_get_int(settings, portKeyCS))
	newSpDC := C.GoString(C.obs_data_get_string(settings, spDCKeyCS))
	newDevID := C.GoString(C.obs_data_get_string(settings, deviceIDKeyCS))
	newExtURL := C.GoString(C.obs_data_get_string(settings, extURLKeyCS))

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
		if err := serverStart(c.Port, c.SpDC, c.DeviceID); err != nil {
			// Open the config dialog on the UI thread so the user sees the
			// error and can supply their sp_dc cookie. Guard with an atomic
			// flag so rapid reconfigurations only queue one open.
			if atomic.CompareAndSwapInt32(&pendingPropsOpen, 0, 1) {
				C.schedule_open_source_properties()
			}
		}
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

		dummySourceMu.Lock()
		dummySource = C.obs_source_create(dummyIDStr, frontMenuCS, nil, nil)
		dummySourceMu.Unlock()
		// Explicitly trigger dummy_update -> go applyConfig() -> server start.
		C.obs_source_update(dummySource, savedSettings)
		C.obs_data_release(savedSettings)

		logger.Info("plugin loaded")

	case C.OBS_FRONTEND_EVENT_EXIT:
		dummySourceMu.RLock()
		s := dummySource
		dummySourceMu.RUnlock()
		if s != nil {
			configFile := C.obs_module_get_config_path(obs_current_module(), configFilenameCS)
			settings := C.obs_source_get_settings(s)
			C.obs_data_save_json(settings, configFile)
			C.obs_data_release(settings)
			C.bfree(unsafe.Pointer(configFile))

			C.obs_source_release(s)
			dummySourceMu.Lock()
			dummySource = nil
			dummySourceMu.Unlock()
		}
		serverStop()
	}
}

//export frontend_cb
func frontend_cb(_ C.uintptr_t) {
	dummySourceMu.RLock()
	s := dummySource
	dummySourceMu.RUnlock()
	if s != nil {
		C.obs_frontend_open_source_properties(s)
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

	// Hide the entire Spotify group when using an external server.
	grpSpotifyCS := C.CString("grp_spotify")
	if g := C.obs_properties_get(props, grpSpotifyCS); g != nil {
		C.obs_property_set_visible(g, C.bool(internal))
	}
	C.free(unsafe.Pointer(grpSpotifyCS))

	for _, name := range []string{"port", "status_info"} {
		cs := C.CString(name)
		if p := C.obs_properties_get(props, cs); p != nil {
			C.obs_property_set_visible(p, C.bool(internal))
		}
		C.free(unsafe.Pointer(cs))
	}
	extCS := C.CString("external_url")
	if p := C.obs_properties_get(props, extCS); p != nil {
		C.obs_property_set_visible(p, C.bool(!internal))
	}
	C.free(unsafe.Pointer(extCS))
}
