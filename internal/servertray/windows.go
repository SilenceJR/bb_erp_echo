//go:build windows

package servertray

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const singletonName = `Global\BoBangERPServerTray-v1`

func AcquireSingleInstance() (release func(), acquired bool, err error) {
	name, err := windows.UTF16PtrFromString(singletonName)
	if err != nil {
		return nil, false, err
	}
	handle, createErr := windows.CreateMutex(nil, false, name)
	if errors.Is(createErr, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}
		return nil, false, nil
	}
	if createErr != nil {
		// A mutex created by another Windows account can be visible but not
		// openable with the current account's default DACL. Fail closed so a
		// second server never reaches database or port initialization.
		if errors.Is(createErr, windows.ERROR_ACCESS_DENIED) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("create server singleton mutex: %w", createErr)
	}
	return func() { _ = windows.CloseHandle(handle) }, true, nil
}

func ConfirmExit() bool {
	const (
		mbYesNo       = 0x00000004
		mbIconWarning = 0x00000030
		mbDefaultTwo  = 0x00000100
		mbForeground  = 0x00010000
		idYes         = 6
	)
	text, _ := windows.UTF16PtrFromString("退出后所有客户端将无法使用，是否继续？")
	title, _ := windows.UTF16PtrFromString("博邦 ERP")
	result, _ := windows.MessageBox(0, text, title, mbYesNo|mbIconWarning|mbDefaultTwo|mbForeground)
	return result == idYes
}

func ShowInfo(message string) {
	const mbIconInformation = 0x00000040
	const mbForeground = 0x00010000
	text, _ := windows.UTF16PtrFromString(message)
	title, _ := windows.UTF16PtrFromString("博邦 ERP")
	_, _ = windows.MessageBox(0, text, title, mbIconInformation|mbForeground)
}

func OpenTarget(target string) error {
	value, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	if err := windows.ShellExecute(0, nil, value, nil, nil, 1); err != nil {
		return fmt.Errorf("open %q: %w", target, err)
	}
	return nil
}

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	shell32             = windows.NewLazySystemDLL("shell32.dll")
	procEnumWindows     = user32.NewProc("EnumWindows")
	procGetClassName    = user32.NewProc("GetClassNameW")
	procGetWindowPID    = user32.NewProc("GetWindowThreadProcessId")
	procShellNotifyIcon = shell32.NewProc("Shell_NotifyIconW")
)

type notifyIconData struct {
	Size                       uint32
	Wnd                        windows.Handle
	ID, Flags, CallbackMessage uint32
	Icon                       windows.Handle
	Tip                        [128]uint16
	State, StateMask           uint32
	Info                       [256]uint16
	Timeout, Version           uint32
	InfoTitle                  [64]uint16
	InfoFlags                  uint32
	GuidItem                   windows.GUID
	BalloonIcon                windows.Handle
}

// ShowBalloon reuses the pinned getlantern/systray v1.2.2 Windows icon. That
// version owns a hidden window named SystrayClass and notification icon ID 100.
func ShowBalloon(title, message string, isError bool) error {
	window := currentSystrayWindow()
	if window == 0 {
		return errors.New("find systray notification window")
	}
	const (
		nimModify = 0x00000001
		nifInfo   = 0x00000010
		niifInfo  = 0x00000001
		niifError = 0x00000003
	)
	data := notifyIconData{Wnd: window, ID: 100, Flags: nifInfo, Timeout: 10000, InfoFlags: niifInfo}
	data.Size = uint32(unsafe.Sizeof(data))
	if isError {
		data.InfoFlags = niifError
	}
	copyUTF16(data.InfoTitle[:], title)
	copyUTF16(data.Info[:], message)
	result, _, callErr := procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&data)))
	if result == 0 {
		return fmt.Errorf("show tray notification: %w", callErr)
	}
	return nil
}

func currentSystrayWindow() windows.Handle {
	processID := windows.GetCurrentProcessId()
	var found windows.Handle
	callback := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		var owner uint32
		procGetWindowPID.Call(hwnd, uintptr(unsafe.Pointer(&owner)))
		if owner != processID {
			return 1
		}
		buffer := make([]uint16, 64)
		length, _, _ := procGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
		if length > 0 && windows.UTF16ToString(buffer[:length]) == "SystrayClass" {
			found = windows.Handle(hwnd)
			return 0
		}
		return 1
	})
	procEnumWindows.Call(callback, 0)
	return found
}

func copyUTF16(destination []uint16, value string) {
	encoded, err := windows.UTF16FromString(value)
	if err != nil {
		return
	}
	if len(encoded) > len(destination) {
		encoded = encoded[:len(destination)]
		encoded[len(encoded)-1] = 0
	}
	copy(destination, encoded)
}
