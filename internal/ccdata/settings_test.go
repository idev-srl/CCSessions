package ccdata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const realSettings = `{
  "enabledPlugins": {
    "chrome-devtools-mcp@claude-plugins-official": true,
    "gopls-lsp@claude-plugins-official": true
  },
  "language": "italian",
  "tui": "fullscreen",
  "voice": {
    "enabled": true,
    "mode": "hold"
  }
}
`

func keysInOrder(t *testing.T, data []byte) []string {
	t.Helper()
	m, err := parseObject(stripBOM(data))
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, x := range m {
		keys = append(keys, x.key)
	}
	return keys
}

func TestSetTopLevelKeyPreservesOthers(t *testing.T) {
	out, err := setTopLevelKey([]byte(realSettings), CleanupKey, json.RawMessage("90"))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSuffix(realSettings, "\n}\n") + ",\n  \"cleanupPeriodDays\": 90\n}\n"
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	var before, after map[string]any
	_ = json.Unmarshal([]byte(realSettings), &before)
	_ = json.Unmarshal(out, &after)
	for k, v := range before {
		if b1, _ := json.Marshal(v); string(b1) != mustJSON(after[k]) {
			t.Errorf("key %s changed", k)
		}
	}
	if after[CleanupKey] != float64(90) {
		t.Errorf("cleanup = %v", after[CleanupKey])
	}
}

func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestSetTopLevelKeyReplacesInPlace(t *testing.T) {
	in := `{"a":1,"cleanupPeriodDays":30,"z":{"nested":[1,2]}}`
	out, err := setTopLevelKey([]byte(in), CleanupKey, json.RawMessage("7"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(keysInOrder(t, out), ","); got != "a,cleanupPeriodDays,z" {
		t.Errorf("order = %s", got)
	}
	if d, found, _ := cleanupDaysFromJSON(out); !found || d != 7 {
		t.Errorf("value = %d %v", d, found)
	}
	// A nested key with the same name must not be touched.
	in = `{"x":{"cleanupPeriodDays":5}}`
	out, _ = setTopLevelKey([]byte(in), CleanupKey, json.RawMessage("9"))
	if !strings.Contains(string(out), `"cleanupPeriodDays": 5`) || !strings.Contains(string(out), `"cleanupPeriodDays": 9`) {
		t.Errorf("nested key handling: %s", out)
	}
}

func TestSetTopLevelKeyCRLFAndBOM(t *testing.T) {
	in := "\xEF\xBB\xBF{\r\n  \"a\": \"x\"\r\n}\r\n"
	out, err := setTopLevelKey([]byte(in), CleanupKey, json.RawMessage("14"))
	if err != nil {
		t.Fatal(err)
	}
	want := "\xEF\xBB\xBF{\r\n  \"a\": \"x\",\r\n  \"cleanupPeriodDays\": 14\r\n}\r\n"
	if string(out) != want {
		t.Errorf("got %q", out)
	}
}

func TestSetTopLevelKeyRejectsInvalid(t *testing.T) {
	for _, in := range []string{`[1,2]`, `{"a":`, `{"a":1} trailing`, `"str"`} {
		if _, err := setTopLevelKey([]byte(in), CleanupKey, json.RawMessage("1")); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
	out, err := setTopLevelKey([]byte("  \n"), CleanupKey, json.RawMessage("3"))
	if err != nil || string(out) != "{\n  \"cleanupPeriodDays\": 3\n}\n" {
		t.Errorf("empty file: %q %v", out, err)
	}
}

func TestWriteCleanupDays(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	now := time.Date(2026, 10, 14, 15, 30, 0, 0, time.Local)

	// Missing file: created with only the key, no backup.
	backup, err := WriteCleanupDays(path, 45, now)
	if err != nil || backup != "" {
		t.Fatalf("create: %q %v", backup, err)
	}
	if d, found, err := ReadCleanupDays(path); err != nil || !found || d != 45 {
		t.Errorf("read back: %d %v %v", d, found, err)
	}

	// Existing file: backup has the original content, other keys survive.
	if err := os.WriteFile(path, []byte(realSettings), 0o644); err != nil {
		t.Fatal(err)
	}
	backup, err = WriteCleanupDays(path, 60, now)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(backup) != "settings.json.bak-20261014-153000" {
		t.Errorf("backup name %q", backup)
	}
	if b, _ := os.ReadFile(backup); string(b) != realSettings {
		t.Errorf("backup content differs")
	}
	got, _ := os.ReadFile(path)
	if keys := strings.Join(keysInOrder(t, got), ","); keys != "enabledPlugins,language,tui,voice,cleanupPeriodDays" {
		t.Errorf("keys = %s", keys)
	}
	// Same second again: a distinct backup name, nothing overwritten.
	backup2, err := WriteCleanupDays(path, 61, now)
	if err != nil || backup2 == backup {
		t.Errorf("second backup: %q %v", backup2, err)
	}
	// No temp files left behind.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
}

func TestWriteCleanupDaysValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	for _, d := range []int{0, -1, MaxCleanupDays + 1} {
		if _, err := WriteCleanupDays(path, d, time.Now()); err == nil {
			t.Errorf("days=%d accepted", d)
		}
	}
	// Invalid existing JSON: refuse and leave the file untouched.
	if err := os.WriteFile(path, []byte(`{broken`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteCleanupDays(path, 10, time.Now()); err == nil {
		t.Error("broken JSON accepted")
	}
	if b, _ := os.ReadFile(path); string(b) != `{broken` {
		t.Error("broken file modified")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("unexpected files: %v", entries)
	}
}

func TestReadCleanupDays(t *testing.T) {
	cases := []struct {
		in    string
		days  int
		found bool
		err   bool
	}{
		{`{}`, 0, false, false},
		{`{"cleanupPeriodDays": 14}`, 14, true, false},
		{`{"cleanupPeriodDays": "x"}`, 0, true, true},
		{`nope`, 0, false, true},
		{"", 0, false, false},
	}
	for _, c := range cases {
		d, f, err := cleanupDaysFromJSON([]byte(c.in))
		if d != c.days || f != c.found || (err != nil) != c.err {
			t.Errorf("%q: %d %v %v", c.in, d, f, err)
		}
	}
	if _, found, err := ReadCleanupDays(filepath.Join(t.TempDir(), "missing.json")); found || err != nil {
		t.Errorf("missing file: %v %v", found, err)
	}
}

func TestPreviewCleanup(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	ss := []Session{
		{ModTime: now.AddDate(0, 0, -40), FileSize: 100, ExtraSize: 1000},
		{ModTime: now.AddDate(0, 0, -10), FileSize: 50},
		{ModTime: now.AddDate(0, 0, -2), FileSize: 10},
	}
	p := PreviewCleanup(ss, 30, now)
	if p.Sessions != 1 || p.TranscriptBytes != 100 || p.ExtraBytes != 1000 || p.TotalSessions != 3 {
		t.Errorf("30d: %+v", p)
	}
	if p := PreviewCleanup(ss, 5, now); p.Sessions != 2 || p.TranscriptBytes != 150 {
		t.Errorf("5d: %+v", p)
	}
}

func TestFindOverrides(t *testing.T) {
	root := t.TempDir()
	old := ManagedSettingsDir
	ManagedSettingsDir = filepath.Join(root, "managed")
	defer func() { ManagedSettingsDir = old }()
	t.Setenv("CLAUDE_CODE_SKIP_PROMPT_HISTORY", "")

	write := func(p, s string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(ManagedSettingsDir, "managed-settings.json"), `{"cleanupPeriodDays": 7}`)
	write(filepath.Join(ManagedSettingsDir, "managed-settings.d", "10-x.json"), `{"model":"x"}`)
	proj := filepath.Join(root, "proj")
	write(filepath.Join(proj, ".claude", "settings.local.json"), `{"cleanupPeriodDays": 3}`)
	base := filepath.Join(root, ".claude")
	write(filepath.Join(base, "remote-settings.json"), `{}`)

	got := FindOverrides(Locations{Base: base}, []string{proj, proj, filepath.Join(root, "missing")})
	var kinds []string
	for _, o := range got {
		if o.Kind == "managed" && o.Source != filepath.Join(ManagedSettingsDir, "managed-settings.json") {
			continue // real registry policies on the test machine
		}
		kinds = append(kinds, o.Kind)
	}
	if strings.Join(kinds, ",") != "managed,project" {
		t.Errorf("got %+v", got)
	}
}
