package ccdata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeEnv builds a fake Claude Code base directory and stubs process checks.
type fakeEnv struct {
	t        *testing.T
	root     string
	loc      Locations
	live     []LiveSession
	procs    []int
	fakeTime time.Time
}

func newFakeEnv(t *testing.T) *fakeEnv {
	t.Helper()
	root := t.TempDir()
	e := &fakeEnv{t: t, root: root, fakeTime: time.Now().Add(time.Hour)}
	e.loc = Locations{Base: filepath.Join(root, ".claude"), ClaudeJSON: filepath.Join(root, ".claude.json")}
	oldLive, oldProcs, oldNow := liveSessionsFn, claudeProcessesFn, nowFn
	liveSessionsFn = func(string) []LiveSession { return e.live }
	claudeProcessesFn = func() []int { return e.procs }
	nowFn = func() time.Time { return e.fakeTime }
	t.Cleanup(func() { liveSessionsFn, claudeProcessesFn, nowFn = oldLive, oldProcs, oldNow })
	return e
}

func (e *fakeEnv) write(p, content string) string {
	e.t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
	return p
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestEncodeProjectPath(t *testing.T) {
	cases := map[string]string{
		`C:\Users\dev\Documents\sources\CCSessions`:   "C--Users-dev-Documents-sources-CCSessions",
		`C:\Users\dev\Documents\sources\Corelink API`: "C--Users-dev-Documents-sources-Corelink-API",
		`C:\src\CiasTuner_NanoPi`:                       "C--src-CiasTuner-NanoPi",
		`F:\droneID_ESP32`:                              "F--droneID-ESP32",
	}
	for in, want := range cases {
		if got := EncodeProjectPath(in); got != want {
			t.Errorf("EncodeProjectPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsChildOf(t *testing.T) {
	base := filepath.Join("C:", "base")
	if !isChildOf(filepath.Join(base, "a"), base, 1) || isChildOf(filepath.Join(base, "a", "b"), base, 1) {
		t.Error("depth 1")
	}
	if !isChildOf(filepath.Join(base, "a", "b.jsonl"), base, 2) {
		t.Error("depth 2")
	}
	for _, p := range []string{base, filepath.Join(base, "..", "x"), filepath.Join("C:", "other", "a")} {
		if isChildOf(p, base, 1) {
			t.Errorf("%s accepted", p)
		}
	}
}

func TestDeleteSession(t *testing.T) {
	e := newFakeEnv(t)
	folder := filepath.Join(e.loc.ProjectsDir(), "C--proj")
	file := e.write(filepath.Join(folder, "s1.jsonl"), "{}\n")
	sub := e.write(filepath.Join(folder, "s1", "subagents", "a.jsonl"), "{}\n")
	other := e.write(filepath.Join(folder, "s2.jsonl"), "{}\n")

	// In use by a running process.
	e.live = []LiveSession{{PID: 42, SessionID: "s1"}}
	if err := DeleteSession(e.loc, "s1", file); err == nil || !strings.Contains(err.Error(), "in use") {
		t.Errorf("active session: %v", err)
	}
	e.live = nil

	// Written moments ago.
	e.fakeTime = time.Now()
	if err := DeleteSession(e.loc, "s1", file); err == nil || !strings.Contains(err.Error(), "may still be in use") {
		t.Errorf("recent session: %v", err)
	}
	e.fakeTime = time.Now().Add(time.Hour)

	// Paths outside the projects folder or not matching the id are refused.
	outside := e.write(filepath.Join(e.root, "x.jsonl"), "{}")
	for _, c := range []struct{ id, file string }{
		{"x", outside}, {"s2", file}, {"../s1", file}, {"s1", folder},
	} {
		if err := DeleteSession(e.loc, c.id, c.file); err == nil {
			t.Errorf("accepted id=%q file=%q", c.id, c.file)
		}
	}

	if err := DeleteSession(e.loc, "s1", file); err != nil {
		t.Fatal(err)
	}
	if exists(file) || exists(sub) || !exists(other) {
		t.Errorf("after delete: file=%v sub=%v other=%v", exists(file), exists(sub), exists(other))
	}
}

const claudeJSONSample = `{
  "numStartups": 3,
  "projects": {
    "C:/Users/me/proj": {
      "allowedTools": []
    },
    "C:\\Users\\me\\proj": {
      "hasTrustDialogAccepted": true
    },
    "C:\\Users\\me\\keep": {
      "allowedTools": [
        "Bash"
      ]
    }
  },
  "userID": "abc"
}
`

func TestRemoveProjectEntries(t *testing.T) {
	e := newFakeEnv(t)
	e.write(e.loc.ClaudeJSON, claudeJSONSample)
	now := time.Date(2026, 10, 14, 16, 0, 0, 0, time.Local)

	backup, removed, err := RemoveProjectEntries(e.loc.ClaudeJSON, NormalizePath(`c:\users\me\proj`), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Errorf("removed = %v", removed)
	}
	if b, _ := os.ReadFile(backup); string(b) != claudeJSONSample {
		t.Error("backup differs from original")
	}
	got, _ := os.ReadFile(e.loc.ClaudeJSON)
	want := `{
  "numStartups": 3,
  "projects": {
    "C:\\Users\\me\\keep": {
      "allowedTools": [
        "Bash"
      ]
    }
  },
  "userID": "abc"
}
`
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}

	// No match: file untouched, no backup.
	before, _ := os.ReadFile(e.loc.ClaudeJSON)
	backup, removed, err = RemoveProjectEntries(e.loc.ClaudeJSON, `c:\nope`, now)
	if err != nil || backup != "" || len(removed) != 0 {
		t.Errorf("no match: %q %v %v", backup, removed, err)
	}
	if after, _ := os.ReadFile(e.loc.ClaudeJSON); string(after) != string(before) {
		t.Error("file changed without a match")
	}

	// Broken file: refused, untouched.
	e.write(e.loc.ClaudeJSON, `{"projects":{`)
	if _, _, err := RemoveProjectEntries(e.loc.ClaudeJSON, `c:\x`, now); err == nil {
		t.Error("broken JSON accepted")
	}
}

func TestPlanAndExecuteRemoval(t *testing.T) {
	e := newFakeEnv(t)
	e.write(e.loc.ClaudeJSON, claudeJSONSample)
	proj := filepath.Join(e.loc.ProjectsDir(), "C--Users-me-proj")
	shared := filepath.Join(e.loc.ProjectsDir(), "C--shared")
	e.write(filepath.Join(proj, "a.jsonl"), "{}")
	e.write(filepath.Join(proj, "a", "tool-results", "t.txt"), "x")
	mem := e.write(filepath.Join(proj, "memory", "MEMORY.md"), "m")
	e.write(filepath.Join(shared, "b.jsonl"), "{}")
	e.write(filepath.Join(shared, "b", "subagents", "s.jsonl"), "{}")
	keepSession := e.write(filepath.Join(shared, "c.jsonl"), "{}")

	key := NormalizePath(`C:\Users\me\proj`)
	target := Directory{
		Path: `C:\Users\me\proj`, Key: key, ProjectFolder: proj, HasMemory: true,
		TranscriptDirs: []string{proj, shared},
		Sessions: []Session{
			{ID: "a", File: filepath.Join(proj, "a.jsonl"), FileSize: 2},
			{ID: "b", File: filepath.Join(shared, "b.jsonl"), FileSize: 3},
		},
	}
	other := Directory{Key: `c:\other`, TranscriptDirs: []string{shared},
		Sessions: []Session{{ID: "c", File: keepSession}}}
	all := []Directory{target, other}

	// Claude Code running: blocked, and execution refuses too.
	e.procs = []int{1234}
	plan := PlanRemoval(e.loc, target, all)
	if len(plan.Blockers) != 1 || !strings.Contains(plan.Blockers[0], "1234") {
		t.Errorf("blockers = %v", plan.Blockers)
	}
	if _, err := ExecuteRemoval(e.loc, plan); err == nil {
		t.Error("executed while Claude Code is running")
	}
	if !exists(mem) {
		t.Error("files deleted while blocked")
	}
	e.procs = nil

	plan = PlanRemoval(e.loc, target, all)
	if len(plan.Blockers) != 0 || plan.ProjectFolder != proj || !plan.HasMemory ||
		len(plan.SessionFiles) != 1 || len(plan.ClaudeJSONKeys) != 2 || plan.Bytes != 5 {
		t.Fatalf("plan = %+v", plan)
	}
	res, err := ExecuteRemoval(e.loc, plan)
	if err != nil || len(res.Errors) != 0 || res.ClaudeJSONBackup == "" || len(res.RemovedKeys) != 2 {
		t.Fatalf("execute: %+v %v", res, err)
	}
	if exists(proj) || exists(filepath.Join(shared, "b.jsonl")) || exists(filepath.Join(shared, "b")) {
		t.Error("files left behind")
	}
	if !exists(keepSession) {
		t.Error("session of another directory deleted")
	}
	var doc struct{ Projects map[string]json.RawMessage }
	b, _ := os.ReadFile(e.loc.ClaudeJSON)
	if json.Unmarshal(b, &doc) != nil || len(doc.Projects) != 1 {
		t.Errorf("projects after removal: %s", b)
	}
}

func TestPlanRemovalSharedFolderKept(t *testing.T) {
	e := newFakeEnv(t)
	folder := filepath.Join(e.loc.ProjectsDir(), "C--x-a-b")
	target := Directory{Key: `c:\x\a b`, ProjectFolder: folder, TranscriptDirs: []string{folder},
		Sessions: []Session{{ID: "s", File: filepath.Join(folder, "s.jsonl")}}}
	other := Directory{Key: `c:\x\a-b`, TranscriptDirs: []string{folder}}
	plan := PlanRemoval(e.loc, target, []Directory{target, other})
	if plan.ProjectFolder != "" || len(plan.SessionFiles) != 1 {
		t.Errorf("shared folder must not be deleted whole: %+v", plan)
	}
}

func TestMarkActive(t *testing.T) {
	dirs := []Directory{
		{Key: `c:\a`, Sessions: []Session{{ID: "s1"}, {ID: "s2"}}},
		{Key: `c:\b`},
		{Key: `c:\c`},
	}
	markActive(dirs, []LiveSession{{SessionID: "s2", Cwd: `C:\x`}, {SessionID: "new", Cwd: `C:/b/`}})
	if !dirs[0].Active || dirs[0].Sessions[0].Active || !dirs[0].Sessions[1].Active {
		t.Errorf("dir a: %+v", dirs[0])
	}
	if !dirs[1].Active || dirs[2].Active {
		t.Errorf("b=%v c=%v", dirs[1].Active, dirs[2].Active)
	}
}

func TestReadLiveSessions(t *testing.T) {
	dir := t.TempDir()
	old := processAlive
	processAlive = func(pid int, _ string) bool { return pid == 1 }
	defer func() { processAlive = old }()
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("1.json", `{"pid":1,"sessionId":"alive","cwd":"C:\\a"}`)
	write("2.json", `{"pid":2,"sessionId":"dead"}`)
	write("3.json", `not json`)
	write("1.abc.key", `{}`)
	write("notes.json", `{"pid":1}`)
	live := ReadLiveSessions(dir)
	if len(live) != 1 || live[0].SessionID != "alive" {
		t.Errorf("live = %+v", live)
	}
}
