package vmexec

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"rye/internal/compute"
)

// fake records the paths it pushes and the commands it runs. Push returns pushErr, and Run returns result and
// runErr.
type fake struct {
	pushErr error
	result  compute.Result
	runErr  error
	pushes  []string
	runs    []compute.Command
}

func (f *fake) Push(_ context.Context, _ string, file compute.File) error {
	f.pushes = append(f.pushes, file.Path)
	return f.pushErr
}

func (f *fake) Run(_ context.Context, _ string, c compute.Command) (compute.Result, error) {
	f.runs = append(f.runs, c)
	return f.result, f.runErr
}

func TestVM_Push(t *testing.T) {
	const path = "/etc/rye/config"
	tests := []struct {
		name     string
		pushErr  error
		wantRuns [][]string
		wantErr  string
	}{
		{name: "renamed into place", wantRuns: [][]string{{"mv", path + ".tmp", path}}},
		{name: "push fails", pushErr: errors.New("disk full"), wantErr: path},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fake{pushErr: tt.pushErr}
			err := New(f, "box").Push(context.Background(), compute.File{Path: path, Body: strings.NewReader("x"), Size: 1})
			if tt.wantErr == "" && err != nil {
				t.Fatalf("Push(%s) = %v, want nil", path, err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("Push(%s) error = %v, want one naming %s", path, err, tt.wantErr)
			}
			if want := []string{path + ".tmp"}; !slices.Equal(f.pushes, want) {
				t.Errorf("pushed %q, want %q", f.pushes, want)
			}
			if len(f.runs) != len(tt.wantRuns) {
				t.Fatalf("ran %d commands, want %d", len(f.runs), len(tt.wantRuns))
			}
			for i, c := range f.runs {
				if !slices.Equal(c.Args, tt.wantRuns[i]) || c.UID != 0 || c.GID != 0 {
					t.Errorf("command %d = %q as %d:%d, want %q as root", i, c.Args, c.UID, c.GID, tt.wantRuns[i])
				}
			}
		})
	}
}

func TestVM_OK(t *testing.T) {
	tests := []struct {
		name    string
		result  compute.Result
		runErr  error
		want    bool
		wantErr bool
	}{
		{name: "exit 0", want: true},
		{name: "nonzero exit", result: compute.Result{Code: 1}},
		{name: "run fails", runErr: errors.New("connection refused"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(&fake{result: tt.result, runErr: tt.runErr}, "box").OK(context.Background(), "test", "-e", "/x")
			if got != tt.want || (err != nil) != tt.wantErr {
				t.Errorf("OK() = %v, %v, want %v, error %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}
