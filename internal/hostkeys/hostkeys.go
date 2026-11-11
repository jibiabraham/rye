// Package hostkeys edits the known_hosts files that rye owns. rye's ssh options turn off hashing in them. Their
// entries name hosts in plain text.
package hostkeys

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"rye/internal/atomicfile"
)

// Set makes key the only entry for host in the file at path. key is the key type and base64 key, as in a .pub file
// without its comment. The file and its directory are created when missing.
func Set(path, host, key string) error {
	kept, err := without(path, host)
	if err != nil {
		return err
	}
	kept = append(kept, host+" "+strings.TrimSpace(key))
	return write(path, kept)
}

// Remove deletes every entry for host from the file at path. A missing file is no error.
func Remove(path, host string) error {
	kept, err := without(path, host)
	if err != nil {
		return err
	}
	return write(path, kept)
}

// without returns the file's lines except the entries for host.
func without(path, host string) ([]string, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var kept []string
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if line != "" && !matches(line, host) {
			kept = append(kept, line)
		}
	}
	return kept, nil
}

// matches reports whether a known_hosts line names host.
func matches(line, host string) bool {
	fields := strings.Fields(line)
	if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
		return false
	}
	names := fields[0]
	if strings.HasPrefix(names, "@") && len(fields) > 2 {
		names = fields[1]
	}
	for _, name := range strings.Split(names, ",") {
		if name == host {
			return true
		}
	}
	return false
}

// write replaces the file in one rename, with mode 0600.
func write(path string, lines []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line + "\n")
	}
	return atomicfile.Write(path, []byte(b.String()))
}
