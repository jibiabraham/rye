// Package atomicfile replaces files in one rename.
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write replaces the file at path with data, with mode 0600. It writes a temporary file in the same directory,
// syncs it and renames it over path. A reader sees the old file or the new one.
func Write(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path))
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
