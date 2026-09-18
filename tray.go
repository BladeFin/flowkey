package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	WM_USER     = 0x0400
	WM_TRAYICON = WM_USER + 1

	WM_COMMAND = 0x0111
	WM_DESTROY = 0x0002
)

var (
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")

	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")

	shell32              = windows.NewLazySystemDLL("shell32.dll")
	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procLoadIconW        = user32.NewProc("LoadIconW")

	//menu functionality
	procCreatePopupMenu = user32.NewProc("CreatePopupMenu")
	procAppendMenuW     = user32.NewProc("AppendMenuW")
	procTrackPopupMenu  = user32.NewProc("TrackPopupMenu")
	procDestroyMenu     = user32.NewProc("DestroyMenu")
	procGetCursorPos    = user32.NewProc("GetCursorPos")
	//procSetForegroundWindow already defined

	//reload logic
	procOpenEventW = kernel32.NewProc("OpenEventW")
	procSetEvent   = kernel32.NewProc("SetEvent")

	//restart logic
	procPostThreadMessageW = user32.NewProc("PostThreadMessageW")
)

const IDI_APPLICATION = 32512 //default application icon

const (
	NIM_ADD    = 0x00000000 //add icon to tray
	NIM_DELETE = 0x00000002

	NIF_MESSAGE = 0x0000001  //send events to window
	NIF_ICON    = 0x00000002 //i provide icon
	NIF_TIP     = 0x00000004 //i provide tooltip

	//menu functionality
	WM_RBUTTONUP = 0x0205
	MF_STRING    = 0x00000000
	MF_SEPARATOR = 0x00000800

	TPM_LEFTALIGN   = 0x00000000
	TPM_BOTTOMALIGN = 0x00000020
	TPM_RIGHTBUTTON = 0x00000002

	MENU_RELOAD   = 1001
	MENU_RESTART  = 1002
	MENU_SETTINGS = 1003
	MENU_EXIT     = 1004
)

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type notifyIconDataW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UTimeout         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     uintptr
}

var trayHwnd uintptr

func registerTrayWindowClass() error {
	className, err := windows.UTF16PtrFromString("FlowkeyTrayClass")
	if err != nil {
		return err
	}
	hInstance, _, _ := procGetModuleHandleW.Call(0)

	callback := windows.NewCallback(windowProc)

	wc := wndClassExW{
		LpfnWndProc:   callback,
		HInstance:     hInstance,
		LpszClassName: className,
	}
	wc.CbSize = uint32(unsafe.Sizeof(wc))

	ret, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if ret == 0 {
		return callErr
	}
	return nil
}

func createTrayWindow() error {
	className, err := windows.UTF16PtrFromString("FlowkeyTrayClass")
	if err != nil {
		return err
	}
	windowName, err := windows.UTF16PtrFromString("Flowkey")
	if err != nil {
		return err
	}
	hInstance, _, _ := procGetModuleHandleW.Call(0)

	hwnd, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		0,
		0, 0, 0, 0,
		0, 0,
		hInstance,
		0,
	)
	if hwnd == 0 {
		return callErr
	}
	trayHwnd = hwnd
	return nil
}

func startTray() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := registerTrayWindowClass(); err != nil {
		log.Printf("tray: failed to register window class: %v", err)
		return
	}
	if err := createTrayWindow(); err != nil {
		log.Printf("tray: failed to create window: %v", err)
		return
	}

	if err := addTrayIcon(); err != nil {
		log.Printf("tray: failed to add icon: %v", err)
		return
	}

	log.Printf("tray: window created, hwnd=%v", trayHwnd)

	if err := messageLoop(); err != nil {
		log.Printf("tray: message loop failed: %v", err)
	}
}

func windowProc(hwnd windows.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_TRAYICON:
		//tray icon event
		if lParam == WM_RBUTTONUP {
			showTrayMenu(hwnd)
		}
	case WM_COMMAND:
		//menu options
		id := wParam & 0xFFFF
		switch id {
		case MENU_RELOAD:
			//reload
			cfg, err := LoadConfig("config.json")
			if err != nil {
				log.Printf("reload failed: %v", err)
				return 0
			}
			currentConfig.Store(cfg)
		case MENU_RESTART:
			//restart
			r, _, err := procPostThreadMessageW.Call(
				uintptr(hotkeyThreadID),
				WM_UNREGISTER_HOTKEYS, //tell original thread to unregister hotkeys
				0,
				0,
			)

			if r == 0 {
				log.Printf("failed to tell hotkey thread to shut down: %v", err)
				return 0
			}

			<-hotkeysStopped //original thread says "yes, I unregistered them"
			removeTrayIcon() //explicitly remove tray to avoid race condition

			exe, err := os.Executable()
			if err != nil {
				log.Printf("restart failed: %v", err)
				return 0
			}

			cmd := exec.Command(exe)

			if err := cmd.Start(); err != nil {
				log.Printf("restart failed: %v", err)
				return 0
			}

			os.Exit(0)
		case MENU_SETTINGS:
			//TODO: launch settings app
		case MENU_EXIT:
			//exit
			// removeTrayIcon()
			// procPostQuitMessage.Call(0)
			os.Exit(0) //works with job handling for now
		}

		return 0
	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(
		uintptr(hwnd),
		uintptr(msg),
		wParam,
		lParam,
	)
	return ret
}

// msg struct in main
func messageLoop() error {
	var m msg

	for {
		ret, _, err := procGetMessageW.Call(
			uintptr(unsafe.Pointer(&m)),
			0,
			0,
			0,
		)

		if int32(ret) == -1 {
			return err
		}

		if ret == 0 {
			//WM_QUIT
			return nil
		}

		procTranslateMessage.Call(
			uintptr(unsafe.Pointer(&m)),
		)

		procDispatchMessageW.Call(
			uintptr(unsafe.Pointer(&m)),
		)
	}
}

func addTrayIcon() error {
	icon, _, _ := procLoadIconW.Call(
		0,
		uintptr(IDI_APPLICATION),
	)

	if icon == 0 {
		return fmt.Errorf("failed to load try icon")
	}

	tip, err := windows.UTF16FromString("Flowkey")
	if err != nil {
		return err
	}

	var nid notifyIconDataW

	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = trayHwnd
	nid.UID = 1
	nid.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	nid.UCallbackMessage = WM_TRAYICON
	nid.HIcon = icon

	copy(nid.SzTip[:], tip)

	ret, _, err := procShellNotifyIconW.Call(
		NIM_ADD,
		uintptr(unsafe.Pointer(&nid)),
	)

	if ret == 0 {
		return fmt.Errorf("Shell_NotifyIconW failed: %w", err)
	}

	return nil
}

func showTrayMenu(hwnd windows.HWND) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}

	appendMenu(menu, MENU_RELOAD, "Reload")
	appendMenu(menu, MENU_RESTART, "Restart")
	appendMenu(menu, MENU_SETTINGS, "Settings")

	procAppendMenuW.Call(
		menu,
		MF_SEPARATOR,
		0,
		0,
	)

	appendMenu(menu, MENU_EXIT, "Exit")

	//get mouse pos
	var point struct {
		X int32
		Y int32
	}

	procGetCursorPos.Call(uintptr(unsafe.Pointer(&point)))

	procSetForegroundWindow.Call(uintptr(hwnd))

	procTrackPopupMenu.Call(
		menu,
		TPM_LEFTALIGN|TPM_BOTTOMALIGN|TPM_RIGHTBUTTON,
		uintptr(point.X),
		uintptr(point.Y),
		0,
		uintptr(hwnd),
		0,
	)

	procDestroyMenu.Call(menu)
}

func appendMenu(menu uintptr, id uintptr, text string) {
	str, _ := windows.UTF16PtrFromString(text)

	procAppendMenuW.Call(
		menu,
		MF_STRING,
		id,
		uintptr(unsafe.Pointer(str)),
	)
}

func removeTrayIcon() {
	nid := notifyIconDataW{
		CbSize: uint32(unsafe.Sizeof(notifyIconDataW{})),
		HWnd:   trayHwnd,
		UID:    1,
	}

	procShellNotifyIconW.Call(
		NIM_DELETE,
		uintptr(unsafe.Pointer(&nid)),
	)
}
