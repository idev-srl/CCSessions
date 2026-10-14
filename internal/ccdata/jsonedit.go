package ccdata

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// member is one key/value pair of a JSON object, kept in file order.
type member struct {
	key   string
	value json.RawMessage
}

var bom = []byte{0xEF, 0xBB, 0xBF}

func stripBOM(b []byte) []byte { return bytes.TrimPrefix(b, bom) }

// parseObject parses a JSON object keeping member order and raw values.
func parseObject(data []byte) ([]member, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("invalid JSON: the root is not an object")
	}
	var members []member
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		key, _ := tok.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		members = append(members, member{key, v})
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("invalid JSON: unexpected content after the object")
	}
	return members, nil
}

// marshalObject serializes members compactly, in order.
func marshalObject(members []member) json.RawMessage {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, m := range members {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, _ := json.Marshal(m.key)
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(m.value)
	}
	buf.WriteByte('}')
	return buf.Bytes()
}

// editObject parses the JSON object in orig, lets edit change its members and
// re-serializes it with two-space indentation. Values are kept verbatim apart
// from re-indentation; line endings (CRLF/LF) and a UTF-8 BOM are preserved.
// Empty input is treated as an empty object.
func editObject(orig []byte, edit func([]member) ([]member, error)) ([]byte, error) {
	hasBOM := bytes.HasPrefix(orig, bom)
	body := stripBOM(orig)
	crlf := bytes.Contains(body, []byte("\r\n"))

	var members []member
	if len(bytes.TrimSpace(body)) > 0 {
		var err error
		if members, err = parseObject(body); err != nil {
			return nil, err
		}
	}
	members, err := edit(members)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, marshalObject(members), "", "  "); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	out := buf.Bytes()
	if crlf {
		out = bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n"))
	}
	if hasBOM {
		out = append(append([]byte{}, bom...), out...)
	}
	return out, nil
}

// backupFile copies data to "<path><suffix><timestamp>", adding a counter if
// that name is taken. It never overwrites an existing file.
func backupFile(path, suffix string, data []byte, stamp string) (string, error) {
	stem := path + suffix + stamp
	name := stem
	for i := 2; ; i++ {
		err := writeFileSync(name, data, 0o644)
		if err == nil {
			return name, nil
		}
		if !errors.Is(err, os.ErrExist) || i > 99 {
			return "", fmt.Errorf("creating backup: %w", err)
		}
		name = fmt.Sprintf("%s-%d", stem, i)
	}
}

// atomicWrite writes data to a temporary file in the same directory and
// renames it over path.
func atomicWrite(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// On Windows os.Rename uses MoveFileEx(MOVEFILE_REPLACE_EXISTING).
	return os.Rename(tmpName, path)
}

func writeFileSync(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
