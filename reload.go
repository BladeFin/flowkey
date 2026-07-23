package main

import (
	"log"

	"golang.org/x/sys/windows"
)

const reloadEventName = "FlowkeyConfigReload"

// creates/opens named auto-reset event and blocks on it in goroutine
// any process that knows event name (like settings app or even custom community apps)
// can call windows.SetEvent to wake this up
// Even clears after each WaitForSingleObject wakes
func startReloadWatcher(configPath string) {
	namePtr, err := windows.UTF16PtrFromString(reloadEventName)
	if err != nil {
		log.Printf("failed to convert reload event name: %v", err)
		return
	}

	//manualRest=0 -> auto-reset event ; initalState=0 -> starts unsignaled
	handle, err := windows.CreateEvent(nil, 0, 0, namePtr)
	if err != nil {
		log.Printf("failed to create reload event: %v", err)
		return
	}

	go func() {
		for {
			_, err := windows.WaitForSingleObject(handle, windows.INFINITE)
			if err != nil {
				log.Printf("reload watcher wait failed: %v", err)
				continue
			}
			log.Println("reload signal received, reloading config...")
			cfg, err := LoadConfig(configPath)
			if err != nil {
				log.Printf("reload failed, keeping current config: %v", err)
				continue
			}
			currentConfig.Store(cfg)
			log.Println("config reloaded")
		}
	}()
}
