package main

import (
	"syscall"
	"unsafe"
)

func (ui *setupUI) apply(update uiUpdate) {
	switch update.event {
	case "begin":
		ui.failed = false
		ui.awaiting = false
		setText(ui.status, update.status)
		setText(ui.invitation, "")
		showWindow.Call(ui.progress, swShow)
		showWindow.Call(ui.invitation, swHide)
		showWindow.Call(ui.primary, swHide)
		setText(ui.cancel, "Отмена")
		ui.layoutActionButtons(384)
		showWindow.Call(ui.cancel, swShow)
		sendMessage.Call(ui.progress, progressMarqueeMessage, 1, 25)
	case "status":
		setText(ui.status, update.status)
	case "failure":
		ui.failed = true
		ui.awaiting = false
		setText(ui.status, update.status)
		setText(ui.invitation, "")
		sendMessage.Call(ui.progress, progressMarqueeMessage, 0, 0)
		showWindow.Call(ui.progress, swHide)
		showWindow.Call(ui.invitation, swHide)
		setText(ui.primary, "Повторить")
		setText(ui.cancel, "Закрыть")
		ui.layoutActionButtons(264)
		showWindow.Call(ui.primary, swShow)
		showWindow.Call(ui.cancel, swShow)
	case "checking":
		setText(ui.status, "Проверка сертификата…")
		startIndeterminateProgress(ui)
	case "setup_started":
		setText(ui.status, "Настройка защищённого прокси…")
		startIndeterminateProgress(ui)
	case "enrolling":
		setText(ui.status, "Регистрация устройства…")
		startIndeterminateProgress(ui)
	case "renewing":
		setText(ui.status, "Обновление сертификата…")
		startIndeterminateProgress(ui)
	case "tunnel_starting":
		setText(ui.status, "Запуск защищённого туннеля…")
		startIndeterminateProgress(ui)
	case "verifying":
		setText(ui.status, "Проверка защищённого соединения…")
		startIndeterminateProgress(ui)
	case "request_prepared":
		setText(ui.status, "Запрос регистрации подготовлен")
		sendMessage.Call(ui.progress, progressMarqueeMessage, 0, 0)
		sendMessage.Call(ui.progress, progressSetPos, 25, 0)
	case "certificate_saved":
		setText(ui.status, "Сертификат сохранён")
		sendMessage.Call(ui.progress, progressMarqueeMessage, 0, 0)
		sendMessage.Call(ui.progress, progressSetPos, 50, 0)
	case "invitation_required":
		ui.awaiting = true
		ui.failed = false
		setText(ui.status, "Введите приглашение для регистрации устройства.")
		setText(ui.primary, "Продолжить")
		setText(ui.cancel, "Отмена")
		ui.layoutActionButtons(264)
		showWindow.Call(ui.progress, swHide)
		showWindow.Call(ui.invitation, swShow)
		showWindow.Call(ui.primary, swShow)
		showWindow.Call(ui.cancel, swShow)
	case "ready":
		ui.awaiting = false
		setText(ui.status, "Настройка завершена")
		sendMessage.Call(ui.progress, progressMarqueeMessage, 0, 0)
		sendMessage.Call(ui.progress, progressSetPos, 100, 0)
		showWindow.Call(ui.invitation, swHide)
		showWindow.Call(ui.primary, swHide)
		showWindow.Call(ui.cancel, swHide)
	}
}

func startIndeterminateProgress(ui *setupUI) {
	sendMessage.Call(ui.progress, progressMarqueeMessage, 1, 25)
	showWindow.Call(ui.progress, swShow)
}

func windowProc(window, message, wparam, lparam uintptr) uintptr {
	value, _ := setupWindows.Load(window)
	ui, _ := value.(*setupUI)
	switch uint32(message) {
	case wmAppUpdate:
		if ui != nil {
			for {
				select {
				case update := <-ui.updates:
					ui.apply(update)
				default:
					return 0
				}
			}
		}
	case wmAppShutdown:
		destroyWindow.Call(window)
		return 0
	case wmCtlColorStatic:
		if ui != nil && lparam == ui.status {
			setBkMode.Call(wparam, 1)
			brush, _, _ := getSysColorBrush.Call(5)
			return brush
		}
	case wmDpiChanged:
		if ui != nil && lparam != 0 {
			ui.dpi = uint32(wparam & 0xffff)
			rectangle := (*[4]int32)(unsafe.Pointer(lparam))
			setWindowPos.Call(window, 0, uintptr(rectangle[0]), uintptr(rectangle[1]), uintptr(rectangle[2]-rectangle[0]), uintptr(rectangle[3]-rectangle[1]), swpNoZOrder|swpNoActivate)
			ui.layoutControls()
			ui.applyFont()
			return 0
		}
	case wmCommand:
		if ui != nil && (wparam&0xffff == primaryButtonID || wparam&0xffff == cancelButtonID) && wparam>>16 == 0 {
			id := wparam & 0xffff
			if id == primaryButtonID {
				if ui.awaiting {
					value := windowText(ui.invitation)
					setText(ui.invitation, "")
					showWindow.Call(ui.invitation, swHide)
					showWindow.Call(ui.primary, swHide)
					showWindow.Call(ui.cancel, swHide)
					ui.awaiting = false
					ui.actions <- setupAction{kind: "invitation", value: value}
				} else if ui.failed {
					ui.actions <- setupAction{kind: "retry"}
				}
			} else if ui.failed {
				ui.actions <- setupAction{kind: "close"}
			} else {
				ui.actions <- setupAction{kind: "cancel"}
			}
			return 0
		}
	case wmClose:
		if ui != nil {
			ui.actions <- setupAction{kind: "cancel"}
		}
		return 0
	case wmDestroy:
		if ui != nil && ui.font != 0 {
			deleteObject.Call(ui.font)
			ui.font = 0
		}
		setupWindows.Delete(window)
		postQuitMessage.Call(0)
		return 0
	}
	result, _, _ := defWindowProc.Call(window, message, wparam, lparam)
	return result
}

func showMessage(title, message string, flags uintptr) int {
	caption, _ := syscall.UTF16PtrFromString(title)
	text, _ := syscall.UTF16PtrFromString(message)
	result, _, _ := messageBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(caption)), flags|0x10000)
	return int(result)
}

func setText(window uintptr, value string) {
	text, _ := syscall.UTF16PtrFromString(value)
	setWindowText.Call(window, uintptr(unsafe.Pointer(text)))
}

func windowText(window uintptr) string {
	length, _, _ := getWindowTextSize.Call(window)
	buffer := make([]uint16, length+1)
	getWindowText.Call(window, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	return syscall.UTF16ToString(buffer)
}
