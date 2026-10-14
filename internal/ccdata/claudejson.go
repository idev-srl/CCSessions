package ccdata

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
)

// ReadProjectPaths returns the keys of the "projects" object in .claude.json:
// the absolute paths of every directory Claude Code was started in. The file
// is opened read-only; Claude Code rewrites it while running, so a partially
// written file is reported as an error rather than retried.
func ReadProjectPaths(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseProjectPaths(f)
}

func parseProjectPaths(r io.Reader) ([]string, error) {
	var doc struct {
		Projects map[string]json.RawMessage `json:"projects"`
	}
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("reading .claude.json: %w", err)
	}
	paths := make([]string, 0, len(doc.Projects))
	for p := range doc.Projects {
		if p != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths, nil
}
