//go:build windows

// Package progressdlg wraps the Windows IProgressDialog COM interface,
// providing a simple Go API for a native Shell progress dialog.
//
// Usage pattern:
//
//	dlg, _ := progressdlg.New()  // nil on failure: all methods are nil-safe
//	defer dlg.Release()
//	dlg.SetTitle("Downloading...")
//	dlg.Start(0, iprogressdialog.FlagAutoTime)
//	...
//	dlg.Stop()
package progressdlg

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ole32DLL             = windows.NewLazySystemDLL("ole32.dll")
	procCoInitializeEx   = ole32DLL.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32DLL.NewProc("CoUninitialize")
	procCoCreateInstance = ole32DLL.NewProc("CoCreateInstance")
)

const (
	coinitApartmentThreaded uintptr = 0x2
	clsctxInprocServer      uintptr = 0x1
)

var (
	clsidProgressDialog = windows.GUID{
		Data1: 0xF8383852, Data2: 0xFCD3, Data3: 0x11D1,
		Data4: [8]byte{0xA6, 0xB9, 0x00, 0x60, 0x97, 0xDF, 0x5B, 0xD4},
	}
	iidIProgressDialog = windows.GUID{
		Data1: 0xEBBC7C04, Data2: 0x315E, Data3: 0x11D2,
		Data4: [8]byte{0xB6, 0x2F, 0x00, 0x60, 0x97, 0xDF, 0x5B, 0xD4},
	}
)

// Flags for Start. These correspond to PROGDLG_* constants from shlobj.h.
const (
	FlagNormal        uint32 = 0x00000000
	FlagModal         uint32 = 0x00000001
	FlagAutoTime      uint32 = 0x00000002 // show estimated remaining time (requires known total)
	FlagNoTime        uint32 = 0x00000004
	FlagNoMinimize    uint32 = 0x00000008
	FlagNoProgressBar uint32 = 0x00000010
	FlagMarquee       uint32 = 0x00000020 // indeterminate/marquee progress bar
	FlagNoCancel      uint32 = 0x00000040
)

// iProgressDialogVtbl matches the IProgressDialog COM vtable layout from shlobj.h.
// Method order after IUnknown (QI, AddRef, Release):
//
//	StartProgressDialog, StopProgressDialog, SetTitle, SetAnimation,
//	HasUserCancelled, SetProgress, SetProgress64, SetLine, SetCancelMsg, Timer
type iProgressDialogVtbl struct {
	QueryInterface      uintptr
	AddRef              uintptr
	Release             uintptr
	StartProgressDialog uintptr
	StopProgressDialog  uintptr
	SetTitle            uintptr
	SetAnimation        uintptr
	HasUserCancelled    uintptr
	SetProgress         uintptr
	SetProgress64       uintptr
	SetLine             uintptr
	SetCancelMsg        uintptr
	Timer               uintptr
}

type rawIProgressDialog struct {
	vtbl *iProgressDialogVtbl
}

// Dialog wraps a Windows IProgressDialog COM object. All methods are safe to
// call on a nil receiver (they silently no-op).
type Dialog struct {
	raw       *rawIProgressDialog
	comInited bool // whether CoInitializeEx succeeded (and needs matching CoUninitialize)
	started   bool // whether StartProgressDialog has been called
}

// New creates a Windows IProgressDialog COM object. The calling goroutine is
// locked to its OS thread and COM is initialised as STA; call Release when done
// to undo both.
//
// Returns nil (not an error) when the dialog cannot be created so callers can
// proceed without a progress UI - all methods on a nil *Dialog are no-ops.
func New() (*Dialog, error) {
	runtime.LockOSThread()

	hr, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	// S_OK (0): freshly initialised; S_FALSE (1): already STA on this thread.
	// Any other code (e.g. RPC_E_CHANGED_MODE) means we cannot use STA here.
	comInited := hr == 0 || hr == 1
	if !comInited {
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("CoInitializeEx: 0x%08X", uint32(hr))
	}

	var raw *rawIProgressDialog
	hr, _, _ = procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidProgressDialog)),
		0,
		clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidIProgressDialog)),
		uintptr(unsafe.Pointer(&raw)),
	)
	if hr != 0 {
		procCoUninitialize.Call() //nolint:errcheck
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("CoCreateInstance(IProgressDialog): 0x%08X", uint32(hr))
	}

	return &Dialog{raw: raw, comInited: comInited}, nil
}

// Release stops the dialog (if still visible), releases the COM object,
// uninitialises COM if this instance initialised it, and unlocks the OS thread.
// Safe to call on a nil *Dialog. Idempotent.
func (d *Dialog) Release() {
	if d == nil || d.raw == nil {
		return
	}
	if d.started {
		d.Stop()
	}
	syscall.SyscallN(d.raw.vtbl.Release, uintptr(unsafe.Pointer(d.raw))) //nolint:errcheck
	d.raw = nil
	if d.comInited {
		procCoUninitialize.Call() //nolint:errcheck
		d.comInited = false
	}
	runtime.UnlockOSThread()
}

// Start shows the progress dialog. hwnd is the optional parent window handle
// (0 for no parent). flags should be a combination of the Flag* constants.
func (d *Dialog) Start(hwnd uintptr, flags uint32) error {
	if d == nil || d.raw == nil {
		return nil
	}
	hr, _, _ := syscall.SyscallN(d.raw.vtbl.StartProgressDialog,
		uintptr(unsafe.Pointer(d.raw)),
		hwnd,
		0, // punkEnableModeless
		uintptr(flags),
		0, // pvReserved
	)
	if hr != 0 {
		return fmt.Errorf("IProgressDialog.StartProgressDialog: 0x%08X", uint32(hr))
	}
	d.started = true
	return nil
}

// Stop hides the progress dialog. Safe to call on a nil *Dialog.
func (d *Dialog) Stop() {
	if d == nil || d.raw == nil || !d.started {
		return
	}
	syscall.SyscallN(d.raw.vtbl.StopProgressDialog, uintptr(unsafe.Pointer(d.raw))) //nolint:errcheck
	d.started = false
}

// SetTitle sets the dialog title. Safe to call on a nil *Dialog.
func (d *Dialog) SetTitle(title string) error {
	if d == nil || d.raw == nil {
		return nil
	}
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return err
	}
	hr, _, _ := syscall.SyscallN(d.raw.vtbl.SetTitle,
		uintptr(unsafe.Pointer(d.raw)),
		uintptr(unsafe.Pointer(titlePtr)),
	)
	if hr != 0 {
		return fmt.Errorf("IProgressDialog.SetTitle: 0x%08X", uint32(hr))
	}
	return nil
}

// SetCancelMsg sets the message shown while the operation is being cancelled.
// Safe to call on a nil *Dialog.
func (d *Dialog) SetCancelMsg(msg string) error {
	if d == nil || d.raw == nil {
		return nil
	}
	msgPtr, err := windows.UTF16PtrFromString(msg)
	if err != nil {
		return err
	}
	hr, _, _ := syscall.SyscallN(d.raw.vtbl.SetCancelMsg,
		uintptr(unsafe.Pointer(d.raw)),
		uintptr(unsafe.Pointer(msgPtr)),
		0, // pvReserved
	)
	if hr != 0 {
		return fmt.Errorf("IProgressDialog.SetCancelMsg: 0x%08X", uint32(hr))
	}
	return nil
}

// SetLine sets text on one of the three status lines (lineNum 1-3). If
// compactPath is true the text is treated as a file path and may be compacted
// with an ellipsis to fit. Safe to call on a nil *Dialog.
func (d *Dialog) SetLine(lineNum uint32, text string, compactPath bool) error {
	if d == nil || d.raw == nil {
		return nil
	}
	textPtr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return err
	}
	var compact uintptr
	if compactPath {
		compact = 1
	}
	hr, _, _ := syscall.SyscallN(d.raw.vtbl.SetLine,
		uintptr(unsafe.Pointer(d.raw)),
		uintptr(lineNum),
		uintptr(unsafe.Pointer(textPtr)),
		compact,
		0, // pvReserved
	)
	if hr != 0 {
		return fmt.Errorf("IProgressDialog.SetLine: 0x%08X", uint32(hr))
	}
	return nil
}

// SetProgress updates the progress bar. Uses SetProgress64 internally.
// Pass (0, 0) only with FlagMarquee (indeterminate) - do not combine with
// FlagAutoTime. Safe to call on a nil *Dialog.
func (d *Dialog) SetProgress(completed, total uint64) error {
	if d == nil || d.raw == nil {
		return nil
	}
	hr, _, _ := syscall.SyscallN(d.raw.vtbl.SetProgress64,
		uintptr(unsafe.Pointer(d.raw)),
		uintptr(completed),
		uintptr(total),
	)
	if hr != 0 {
		return fmt.Errorf("IProgressDialog.SetProgress64: 0x%08X", uint32(hr))
	}
	return nil
}

// HasUserCancelled returns true if the user clicked the Cancel button.
// Safe to call on a nil *Dialog (always returns false).
func (d *Dialog) HasUserCancelled() bool {
	if d == nil || d.raw == nil {
		return false
	}
	ret, _, _ := syscall.SyscallN(d.raw.vtbl.HasUserCancelled,
		uintptr(unsafe.Pointer(d.raw)),
	)
	return ret != 0
}
