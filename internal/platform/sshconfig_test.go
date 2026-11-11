package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteSSHConfig(t *testing.T) {
	tests := []struct {
		name         string
		config       *string
		wantIncluded bool
	}{
		{name: "no config"},
		{name: "config without the include", config: ptr("Host *\n  ForwardAgent no\n")},
		{name: "config with the include", config: ptr("Include config.d/rye\n\nHost x\n"), wantIncluded: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if err := os.Mkdir(filepath.Join(home, ".ssh"), 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(home, ".ssh", "config")
			if tt.config != nil {
				if err := os.WriteFile(path, []byte(*tt.config), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			included, err := WriteSSHConfig(home, "Host rye\n")
			if err != nil {
				t.Fatal(err)
			}
			if included != tt.wantIncluded {
				t.Errorf("WriteSSHConfig() included = %v, want %v", included, tt.wantIncluded)
			}
			got, _ := os.ReadFile(filepath.Clean(path))
			if want := ""; tt.config != nil {
				want = *tt.config
				if string(got) != want {
					t.Errorf("~/.ssh/config = %q, want it unchanged as %q", got, want)
				}
			}
			if got, _ := os.ReadFile(filepath.Clean(filepath.Join(home, ".ssh", "config.d", "rye"))); string(got) != "Host rye\n" {
				t.Errorf("config.d/rye = %q, want the aliases", got)
			}
		})
	}
}

func ptr(s string) *string { return &s }
