package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/idev-srl/CCSessions/internal/ccdata"
)

// Windows process creation flags.
const (
	createNewConsole = 0x00000010
	createNoWindow   = 0x08000000
)

// Project links shown in the About page. OpenURL only opens these.
const (
	websiteURL = "https://idev-srl.com"
	repoURL    = "https://github.com/idev-srl/CCSessions"
	licenseURL = "https://github.com/idev-srl/CCSessions/blob/main/LICENSE"
)

// App is the object bound to the frontend.
type App struct {
	ctx     context.Context
	scanner *ccdata.Scanner

	mu   sync.Mutex
	last ccdata.ScanResult
}

// NewApp creates the application backend.
func NewApp() *App {
	return &App{scanner: ccdata.NewScanner(ccdata.DefaultLocations())}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// Scan rescans Claude Code's data and returns the merged directory list.
func (a *App) Scan() ccdata.ScanResult {
	res := a.scanner.Scan()
	a.mu.Lock()
	a.last = res
	a.mu.Unlock()
	return res
}

// Environment describes the app and the tools available on this machine.
type Environment struct {
	Version    string `json:"version"`
	HasWT      bool   `json:"hasWT"`
	HasVSCode  bool   `json:"hasVSCode"`
	License    string `json:"license"`
	Company    string `json:"company"`
	WebsiteURL string `json:"websiteUrl"`
	RepoURL    string `json:"repoUrl"`
	LicenseURL string `json:"licenseUrl"`
	ConfigDir  string `json:"configDir"`
	ClaudeJSON string `json:"claudeJson"`
}

// GetEnvironment returns version, licensing and tool availability.
func (a *App) GetEnvironment() Environment {
	_, wtErr := exec.LookPath("wt.exe")
	_, codeErr := exec.LookPath("code")
	return Environment{
		Version:    version,
		HasWT:      wtErr == nil,
		HasVSCode:  codeErr == nil,
		License:    "Apache License 2.0",
		Company:    "iDev srl",
		WebsiteURL: websiteURL,
		RepoURL:    repoURL,
		LicenseURL: licenseURL,
		ConfigDir:  a.scanner.Loc.Base,
		ClaudeJSON: a.scanner.Loc.ClaudeJSON,
	}
}

// OpenURL opens one of the project's links in the default browser.
func (a *App) OpenURL(url string) error {
	switch url {
	case websiteURL, repoURL, licenseURL:
		runtime.BrowserOpenURL(a.ctx, url)
		return nil
	}
	return fmt.Errorf("URL not allowed: %s", url)
}

// lookupDir returns the directory with the given path or key from the last scan.
func (a *App) lookupDir(pathOrKey string) (ccdata.Directory, []ccdata.Directory, error) {
	key := ccdata.NormalizePath(pathOrKey)
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, d := range a.last.Directories {
		if d.Key == key || d.Key == pathOrKey {
			return d, a.last.Directories, nil
		}
	}
	return ccdata.Directory{}, nil, fmt.Errorf("unknown directory: %s (refresh and try again)", pathOrKey)
}

// existingDir checks that path is a directory from the last scan that still
// exists, so the frontend cannot make us launch programs on arbitrary paths.
func (a *App) existingDir(path string) (string, error) {
	d, _, err := a.lookupDir(path)
	if err != nil {
		return "", err
	}
	if d.Unresolved {
		return "", errors.New("the directory of this transcript folder is unknown")
	}
	if fi, err := os.Stat(d.Path); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("the folder %s no longer exists", d.Path)
	}
	return d.Path, nil
}

// OpenExplorer opens the directory in File Explorer.
func (a *App) OpenExplorer(path string) error {
	dir, err := a.existingDir(path)
	if err != nil {
		return err
	}
	// explorer.exe returns exit code 1 even on success: start, don't wait.
	return startDetached(exec.Command("explorer.exe", dir))
}

// OpenVSCode opens the directory in Visual Studio Code.
func (a *App) OpenVSCode(path string) error {
	dir, err := a.existingDir(path)
	if err != nil {
		return err
	}
	code, err := exec.LookPath("code")
	if err != nil {
		return errors.New("Visual Studio Code (the 'code' command) is not in PATH")
	}
	cmd := exec.Command(code, dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow} // code is a .cmd script
	return startDetached(cmd)
}

// OpenTerminal opens a terminal in the directory. mode is "" (plain shell),
// "continue" (claude --continue) or "resume" (claude --resume [sessionID]).
// It uses Windows Terminal when available, otherwise a PowerShell console.
func (a *App) OpenTerminal(path, mode, sessionID string) error {
	dir, err := a.existingDir(path)
	if err != nil {
		return err
	}
	var claudeArgs []string
	switch mode {
	case "":
	case "continue":
		claudeArgs = []string{"claude", "--continue"}
	case "resume":
		claudeArgs = []string{"claude", "--resume"}
		if sessionID != "" {
			if !ccdata.ValidSessionID(sessionID) {
				return errors.New("invalid session id")
			}
			claudeArgs = append(claudeArgs, sessionID)
		}
	default:
		return fmt.Errorf("unknown mode: %s", mode)
	}

	if wt, err := exec.LookPath("wt.exe"); err == nil {
		// "-d" sets the starting directory; ';' separates wt commands, so escape it.
		args := []string{"-d", escapeWT(dir)}
		if claudeArgs != nil {
			args = append(append(args, "cmd.exe", "/k"), claudeArgs...)
		}
		return startDetached(exec.Command(wt, args...))
	}

	args := []string{"-NoExit"}
	if claudeArgs != nil {
		args = append(append(args, "-Command"), claudeArgs...)
	}
	cmd := exec.Command("powershell.exe", args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
	return startDetached(cmd)
}

func escapeWT(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == ';' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	return string(out)
}

func startDetached(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // reap the process handle
	return nil
}

// CopyText puts text on the clipboard.
func (a *App) CopyText(text string) error {
	return runtime.ClipboardSetText(a.ctx, text)
}

// DeleteSession deletes one session of a directory (transcript and its
// sibling folder). The frontend asks for confirmation first.
func (a *App) DeleteSession(dirKey, sessionID string) error {
	d, _, err := a.lookupDir(dirKey)
	if err != nil {
		return err
	}
	for _, s := range d.Sessions {
		if s.ID == sessionID {
			return ccdata.DeleteSession(a.scanner.Loc, s.ID, s.File)
		}
	}
	return fmt.Errorf("unknown session: %s (refresh and try again)", sessionID)
}

// PlanDirectoryRemoval returns what removing a directory would delete, for
// the confirmation dialog.
func (a *App) PlanDirectoryRemoval(dirKey string) (ccdata.RemovalPlan, error) {
	d, all, err := a.lookupDir(dirKey)
	if err != nil {
		return ccdata.RemovalPlan{}, err
	}
	return ccdata.PlanRemoval(a.scanner.Loc, d, all), nil
}

// RemoveDirectory removes a directory completely: its .claude.json entry
// (after a backup), its transcripts and its project folder. It rescans
// first and refuses if the data changed since the user confirmed.
func (a *App) RemoveDirectory(dirKey string, confirmedSessions int) (ccdata.RemovalResult, error) {
	a.Scan()
	d, all, err := a.lookupDir(dirKey)
	if err != nil {
		return ccdata.RemovalResult{}, err
	}
	plan := ccdata.PlanRemoval(a.scanner.Loc, d, all)
	if plan.Sessions != confirmedSessions {
		return ccdata.RemovalResult{}, fmt.Errorf("the directory changed (%d sessions instead of %d): review it and try again", plan.Sessions, confirmedSessions)
	}
	if len(plan.Blockers) > 0 {
		return ccdata.RemovalResult{}, fmt.Errorf("cannot remove: %s", plan.Blockers[0])
	}
	return ccdata.ExecuteRemoval(a.scanner.Loc, plan)
}

// CleanupInfo describes the current cleanupPeriodDays configuration.
type CleanupInfo struct {
	SettingsPath string            `json:"settingsPath"`
	Days         int               `json:"days"`
	Found        bool              `json:"found"`
	Error        string            `json:"error"`
	DefaultDays  int               `json:"defaultDays"`
	MinDays      int               `json:"minDays"`
	MaxDays      int               `json:"maxDays"`
	LastCleanup  string            `json:"lastCleanup"`
	Overrides    []ccdata.Override `json:"overrides"`
}

// GetCleanupInfo reads cleanupPeriodDays and looks for overriding sources.
func (a *App) GetCleanupInfo() CleanupInfo {
	loc := a.scanner.Loc
	info := CleanupInfo{
		SettingsPath: loc.SettingsFile(),
		DefaultDays:  ccdata.DefaultCleanupDays,
		MinDays:      ccdata.MinCleanupDays,
		MaxDays:      ccdata.MaxCleanupDays,
	}
	days, found, err := ccdata.ReadCleanupDays(info.SettingsPath)
	info.Days, info.Found = days, found
	if err != nil {
		info.Error = err.Error()
	}
	if t, ok := ccdata.ReadLastCleanup(loc.LastCleanupFile()); ok {
		info.LastCleanup = t.Format(time.RFC3339)
	}
	a.mu.Lock()
	var dirs []string
	for _, d := range a.last.Directories {
		if d.Exists {
			dirs = append(dirs, d.Path)
		}
	}
	a.mu.Unlock()
	info.Overrides = ccdata.FindOverrides(loc, dirs)
	if info.Overrides == nil {
		info.Overrides = []ccdata.Override{}
	}
	return info
}

// PreviewCleanup estimates what a cleanup with the given period would delete.
func (a *App) PreviewCleanup(days int) ccdata.CleanupPreview {
	a.mu.Lock()
	sessions := a.last.AllSessions()
	a.mu.Unlock()
	return ccdata.PreviewCleanup(sessions, days, time.Now())
}

// SaveCleanupDays writes cleanupPeriodDays to the user settings.json and
// returns the path of the backup copy ("" if the file did not exist).
func (a *App) SaveCleanupDays(days int) (string, error) {
	return ccdata.WriteCleanupDays(a.scanner.Loc.SettingsFile(), days, time.Now())
}
