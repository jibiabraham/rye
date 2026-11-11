package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	for _, data := range []string{"first", "second"} {
		if err := Write(path, []byte(data)); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Clean(path))
		if err != nil || string(got) != data {
			t.Errorf("after Write(%q), file = %q, %v", data, got, err)
		}
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, %v, want 0600", info.Mode().Perm(), err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Errorf("directory holds %d entries, want only the file", len(entries))
	}
}
