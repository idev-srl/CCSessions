package ccdata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LiveSession is a running Claude Code process, as recorded in
// <base>\sessions\<pid>.json.
type LiveSession struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	// ProcStart is the process creation time as a Windows FILETIME
	// (100 ns intervals since 1601), used to detect PID reuse.
	ProcStart string `json:"procStart"`
	Status    string `json:"status"`
}

// SessionsDir holds one <pid>.json file per running Claude Code process.
func (l Locations) SessionsDir() string { return filepath.Join(l.Base, "sessions") }

// processAlive reports whether pid is running and, when procStart is known,
// was started at that exact time. Replaced in tests.
var processAlive = isProcessAlive

// ReadLiveSessions returns the Claude Code processes that are still running.
// Stale files left behind by crashed processes are ignored.
func ReadLiveSessions(dir string) []LiveSession {
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var live []LiveSession
	for _, f := range files {
		if _, err := strconv.Atoi(strings.TrimSuffix(filepath.Base(f), ".json")); err != nil {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var s LiveSession
		if json.Unmarshal(data, &s) != nil || s.PID <= 0 {
			continue
		}
		if processAlive(s.PID, s.ProcStart) {
			live = append(live, s)
		}
	}
	return live
}

// markActive flags sessions and directories that belong to a running process.
func markActive(dirs []Directory, live []LiveSession) {
	ids := map[string]bool{}
	cwds := map[string]bool{}
	for _, l := range live {
		if l.SessionID != "" {
			ids[l.SessionID] = true
		}
		if l.Cwd != "" {
			cwds[NormalizePath(l.Cwd)] = true
		}
	}
	for i := range dirs {
		d := &dirs[i]
		d.Active = cwds[d.Key]
		for j := range d.Sessions {
			if ids[d.Sessions[j].ID] {
				d.Sessions[j].Active = true
				d.Active = true
			}
		}
	}
}
