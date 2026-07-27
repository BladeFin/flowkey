package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"unicode"
)

// Note: the line below actually does stuff
//
//go:embed default_config.json
var defaultConfigJSON []byte

var currentConfig atomic.Pointer[Config]

// returns currently active config
// safe to call from any goroutine
func GetConfig() *Config {
	return currentConfig.Load()
}

type miniAppInfo struct {
	Path        string
	WindowTitle string
	AlwaysOnTop bool
}

type rawApp struct {
	Type        string `json:"type"` // "launch" or "mini"
	Title       string `json:"title"`
	Path        string `json:"path"`
	Hotkey      string `json:"hotkey"`
	WindowTitle string `json:"windowTitle,omitempty"`
	AlwaysOnTop bool   `json:"alwaysOnTop,omitempty"`
}

type rawConfig struct {
	Apps map[string]rawApp `json:"apps"` // id -> app
}

type AppInfo struct {
	Type        string
	Title       string
	Path        string
	WindowTitle string
	AlwaysOnTop bool
}

type Config struct {
	Apps          map[string]AppInfo // id -> app info
	HotkeyActions map[uint32]string  // VK code -> app id
}

// LoadConfig reads and parses config.json at the given path into a Config
// ready for use by the rest of the daemon.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		// no config.json on disk, dupe default
		log.Printf("config.json not found at %q, writing default config", path)
		if writeErr := os.WriteFile(path, defaultConfigJSON, 0644); writeErr != nil {
			log.Printf("warning: failed to write default config.json: %v", writeErr)
		}
		data = defaultConfigJSON
	}

	var raw rawConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing config JSON: %w", err)
	}

	apps := make(map[string]AppInfo, len(raw.Apps))
	hotkeyActions := make(map[uint32]string, len(raw.Apps))

	for id, a := range raw.Apps {
		apps[id] = AppInfo{
			Type:        a.Type,
			Title:       a.Title,
			Path:        a.Path,
			WindowTitle: a.WindowTitle,
			AlwaysOnTop: a.AlwaysOnTop,
		}

		if a.Hotkey == "" {
			//intentionally disabled, skip
			continue
		}
		vk, err := parseHotkeyChar(a.Hotkey)
		if err != nil {
			return nil, fmt.Errorf("app %q: %w", id, err)
		}
		if existing, taken := hotkeyActions[vk]; taken {
			return nil, fmt.Errorf("hotkey %q used by both %q and %q", a.Hotkey, existing, id)
		}
		hotkeyActions[vk] = id
	}

	return &Config{
		Apps:          apps,
		HotkeyActions: hotkeyActions,
	}, nil
}

// parses one hotkey string like "N" into its VK code.
func parseHotkeyChar(k string) (uint32, error) {
	if len(k) != 1 {
		return 0, fmt.Errorf("invalid hotkey %q: expected a single letter", k)
	}
	r := unicode.ToUpper(rune(k[0]))
	if r < 'A' || r > 'Z' {
		return 0, fmt.Errorf("invalid hotkey %q: only A-Z letters are supported right now", k)
	}
	return uint32(r), nil
}
