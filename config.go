package main

import (
	"encoding/json"
	"fmt"
	"os"
	"unicode"
)

// rawConfig mirrors the JSON file on disk exactly.
type rawConfig struct {
	Hotkeys        map[string]string      `json:"hotkeys"`        // single letter -> action name
	LaunchDetached map[string]string      `json:"launchDetached"` // action name -> exe path
	MiniApps       map[string]miniAppInfo `json:"miniApps"`       // action name -> mini-app info
}

// Config is what the rest of the daemon (main.go, hook.go, dispatch.go)
// actually reads from at runtime.
type Config struct {
	HotkeyActions         map[uint32]string
	LaunchDetachedActions map[string]string
	MiniActions           map[string]miniAppInfo
}

// LoadConfig reads and parses config.json at the given path into a Config
// ready for use by the rest of the daemon.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var raw rawConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing config JSON: %w", err)
	}

	hotkeyActions, err := parseVKMap(raw.Hotkeys)
	if err != nil {
		return nil, fmt.Errorf("parsing hotkeys: %w", err)
	}

	return &Config{
		HotkeyActions:         hotkeyActions,
		LaunchDetachedActions: raw.LaunchDetached, // string-keyed already, no conversion needed
		MiniActions:           raw.MiniApps,       // string-keyed already, no conversion needed
	}, nil
}

// parseVKMap converts single-letter hotkey keys (e.g. "N") into their
// Windows VK code equivalents. For A-Z, the VK code is just the uppercase
// ASCII value of the letter, so no lookup table is needed.
//
// NOTE: this only handles single letters A-Z. Function keys (F1, F2...),
// digits, and named keys (Tab, Esc, etc.) have VK codes that are NOT
// derivable from the character itself and will need a name->VK lookup
// table added here later if/when the keymap grows beyond letters.
func parseVKMap(raw map[string]string) (map[uint32]string, error) {
	result := make(map[uint32]string, len(raw))
	for k, v := range raw {
		if len(k) != 1 {
			return nil, fmt.Errorf("invalid hotkey %q: expected a single letter", k)
		}
		r := unicode.ToUpper(rune(k[0]))
		if r < 'A' || r > 'Z' {
			return nil, fmt.Errorf("invalid hotkey %q: only A-Z letters are supported right now", k)
		}
		result[uint32(r)] = v
	}
	return result, nil
}
