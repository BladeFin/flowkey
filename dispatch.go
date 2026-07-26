package main

import (
	"log"
	"os/exec"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func dispatch(id string) {
	cfg := GetConfig()
	app, ok := cfg.Apps[id]
	if !ok {
		log.Printf("Yikes we have the '%v' action but nothing to do with it", id)
		return
	}

	switch app.Type {
	case "launch":
		if err := launchDetached(app.Path); err != nil {
			log.Printf("failed to launch %v application with error: %v", id, err)
		}
	case "mini":
		if hwnd := findWindow(app.WindowTitle); hwnd != 0 {
			toggleWindow(hwnd)
		} else {
			launchAttached(app.Path, app.AlwaysOnTop, app.WindowTitle)
		}
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
