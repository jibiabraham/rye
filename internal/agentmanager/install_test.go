package agentmanager

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"rye/internal/compute"
	"rye/internal/vmconfig"
)

// fakeVM answers id with 1001 and 1002, reports the user manager active after inactive replies, and records
// every command and push.
type fakeVM struct {
	inactive int
	runs     []compute.Command
	pushed   compute.File
	body     string
}

func (f *fakeVM) Push(_ context.Context, _ string, file compute.File) error {
	data, _ := io.ReadAll(file.Body)
	f.pushed, f.body = file, string(data)
	return nil
}

func (f *fakeVM) Run(_ context.Context, _ string, c compute.Command) (compute.Result, error) {
	f.runs = append(f.runs, c)
	line := strings.Join(c.Args, " ")
	switch {
	case line == "id -u dev":
		return compute.Result{Stdout: "1001\n"}, nil
	case line == "id -g dev":
		return compute.Result{Stdout: "1002\n"}, nil
	case strings.HasPrefix(line, "systemctl is-active"):
		if f.inactive > 0 {
			f.inactive--
			return compute.Result{Code: 3}, nil
		}
	case c.Args[0] == "sh" && c.UID != 0:
		return compute.Result{Stdout: "agent-manager is listening on 127.0.0.1:7420\n"}, nil
	}
	return compute.Result{}, nil
}

func TestInstall(t *testing.T) {
	t.Run("installs", func(t *testing.T) {
		vm := &fakeVM{inactive: 1}
		var out bytes.Buffer
		if err := Install(context.Background(), vm, "box", "dev", &out); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, c := range vm.runs {
			got = append(got, strings.Join(c.Args, " "))
		}
		want := []string{"id -u dev", "id -g dev", "sh -c " + vmconfig.NodeSetup, "loginctl enable-linger dev",
			"systemctl is-active --quiet user@1001.service", "systemctl is-active --quiet user@1001.service",
			"mv " + packagePath + ".tmp " + packagePath, "sh -c " + script, "sh -c " + vmconfig.ClaudeLink("/home/dev")}
		if !slices.Equal(got, want) {
			t.Errorf("commands =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
		p := vm.pushed
		if p.Path != packagePath+".tmp" || p.Size != int64(len(vm.body)) || p.UID != 1001 || p.GID != 1002 || p.Mode != 0o600 {
			t.Errorf("pushed %+v, want %s.tmp with its size, 1001:1002, mode 0600", p, packagePath)
		}
		last := vm.runs[len(vm.runs)-2]
		if last.UID != 1001 || last.GID != 1002 || last.Dir != "/home/dev" || last.Env["XDG_RUNTIME_DIR"] != "/run/user/1001" ||
			last.Env["HOME"] != "/home/dev" {
			t.Errorf("script ran as %+v, want 1001:1002 in /home/dev with HOME and XDG_RUNTIME_DIR set", last)
		}
		if got, want := out.String(), "agent-manager is listening on 127.0.0.1:7420\n"; got != want {
			t.Errorf("out = %q, want %q", got, want)
		}
	})

	tests := []struct {
		name    string
		vm      *fakeVM
		noWait  bool
		wantErr string
	}{
		{name: "manager never starts", vm: &fakeVM{inactive: 1 << 30}, noWait: true,
			wantErr: "user@1001.service in box did not start within 0s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.noWait {
				saved := managerWait
				managerWait = 0
				t.Cleanup(func() { managerWait = saved })
			}
			start := time.Now()
			err := Install(context.Background(), tt.vm, "box", "dev", io.Discard)
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("Install() error = %v, want %q", err, tt.wantErr)
			}
			if elapsed := time.Since(start); elapsed > 5*time.Second {
				t.Errorf("Install() took %v, want under 5s", elapsed)
			}
			if tt.noWait && tt.vm.pushed.Path != "" {
				t.Errorf("pushed %+v before the manager started", tt.vm.pushed)
			}
		})
	}
}

func TestTarball(t *testing.T) {
	data, err := tarball()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for tr := tar.NewReader(zr); ; {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
	}
	for _, want := range []string{"install.sh", "main.ts", "package.json", "pnpm-lock.yaml", "agent-manager.service"} {
		if !slices.Contains(names, want) {
			t.Errorf("tarball() holds %q, want it to hold %s", names, want)
		}
	}
}
