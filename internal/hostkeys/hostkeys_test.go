package hostkeys

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSet(t *testing.T) {
	tests := []struct {
		name      string
		dir       string
		start     string
		host, key string
		want      string
	}{
		{
			name:  "existing file",
			start: "other ssh-ed25519 KEEP\n198.18.0.10 ssh-ed25519 OLD\n# comment\nb,198.18.0.10 ssh-rsa OLD3\n",
			host:  "198.18.0.10",
			key:   "ssh-ed25519 NEW\n",
			want:  "other ssh-ed25519 KEEP\n# comment\n198.18.0.10 ssh-ed25519 NEW\n",
		},
		{
			name: "missing file",
			dir:  "ssh",
			host: "a.rye",
			key:  "ssh-ed25519 K",
			want: "a.rye ssh-ed25519 K\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tt.dir, "known_hosts")
			if tt.start != "" {
				if err := os.WriteFile(path, []byte(tt.start), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := Set(path, tt.host, tt.key); err != nil {
				t.Fatalf("Set(%q, %q, %q) = %v, want nil", path, tt.host, tt.key, err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Mode().Perm(); got != 0o600 {
				t.Errorf("mode = %o, want %o", got, 0o600)
			}
			got, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("file = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRemove(t *testing.T) {
	tests := []struct {
		name  string
		start string
		want  string
	}{
		{name: "last entry", start: "a.rye ssh-ed25519 K\n", want: ""},
		{name: "other hosts", start: "b.rye ssh-ed25519 B\na.rye ssh-ed25519 K\n", want: "b.rye ssh-ed25519 B\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "known_hosts")
			if err := os.WriteFile(path, []byte(tt.start), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := Remove(path, "a.rye"); err != nil {
				t.Fatalf("Remove(%q, %q) = %v, want nil", path, "a.rye", err)
			}
			got, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("file = %q, want %q", got, tt.want)
			}
		})
	}
	t.Run("missing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing")
		if err := Remove(path, "a.rye"); err != nil {
			t.Errorf("Remove(%q, %q) = %v, want nil", path, "a.rye", err)
		}
	})
}
