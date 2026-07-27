package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const synchronize = 0x00100000

// App struct
type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// configPath is relative to the process's working directory.
// Under `wails dev` / a normal build run, that's the flowkey-settings
// project root, so ../config.json reaches the daemon's config file.
const configPath = "../config.json"

// LoadConfig reads config.json and returns its raw contents.
func (a *App) LoadConfig() (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("reading config: %w", err)
	}
	return string(data), nil
}

// SaveConfig writes jsonStr to config.json, then best-effort signals
// the daemon to reload. The write and the reload are reported separately:
// a write failure is a real error, a reload failure is not (daemon may
// simply not be running) and is only logged.
func (a *App) SaveConfig(jsonStr string) error {
	if !json.Valid([]byte(jsonStr)) {
		return fmt.Errorf("refusing to write invalid JSON")
	}
	if err := os.WriteFile(configPath, []byte(jsonStr), 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	if err := triggerReload(); err != nil {
		// Config was saved successfully; reload signal just didn't land.
		// Not fatal — daemon will pick up the change next time it starts
		// or the user can hit "Reload now" once it's running.
		fmt.Printf("reload signal not sent: %v\n", err)
	}
	return nil
}

// TriggerReload signals the daemon without touching config.json
// (used by the manual "Reload now" button).
func (a *App) TriggerReload() error {
	return triggerReload()
}

// ---------- named event signal (mirrors trigger_reload.py) ----------

var (
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	procOpenEventW  = kernel32.NewProc("OpenEventW")
	procSetEvent    = kernel32.NewProc("SetEvent")
	procCloseHandle = kernel32.NewProc("CloseHandle")
)

const (
	eventModifyState = 0x0002
	reloadEventName  = "FlowkeyConfigReload"
)

func triggerReload() error {
	namePtr, err := syscall.UTF16PtrFromString(reloadEventName)
	if err != nil {
		return err
	}
	h, _, callErr := procOpenEventW.Call(
		uintptr(eventModifyState),
		0,
		uintptr(unsafe.Pointer(namePtr)),
	)
	if h == 0 {
		// Daemon likely isn't running — not fatal, config was still saved.
		return fmt.Errorf("could not open reload event (daemon not running?): %w", callErr)
	}
	defer procCloseHandle.Call(h)

	ok, _, setErr := procSetEvent.Call(h)
	if ok == 0 {
		return fmt.Errorf("SetEvent failed: %w", setErr)
	}
	return nil
}

// opens native file picker filtered to .exe files
// Returns "" (no error) if user cancels
func (a *App) BrowseExecutable() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select an executable",
		Filters: []runtime.FileFilter{
			{DisplayName: "Executables (*.exe)", Pattern: "*.exe"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("open dialog: %w", err)
	}
	return path, nil
}

// checks whether daemon's reload event exists
// only true while daemon process is alive
func (a *App) IsDaemonRunning() bool {
	namePtr, err := syscall.UTF16PtrFromString(reloadEventName)
	if err != nil {
		return false
	}
	h, _, _ := procOpenEventW.Call(uintptr(synchronize), 0, uintptr(unsafe.Pointer(namePtr)))
	if h == 0 {
		return false
	}
	procCloseHandle.Call(h)
	return true
}
