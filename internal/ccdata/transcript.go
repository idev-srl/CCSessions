package ccdata

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// PreviewLen is the maximum number of characters kept for previews.
const PreviewLen = 120

// maxLineBytes bounds memory use: lines longer than this (huge pasted images
// or tool outputs) are skipped and counted as unreadable. A var for tests.
var maxLineBytes = 64 << 20

// Session is the summary of one transcript file (<sessionId>.jsonl).
type Session struct {
	ID   string `json:"id"`
	File string `json:"file"`
	// Cwd is the first working directory found in the transcript; empty if none.
	Cwd               string    `json:"cwd"`
	FirstEvent        time.Time `json:"firstEvent" ts_type:"string"`
	LastEvent         time.Time `json:"lastEvent" ts_type:"string"`
	ModTime           time.Time `json:"modTime" ts_type:"string"`
	UserMessages      int       `json:"userMessages"`
	AssistantMessages int       `json:"assistantMessages"`
	// FileSize is the size of the .jsonl; ExtraSize is the size of the
	// sibling <sessionId>\ folder (subagent transcripts, tool results).
	FileSize  int64 `json:"fileSize"`
	ExtraSize int64 `json:"extraSize"`
	// Title is the AI-generated title (or a legacy "summary" line), FirstPrompt
	// the first real user prompt. Preview is Title if present, else FirstPrompt.
	Title       string `json:"title"`
	FirstPrompt string `json:"firstPrompt"`
	Preview     string `json:"preview"`
	// BadLines counts lines that were not valid JSON objects.
	BadLines int `json:"badLines"`
	// Active is set when a running Claude Code process owns this session.
	Active bool `json:"active"`
}

// TotalSize is the on-disk footprint of the session.
func (s Session) TotalSize() int64 { return s.FileSize + s.ExtraSize }

// line holds only the fields we need; everything else is skipped by the decoder.
type line struct {
	Type        string `json:"type"`
	Timestamp   string `json:"timestamp"`
	Cwd         string `json:"cwd"`
	IsMeta      bool   `json:"isMeta"`
	IsSidechain bool   `json:"isSidechain"`
	IsCompact   bool   `json:"isCompactSummary"`
	AiTitle     string `json:"aiTitle"`
	Summary     string `json:"summary"`
	Message     *struct {
		ID      string          `json:"id"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// ParseTranscript reads a transcript stream line by line. It never fails on
// malformed content: bad lines are counted and skipped. Only I/O errors from
// the reader are returned, together with whatever was parsed up to that point.
func ParseTranscript(r io.Reader) (Session, error) {
	var (
		s          Session
		br         = bufio.NewReaderSize(r, 256<<10)
		assistants = map[string]struct{}{}
		anonAssist int
		buf        []byte
		skipping   bool // current line exceeded maxLineBytes
	)
	for {
		chunk, err := br.ReadSlice('\n')
		if !skipping {
			if len(buf)+len(chunk) > maxLineBytes {
				skipping, buf = true, buf[:0]
			} else {
				buf = append(buf, chunk...)
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue // line continues in the next chunk
		}
		if skipping {
			s.BadLines++
			skipping = false
		} else {
			parseLine(bytes.TrimSpace(buf), &s, assistants, &anonAssist)
		}
		buf = buf[:0]
		if err == io.EOF {
			break
		}
		if err != nil {
			finish(&s, assistants, anonAssist)
			return s, err
		}
	}
	finish(&s, assistants, anonAssist)
	return s, nil
}

func finish(s *Session, assistants map[string]struct{}, anon int) {
	s.AssistantMessages = len(assistants) + anon
	s.Preview = s.Title
	if s.Preview == "" {
		s.Preview = s.FirstPrompt
	}
}

func parseLine(raw []byte, s *Session, assistants map[string]struct{}, anon *int) {
	if len(raw) == 0 {
		return
	}
	var l line
	if raw[0] != '{' || json.Unmarshal(raw, &l) != nil {
		s.BadLines++
		return
	}
	if ts, ok := parseTime(l.Timestamp); ok {
		if s.FirstEvent.IsZero() || ts.Before(s.FirstEvent) {
			s.FirstEvent = ts
		}
		if ts.After(s.LastEvent) {
			s.LastEvent = ts
		}
	}
	if s.Cwd == "" && IsAbsWindowsPath(l.Cwd) {
		s.Cwd = l.Cwd
	}
	switch l.Type {
	case "ai-title":
		if t := cleanText(l.AiTitle); t != "" {
			s.Title = t // later titles supersede earlier ones
		}
	case "summary": // older Claude Code versions
		if t := cleanText(l.Summary); t != "" && s.Title == "" {
			s.Title = t
		}
	case "user":
		// Compaction summaries are injected by Claude Code, not typed by the user.
		if l.IsMeta || l.IsSidechain || l.IsCompact || l.Message == nil {
			return
		}
		text, ok := userText(l.Message.Content)
		if !ok {
			return
		}
		s.UserMessages++
		if s.FirstPrompt == "" {
			s.FirstPrompt = cleanText(text)
		}
	case "assistant":
		if l.IsSidechain || l.Message == nil {
			return
		}
		// One assistant message is split over several lines (one per content
		// block) that share the same message id.
		if l.Message.ID != "" {
			assistants[l.Message.ID] = struct{}{}
		} else {
			*anon++
		}
	}
}

// userText extracts the text of a prompt typed by the user. Tool results and
// command wrappers (slash commands, local command output) are not prompts.
func userText(content json.RawMessage) (string, bool) {
	content = bytes.TrimSpace(content)
	if len(content) == 0 {
		return "", false
	}
	var text string
	switch content[0] {
	case '"':
		if json.Unmarshal(content, &text) != nil {
			return "", false
		}
	case '[':
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(content, &blocks) != nil {
			return "", false
		}
		hasImage := false
		for _, b := range blocks {
			switch b.Type {
			case "tool_result":
				return "", false
			case "text":
				if text == "" {
					text = b.Text
				}
			case "image":
				hasImage = true
			}
		}
		if text == "" && hasImage {
			text = "[image]"
		}
	default:
		return "", false
	}
	t := strings.TrimSpace(text)
	if t == "" || isCommandWrapper(t) {
		return "", false
	}
	return t, true
}

var wrapperPrefixes = []string{
	"<local-command-", "<command-name>", "<command-message>", "<command-args>",
	"<bash-input>", "<bash-stdout>", "<bash-stderr>", "<task-notification>",
	"<system-reminder>", "Caveat: The messages below",
}

func isCommandWrapper(t string) bool {
	for _, p := range wrapperPrefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

var (
	tagRe   = regexp.MustCompile(`</?[a-zA-Z][a-zA-Z0-9_-]*(\s[^<>]*)?>`)
	spaceRe = regexp.MustCompile(`\s+`)
)

// cleanText strips XML-like wrapper tags, collapses whitespace and truncates
// to PreviewLen characters.
func cleanText(t string) string {
	t = tagRe.ReplaceAllString(t, " ")
	t = strings.TrimSpace(spaceRe.ReplaceAllString(t, " "))
	if !utf8.ValidString(t) {
		t = strings.ToValidUTF8(t, "�")
	}
	if utf8.RuneCountInString(t) > PreviewLen {
		r := []rune(t)
		t = strings.TrimSpace(string(r[:PreviewLen-1])) + "…"
	}
	return t
}

func parseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil || t.Year() < 2000 {
		return time.Time{}, false
	}
	return t, true
}

// ReadSession parses a transcript file and fills in file metadata. Timestamps
// fall back to the file's mtime when no line carries one.
func ReadSession(path string) (Session, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return Session{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return Session{}, err
	}
	defer f.Close()
	s, err := ParseTranscript(f)
	s.ID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	s.File = path
	s.FileSize = fi.Size()
	s.ModTime = fi.ModTime()
	if s.LastEvent.IsZero() {
		s.LastEvent = s.ModTime
	}
	if s.FirstEvent.IsZero() {
		s.FirstEvent = s.LastEvent
	}
	s.ExtraSize = dirSize(filepath.Join(filepath.Dir(path), s.ID))
	return s, err
}

func dirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // missing dir or unreadable entry: count what we can
		}
		if !d.IsDir() {
			if info, e := d.Info(); e == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}
