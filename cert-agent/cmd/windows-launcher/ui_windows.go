package main

import (
	"sync"
	"syscall"
)

const (
	wmDestroy              = 0x0002
	wmClose                = 0x0010
	wmCommand              = 0x0111
	wmAppUpdate            = 0x8001
	wmAppShutdown          = 0x8002
	wmCtlColorStatic       = 0x0138
	wmSetFont              = 0x0030
	wmDpiChanged           = 0x02E0
	wsChild                = 0x40000000
	wsVisible              = 0x10000000
	wsTabStop              = 0x00010000
	wsBorder               = 0x00800000
	wsCaption              = 0x00C00000
	wsSystemMenu           = 0x00080000
	ssNoPrefix             = 0x00000080
	esPassword             = 0x0020
	esAutoHScroll          = 0x0080
	swHide                 = 0
	swShow                 = 5
	swpNoZOrder            = 0x0004
	swpNoActivate          = 0x0010
	progressMarquee        = 0x0008
	progressSetPos         = 0x0402
	progressMarqueeMessage = 0x040A
	primaryButtonID        = 1001
	cancelButtonID         = 1002
)

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	comctl32            = syscall.NewLazyDLL("comctl32.dll")
	getMessage          = user32.NewProc("GetMessageW")
	translateMessage    = user32.NewProc("TranslateMessage")
	dispatchMessage     = user32.NewProc("DispatchMessageW")
	createWindowEx      = user32.NewProc("CreateWindowExW")
	registerClassEx     = user32.NewProc("RegisterClassExW")
	setWindowText       = user32.NewProc("SetWindowTextW")
	getSysColorBrush    = user32.NewProc("GetSysColorBrush")
	getWindowText       = user32.NewProc("GetWindowTextW")
	getWindowTextSize   = user32.NewProc("GetWindowTextLengthW")
	showWindow          = user32.NewProc("ShowWindow")
	setForegroundWindow = user32.NewProc("SetForegroundWindow")
	updateWindow        = user32.NewProc("UpdateWindow")
	postMessage         = user32.NewProc("PostMessageW")
	destroyWindow       = user32.NewProc("DestroyWindow")
	postQuitMessage     = user32.NewProc("PostQuitMessage")
	getSystemMetrics    = user32.NewProc("GetSystemMetrics")
	setWindowPos        = user32.NewProc("SetWindowPos")
	defWindowProc       = user32.NewProc("DefWindowProcW")
	sendMessage         = user32.NewProc("SendMessageW")
	loadCursor          = user32.NewProc("LoadCursorW")
	setProcessDPI       = user32.NewProc("SetProcessDpiAwarenessContext")
	setProcessDPIAware  = user32.NewProc("SetProcessDPIAware")
	getDpiForSystem     = user32.NewProc("GetDpiForSystem")
	getModuleHandle     = syscall.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
	initCommonControls  = comctl32.NewProc("InitCommonControlsEx")
	messageBox          = user32.NewProc("MessageBoxW")
	createFont          = syscall.NewLazyDLL("gdi32.dll").NewProc("CreateFontW")
	deleteObject        = syscall.NewLazyDLL("gdi32.dll").NewProc("DeleteObject")
	setBkMode           = syscall.NewLazyDLL("gdi32.dll").NewProc("SetBkMode")
	setWindowTheme      = syscall.NewLazyDLL("uxtheme.dll").NewProc("SetWindowTheme")
	setupWindows        sync.Map
	setupWindowProc     = syscall.NewCallback(windowProc)
)

type commonControlsInit struct {
	Size uint32
	ICC  uint32
}

type windowClassEx struct {
	Size        uint32
	Style       uint32
	WindowProc  uintptr
	ClassExtra  int32
	WindowExtra int32
	Instance    uintptr
	Icon        uintptr
	Cursor      uintptr
	Background  uintptr
	MenuName    *uint16
	ClassName   *uint16
	IconSmall   uintptr
}

type nativeMessage struct {
	Window  uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   struct{ X, Y int32 }
}

type setupAction struct {
	kind  string
	value string
}

type setupUI struct {
	window     uintptr
	module     uintptr
	font       uintptr
	dpi        uint32
	status     uintptr
	progress   uintptr
	invitation uintptr
	primary    uintptr
	cancel     uintptr
	updates    chan uiUpdate
	actions    chan setupAction
	ready      chan error
	awaiting   bool
	failed     bool
}

type uiUpdate struct {
	event  string
	status string
}

func (ui *setupUI) update(event string) {
	ui.updates <- uiUpdate{event: event}
	postMessage.Call(ui.window, wmAppUpdate, 0, 0)
}

func (ui *setupUI) showStatus(status string) {
	ui.updates <- uiUpdate{event: "status", status: status}
	postMessage.Call(ui.window, wmAppUpdate, 0, 0)
}

func (ui *setupUI) begin(status string) {
	ui.updates <- uiUpdate{event: "begin", status: status}
	postMessage.Call(ui.window, wmAppUpdate, 0, 0)
}

func (ui *setupUI) fail(status string) {
	ui.updates <- uiUpdate{event: "failure", status: status}
	postMessage.Call(ui.window, wmAppUpdate, 0, 0)
}

func (ui *setupUI) close() {
	postMessage.Call(ui.window, wmAppShutdown, 0, 0)
}

func enablePerMonitorDPI() {
	if setProcessDPI.Find() == nil {
		result, _, _ := setProcessDPI.Call(^uintptr(3))
		if result != 0 {
			return
		}
	}
	setProcessDPIAware.Call()
}
