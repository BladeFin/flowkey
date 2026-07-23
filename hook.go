package main

import (
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type kbdllhookstruct struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

var listening = false
var listenGen uint32 = 0

// turns on listening flag and timeout
func startListening() {
	listening = true
	listenGen++
	gen := listenGen
	time.AfterFunc(2*time.Second, func() { //time.AfterFunc is non-blocking by construction
		if listenGen == gen {
			listening = false
			listenGen++
			fmt.Println("-> timed out, no spec key pressed")
		}
	})
}

// called by windows on every keystroke
func keyboardHookProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 && listening {
		if wParam == 0x0100 || wParam == 0x0104 { //WM_KEYDOWN or WM_SYSKEYDOWN
			kb := (*kbdllhookstruct)(unsafe.Pointer(lParam))
			cfg := GetConfig()

			if action, ok := cfg.HotkeyActions[kb.VkCode]; ok {
				listening = false
				listenGen++
				fmt.Printf("-> matched spec key: %s\n", action)
				dispatch(action)
				return 1
			} else {
				listening = false
				listenGen++
				fmt.Println("-> unmatched key pressed: cancelled operation")
				ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
				return ret
			}
		}
	}
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

// installs global low-level keyboard hook once for lifeetime of program
func installKeyboardHook() uintptr {
	callback := windows.NewCallback(keyboardHookProc)
	hMod, _, _ := procGetModuleHandleW.Call(0)
	hook, _, _ := procSetWindowsHookExW.Call(13, callback, hMod, 0)
	return hook
}
