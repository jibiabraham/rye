package integration

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// testStore returns a store holding the integration forgejo.
func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add("forgejo", Integration{URL: "http://f", Header: "Authorization", Value: "secret"}); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestStore_Add(t *testing.T) {
	tests := []struct {
		name    string
		add     string
		wantErr bool
	}{
		{name: "lowercase", add: "gitea2"},
		{name: "inner dash", add: "a-b"},
		{name: "reserved keys", add: "keys", wantErr: true},
		{name: "reserved docs", add: "docs", wantErr: true},
		{name: "empty", add: "", wantErr: true},
		{name: "uppercase", add: "Upper", wantErr: true},
		{name: "dot", add: "a.b", wantErr: true},
		{name: "leading dash", add: "-a", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := testStore(t)
			err := store.Add(tt.add, Integration{})
			if (err != nil) != tt.wantErr {
				t.Errorf("Add(%q) error = %v, want error %v", tt.add, err, tt.wantErr)
			}
			if _, ok := store.Integrations[tt.add]; ok == tt.wantErr {
				t.Errorf("after Add(%q), stored = %v, want %v", tt.add, ok, !tt.wantErr)
			}
		})
	}
}

func TestStore_Save(t *testing.T) {
	store := testStore(t)
	path := filepath.Join(t.TempDir(), "integrations.json")
	if err := store.Attach("forgejo", "a"); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want %o", got, 0o600)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Attached("forgejo", "a") || loaded.Integrations["forgejo"].Value != "secret" {
		t.Errorf("Load(%q) = %+v, want forgejo with value secret attached to a", path, loaded)
	}
}

func TestStore_Remove(t *testing.T) {
	store := testStore(t)
	if err := store.Attach("forgejo", "a"); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove("forgejo"); err != nil {
		t.Fatal(err)
	}
	if len(store.Attachments) != 0 {
		t.Errorf("Attachments = %v, want none", store.Attachments)
	}
}

func TestStore_Replace(t *testing.T) {
	store := testStore(t)
	if err := store.Attach("forgejo", "a"); err != nil {
		t.Fatal(err)
	}
	if err := store.Replace("a", nil); err != nil {
		t.Fatal(err)
	}
	if store.Attached("forgejo", "a") {
		t.Errorf("Attached(%q, %q) = true after Replace(%q, nil), want false", "forgejo", "a", "a")
	}
}

func TestStore_Attach(t *testing.T) {
	tests := []struct {
		name        string
		integration string
		wantErr     bool
	}{
		{name: "stored integration", integration: "forgejo"},
		{name: "missing integration", integration: "missing", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := testStore(t)
			err := store.Attach(tt.integration, "a")
			if (err != nil) != tt.wantErr {
				t.Errorf("Attach(%q, %q) error = %v, want error %v", tt.integration, "a", err, tt.wantErr)
			}
			if got := store.Attached(tt.integration, "a"); got == tt.wantErr {
				t.Errorf("Attached(%q, %q) = %v, want %v", tt.integration, "a", got, !tt.wantErr)
			}
		})
	}
}

func TestStore_RemoveTargeting(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want []string
	}{
		{"URL on the VM", "http://forge.rye:3000", []string{"target"}},
		{"URL on another VM", "http://forge2.rye:3000", nil},
		{"URL outside rye", "https://forge.example.com", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := testStore(t)
			if err := store.Add("target", Integration{URL: tt.url, Header: "Authorization", Value: "token"}); err != nil {
				t.Fatal(err)
			}
			if err := store.Attach("target", "a"); err != nil {
				t.Fatal(err)
			}
			got := store.RemoveTargeting("forge.rye")
			if !slices.Equal(got, tt.want) {
				t.Errorf("RemoveTargeting(%q) = %v, want %v", "forge.rye", got, tt.want)
			}
			if _, kept := store.Integrations["target"]; kept == (tt.want != nil) {
				t.Errorf("integration kept = %v, want %v", kept, tt.want == nil)
			}
			if store.Attached("target", "a") != (tt.want == nil) {
				t.Errorf("Attached(%q, %q) = %v, want %v", "target", "a", store.Attached("target", "a"), tt.want == nil)
			}
		})
	}
}
