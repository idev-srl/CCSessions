package ccdata

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CleanupKey is the settings key controlling transcript retention.
const CleanupKey = "cleanupPeriodDays"

// DefaultCleanupDays is Claude Code's documented default for cleanupPeriodDays.
const DefaultCleanupDays = 30

// MinCleanupDays is the documented minimum: 0 is rejected by Claude Code
// with a validation error (it does not disable transcripts).
const MinCleanupDays = 1

// MaxCleanupDays is an arbitrary sanity bound for the UI (~100 years).
const MaxCleanupDays = 36500

// backupStamp formats timestamps used in backup file names.
const backupStamp = "20060102-150405"

// ReadCleanupDays returns the cleanupPeriodDays value stored in a settings
// file. found is false when the file or the key is missing.
func ReadCleanupDays(path string) (days int, found bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return cleanupDaysFromJSON(data)
}

func cleanupDaysFromJSON(data []byte) (int, bool, error) {
	data = stripBOM(data)
	if len(bytes.TrimSpace(data)) == 0 {
		return 0, false, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return 0, false, fmt.Errorf("invalid JSON: %w", err)
	}
	raw, ok := m[CleanupKey]
	if !ok {
		return 0, false, nil
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, true, fmt.Errorf("%s is not a number: %s", CleanupKey, raw)
	}
	return int(f), true, nil
}

// WriteCleanupDays sets cleanupPeriodDays in the settings file at path,
// preserving every other key and the key order. It first copies the existing
// file to "<path>.bak-<timestamp>" and then replaces it atomically (temporary
// file in the same directory + rename). If the file does not exist it is
// created with just this key. It returns the backup path ("" if none).
func WriteCleanupDays(path string, days int, now time.Time) (backup string, err error) {
	if days < MinCleanupDays || days > MaxCleanupDays {
		return "", fmt.Errorf("invalid value %d: must be between %d and %d", days, MinCleanupDays, MaxCleanupDays)
	}
	orig, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	out, err := setTopLevelKey(orig, CleanupKey, []byte(fmt.Sprint(days)))
	if err != nil {
		return "", fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if exists {
		if backup, err = backupFile(path, ".bak-", orig, now.Format(backupStamp)); err != nil {
			return "", err
		}
	} else if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := atomicWrite(path, out); err != nil {
		return backup, err
	}
	return backup, nil
}

// setTopLevelKey sets key to value in a JSON object (replacing it in place,
// or appending it) and keeps every other member and its position.
func setTopLevelKey(orig []byte, key string, value json.RawMessage) ([]byte, error) {
	return editObject(orig, func(members []member) ([]member, error) {
		replaced := false
		for i := range members {
			if members[i].key == key {
				members[i].value = value
				replaced = true
			}
		}
		if !replaced {
			members = append(members, member{key, value})
		}
		return members, nil
	})
}

// CleanupPreview estimates what Claude Code's next cleanup would delete:
// sessions whose transcript mtime is older than days. The documentation does
// not say which timestamp Claude Code uses, so this is an estimate.
type CleanupPreview struct {
	Days            int   `json:"days"`
	Sessions        int   `json:"sessions"`
	TranscriptBytes int64 `json:"transcriptBytes"`
	ExtraBytes      int64 `json:"extraBytes"`
	TotalSessions   int   `json:"totalSessions"`
}

// PreviewCleanup computes a CleanupPreview over the given sessions.
func PreviewCleanup(sessions []Session, days int, now time.Time) CleanupPreview {
	p := CleanupPreview{Days: days, TotalSessions: len(sessions)}
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
	for _, s := range sessions {
		if s.ModTime.Before(cutoff) {
			p.Sessions++
			p.TranscriptBytes += s.FileSize
			p.ExtraBytes += s.ExtraSize
		}
	}
	return p
}

// ReadLastCleanup returns the content of Claude Code's .last-cleanup marker.
func ReadLastCleanup(path string) (time.Time, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(b)))
	return t, err == nil
}
