package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

func newSetupUI() (*setupUI, error) {
	ui := &setupUI{
		updates: make(chan uiUpdate, 32),
		actions: make(chan setupAction, 8),
		ready:   make(chan error, 1),
	}
	go ui.run()
	if err := <-ui.ready; err != nil {
		return nil, err
	}
	return ui, nil
}

func (ui *setupUI) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ui.dpi = systemDPI()
	className, _ := syscall.UTF16PtrFromString("SprutDockProxySetupWindow")
	module, _, _ := getModuleHandle.Call(0)
	ui.module = module
	cursor, _, _ := loadCursor.Call(0, uintptr(unsafe.Pointer(uint16Pointer(32512))))
	class := windowClassEx{
		Size: uint32(unsafe.Sizeof(windowClassEx{})), WindowProc: setupWindowProc,
		Instance: module, Cursor: cursor, Background: 6, ClassName: className,
	}
	if result, _, err := registerClassEx.Call(uintptr(unsafe.Pointer(&class))); result == 0 && err != syscall.Errno(1410) {
		ui.ready <- fmt.Errorf("could not create setup window: %w", err)
		return
	}
	controls := commonControlsInit{Size: uint32(unsafe.Sizeof(commonControlsInit{})), ICC: 0x20}
	initCommonControls.Call(uintptr(unsafe.Pointer(&controls)))
	ui.window = ui.createControl(0, className, "ChatGPT Proxy", wsCaption|wsSystemMenu, 520, 230, 0, 0)
	if ui.window == 0 {
		ui.ready <- fmt.Errorf("could not create setup window")
		return
	}
	setupWindows.Store(ui.window, ui)
	ui.font = ui.createUIFont()
	ui.status = ui.createControl(ui.window, utf16("STATIC"), "Проверка сертификата…", wsChild|wsVisible|ssNoPrefix, 472, 56, 24, 20)
	ui.progress = ui.createControl(ui.window, utf16("msctls_progress32"), "", wsChild|wsVisible|progressMarquee, 472, 12, 24, 92)
	ui.invitation = ui.createControl(ui.window, utf16("EDIT"), "", wsChild|wsBorder|wsTabStop|esPassword|esAutoHScroll, 472, 30, 24, 92)
	ui.primary = ui.createControl(ui.window, utf16("BUTTON"), "Продолжить", wsChild|wsTabStop, 112, 34, 384, 142, primaryButtonID)
	ui.cancel = ui.createControl(ui.window, utf16("BUTTON"), "Отмена", wsChild|wsTabStop, 112, 34, 384, 142, cancelButtonID)
	if ui.status == 0 || ui.progress == 0 || ui.invitation == 0 || ui.primary == 0 || ui.cancel == 0 {
		setupWindows.Delete(ui.window)
		destroyWindow.Call(ui.window)
		ui.ready <- fmt.Errorf("could not create setup controls")
		return
	}
	sendMessage.Call(ui.invitation, 0x00C5, 256, 0)
	ui.applyFont()
	x, _, _ := getSystemMetrics.Call(0)
	y, _, _ := getSystemMetrics.Call(1)
	width, height := ui.scale(520), ui.scale(230)
	setWindowPos.Call(ui.window, 0, uintptr((int(x)-width)/2), uintptr((int(y)-height)/2), uintptr(width), uintptr(height), 0x0040)
	showWindow.Call(ui.window, swShow)
	setForegroundWindow.Call(ui.window)
	updateWindow.Call(ui.window)
	ui.ready <- nil

	var message nativeMessage
	for {
		result, _, err := getMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) == 0 {
			return
		}
		if int32(result) == -1 {
			ui.actions <- setupAction{kind: "error", value: err.Error()}
			return
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&message)))
		dispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
	}
}

func (ui *setupUI) createControl(parent uintptr, className *uint16, title string, style uint32, width, height, x, y int, identifier ...int) uintptr {
	text, _ := syscall.UTF16PtrFromString(title)
	var id uintptr
	if len(identifier) > 0 {
		id = uintptr(identifier[0])
	}
	width, height, x, y = ui.scale(width), ui.scale(height), ui.scale(x), ui.scale(y)
	handle, _, _ := createWindowEx.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(text)), uintptr(style), uintptr(x), uintptr(y), uintptr(width), uintptr(height), parent, id, ui.module, 0)
	if handle != 0 && parent != 0 {
		setWindowTheme.Call(handle, 0, 0)
	}
	return handle
}

func systemDPI() uint32 {
	dpi, _, _ := getDpiForSystem.Call()
	if dpi < 96 || dpi > 960 {
		return 96
	}
	return uint32(dpi)
}

func (ui *setupUI) scale(value int) int {
	return (value*int(ui.dpi) + 48) / 96
}

func (ui *setupUI) createUIFont() uintptr {
	face := utf16("Segoe UI")
	height := -((10*int(ui.dpi) + 36) / 72)
	font, _, _ := createFont.Call(uintptr(height), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
	return font
}

func (ui *setupUI) applyFont() {
	if ui.font == 0 {
		return
	}
	for _, control := range []uintptr{ui.status, ui.progress, ui.invitation, ui.primary, ui.cancel} {
		sendMessage.Call(control, wmSetFont, ui.font, 1)
	}
}

func (ui *setupUI) layoutActionButtons(cancelX int) {
	setWindowPos.Call(ui.cancel, 0, uintptr(ui.scale(cancelX)), uintptr(ui.scale(142)), uintptr(ui.scale(112)), uintptr(ui.scale(34)), 0x0014)
}

func (ui *setupUI) layoutControls() {
	move := func(window uintptr, x, y, width, height int) {
		setWindowPos.Call(window, 0, uintptr(ui.scale(x)), uintptr(ui.scale(y)), uintptr(ui.scale(width)), uintptr(ui.scale(height)), swpNoZOrder|swpNoActivate)
	}
	move(ui.status, 24, 20, 472, 56)
	move(ui.progress, 24, 92, 472, 12)
	move(ui.invitation, 24, 92, 472, 30)
	cancelX := 384
	if ui.failed || ui.awaiting {
		cancelX = 264
	}
	ui.layoutActionButtons(cancelX)
}

func utf16(value string) *uint16 {
	text, _ := syscall.UTF16PtrFromString(value)
	return text
}

func uint16Pointer(value uint16) *uint16 {
	return (*uint16)(unsafe.Pointer(uintptr(value)))
}
