package ccdata

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

// Directory is a working directory in which Claude Code was started,
// merged from .claude.json and the transcripts found on disk.
type Directory struct {
	// Path is the display path (original case, Windows separators).
	Path string `json:"path"`
	// Key is the normalized comparison key (see NormalizePath).
	Key string `json:"key"`
	// Exists reports whether the directory is still present on disk.
	Exists bool `json:"exists"`
	// InClaudeJSON reports whether the directory has an entry in .claude.json.
	InClaudeJSON bool `json:"inClaudeJson"`
	// Unresolved is set when transcripts carried no cwd, so Path is only the
	// (ambiguous) encoded folder name.
	Unresolved     bool      `json:"unresolved"`
	Sessions       []Session `json:"sessions"`
	SessionCount   int       `json:"sessionCount"`
	LastSession    time.Time `json:"lastSession" ts_type:"string"`
	TotalSize      int64     `json:"totalSize"`
	TranscriptDirs []string  `json:"transcriptDirs"`
	// ProjectFolder is <base>\projects\<encoded path> when it exists. Besides
	// transcripts it may hold the project's auto-memory (memory\).
	ProjectFolder string `json:"projectFolder"`
	HasMemory     bool   `json:"hasMemory"`
	// Active is set when a running Claude Code process uses this directory.
	Active bool `json:"active"`
}

// ScanResult is the outcome of a full scan. Errors are non-fatal problems
// (unreadable files etc.) meant for a status bar.
type ScanResult struct {
	Directories []Directory `json:"directories"`
	Errors      []string    `json:"errors"`
	ScannedAt   time.Time   `json:"scannedAt" ts_type:"string"`
	DurationMs  int64       `json:"durationMs"`
	Locations   Locations   `json:"locations"`
}

// AllSessions returns every session across all directories.
func (r ScanResult) AllSessions() []Session {
	var all []Session
	for _, d := range r.Directories {
		all = append(all, d.Sessions...)
	}
	return all
}

type cacheEntry struct {
	size    int64
	modTime time.Time
	session Session
}

// Scanner scans Claude Code data. It caches parsed transcripts keyed by
// path, size and mtime, so a refresh only re-reads files that changed.
// It is safe for concurrent use.
type Scanner struct {
	Loc Locations

	mu    sync.Mutex
	cache map[string]cacheEntry
}

// NewScanner returns a scanner for the given locations.
func NewScanner(loc Locations) *Scanner {
	return &Scanner{Loc: loc, cache: map[string]cacheEntry{}}
}

// Scan reads .claude.json and all transcripts and merges them by directory.
func (s *Scanner) Scan() ScanResult {
	start := time.Now()
	res := ScanResult{Locations: s.Loc}
	var errMu sync.Mutex
	addErr := func(format string, args ...any) {
		errMu.Lock()
		res.Errors = append(res.Errors, fmt.Sprintf(format, args...))
		errMu.Unlock()
	}

	var known []string
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		p, err := ReadProjectPaths(s.Loc.ClaudeJSON)
		if err != nil {
			addErr("%s: %v", s.Loc.ClaudeJSON, err)
		}
		known = p
	}()

	folders, err := os.ReadDir(s.Loc.ProjectsDir())
	if err != nil && !os.IsNotExist(err) {
		addErr("%s: %v", s.Loc.ProjectsDir(), err)
	}
	type folderResult struct {
		folder   string
		sessions []Session
	}
	jobs := make(chan string)
	results := make(chan folderResult)
	workers := min(runtime.NumCPU(), 16)
	var wwg sync.WaitGroup
	for range workers {
		wwg.Add(1)
		go func() {
			defer wwg.Done()
			for folder := range jobs {
				results <- folderResult{folder, s.scanFolder(folder, addErr)}
			}
		}()
	}
	go func() {
		for _, f := range folders {
			if f.IsDir() {
				jobs <- filepath.Join(s.Loc.ProjectsDir(), f.Name())
			}
		}
		close(jobs)
		wwg.Wait()
		close(results)
	}()
	var scanned []folderResult
	for r := range results {
		scanned = append(scanned, r)
	}
	wg.Wait()

	byFolder := map[string][]Session{}
	for _, r := range scanned {
		byFolder[r.folder] = r.sessions
	}
	res.Directories = Merge(known, byFolder)
	markExisting(res.Directories)
	s.attachProjectFolders(res.Directories)
	markActive(res.Directories, ReadLiveSessions(s.Loc.SessionsDir()))
	s.prune(byFolder)

	res.ScannedAt = time.Now()
	res.DurationMs = time.Since(start).Milliseconds()
	sort.Strings(res.Errors)
	return res
}

func (s *Scanner) scanFolder(folder string, addErr func(string, ...any)) []Session {
	entries, err := os.ReadDir(folder)
	if err != nil {
		addErr("%s: %v", folder, err)
		return nil
	}
	var sessions []Session
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".jsonl") {
			continue
		}
		path := filepath.Join(folder, e.Name())
		info, err := e.Info()
		if err != nil {
			addErr("%s: %v", path, err)
			continue
		}
		if sess, ok := s.cached(path, info); ok {
			sessions = append(sessions, sess)
			continue
		}
		sess, err := ReadSession(path)
		if err != nil {
			addErr("%s: %v", path, err)
			if sess.ID == "" {
				continue
			}
		}
		if sess.BadLines > 0 {
			addErr("%s: skipped %d invalid lines", path, sess.BadLines)
		}
		s.store(path, info, sess)
		sessions = append(sessions, sess)
	}
	return sessions
}

func (s *Scanner) cached(path string, info os.FileInfo) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.cache[path]
	if !ok || e.size != info.Size() || !e.modTime.Equal(info.ModTime()) {
		return Session{}, false
	}
	// The sibling folder may have grown without the .jsonl changing.
	sess := e.session
	sess.ExtraSize = dirSize(filepath.Join(filepath.Dir(path), sess.ID))
	return sess, true
}

func (s *Scanner) store(path string, info os.FileInfo, sess Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[path] = cacheEntry{size: info.Size(), modTime: info.ModTime(), session: sess}
}

// prune drops cache entries for transcripts that no longer exist.
func (s *Scanner) prune(byFolder map[string][]Session) {
	live := map[string]bool{}
	for _, ss := range byFolder {
		for _, x := range ss {
			live[x.File] = true
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.cache {
		if !live[k] {
			delete(s.cache, k)
		}
	}
}

// Merge combines the paths from .claude.json with the sessions found per
// transcript folder. Each session is attributed to its own cwd (not to the
// folder name, whose encoding is ambiguous). Sessions without a cwd are
// grouped under their folder name and flagged Unresolved. Paths are matched
// case- and separator-insensitively; directories with no sessions are kept.
func Merge(known []string, byFolder map[string][]Session) []Directory {
	dirs := map[string]*Directory{}
	get := func(key, display string) *Directory {
		d, ok := dirs[key]
		if !ok {
			d = &Directory{Key: key, Path: display}
			dirs[key] = d
		}
		return d
	}
	for _, p := range known {
		key := NormalizePath(p)
		if key == "" {
			continue
		}
		get(key, DisplayPath(p)).InClaudeJSON = true
	}
	folders := make([]string, 0, len(byFolder))
	for f := range byFolder {
		folders = append(folders, f)
	}
	sort.Strings(folders)
	for _, folder := range folders {
		for _, sess := range byFolder[folder] {
			var d *Directory
			if sess.Cwd != "" {
				d = get(NormalizePath(sess.Cwd), DisplayPath(sess.Cwd))
			} else {
				name := filepath.Base(folder)
				d = get("?"+strings.ToLower(name), name)
				d.Unresolved = true
			}
			d.Sessions = append(d.Sessions, sess)
			if !containsFold(d.TranscriptDirs, folder) {
				d.TranscriptDirs = append(d.TranscriptDirs, folder)
			}
		}
	}

	out := make([]Directory, 0, len(dirs))
	for _, d := range dirs {
		sort.Slice(d.Sessions, func(i, j int) bool { return d.Sessions[i].LastEvent.After(d.Sessions[j].LastEvent) })
		d.SessionCount = len(d.Sessions)
		for _, s := range d.Sessions {
			d.TotalSize += s.TotalSize()
			if s.LastEvent.After(d.LastSession) {
				d.LastSession = s.LastEvent
			}
		}
		// Prefer the spelling Claude Code recorded most recently (a real cwd).
		if len(d.Sessions) > 0 && d.Sessions[0].Cwd != "" {
			d.Path = DisplayPath(d.Sessions[0].Cwd)
		}
		if d.Sessions == nil {
			d.Sessions = []Session{}
		}
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].LastSession.Equal(out[j].LastSession) {
			return out[i].LastSession.After(out[j].LastSession)
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// markExisting stats every directory in parallel (offline network or
// removable drives can be slow to answer).
func markExisting(dirs []Directory) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for i := range dirs {
		if dirs[i].Unresolved {
			continue
		}
		wg.Add(1)
		go func(d *Directory) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			fi, err := os.Stat(d.Path)
			d.Exists = err == nil && fi.IsDir()
		}(&dirs[i])
	}
	wg.Wait()
}

// maxProjectDirName is the length above which Claude Code truncates the
// encoded folder name and appends a hash of the original path.
const maxProjectDirName = 200

// EncodeProjectPath returns the folder name Claude Code uses under
// <base>\projects for a working directory. Claude Code applies the
// JavaScript regex /[^a-zA-Z0-9]/g, which works on UTF-16 code units: every
// code unit that is not an ASCII letter or digit becomes '-', so "à" gives one
// '-' and a character outside the BMP (e.g. an emoji) gives two. Names longer
// than 200 characters are cut to 200 and suffixed with "-" and the base-36
// absolute value of a 32-bit string hash of the original path. This matches
// Claude Code 2.1.x; the rule is not documented, so callers should not rely
// on it alone. The mapping is not reversible.
func EncodeProjectPath(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r < 128 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		units := utf16.RuneLen(r)
		if units < 1 {
			units = 1
		}
		b.WriteString(strings.Repeat("-", units))
	}
	name := b.String() // ASCII only, so len is the UTF-16 length
	if len(name) <= maxProjectDirName {
		return name
	}
	return name[:maxProjectDirName] + "-" + strconv.FormatInt(abs64(int64(jsStringHash(p))), 36)
}

// jsStringHash is Claude Code's path hash: h = h*31 + c over the UTF-16 code
// units of s, wrapping at 32 bits.
func jsStringHash(s string) int32 {
	var h int32
	for _, u := range utf16.Encode([]rune(s)) {
		h = h*31 + int32(u)
	}
	return h
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// attachProjectFolders finds each directory's folder under <base>\projects.
// The folder name is computed with EncodeProjectPath; when no such folder
// exists but all the directory's transcripts live in a single folder, that
// folder is used instead, so the mapping survives encoding changes.
func (s *Scanner) attachProjectFolders(dirs []Directory) {
	for i := range dirs {
		d := &dirs[i]
		if !d.Unresolved {
			for _, candidate := range projectPathSpellings(*d) {
				if f := filepath.Join(s.Loc.ProjectsDir(), EncodeProjectPath(candidate)); isDir(f) {
					d.ProjectFolder = f
					break
				}
			}
		}
		if d.ProjectFolder == "" && len(d.TranscriptDirs) == 1 &&
			isChildOf(d.TranscriptDirs[0], s.Loc.ProjectsDir(), 1) && isDir(d.TranscriptDirs[0]) {
			d.ProjectFolder = d.TranscriptDirs[0]
		}
		if d.ProjectFolder != "" {
			d.HasMemory = isDir(filepath.Join(d.ProjectFolder, "memory"))
		}
	}
}

// projectPathSpellings returns the spellings of a directory's path that
// Claude Code may have encoded. They only differ for truncated (hashed)
// names, where the hash depends on the exact separators and case.
func projectPathSpellings(d Directory) []string {
	out := []string{d.Path}
	add := func(p string) {
		for _, x := range out {
			if x == p {
				return
			}
		}
		out = append(out, p)
	}
	for _, s := range d.Sessions {
		if s.Cwd != "" {
			add(s.Cwd)
		}
	}
	add(strings.ReplaceAll(d.Path, `\`, "/"))
	return out
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
