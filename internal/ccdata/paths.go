// Package ccdata reads Claude Code's local data (project list, session
// transcripts, settings) without depending on any UI framework.
//
// Everything here is read-only with respect to Claude Code's own state files
// (~/.claude.json and the *.jsonl transcripts). The only file this package
// ever writes is the user settings.json, and only the cleanupPeriodDays key.
package ccdata

import (
	"os"
	"path/filepath"
	"strings"
)

// Locations holds the resolved paths of Claude Code's data files.
type Locations struct {
	// Base is the Claude config directory: %CLAUDE_CONFIG_DIR% or %USERPROFILE%\.claude.
	Base string
	// ClaudeJSON is the global state file containing the "projects" map.
	ClaudeJSON string
}

// ProjectsDir is the directory holding one folder of transcripts per working directory.
func (l Locations) ProjectsDir() string { return filepath.Join(l.Base, "projects") }

// SettingsFile is the user settings file (where cleanupPeriodDays lives).
func (l Locations) SettingsFile() string { return filepath.Join(l.Base, "settings.json") }

// LastCleanupFile is written by Claude Code with the time of its last cleanup sweep.
func (l Locations) LastCleanupFile() string { return filepath.Join(l.Base, ".last-cleanup") }

// RemoteSettingsFile caches server-managed settings, which rank above user settings.
func (l Locations) RemoteSettingsFile() string { return filepath.Join(l.Base, "remote-settings.json") }

// DefaultLocations resolves the locations from the environment.
//
// The documentation states that CLAUDE_CONFIG_DIR replaces ~/.claude. It does
// not say where .claude.json goes in that case, so with CLAUDE_CONFIG_DIR set
// we prefer <CLAUDE_CONFIG_DIR>\.claude.json when it exists and otherwise fall
// back to %USERPROFILE%\.claude.json.
func DefaultLocations() Locations {
	return resolveLocations(os.Getenv("CLAUDE_CONFIG_DIR"), userHome(), fileExists)
}

func resolveLocations(configDir, home string, exists func(string) bool) Locations {
	homeJSON := filepath.Join(home, ".claude.json")
	if configDir == "" {
		return Locations{Base: filepath.Join(home, ".claude"), ClaudeJSON: homeJSON}
	}
	loc := Locations{Base: configDir, ClaudeJSON: homeJSON}
	if candidate := filepath.Join(configDir, ".claude.json"); exists(candidate) {
		loc.ClaudeJSON = candidate
	}
	return loc
}

func userHome() string {
	if h := os.Getenv("USERPROFILE"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// NormalizePath returns a comparison key for a Windows path: forward slashes
// become backslashes, duplicate separators and trailing separators are removed
// (except for a drive root) and the result is lower-cased. It is a pure string
// operation, so it behaves the same on every OS.
func NormalizePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, `\\?\`)
	p = strings.TrimPrefix(p, `//?/`)
	p = strings.ReplaceAll(p, "/", `\`)
	unc := strings.HasPrefix(p, `\\`)
	for strings.Contains(p, `\\`) {
		p = strings.ReplaceAll(p, `\\`, `\`)
	}
	if unc {
		p = `\` + p
	}
	for len(p) > 1 && strings.HasSuffix(p, `\`) && !isDriveRoot(p) {
		p = strings.TrimSuffix(p, `\`)
	}
	if len(p) == 2 && p[1] == ':' { // "C:" -> "C:\"
		p += `\`
	}
	return strings.ToLower(p)
}

// DisplayPath returns the path with Windows separators but its original case.
func DisplayPath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "/", `\`)
	for len(p) > 3 && strings.HasSuffix(p, `\`) {
		p = strings.TrimSuffix(p, `\`)
	}
	return p
}

// IsAbsWindowsPath reports whether p looks like an absolute Windows path
// (drive-letter or UNC), independent of the host OS.
func IsAbsWindowsPath(p string) bool {
	if len(p) >= 3 && isLetter(p[0]) && p[1] == ':' && (p[2] == '\\' || p[2] == '/') {
		return true
	}
	return strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, "//")
}

func isDriveRoot(p string) bool { return len(p) == 3 && isLetter(p[0]) && p[1] == ':' && p[2] == '\\' }

func isLetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }
