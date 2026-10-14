package ccdata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ts(h int) time.Time { return time.Date(2026, 10, 1, h, 0, 0, 0, time.UTC) }

func TestMerge(t *testing.T) {
	known := []string{
		`C:/Users/dev/Documents/sources/Corelink`,
		`C:\Users\dev\Documents\sources\Corelink`, // duplicate with other separator
		`C:\Users\dev\Old`,                        // no transcripts left
	}
	byFolder := map[string][]Session{
		`P\C--Users-dev-Documents-sources-Corelink`: {
			{ID: "a", Cwd: `c:\users\dev\documents\sources\corelink`, LastEvent: ts(1), FileSize: 10},
			{ID: "b", Cwd: `C:\Users\dev\Documents\sources\Corelink\`, LastEvent: ts(5), FileSize: 20, ExtraSize: 5},
		},
		// Same encoded name for "a-b" and "a b": attribution must use cwd, not the name.
		`P\C--x-a-b`: {
			{ID: "c", Cwd: `C:\x\a b`, LastEvent: ts(3)},
			{ID: "d", Cwd: `C:\x\a-b`, LastEvent: ts(2)},
		},
		`P\C--nocwd`: {{ID: "e", LastEvent: ts(4)}},
	}
	dirs := Merge(known, byFolder)
	got := map[string]Directory{}
	for _, d := range dirs {
		got[d.Key] = d
	}
	if len(dirs) != 5 {
		t.Fatalf("got %d dirs: %+v", len(dirs), dirs)
	}

	cl := got[`c:\users\dev\documents\sources\corelink`]
	if !cl.InClaudeJSON || cl.SessionCount != 2 || cl.TotalSize != 35 || !cl.LastSession.Equal(ts(5)) {
		t.Errorf("corelink: %+v", cl)
	}
	if cl.Sessions[0].ID != "b" {
		t.Errorf("sessions not sorted by last event desc")
	}
	if cl.Path != `C:\Users\dev\Documents\sources\Corelink` {
		t.Errorf("display path = %q", cl.Path)
	}

	old := got[`c:\users\dev\old`]
	if !old.InClaudeJSON || old.SessionCount != 0 || old.Sessions == nil {
		t.Errorf("old: %+v", old)
	}

	if got[`c:\x\a b`].SessionCount != 1 || got[`c:\x\a-b`].SessionCount != 1 {
		t.Errorf("ambiguous folder not split by cwd")
	}
	if got[`c:\x\a b`].InClaudeJSON {
		t.Errorf("transcript-only dir marked as in .claude.json")
	}

	var unresolved *Directory
	for i := range dirs {
		if dirs[i].Unresolved {
			unresolved = &dirs[i]
		}
	}
	if unresolved == nil || unresolved.Path != "C--nocwd" || unresolved.SessionCount != 1 {
		t.Errorf("unresolved: %+v", unresolved)
	}

	// Default order: most recent session first, empty dirs last.
	if dirs[0].Key != `c:\users\dev\documents\sources\corelink` || dirs[len(dirs)-1].Key != `c:\users\dev\old` {
		t.Errorf("order: first %q last %q", dirs[0].Key, dirs[len(dirs)-1].Key)
	}
}

func TestScannerEndToEnd(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, ".claude")
	work := filepath.Join(root, "work dir")
	gone := filepath.Join(root, "gone")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(base, "projects", "encoded")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	cwdJSON := strings.ReplaceAll(work, `\`, `\\`)
	line := `{"type":"user","cwd":"` + cwdJSON + `","timestamp":"2026-10-01T10:00:00Z","message":{"content":"hello"}}` + "\n"
	if err := os.WriteFile(filepath.Join(folder, "s1.jsonl"), []byte(line+"garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cj := `{"numStartups":3,"projects":{"` + strings.ReplaceAll(gone, `\`, `\\`) + `":{"allowedTools":[]}}}`
	claudeJSON := filepath.Join(root, ".claude.json")
	if err := os.WriteFile(claudeJSON, []byte(cj), 0o644); err != nil {
		t.Fatal(err)
	}

	sc := NewScanner(Locations{Base: base, ClaudeJSON: claudeJSON})
	res := sc.Scan()
	if len(res.Directories) != 2 {
		t.Fatalf("dirs: %+v errors: %v", res.Directories, res.Errors)
	}
	for _, d := range res.Directories {
		switch d.Key {
		case NormalizePath(work):
			if !d.Exists || d.SessionCount != 1 || d.Sessions[0].Preview != "hello" {
				t.Errorf("work: %+v", d)
			}
		case NormalizePath(gone):
			if d.Exists || !d.InClaudeJSON {
				t.Errorf("gone: %+v", d)
			}
		default:
			t.Errorf("unexpected dir %q", d.Key)
		}
	}
	if len(res.Errors) != 1 || !strings.Contains(res.Errors[0], "invalid lines") {
		t.Errorf("errors: %v", res.Errors)
	}
	// Second scan hits the cache and gives the same result.
	if res2 := sc.Scan(); len(res2.Directories) != 2 || len(sc.cache) != 1 {
		t.Errorf("rescan: %+v", res2)
	}
}

func TestScannerMissingSources(t *testing.T) {
	root := t.TempDir()
	res := NewScanner(Locations{Base: filepath.Join(root, "none"), ClaudeJSON: filepath.Join(root, "none.json")}).Scan()
	if len(res.Directories) != 0 || len(res.Errors) != 1 {
		t.Errorf("got %+v", res)
	}
}

func TestParseProjectPaths(t *testing.T) {
	p, err := parseProjectPaths(strings.NewReader(`{"projects":{"C:/b":{},"C:\\a":{"x":1},"":{}},"other":1}`))
	if err != nil || len(p) != 2 || p[0] != "C:/b" || p[1] != `C:\a` {
		t.Errorf("got %v %v", p, err)
	}
	if _, err := parseProjectPaths(strings.NewReader(`{"projects":`)); err == nil {
		t.Error("expected error for truncated file")
	}
	p, err = parseProjectPaths(strings.NewReader(`{}`))
	if err != nil || len(p) != 0 {
		t.Errorf("no projects key: %v %v", p, err)
	}
}
