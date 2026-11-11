package platform

import (
	"context"
	"slices"
	"strings"
	"testing"

	"rye/internal/compute"
	"rye/internal/vmexec"
)

// fakeRunner answers commands from replies by their joined arguments, and records them.
type fakeRunner struct {
	replies map[string]compute.Result
	runs    []string
}

func (f *fakeRunner) Push(context.Context, string, compute.File) error { return nil }

func (f *fakeRunner) Run(_ context.Context, _ string, c compute.Command) (compute.Result, error) {
	line := strings.Join(c.Args, " ")
	f.runs = append(f.runs, line)
	return f.replies[line], nil
}

func TestInstallPackages(t *testing.T) {
	tests := []struct {
		name     string
		packages map[string]string
		replies  map[string]compute.Result
		want     []string
	}{
		{
			name:     "some missing",
			packages: map[string]string{"openssh-server": "/usr/sbin/sshd", "b": "/usr/bin/b", "a": "/usr/bin/a"},
			replies:  map[string]compute.Result{"test -e /usr/bin/b": {Code: 1}, "test -e /usr/bin/a": {Code: 1}},
			want:     []string{"apt-get update -qq", "apt-get install -y -qq a b"},
		},
		{
			name:     "all present",
			packages: map[string]string{"a": "/a"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeRunner{replies: tt.replies}
			if err := installPackages(context.Background(), vmexec.New(f, "box"), tt.packages); err != nil {
				t.Fatalf("installPackages(%v) = %v, want nil", tt.packages, err)
			}
			var probes []string
			for _, file := range tt.packages {
				probes = append(probes, "test -e "+file)
			}
			if len(f.runs) < len(probes) {
				t.Fatalf("ran %q, want %d probes first", f.runs, len(probes))
			}
			got := slices.Clone(f.runs[:len(probes)])
			slices.Sort(got)
			slices.Sort(probes)
			if !slices.Equal(got, probes) {
				t.Errorf("probes = %q, want %q", got, probes)
			}
			if rest := f.runs[len(probes):]; !slices.Equal(rest, tt.want) {
				t.Errorf("commands after the probes = %q, want %q", rest, tt.want)
			}
		})
	}
}

func TestWithKeys(t *testing.T) {
	tests := []struct {
		name    string
		current string
		keys    []string
		want    string
	}{
		{name: "empty", current: "", keys: []string{"a", "b"}, want: "a\nb\n"},
		{name: "existing key", current: "a\nc\n", keys: []string{"a", "b"}, want: "a\nc\nb\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := withKeys(tt.current, tt.keys...); got != tt.want {
				t.Errorf("withKeys(%q, %q) = %q, want %q", tt.current, tt.keys, got, tt.want)
			}
		})
	}
}

func TestWithoutComment(t *testing.T) {
	tests := []struct {
		name    string
		current string
		comment string
		want    string
	}{
		{
			name:    "one of three",
			current: "ssh-ed25519 A owner@laptop\nssh-ed25519 B rye-service\nssh-ed25519 C rye-admin\n",
			comment: "rye-admin",
			want:    "ssh-ed25519 A owner@laptop\nssh-ed25519 B rye-service\n",
		},
		{name: "last key", current: "ssh-ed25519 C rye-admin\n", comment: "rye-admin", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := withoutComment(tt.current, tt.comment); got != tt.want {
				t.Errorf("withoutComment(%q, %q) = %q, want %q", tt.current, tt.comment, got, tt.want)
			}
		})
	}
}
