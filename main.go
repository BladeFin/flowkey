package main

import (
	"fmt"
	"log"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	hotkeyActions         map[uint32]string
	launchDetachedActions map[string]string
	miniActions           map[string]miniAppInfo
)

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

	//load config or default config
	cfg, err := LoadConfig("config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	hotkeyActions = cfg.HotkeyActions
	launchDetachedActions = cfg.LaunchDetachedActions
	miniActions = cfg.MiniActions

	//make a job for all the mini-apps to attach themselves too
	if err := initJobObject(); err != nil {
		panic(err) //crash because we don't want a bunch of mini-apps hanging around
	}

	//register alt+space hotkey
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

	//keylogger
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
