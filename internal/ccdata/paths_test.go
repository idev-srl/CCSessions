package ccdata

import (
	"path/filepath"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	cases := map[string]string{
		`C:\Users\dev\Proj`:          `c:\users\dev\proj`,
		`C:/Users/dev/Proj`:          `c:\users\dev\proj`,
		`c:\users\dev\proj\`:         `c:\users\dev\proj`,
		`C:\Users\\dev//Proj/`:       `c:\users\dev\proj`,
		`C:\`:                        `c:\`,
		`C:/`:                        `c:\`,
		`C:`:                         `c:\`,
		`F:\NEW SDK PACKET 2025\SDK`: `f:\new sdk packet 2025\sdk`,
		`\\server\share\dir\`:        `\\server\share\dir`,
		`//server/share/dir`:         `\\server\share\dir`,
		`\\?\C:\Long\Path`:           `c:\long\path`,
		`  C:\Trim  `:                `c:\trim`,
	}
	for in, want := range cases {
		if got := NormalizePath(in); got != want {
			t.Errorf("NormalizePath(%q) = %q, want %q", in, got, want)
		}
	}
	if NormalizePath(`C:/Users/dev/Documents/sources/Corelink`) != NormalizePath(`C:\Users\dev\Documents\sources\Corelink`) {
		t.Error("forward and back slash variants must match")
	}
}

func TestDisplayPath(t *testing.T) {
	if got := DisplayPath(`C:/Users/dev/Proj/`); got != `C:\Users\dev\Proj` {
		t.Errorf("got %q", got)
	}
	if got := DisplayPath(`C:/`); got != `C:\` {
		t.Errorf("got %q", got)
	}
}

func TestIsAbsWindowsPath(t *testing.T) {
	for _, p := range []string{`C:\x`, `c:/x`, `\\srv\share`, `//srv/share`} {
		if !IsAbsWindowsPath(p) {
			t.Errorf("%q should be absolute", p)
		}
	}
	for _, p := range []string{``, `x`, `relative\path`, `C:`, `/usr/bin`, `1:\x`} {
		if IsAbsWindowsPath(p) {
			t.Errorf("%q should not be absolute", p)
		}
	}
}

func TestResolveLocations(t *testing.T) {
	never := func(string) bool { return false }
	always := func(string) bool { return true }
	home := filepath.Join("H", "home")

	l := resolveLocations("", home, never)
	if l.Base != filepath.Join(home, ".claude") || l.ClaudeJSON != filepath.Join(home, ".claude.json") {
		t.Errorf("default: %+v", l)
	}
	l = resolveLocations("cfg", home, never)
	if l.Base != "cfg" || l.ClaudeJSON != filepath.Join(home, ".claude.json") {
		t.Errorf("config dir without json: %+v", l)
	}
	l = resolveLocations("cfg", home, always)
	if l.ClaudeJSON != filepath.Join("cfg", ".claude.json") {
		t.Errorf("config dir with json: %+v", l)
	}
}
