package ccdata

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Override is a settings source other than the user settings.json that sets
// cleanupPeriodDays and may therefore take precedence over it.
type Override struct {
	Source string `json:"source"` // file path or registry key
	Kind   string `json:"kind"`   // "managed", "remote", "project", "env"
	Days   int    `json:"days"`
	// Note is a human-readable explanation shown in the UI.
	Note string `json:"note"`
}

// ManagedSettingsDir is where Claude Code reads managed settings on Windows.
// The legacy C:\ProgramData\ClaudeCode path is no longer read.
var ManagedSettingsDir = `C:\Program Files\ClaudeCode`

// FindOverrides looks for cleanupPeriodDays in the sources that rank above
// the user settings: managed settings (file, drop-in folder, registry
// policies), server-managed settings cached in remote-settings.json, and the
// project settings of the given directories. Unreadable sources are ignored.
func FindOverrides(loc Locations, projectDirs []string) []Override {
	var out []Override
	check := func(path, kind, note string) {
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		if days, found, err := cleanupDaysFromJSON(data); err == nil && found {
			out = append(out, Override{Source: path, Kind: kind, Days: days, Note: note})
		}
	}

	check(filepath.Join(ManagedSettingsDir, "managed-settings.json"), "managed",
		"Managed setting: takes precedence over the user value.")
	dropins, _ := filepath.Glob(filepath.Join(ManagedSettingsDir, "managed-settings.d", "*.json"))
	sort.Strings(dropins)
	for _, f := range dropins {
		check(f, "managed", "Managed setting (drop-in file): takes precedence over the user value.")
	}
	out = append(out, registryOverrides()...)
	check(loc.RemoteSettingsFile(), "remote",
		"Server-managed setting: takes precedence over the user value.")

	seen := map[string]bool{}
	for _, dir := range projectDirs {
		key := NormalizePath(dir)
		if seen[key] {
			continue
		}
		seen[key] = true
		for _, name := range []string{"settings.json", "settings.local.json"} {
			check(filepath.Join(dir, ".claude", name), "project",
				"Project setting: applies when Claude Code is started in this directory.")
		}
	}

	if v := strings.TrimSpace(os.Getenv("CLAUDE_CODE_SKIP_PROMPT_HISTORY")); v != "" && v != "0" && !strings.EqualFold(v, "false") {
		out = append(out, Override{Source: "CLAUDE_CODE_SKIP_PROMPT_HISTORY", Kind: "env",
			Note: "Environment variable set: Claude Code does not save transcripts."})
	}
	return out
}
