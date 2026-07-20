package main

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var hotkeyActions = map[uint32]string{
	0x4E: "notepad",    // N
	0x43: "calculator", // C
	0x53: "chrome",     // S
	0x56: "vscode",     // V
}

var launchDetachedActions = map[string]string{
	"chrome": `C:\Program Files\Google\Chrome\Application\chrome.exe`,
	"vscode": `C:\Users\blade\AppData\Local\Programs\Microsoft VS Code\Code.exe`,
}

type miniAppInfo struct {
	path        string
	windowTitle string
	alwaysOnTop bool
}

var miniActions = map[string]miniAppInfo{
	"calculator": {path: `.\flowkey-calc\build\bin\flowkey-calc.exe`, windowTitle: "flowkey-calc", alwaysOnTop: true},
}

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procRegisterHotKey      = user32.NewProc("RegisterHotKey")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
	procFindWindowW         = user32.NewProc("FindWindowW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
)

type msg struct {
	Hwnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type kbdllhookstruct struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

var listening = false
var listenGen uint32 = 0

var jobObject windows.Handle

// Creates windows job object configured to kill all assigned processes when this handle is closed
// includes daemon crash
func initJobObject() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}

	_, err = windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		windows.CloseHandle(job)
		return err
	}

	jobObject = job
	return nil
}

func main() {
	runtime.LockOSThread() //prevents thread switching that would break the hotkey registration
	if err := initJobObject(); err != nil {
		panic(err) //crash because we don't want a bunch of mini-apps hanging around
	}

	r, _, err := procRegisterHotKey.Call(
		0,      //hwnd
		1,      //id - arbitrary
		0x0001, //alt
		0x20,   //space bar
	)
	if r == 0 {
		fmt.Printf("Aw fuck, %v\n", err)
		return
	} else {
		fmt.Printf("%s\n", "Success!")
	}

	hook := installKeyboardHook()
	if hook == 0 {
		fmt.Println("failed to install keyboard hook")
		return
	}

	var m msg
	for {
		procGetMessageW.Call(
			uintptr(unsafe.Pointer(&m)),
			0,
			0,
			0,
		)
		if m.Message == 0x0312 && m.WParam == 1 { //WM_HOTKEY, our id
			fmt.Println("activation pressed! listening for spec key...")
			startListening()
		}
	}
}

// turns on listening flag and timeout
func startListening() {
	listening = true
	listenGen++
	gen := listenGen
	time.AfterFunc(2*time.Second, func() { //time.AfterFunc is non-blocking by construction
		if listenGen == gen {
			listening = false
			fmt.Println("-> timed out, no spec key pressed")
		}
	})
}

// called by windows on every keystroke
func keyboardHookProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 && listening {
		if wParam == 0x0100 || wParam == 0x0104 { //WM_KEYDOWN or WM_SYSKEYDOWN
			kb := (*kbdllhookstruct)(unsafe.Pointer(lParam))

			if action, ok := hotkeyActions[kb.VkCode]; ok {
				listening = false
				fmt.Printf("-> matched spec key: %s\n", action)
				dispatch(action)
				return 1
			} else {
				listening = false
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

func dispatch(action string) {
	if path, ok := launchDetachedActions[action]; ok {
		// launch the corersponding application
		if err := launchDetached(path); err != nil {
			log.Printf("failed to launch %v application with error: %v", action, err)
		}

	} else if info, exists := miniActions[action]; exists {
		if hwnd := findWindow(info.windowTitle); hwnd != 0 {
			toggleWindow(hwnd)
		} else {
			launchAttached(info.path, info.alwaysOnTop, info.windowTitle)
		}
		// show the corresponding app
	} else {
		log.Printf("Yikes we have the '%v' action but nothing to do with it", action)
	}
}

// starts external app and does NOT attach lifetime to daemon's (will persist after daemon)
func launchDetached(path string, args ...string) error {
	cmd := exec.Command(path, args...)
	if err := cmd.Start(); err != nil { //Start = fire and forget
		return err
	}
	cmd.Process.Release()
	return nil
}

// starts mini-app and ties lifetime to daemon's via job object
func launchAttached(path string, alwaysOnTop bool, windowTitle string, args ...string) error {
	cmd := exec.Command(path, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	pid := cmd.Process.Pid
	cmd.Process.Release()

	if jobObject == 0 {
		log.Printf("job object unavailable, %s (pid %d) will not die with daemon", path, pid)
		return nil
	}
	if err := assignToJob(pid); err != nil {
		log.Printf("failed to assign pid %d to job object: %v", pid, err)
	}

	if alwaysOnTop {
		go pinTopmostWhenReady(windowTitle)
	}

	return nil
}

// opens handle to process with given pid and assigns to daemon's job object
func assignToJob(pid int) error {
	hProcess, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(pid),
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(hProcess)

	return windows.AssignProcessToJobObject(jobObject, hProcess)
}

// polls for a window with the given title to appear, then pins it
// topmost. Gives up after ~3 seconds in case something's wrong.
func pinTopmostWhenReady(windowTitle string) {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if hwnd := findWindow(windowTitle); hwnd != 0 {
			procSetWindowPos.Call(hwnd, ^uintptr(0), 0, 0, 0, 0, 0x0002|0x0001)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	log.Printf("gave up waiting for window %q to appear for topmost pin", windowTitle)
}

// returns HWND of the top-level window with given title
// returns 0 if no such iwndow exists
func findWindow(title string) uintptr {
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		log.Printf("failed to convert title %q: %v", title, err)
		return 0
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	return hwnd
}

// reports whether the given winidow is currently shown
// returns 1 if shown, 0 if hidden
func isWindowVisible(hwnd uintptr) bool {
	ret, _, _ := procIsWindowVisible.Call(hwnd)
	return ret != 0
}

// shows hidden window and brings to foreground
func showWindow(hwnd uintptr) {
	procShowWindow.Call(hwnd, 5)
	procSetForegroundWindow.Call(hwnd)

}

// hides visible window without destroying it
func hideWindow(hwnd uintptr) {
	procShowWindow.Call(hwnd, 0)
}

// hideWindow if window is showing, showWindow if window is hidden
// returns 1 if now shown, 0 if now hidden
func toggleWindow(hwnd uintptr) int {
	if isWindowVisible(hwnd) {
		hideWindow(hwnd)
		return 0
	} else {
		showWindow(hwnd)
		return 1
	}

}
