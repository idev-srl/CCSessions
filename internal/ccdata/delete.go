package ccdata

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// RecentWriteWindow protects transcripts that were written very recently:
// they may belong to a session whose process is not registered (yet).
const RecentWriteWindow = 2 * time.Minute

var sessionIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

// ValidSessionID reports whether id looks like a Claude Code session id.
func ValidSessionID(id string) bool { return sessionIDRe.MatchString(id) }

// Hooks for tests.
var (
	liveSessionsFn    = ReadLiveSessions
	claudeProcessesFn = RunningClaudeProcesses
	nowFn             = time.Now
)

// isChildOf reports whether p is exactly depth levels below parent.
func isChildOf(p, parent string, depth int) bool {
	rel, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(p))
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return false
	}
	return len(strings.Split(rel, string(filepath.Separator))) == depth
}

// checkSessionFile validates that file is <projects>\<folder>\<id>.jsonl.
func checkSessionFile(loc Locations, id, file string) error {
	if !ValidSessionID(id) {
		return fmt.Errorf("invalid session id %q", id)
	}
	if !strings.EqualFold(filepath.Base(file), id+".jsonl") || !isChildOf(file, loc.ProjectsDir(), 2) {
		return fmt.Errorf("refusing to delete %s: not a transcript inside %s", file, loc.ProjectsDir())
	}
	return nil
}

// DeleteSession deletes one session: its transcript (<id>.jsonl) and the
// sibling <id>\ folder (subagent transcripts, tool results). It refuses
// sessions owned by a running Claude Code process or written to within
// RecentWriteWindow.
func DeleteSession(loc Locations, id, file string) error {
	if err := checkSessionFile(loc, id, file); err != nil {
		return err
	}
	for _, l := range liveSessionsFn(loc.SessionsDir()) {
		if l.SessionID == id {
			return fmt.Errorf("session %s is in use by Claude Code (PID %d)", id, l.PID)
		}
	}
	fi, err := os.Stat(file)
	if err != nil {
		return err
	}
	if age := nowFn().Sub(fi.ModTime()); age < RecentWriteWindow {
		return fmt.Errorf("session %s was written %s ago and may still be in use; try again later", id, age.Round(time.Second))
	}
	if err := os.Remove(file); err != nil {
		return err
	}
	return removeAllIfExists(filepath.Join(filepath.Dir(file), id))
}

func removeAllIfExists(p string) error {
	if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return os.RemoveAll(p)
}

// RemovalPlan describes everything a full directory removal will delete.
// It is shown to the user for confirmation before ExecuteRemoval runs.
type RemovalPlan struct {
	Path string `json:"path"`
	Key  string `json:"key"`
	// ClaudeJSONKeys are the entries of .claude.json "projects" to remove
	// (the same directory may be listed with / and with \).
	ClaudeJSONKeys []string `json:"claudeJsonKeys"`
	// ProjectFolder is deleted entirely (transcripts and auto-memory) when no
	// other directory has sessions in it.
	ProjectFolder string `json:"projectFolder"`
	HasMemory     bool   `json:"hasMemory"`
	// SessionFiles are transcripts deleted individually (with their sibling
	// folder) because they live in a folder shared with other directories.
	SessionFiles []string `json:"sessionFiles"`
	Sessions     int      `json:"sessions"`
	Bytes        int64    `json:"bytes"`
	// Blockers are reasons the removal cannot run now.
	Blockers []string `json:"blockers"`
}

// PlanRemoval works out what removing directory d would delete. all is the
// full directory list from the same scan, used to detect shared folders.
func PlanRemoval(loc Locations, d Directory, all []Directory) RemovalPlan {
	p := RemovalPlan{Path: d.Path, Key: d.Key, Sessions: len(d.Sessions), ClaudeJSONKeys: []string{}, SessionFiles: []string{}, Blockers: []string{}}

	if paths, err := ReadProjectPaths(loc.ClaudeJSON); err == nil {
		for _, k := range paths {
			if NormalizePath(k) == d.Key {
				p.ClaudeJSONKeys = append(p.ClaudeJSONKeys, k)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		p.Blockers = append(p.Blockers, fmt.Sprintf("cannot read %s: %v", loc.ClaudeJSON, err))
	}

	shared := map[string]bool{}
	for _, o := range all {
		if o.Key == d.Key {
			continue
		}
		for _, f := range o.TranscriptDirs {
			shared[strings.ToLower(filepath.Clean(f))] = true
		}
	}
	if d.ProjectFolder != "" && isChildOf(d.ProjectFolder, loc.ProjectsDir(), 1) &&
		!shared[strings.ToLower(filepath.Clean(d.ProjectFolder))] {
		p.ProjectFolder = d.ProjectFolder
		p.HasMemory = d.HasMemory
	}
	for _, s := range d.Sessions {
		p.Bytes += s.TotalSize()
		if p.ProjectFolder != "" && strings.EqualFold(filepath.Dir(s.File), p.ProjectFolder) {
			continue
		}
		p.SessionFiles = append(p.SessionFiles, s.File)
	}
	sort.Strings(p.SessionFiles)

	p.Blockers = append(p.Blockers, claudeRunningBlockers(loc)...)
	if len(p.ClaudeJSONKeys) == 0 && p.ProjectFolder == "" && len(p.SessionFiles) == 0 {
		p.Blockers = append(p.Blockers, "nothing to delete")
	}
	return p
}

// claudeRunningBlockers returns a message for each sign that Claude Code is
// running. Claude Code rewrites .claude.json while running, so editing it
// is only safe when no instance is open.
func claudeRunningBlockers(loc Locations) []string {
	var out []string
	seen := map[int]bool{}
	for _, l := range liveSessionsFn(loc.SessionsDir()) {
		seen[l.PID] = true
		out = append(out, fmt.Sprintf("Claude Code is running in %s (PID %d)", DisplayPath(l.Cwd), l.PID))
	}
	var others []string
	for _, pid := range claudeProcessesFn() {
		if !seen[pid] {
			others = append(others, fmt.Sprint(pid))
		}
	}
	if len(others) > 0 {
		out = append(out, fmt.Sprintf("other claude.exe processes are running (PID %s)", strings.Join(others, ", ")))
	}
	return out
}

// RemovalResult reports what ExecuteRemoval did.
type RemovalResult struct {
	ClaudeJSONBackup string   `json:"claudeJsonBackup"`
	RemovedKeys      []string `json:"removedKeys"`
	Errors           []string `json:"errors"`
}

// ExecuteRemoval carries out a plan made by PlanRemoval. It re-checks that
// Claude Code is not running, removes the .claude.json entries first (after
// a backup) and only then deletes the files, so a failure to update
// .claude.json leaves everything untouched.
func ExecuteRemoval(loc Locations, p RemovalPlan) (RemovalResult, error) {
	res := RemovalResult{RemovedKeys: []string{}, Errors: []string{}}
	if b := claudeRunningBlockers(loc); len(b) > 0 {
		return res, fmt.Errorf("close Claude Code first: %s", strings.Join(b, "; "))
	}
	for _, f := range p.SessionFiles {
		if err := checkSessionFile(loc, strings.TrimSuffix(filepath.Base(f), filepath.Ext(f)), f); err != nil {
			return res, err
		}
	}
	if p.ProjectFolder != "" && !isChildOf(p.ProjectFolder, loc.ProjectsDir(), 1) {
		return res, fmt.Errorf("refusing to delete %s: not inside %s", p.ProjectFolder, loc.ProjectsDir())
	}

	if p.Key != "" {
		backup, removed, err := RemoveProjectEntries(loc.ClaudeJSON, p.Key, nowFn())
		if err != nil {
			return res, err
		}
		res.ClaudeJSONBackup, res.RemovedKeys = backup, removed
	}

	if p.ProjectFolder != "" {
		if err := os.RemoveAll(p.ProjectFolder); err != nil {
			res.Errors = append(res.Errors, err.Error())
		}
	}
	for _, f := range p.SessionFiles {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			res.Errors = append(res.Errors, err.Error())
			continue
		}
		id := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
		if err := removeAllIfExists(filepath.Join(filepath.Dir(f), id)); err != nil {
			res.Errors = append(res.Errors, err.Error())
		}
	}
	return res, nil
}

// RemoveProjectEntries deletes from the "projects" object of .claude.json
// every key that normalizes to key, keeping everything else intact. The
// original is first copied to "<path>.ccsessions-bak-<timestamp>" and the
// file is replaced atomically. Nothing is written when no key matches.
// Callers must make sure Claude Code is not running.
func RemoveProjectEntries(path, key string, now time.Time) (backup string, removed []string, err error) {
	orig, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", []string{}, nil
	}
	if err != nil {
		return "", nil, err
	}
	removed = []string{}
	out, err := editObject(orig, func(members []member) ([]member, error) {
		for i, m := range members {
			if m.key != "projects" {
				continue
			}
			projects, err := parseObject(m.value)
			if err != nil {
				return nil, fmt.Errorf("projects: %w", err)
			}
			kept := projects[:0]
			for _, pm := range projects {
				if NormalizePath(pm.key) == key {
					removed = append(removed, pm.key)
				} else {
					kept = append(kept, pm)
				}
			}
			members[i].value = marshalObject(kept)
		}
		return members, nil
	})
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if len(removed) == 0 {
		return "", removed, nil
	}
	// Sanity check before replacing a file Claude Code depends on.
	if !json.Valid(stripBOM(out)) {
		return "", nil, errors.New("internal error: produced invalid JSON")
	}
	if backup, err = backupFile(path, ".ccsessions-bak-", orig, now.Format(backupStamp)); err != nil {
		return "", nil, err
	}
	if err := atomicWrite(path, out); err != nil {
		return backup, nil, err
	}
	return backup, removed, nil
}
